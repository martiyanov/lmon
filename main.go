// Command lmon is a minimal terminal monitor: a fixed header row followed by
// a stream of measurements — ping to the internet, ping to the gateway,
// load average, memory and CPU temperature — colored green/yellow/red.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/martiyanov/lmon/internal/meter"
	"github.com/martiyanov/lmon/internal/netping"

	"golang.org/x/sys/unix"
)

const version = "1.0.1"

const helpText = `lmon ` + version + ` — минимальный монитор сети и нагрузки.

Заголовок закреплён вверху окна, ниже идут строки значений.
При изменении размера терминала число строк подстраивается,
а заголовок остаётся на месте.

Колонки:
  N  — пинг до внешнего хоста (--target / LMON_TARGET, по умолчанию 8.8.8.8), мс.
       Показывает качество канала до интернета.
  G  — пинг до шлюза (--gw / LMON_GW, по умолчанию 192.168.0.1), мс.
       Если N и G прыгают вместе — проблема локальная (Wi-Fi/роутер).
       Если только N — внешний маршрут или провайдер.
  L  — load average, целая часть. Загрузка системы.
  M  — занятая память, % (знак % указан в заголовке).
  T  — температура CPU, °C.

Цвета: зелёный — норма, жёлтый — внимание, красный — плохо.
Пороги:
  N: <=50 мс — зелёный, <=150 — жёлтый, >150 — красный
  G: <=10 мс — зелёный, <=50 — жёлтый, >50 — красный
  L: <=4 — зелёный, <=8 — жёлтый, >8 — красный
  M: <=60% — зелёный, <=85% — жёлтый, >85% — красный
  T: <=60°C — зелёный, <=75°C — жёлтый, >75°C — красный

Использование:
  lmon              — бесконечно, примерно раз в секунду
  lmon 20           — 20 строк и выход
  lmon help         — эта справка
  lmon --version    — версия

Настройки (флаги и переменные окружения):
  --target    LMON_TARGET     внешний хост (по умолчанию 8.8.8.8)
  --gw        LMON_GW         шлюз (по умолчанию 192.168.0.1)
  --interval  LMON_INTERVAL   пауза между строками, сек (по умолчанию 1)

Примеры:
  lmon --target 1.1.1.1
  lmon --interval 2 20
  LMON_GW=192.168.0.1 lmon 10

Первая строка после запуска может быть завышена: Wi-Fi-радио
просыпается. Дальше значения стабилизируются.

Пинг выполняется системной утилитой ping. Температура (колонка T)
на macOS требует установленной утилиты smctemp
(https://github.com/holtmann/osx/tree/master/smctemp), без неё — 0;
на Linux читается из sysfs (hwmon/thermal_zone).
`

const (
	reset = "\x1b[0m"
	grn   = "\x1b[1;32m"
	yel   = "\x1b[1;33m"
	red   = "\x1b[1;31m"
	dim   = "\x1b[2m"

	headerRow = "  N   G   L  M%   T°"
	sepRow    = "--- --- --- --- ----"
)

type config struct {
	target   string
	gw       string
	interval time.Duration
	iters    int
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout))
}

func run(args []string, w io.Writer) int {
	if len(args) > 0 {
		switch args[0] {
		case "help", "-h", "--help":
			fmt.Fprint(w, helpText)
			return 0
		case "--version":
			fmt.Fprintf(w, "lmon %s\n", version)
			return 0
		}
	}

	cfg, err := parseConfig(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "lmon: %v (см. lmon help)\n", err)
		return 2
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	return monitor(ctx, cfg, w)
}

func parseConfig(args []string) (config, error) {
	fs := flag.NewFlagSet("lmon", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	target := fs.String("target", envStr("LMON_TARGET", "8.8.8.8"), "external host")
	gw := fs.String("gw", envStr("LMON_GW", "192.168.0.1"), "gateway")
	interval := fs.Float64("interval", envFloat("LMON_INTERVAL", 1), "seconds between lines")

	if err := fs.Parse(args); err != nil {
		return config{}, err
	}

	var iters int
	if fs.NArg() > 0 {
		n, err := strconv.Atoi(fs.Arg(0))
		if err != nil || n < 0 {
			return config{}, fmt.Errorf("аргумент %q: ожидается число строк", fs.Arg(0))
		}
		iters = n
	}

	d := time.Duration(*interval * float64(time.Second))
	if d <= 0 {
		d = time.Second
	}
	return config{target: *target, gw: *gw, interval: d, iters: iters}, nil
}

func envStr(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}

func envFloat(name string, def float64) float64 {
	if v := os.Getenv(name); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 {
			return f
		}
	}
	return def
}

