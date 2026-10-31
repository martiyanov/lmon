// Package netping измеряет ICMP-задержку через системную утилиту ping,
// поэтому прав root не нужны.
package netping

import (
	"context"
	"errors"
	"net/netip"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// State — состояние измерения: значение и его природа разделены,
// чтобы измеренные 999 мс не путались с «узел не ответил».
type State uint8

const (
	StateValid       State = iota // получен ответ, ValueMS — честная задержка
	StateUnavailable              // измерение невозможно (нет ping, отмена)
	StateUnreachable              // узел не ответил
)

// Result — результат измерения задержки.
type Result struct {
	ValueMS int
	State   State
}

// unreachable — внутренняя метка «нет ответа».
const unreachable = 999

// timeRe понимает и локализованный вывод ping (например, «время=45мс»
// в русской Windows).
var timeRe = regexp.MustCompile(`(?:time|время)[=<]([0-9]+(?:[.,][0-9]+)?)`)

// Latency возвращает время отклика до host в миллисекундах (усечённое до
// целого). Отправляются два пакета: первый часто только «будит» Wi-Fi-радио,
// поэтому честной задержкой считается последний ответ. Если оба пакета
// потеряны (частая история — ICMP rate-limit), делается один ретрай
// одиночным пакетом. budget ограничивает всё измерение (0 — умолчание 3с,
// максимум 4с): ретрай запускается, только если укладывается в бюджет —
// при коротком периоде обновления строка не ждёт два полных таймаута.
func Latency(ctx context.Context, host string, budget time.Duration) Result {
	if budget <= 0 {
		budget = 3 * time.Second
	}
	budget = min(budget, 4*time.Second)
	goos := runtime.GOOS
	primary, fallback := pingCmds(goos, host, "2")
	start := time.Now()
	if r := ping(ctx, primary, fallback); r.State != StateUnreachable {
		return r
	}
	if !retryAllowed(time.Since(start), budget) {
		return Result{State: StateUnreachable}
	}
	select {
	case <-ctx.Done():
		return Result{State: StateUnavailable}
	case <-time.After(300 * time.Millisecond):
	}
	retry, retryAlt := pingCmds(goos, host, "1")
	return ping(ctx, retry, retryAlt)
}

// retryAllowed: ретрай одиночным пакетом стоит ~1с ожидания таймаута —
// запускаем его, только если бюджет измерения это позволяет.
func retryAllowed(elapsed, budget time.Duration) bool {
	return elapsed+time.Second <= budget
}

func ping(parent context.Context, argv, fallback []string) Result {
	ctx, cancel := context.WithTimeout(parent, 4*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, argv[0], argv[1:]...).Output()
	var execErr *exec.Error
	if errors.As(err, &execErr) && fallback != nil {
		// основного бинарника/флага нет — пробуем запасной (ping6)
		out, err = exec.CommandContext(ctx, fallback[0], fallback[1:]...).Output()
		execErr = nil
		var e2 *exec.Error
		if errors.As(err, &e2) {
			execErr = e2
		}
	}
	switch {
	case execErr != nil:
		return Result{State: StateUnavailable} // нет бинарника
	case parent.Err() != nil:
		return Result{State: StateUnavailable} // цикл отменён (Ctrl+C)
	case ctx.Err() == context.DeadlineExceeded:
		// наш таймаут: ping6 без -W ждёт сам — это «нет ответа», не «нет измерения»
		return Result{State: StateUnreachable}
	}
	if times := parseTimes(string(out)); len(times) > 0 {
		return Result{ValueMS: int(times[len(times)-1]), State: StateValid}
	}
	return Result{State: StateUnreachable}
}

// pingCmds возвращает полную командную строку пинга (argv[0] — бинарник)
// для данной ОС и цели; count — число пакетов («2» обычно, «1» для ретрая).
// Для IPv6 на Linux есть запасной вариант: старые дистрибутивы держат
// отдельный бинарник ping6 без флага -6.
func pingCmds(goos, host, count string) (primary, fallback []string) {
	v6 := isV6(host)
	switch goos {
	case "darwin":
		if v6 {
			// отдельный бинарник; флага -W у ping6 нет — таймаут
			// необязательности даёт внешний контекст процесса
			return []string{"ping6", "-c", count, "-i", "0.1", host}, nil
		}
		// на macOS -W в миллисекундах; интервал меньше 0.2 с разрешён
		return []string{"ping", "-c", count, "-i", "0.1", "-W", "1000", host}, nil
	case "windows":
		a := []string{"ping"}
		if v6 {
			a = append(a, "-6")
		}
		// счёт пакетов -n, таймаут -w в миллисекундах
		return append(a, "-n", count, "-w", "1000", host), nil
	}
	// Linux: -W в секундах, интервал меньше 0.2 с требует root;
	// IPv6 — через -6 (НЕ -N: это nodeinfo-запросы), не сработало — ping6
	if v6 {
		return []string{"ping", "-6", "-c", count, "-i", "0.2", "-W", "1", host},
			[]string{"ping6", "-c", count, "-i", "0.2", "-W", "1", host}
	}
	return []string{"ping", "-c", count, "-i", "0.2", "-W", "1", host}, nil
}

func isV6(host string) bool {
	a, err := netip.ParseAddr(host)
	return err == nil && a.Is6() && !a.Is4In6()
}

func parseTimes(out string) []float64 {
	ms := timeRe.FindAllStringSubmatch(out, -1)
	times := make([]float64, 0, len(ms))
	for _, m := range ms {
		if v, err := strconv.ParseFloat(strings.ReplaceAll(m[1], ",", "."), 64); err == nil {
			times = append(times, v)
		}
	}
	return times
}
