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
	"strings"
	"syscall"
	"time"

	"github.com/martiyanov/lmon/internal/meter"
	"github.com/martiyanov/lmon/internal/netping"
)

const version = "1.2.0"

const helpText = `lmon ` + version + ` — минимальный монитор сети и нагрузки.

Заголовок закреплён вверху окна, ниже идут строки значений.
При изменении размера терминала число строк подстраивается,
а заголовок остаётся на месте.

Базовые колонки (всегда):
  N  — пинг до внешнего хоста (--target / LMON_TARGET, по умолчанию
       8.8.8.8), мс. Качество канала до интернета.
  G  — пинг до шлюза (--gw / LMON_GW; по умолчанию определяется
       автоматически), мс.
  L  — load average, целая часть. Загрузка системы.
  M% — занятая память, %.
  T° — температура CPU, °C (на macOS нужен smctemp, без него 0).

Дополнительные колонки (включаются флагами):
  --cpu      C% — загрузка CPU, % (на macOS — окно в 1 с через iostat)
  --iowait   I% — iowait CPU, % (только Linux; на macOS — «·»)
  --swap     S% — занятый swap, %
  --disk     U% — занято на корневом разделе, %
  --batt     B% — заряд батареи, % (нет батареи — «·»)

Режим квадратов:
  -b, --blocks — вместо цифр цветные квадраты ███, градация цвета
  от зелёного через жёлтый к красному по шкале каждой колонки.
  Статус видно боковым зрением, на цифры смотреть не нужно.
  Тусклый квадрат — датчика нет.

Цвета: зелёный — норма, жёлтый — внимание, красный — плохо.
Пороги (зелёный ≤ / жёлтый ≤ / дальше красный):
  N: 50/150 мс    G: 10/50 мс     L: 4/8      M%: 60/85
  T°: 60/75       C%: 50/80       I%: 5/20    S%: 30/70
  U%: 80/90       B%: наоборот — ≥50 зелёный, ≥20 жёлтый, ниже красный

Диагностика: что значат плохие цифры
  • N и G красные вместе — проблема локальная: Wi-Fi, роутер, кабель.
    Только N красный — внешний маршрут или провайдер.
  • L высокий, а C% низкий — система не считает, а ждёт: диск, сеть,
    локи. Высокий I% подтверждает: ждём диск (Linux).
  • M% красный и S% растёт — память кончилась, ушло в своп, всё станет
    медленным. Закрыть пожирателей памяти или добавить RAM.
  • C% ≈ 100 долго — какой-то процесс съел процессор; виновника
    покажет top.
  • T° > 80–90 — перегрев/троттлинг: забились вентиляторы, пыль
    или длительная пиковая нагрузка.
  • U% > 90 — кончается место на диске; система начинает сбоить.

Использование:
  lmon               — бесконечно, примерно раз в секунду
  lmon 20            — 20 строк и выход
  lmon -b            — режим цветных квадратов
  lmon --cpu --swap  — добавить колонки CPU и swap
  lmon help          — эта справка
  lmon --version     — версия

Настройки (флаги и переменные окружения):
  --target    LMON_TARGET     внешний хост (по умолчанию 8.8.8.8)
  --gw        LMON_GW         шлюз (по умолчанию — автоопределение)
  --interval  LMON_INTERVAL   пауза между строками, сек (по умолчанию 1)

Примеры:
  lmon --target 1.1.1.1
  lmon -b --cpu --batt 30
  LMON_GW=192.168.1.1 lmon 10

Первая строка после запуска может быть завышена: Wi-Fi-радио
просыпается. Дальше значения стабилизируются.

Пинг выполняется системной утилитой ping. Температура (колонка T)
на macOS требует установленной утилиты smctemp
(https://github.com/holtmann/osx/tree/master/smctemp), без неё — 0;
на Linux читается из sysfs (hwmon/thermal_zone).
На Windows (10+) работает всё, кроме L (load average отсутствует)
и T (датчика нет) — вместо них «·»; шлюз ищется через route print.
`

const (
	reset = "\x1b[0m"
	grn   = "\x1b[1;32m"
	yel   = "\x1b[1;33m"
	red   = "\x1b[1;31m"
	dim   = "\x1b[2m"
)