func monitor(ctx context.Context, cfg config, w io.Writer) int {
	tty := stdoutIsTTY()
	u := &ui{w: w, tty: tty, color: tty && os.Getenv("NO_COLOR") == ""}
	defer u.restore()
	u.setup()

	for it := 1; ; it++ {
		select {
		case <-ctx.Done():
			return 130
		default:
		}
		u.maybeResize()
		u.row(u.sampleRow(ctx, cfg))
		if cfg.iters > 0 && it >= cfg.iters {
			return 0
		}
		select {
		case <-ctx.Done():
			return 130
		case <-time.After(cfg.interval):
		}
	}
}

func (u *ui) sampleRow(ctx context.Context, cfg config) string {
	// Оба пинга — параллельно, чтобы цикл не длился два таймаута подряд.
	res := make(chan int, 2)
	go func() { res <- netping.Latency(ctx, cfg.target) }()
	go func() { res <- netping.Latency(ctx, cfg.gw) }()
	netMS, gwMS := <-res, <-res

	load, _ := meter.LoadAvg()
	mem := meter.MemPercent()
	temp := meter.TempC()

	return u.cell(netMS, 50, 150) + " " +
		u.cell(gwMS, 10, 50) + " " +
		u.loadCell(load) + " " +
		u.cell(mem, 60, 85) + " " +
		u.tempCell(temp, 60, 75)
}

type ui struct {
	w     io.Writer
	tty   bool
	color bool
	rows  int
}

func (u *ui) setup() {
	if !u.tty {
		u.printHeader()
		return
	}
	fmt.Fprint(u.w, "\x1b[?25l") // спрятать курсор — не мелькает
	u.redraw(termRows())
}

func (u *ui) maybeResize() {
	if u.tty {
		if r := termRows(); r != u.rows {
			u.redraw(r)
		}
	}
}

func (u *ui) redraw(rows int) {
	if rows < 4 {
		rows = 4
	}
	fmt.Fprint(u.w, "\x1b[2J\x1b[H")
	u.printHeader()
	// прокрутка только области под заголовком
	fmt.Fprintf(u.w, "\x1b[3;%dr", rows)
	fmt.Fprintf(u.w, "\x1b[%d;1H", rows)
	u.rows = rows
}

func (u *ui) printHeader() {
	if u.color {
		fmt.Fprintf(u.w, "%s%s%s\n%s%s%s\n", dim, headerRow, reset, dim, sepRow, reset)
	} else {
		fmt.Fprintf(u.w, "%s\n%s\n", headerRow, sepRow)
	}
}

func (u *ui) row(s string) {
	if u.tty {
		fmt.Fprintf(u.w, "\r\n%s", s)
	} else {
		fmt.Fprintln(u.w, s)
	}
}

// restore возвращает нормальный режим терминала (в т.ч. после Ctrl+C).
func (u *ui) restore() {
	if u.tty {
		fmt.Fprint(u.w, "\x1b[r\x1b[?25h\n")
	}
}

func (u *ui) cell(v, green, yellow int) string {
	s := fmt.Sprintf("%3d", v)
	if !u.color {
		return s
	}
	switch {
	case v <= green:
		return grn + s + reset
	case v <= yellow:
		return yel + s + reset
	default:
		return red + s + reset
	}
}

func (u *ui) loadCell(load float64) string { // целая часть load average
	return u.cell(int(load), 4, 8)
}

func (u *ui) tempCell(t, green, yellow int) string {
	s := fmt.Sprintf("%3d°", t)
	if !u.color {
		return s
	}
	switch {
	case t <= green:
		return grn + s + reset
	case t <= yellow:
		return yel + s + reset
	default:
		return red + s + reset
	}
}

func stdoutIsTTY() bool {
	fi, err := os.Stdout.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

func termRows() int {
	ws, err := unix.IoctlGetWinsize(int(os.Stdout.Fd()), unix.TIOCGWINSZ)
	if err != nil || ws.Row == 0 {
		return 24
	}
	return int(ws.Row)
}
