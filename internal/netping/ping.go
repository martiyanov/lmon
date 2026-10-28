// Package netping измеряет ICMP-задержку через системную утилиту ping,
// поэтому прав root не нужны.
package netping

import (
	"context"
	"errors"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// unreachable показывается, когда хост не отвечает.
const unreachable = 999

// timeRe понимает и локализованный вывод ping (например, «время=45мс»
// в русской Windows).
var timeRe = regexp.MustCompile(`(?:time|время)[=<]([0-9]+(?:[.,][0-9]+)?)`)

// Latency возвращает время отклика до host в миллисекундах (усечённое до
// целого). Отправляются два пакета: первый часто только «будит» Wi-Fi-радио,
// поэтому честной задержкой считается последний ответ. Если оба пакета
// потеряны (частая история — ICMP rate-limit), делается один ретрай
// одиночным пакетом.
func Latency(ctx context.Context, host string) int {
	start := time.Now()
	if t := ping(ctx, args(host)); t != unreachable {
		return t
	}
	// ретрай — только если бюджет измерения ещё позволяет,
	// чтобы строка не ждала два полных таймаута подряд
	if time.Since(start) > 3*time.Second {
		return unreachable
	}
	select {
	case <-ctx.Done():
		return -1
	case <-time.After(300 * time.Millisecond):
	}
	return ping(ctx, retryArgs(host))
}

func ping(ctx context.Context, args []string) int {
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, "ping", args...).Output()
	// сам ping не запустился (нет бинарника) или цикл отменён —
	// измерение невозможно: -1 («·»), а не «хост недоступен»
	var execErr *exec.Error
	if errors.As(err, &execErr) || ctx.Err() != nil {
		return -1
	}
	if times := parseTimes(string(out)); len(times) > 0 {
		return int(times[len(times)-1])
	}
	return unreachable
}

func args(host string) []string {
	switch runtime.GOOS {
	case "darwin":
		// на macOS -W в миллисекундах; интервал меньше 0.2 с разрешён
		return []string{"-c", "2", "-i", "0.1", "-W", "1000", host}
	case "windows":
		// на Windows счёт пакетов -n, таймаут -w в миллисекундах
		return []string{"-n", "2", "-w", "1000", host}
	}
	// на Linux -W в секундах, а интервал меньше 0.2 с требует root
	return []string{"-c", "2", "-i", "0.2", "-W", "1", host}
}

func retryArgs(host string) []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{"-c", "1", "-W", "1000", host}
	case "windows":
		return []string{"-n", "1", "-w", "1000", host}
	}
	return []string{"-c", "1", "-W", "1", host}
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