type config struct {
	target   string
	gw       string
	interval time.Duration
	iters    int
	blocks   bool
	cpu      bool
	iowait   bool
	swap     bool
	disk     bool
	batt     bool
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
	gw := fs.String("gw", envStr("LMON_GW", ""), "gateway (auto-detected by default)")
	interval := fs.Float64("interval", envFloat("LMON_INTERVAL", 1), "seconds between lines")
	var blocks bool
	fs.BoolVar(&blocks, "blocks", false, "colored blocks instead of digits")
	fs.BoolVar(&blocks, "b", false, "shorthand for --blocks")
	var cpu, iowait, swap, disk, batt bool
	fs.BoolVar(&cpu, "cpu", false, "add CPU usage column")
	fs.BoolVar(&iowait, "iowait", false, "add iowait column (Linux only)")
	fs.BoolVar(&swap, "swap", false, "add swap usage column")
	fs.BoolVar(&disk, "disk", false, "add root disk usage column")
	fs.BoolVar(&batt, "batt", false, "add battery charge column")

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

	gwHost := *gw
	if gwHost == "" {
		// шлюз по умолчанию определяется автоматически (route / /proc/net/route)
		if detected, err := meter.DefaultGateway(); err == nil {
			gwHost = detected
		} else {
			gwHost = "192.168.0.1"
		}
	}

	d := time.Duration(*interval * float64(time.Second))
	if d <= 0 {
		d = time.Second
	}
	return config{target: *target, gw: gwHost, interval: d, iters: iters,
		blocks: blocks, cpu: cpu, iowait: iowait, swap: swap, disk: disk, batt: batt}, nil
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
	termSetup()
	defer termRestore()

	tty := stdoutIsTTY()
	u := &ui{w: w, tty: tty, color: tty && os.Getenv("NO_COLOR") == "", blocks: cfg.blocks}
	u.head, u.sep = headerOf(layout(cfg))
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

// reading — одно измерение: значение, пороги цвета и оформление колонки.
// Пороги: green/yellow — как в цифровом режиме, red — точка, где градиент
// блоков становится полностью красным. v < 0 — датчика нет («·»).
type reading struct {
	head               string
	v                  int
	green, yellow, red int
	deg                bool // приписать ° (температура)
	invert             bool // больше — лучше (батарея)
}

// layout возвращает колонки в порядке вывода; значения заполнит sampleRow.
func layout(cfg config) []reading {
	rs := []reading{
		{head: "N", green: 50, yellow: 150, red: 300},
		{head: "G", green: 10, yellow: 50, red: 100},
		{head: "L", green: 4, yellow: 8, red: 16},
		{head: "M%", green: 60, yellow: 85, red: 97},
		{head: "T°", green: 60, yellow: 75, red: 90, deg: true},
	}
	if cfg.cpu {
		rs = append(rs, reading{head: "C%", green: 50, yellow: 80, red: 100})
	}
	if cfg.iowait {
		rs = append(rs, reading{head: "I%", green: 5, yellow: 20, red: 50})
	}
	if cfg.swap {
		rs = append(rs, reading{head: "S%", green: 30, yellow: 70, red: 100})
	}
	if cfg.disk {
		rs = append(rs, reading{head: "U%", green: 80, yellow: 90, red: 100})
	}
	if cfg.batt {
		rs = append(rs, reading{head: "B%", green: 50, yellow: 20, red: 0, invert: true})
	}
	return rs
}

// severity нормирует значение в 0..1 (0 — зелёный, 1 — красный) для блоков.
func (r reading) severity() float64 {
	if r.invert { // батарея: полный заряд — зелёный, разряд — к красному
		switch {
		case r.v >= r.green:
			return 0
		case r.v <= 0:
			return 1
		}
		return float64(r.green-r.v) / float64(r.green)
	}
	switch {
	case r.v <= r.green:
		return 0
	case r.v <= r.yellow:
		return 0.5 * float64(r.v-r.green) / float64(r.yellow-r.green)
	case r.v >= r.red:
		return 1
	}
	return 0.5 + 0.5*float64(r.v-r.yellow)/float64(r.red-r.yellow)
}

func (u *ui) sampleRow(ctx context.Context, cfg config) string {
	rs := layout(cfg)

	// Медленные измерения — параллельно, каждый со своим каналом,
	// чтобы значения не перепутались, а цикл не длился сумму задержек.
	netCh := make(chan int, 1)
	gwCh := make(chan int, 1)
	go func() { netCh <- netping.Latency(ctx, cfg.target) }()
	go func() { gwCh <- netping.Latency(ctx, cfg.gw) }()
	var cpuCh chan int
	if cfg.cpu || cfg.iowait {
		cpuCh = make(chan int, 2)
		go func() { a, i := meter.CPUSample(); cpuCh <- a; cpuCh <- i }()
	}

	// быстрые метрики
	load, loadErr := meter.LoadAvg()
	rs[0].v, rs[1].v = <-netCh, <-gwCh
	rs[2].v = -1
	if loadErr == nil {
		rs[2].v = int(load)
	}
	rs[3].v = meter.MemPercent()
	rs[4].v = meter.TempC()
	i := 5
	if cpuCh != nil {
		cpu, iowait := <-cpuCh, <-cpuCh
		if cfg.cpu {
			rs[i].v = cpu
			i++
		}
		if cfg.iowait {
			rs[i].v = iowait
			i++
		}
	}
	if cfg.swap {
		rs[i].v = meter.SwapPercent()
		i++
	}
	if cfg.disk {
		rs[i].v = meter.DiskPercent()
		i++
	}
	if cfg.batt {
		rs[i].v = meter.BatteryPercent()
	}

	cells := make([]string, len(rs))
	for j, r := range rs {
		if u.blocks && u.color {
			cells[j] = u.blockCell(r)
		} else {
			cells[j] = u.digitCell(r)
		}
	}
	return strings.Join(cells, " ")
}

// gradient256 возвращает код цвета 256-цветной палитры для точки шкалы
// 0..1: 0 — зелёный, 0.5 — жёлтый, 1 — красный.
func gradient256(s float64) string {
	r, g, b := rampRGB(s)
	q := func(v int) int { return (v*5 + 128) / 255 } // ближайший куб 6×6×6
	return strconv.Itoa(16 + 36*q(r) + 6*q(g) + q(b))
}

func rampRGB(s float64) (r, g, b int) {
	// зелёный (0,220,0) → жёлтый (255,220,0) → красный (255,40,0)
	switch {
	case s <= 0:
		return 0, 220, 0
	case s >= 1:
		return 255, 40, 0
	}
	t := int(s * 511)
	if t < 256 { // зелёный → жёлтый: красный канал растёт 0..255
		return t, 220, 0
	}
	// жёлтый → красный: зелёный канал падает 220..40
	return 255, 220 - (t-256)*180/255, 0
}

type ui struct {
	w      io.Writer
	tty    bool
	color  bool
	blocks bool
	rows   int
	head   string
	sep    string
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

// headerOf строит строки заголовка и разделителя из списка колонок.
func headerOf(rs []reading) (head, sep string) {
	hs := make([]string, len(rs))
	ss := make([]string, len(rs))
	for i, r := range rs {
		hs[i] = fmt.Sprintf("%3s", r.head)
		ss[i] = "---"
	}
	return strings.Join(hs, " "), strings.Join(ss, " ")
}

func (u *ui) printHeader() {
	if u.color {
		fmt.Fprintf(u.w, "%s%s%s\n%s%s%s\n", dim, u.head, reset, dim, u.sep, reset)
	} else {
		fmt.Fprintf(u.w, "%s\n%s\n", u.head, u.sep)
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

// digitCell — классическое цифровое представление с трёхступенчатым цветом.
func (u *ui) digitCell(r reading) string {
	if r.v < 0 { // датчика нет
		s := fmt.Sprintf("%3s", "·")
		if u.color {
			return dim + s + reset
		}
		return s
	}
	s := fmt.Sprintf("%3d", r.v)
	if r.deg {
		s += "°"
	}
	if !u.color {
		return s
	}
	var c string
	if r.invert {
		switch {
		case r.v >= r.green:
			c = grn
		case r.v >= r.yellow:
			c = yel
		default:
			c = red
		}
	} else {
		switch {
		case r.v <= r.green:
			c = grn
		case r.v <= r.yellow:
			c = yel
		default:
			c = red
		}
	}
	return c + s + reset
}

// blockCell — квадрат-индикатор с непрерывной градацией цвета.
func (u *ui) blockCell(r reading) string {
	if r.v < 0 { // датчика нет
		return dim + "███" + reset
	}
	s := "\x1b[38;5;" + gradient256(r.severity()) + "m███" + reset
	if r.deg {
		s += "°"
	}
	return s
}

func stdoutIsTTY() bool {
	fi, err := os.Stdout.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}
