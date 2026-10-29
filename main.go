// Command lmon is a minimal terminal monitor: a fixed header row followed by
// a stream of measurements — ping to the internet, ping to the gateway,
// load average, memory and CPU temperature — colored green/yellow/red.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"net/netip"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/martiyanov/lmon/internal/meter"
	"github.com/martiyanov/lmon/internal/netping"
)

const version = "1.4.0"

const helpText = `lmon ` + version + ` — минимальный монитор сети и нагрузки.

Заголовок закреплён вверху окна, ниже идут строки значений.
При изменении размера терминала число строк подстраивается,
а заголовок остаётся на месте.

Колонки (в скобках — буква для --cols):
  NET (N)  — пинг до внешнего хоста (--target / LMON_TARGET, по умолчанию
             8.8.8.8), мс. Поддерживается IPv6-литерал, в том числе
             link-local с зоной: fe80::1%%en0. Качество канала до интернета.
  GW  (G)  — пинг до шлюза (--gw / LMON_GW; по умолчанию определяется
             автоматически: route/netstat/ipconfig на macOS, ip route и
             /proc/net/route на Linux, route print на Windows).
             Не определился — «·».
  AVG (L)  — load average, целая часть. На Windows отсутствует («·»).
  MEM (M)  — занятая память, %.
  TMP (T)  — температура CPU, °C. На macOS нужен smctemp (без него «·»);
             на Linux читается из sysfs; на Windows нет датчика («·»).
  CPU (C)  — загрузка CPU, % (--cpu).
  IOW (I)  — iowait CPU, % (--iowait; только Linux, иначе «·»).
  SWP (S)  — занятый swap, % (--swap; на Windows «·»).
  DSK (U)  — занято на системном диске, % (--disk).
  BAT (B)  — заряд батареи, % (--batt; шкала перевёрнута: меньше — хуже).

Режим квадратов:
  -b, --blocks — вместо цифр цветные квадраты ███, градация цвета
  от зелёного через жёлтый к красному по шкале каждой колонки.
  Статус видно боковым зрением. Тусклый квадрат — датчика нет.

Набор колонок:
  --cols NG,CSB  — свой набор и порядок (каждая буква — колонка,
                   разделители запятая/пробел/плюс; LMON_COLS).
                   Флаги --cpu и др. добавляют колонки справа.
  --once         — одна строка значений без заголовка и без ANSI —
                   для tmux-статусбара и скриптов:
                   set -g status-right '#(lmon --once --cols=NGMC)'

Цвета: зелёный — норма, жёлтый — внимание, красный — плохо.
Пороги (зелёный ≤ / жёлтый ≤ / дальше красный):
  NET: 50/150 мс   GW: 10/50 мс    AVG: 4/8     MEM: 60/85
  TMP: 60/75       CPU: 50/80      IOW: 5/20    SWP: 30/70
  DSK: 80/90       BAT: наоборот — ≥50 зелёный, ≥20 жёлтый, ниже красный
Значения крупнее 999 показываются как +++.

Диагностика: что значат плохие цифры
  • NET и GW красные вместе — проблема локальная: Wi-Fi, роутер, кабель.
    Только NET красный — внешний маршрут или провайдер.
  • AVG высокий, а CPU низкий — система не считает, а ждёт: диск, сеть,
    локи. Высокий IOW подтверждает: ждём диск (Linux).
  • MEM красный и SWP растёт — память кончилась, ушло в своп, всё станет
    медленным. Закрыть пожирателей памяти или добавить RAM.
  • CPU ≈ 100 долго — какой-то процесс съел процессор; виновника
    покажет top.
  • TMP > 80–90 — перегрев/троттлинг: забились вентиляторы, пыль
    или длительная пиковая нагрузка.
  • DSK > 90 — кончается место на диске; система начинает сбоить.

Использование:
  lmon               — бесконечно, примерно раз в секунду
  lmon 20            — 20 строк и выход
  lmon -b            — режим цветных квадратов
  lmon --cpu --swap  — добавить колонки CPU и swap
  lmon --cols TNGC   — свой набор и порядок колонок
  lmon --once        — одна строка значений (tmux/скрипты)
  lmon help          — эта справка
  lmon --version     — версия

Настройки (флаги и переменные окружения):
  --target    LMON_TARGET     внешний хост: IPv4, DNS-имя или IPv6-литерал
  --gw        LMON_GW         шлюз (по умолчанию — автоопределение;
                              семейство адресов — как у --target)
  --interval  LMON_INTERVAL   пауза между циклами, сек (0 < x ≤ 3600)

Примеры:
  lmon --target 1.1.1.1
  lmon -b --cpu --batt 30
  LMON_COLS=NGMC lmon --once

Первая строка после запуска может быть завышена: Wi-Fi-радио
просыпается. Дальше значения стабилизируются.

Пинг выполняется системной утилитой ping (нет утилиты — «·»).
Интервал отсчитывается от начала цикла, поэтому долгие измерения
не накапливают задержку.
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
	gwAuto   bool // шлюз определён автоматически — можно переопределить позже
	interval time.Duration
	iters    int
	once     bool
	blocks   bool
	cpu      bool
	iowait   bool
	swap     bool
	disk     bool
	batt     bool
	cols     []string
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
	colsFlag := fs.String("cols", os.Getenv("LMON_COLS"), "base column set and order, e.g. NG,CSB (flags add more)")
	var once bool
	fs.BoolVar(&once, "once", false, "print one line of values and exit (for tmux/status bars)")

	if err := fs.Parse(args); err != nil {
		return config{}, err
	}

	var iters int
	if fs.NArg() > 1 {
		return config{}, errors.New("ожидается не более одного позиционного аргумента (число строк)")
	}
	if fs.NArg() == 1 {
		n, err := strconv.Atoi(fs.Arg(0))
		if err != nil || n < 0 {
			return config{}, fmt.Errorf("аргумент %q: ожидается число строк", fs.Arg(0))
		}
		iters = n
	}

	if math.IsNaN(*interval) || math.IsInf(*interval, 0) || *interval <= 0 || *interval > 3600 {
		return config{}, fmt.Errorf("--interval=%v: нужно число секунд в (0; 3600]", *interval)
	}
	if err := checkHost(*target); err != nil {
		return config{}, fmt.Errorf("--target: %w", err)
	}

	// gwAuto — режим автоопределения (шлюз не задан вручную), а не факт
	// удачного обнаружения: сеть может появиться и позже.
	gwHost := *gw
	gwAuto := gwHost == ""
	if !gwAuto {
		if err := checkHost(gwHost); err != nil {
			return config{}, fmt.Errorf("--gw: %w", err)
		}
		// NET и GW работают в одном семействе адресов
		if isV6(*target) != isV6(gwHost) {
			return config{}, errors.New("--target и --gw должны быть оба IPv4 или оба IPv6")
		}
	}

	// --cols / LMON_COLS: свой набор и порядок колонок; перекрывает флаги.
	// Каждая буква — колонка; запятая/пробел/плюс — разделители: «NG,CSB».
	var cols []string
	if spec := *colsFlag; spec != "" {
		seen := map[string]bool{}
		for _, r := range strings.ToUpper(spec) {
			id := string(r)
			if r == ',' || r == ' ' || r == '+' {
				continue
			}
			if _, ok := colRegistry[id]; !ok {
				return config{}, fmt.Errorf("--cols: неизвестная колонка %q (есть: N G L M T C I S U B)", id)
			}
			if seen[id] {
				return config{}, fmt.Errorf("--cols: колонка %q указана дважды", id)
			}
			seen[id] = true
			cols = append(cols, id)
		}
		if len(cols) == 0 {
			return config{}, errors.New("--cols: пустой набор колонок")
		}
	}

	d := time.Duration(*interval * float64(time.Second))
	if d <= 0 {
		d = time.Second
	}
	return config{target: *target, gw: gwHost, gwAuto: gwAuto, interval: d, iters: iters, cols: cols, once: once,
		blocks: blocks, cpu: cpu, iowait: iowait, swap: swap, disk: disk, batt: batt}, nil
}

// checkHost допускает IPv4-адрес или DNS-имя; остальное (пустая строка,
// аргумент, начинающийся с «-», пробелы, управляющие символы) — ошибка:
// значение попадает в argv системного ping.
func checkHost(h string) error {
	if h == "" {
		return errors.New("пустое значение")
	}
	if strings.HasPrefix(h, "-") {
		return fmt.Errorf("%q начинается с дефиса", h)
	}
	if ip, err := netip.ParseAddr(h); err == nil {
		switch {
		case ip.Is4():
			return nil
		case ip.Is4In6():
			return errors.New("IPv4-mapped IPv6 не поддерживается — укажи обычный IPv4")
		}
		return nil // IPv6-литерал, в том числе с зоной: fe80::1%en0
	}
	for _, r := range h {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '.' || r == '-' || r == '_':
		default:
			return fmt.Errorf("недопустимый символ %q", r)
		}
	}
	return nil
}

func isV6(host string) bool {
	a, err := netip.ParseAddr(host)
	return err == nil && a.Is6() && !a.Is4In6()
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
	// шлюз нужен только если показывается колонка GW
	cols := layout(cfg)
	wantGW := false
	for _, r := range cols {
		if r.id == "G" {
			wantGW = true
			break
		}
	}
	if wantGW {
		refreshGateway(&cfg, meter.DefaultGateway)
	}

	// разовый вывод значений одной строкой — для tmux-статусбара и скриптов:
	// без ANSI, без заголовка, без изменения режима терминала
	if cfg.once {
		fmt.Fprintln(w, (&ui{w: w}).sampleRow(ctx, cfg))
		return 0
	}

	termSetup()
	defer termRestore()

	tty := stdoutIsTTY()
	u := &ui{w: w, tty: tty, color: tty && os.Getenv("NO_COLOR") == "", blocks: cfg.blocks}
	u.head, u.sep = headerOf(cols)

	defer u.restore()
	u.setup()
	u.warnIfNarrow()

	// тикер отсчитывает интервал от начала цикла, а не от конца:
	// долгие измерения не накапливают задержку, потерянные тики пропускаются
	ticker := time.NewTicker(cfg.interval)
	defer ticker.Stop()

	// сеть меняется (Wi-Fi↔Ethernet, VPN) — переопределяем шлюз, но не чаще раза в 30 с
	refreshGW := time.NewTicker(30 * time.Second)
	defer refreshGW.Stop()

	for it := 1; ; it++ {
		select {
		case <-ctx.Done():
			return 130
		default:
		}
		select {
		case <-refreshGW.C:
			refreshGateway(&cfg, meter.DefaultGateway)
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
		case <-ticker.C:
		}
	}
}

// pingValue переводит состояние измерения в значение колонки:
// valid — задержка, unreachable — 999, unavailable — «·» (-1).
func pingValue(r netping.Result) int {
	switch r.State {
	case netping.StateValid:
		return r.ValueMS
	case netping.StateUnreachable:
		return 999
	default:
		return -1
	}
}

// refreshGateway обновляет автоматически определяемый шлюз: появление,
// смена и исчезновение обрабатываются одинаково; ручной --gw не трогаем.
func refreshGateway(cfg *config, detect func(string) (string, error)) {
	if !cfg.gwAuto {
		return
	}
	if gw, err := detect(cfg.target); err == nil && gw != "" {
		cfg.gw = gw
	} else {
		cfg.gw = "" // шлюза сейчас нет — колонка GW покажет «·»
	}
}

// reading — одно измерение: значение, пороги цвета и оформление колонки.
// Пороги: green/yellow — как в цифровом режиме, red — точка, где градиент
// блоков становится полностью красным. v < 0 — датчика нет («·»).
type reading struct {
	id                 string
	head               string
	v                  int
	green, yellow, red int
	deg                bool // приписать ° (температура)
	invert             bool // больше — лучше (батарея)
}

// colRegistry — все известные колонки; ключ — однобуквенный id из --cols.
var colRegistry = map[string]reading{
	"N": {id: "N", head: "NET", green: 50, yellow: 150, red: 300},
	"G": {id: "G", head: "GW", green: 10, yellow: 50, red: 100},
	"L": {id: "L", head: "AVG", green: 4, yellow: 8, red: 16},
	"M": {id: "M", head: "MEM", green: 60, yellow: 85, red: 97},
	"T": {id: "T", head: "TMP", green: 60, yellow: 75, red: 90, deg: true},
	"C": {id: "C", head: "CPU", green: 50, yellow: 80, red: 100},
	"I": {id: "I", head: "IOW", green: 5, yellow: 20, red: 50},
	"S": {id: "S", head: "SWP", green: 30, yellow: 70, red: 100},
	"U": {id: "U", head: "DSK", green: 80, yellow: 90, red: 100},
	"B": {id: "B", head: "BAT", green: 50, yellow: 20, red: 0, invert: true},
}

// layout возвращает колонки в порядке вывода; значения заполнит sampleRow.
// --cols задаёт набор и порядок целиком; без него — базовые колонки
// плюс включённые флагами.
func layout(cfg config) []reading {
	base := []string{"N", "G", "L", "M", "T"}
	if len(cfg.cols) > 0 {
		base = cfg.cols
	}
	rs := make([]reading, 0, len(base)+5)
	for _, id := range base {
		rs = append(rs, colRegistry[id])
	}
	// флаги добавляют колонки справа; уже указанные в --cols не дублируются
	add := func(id string, on bool) {
		if !on {
			return
		}
		for _, r := range rs {
			if r.id == id {
				return
			}
		}
		rs = append(rs, colRegistry[id])
	}
	add("C", cfg.cpu)
	add("I", cfg.iowait)
	add("S", cfg.swap)
	add("U", cfg.disk)
	add("B", cfg.batt)
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
	need := func(id string) bool {
		for _, r := range rs {
			if r.id == id {
				return true
			}
		}
		return false
	}

	// Медленные измерения — параллельно, каждый со своим каналом,
	// чтобы значения не перепутались, а цикл не длился сумму задержек.
	vals := make(map[string]int, len(rs))
	var netCh, gwCh chan netping.Result
	if need("N") {
		netCh = make(chan netping.Result, 1)
		go func() { netCh <- netping.Latency(ctx, cfg.target) }()
	}
	if need("G") {
		if cfg.gw == "" { // шлюз не определился — датчика нет
			vals["G"] = -1
		} else {
			gwCh = make(chan netping.Result, 1)
			go func() { gwCh <- netping.Latency(ctx, cfg.gw) }()
		}
	}
	var cpuCh chan int
	if need("C") || need("I") {
		cpuCh = make(chan int, 2)
		go func() { a, i := meter.CPUSample(); cpuCh <- a; cpuCh <- i }()
	}

	// быстрые метрики — считаем только то, что показывается
	if netCh != nil {
		vals["N"] = pingValue(<-netCh)
	}
	if gwCh != nil {
		vals["G"] = pingValue(<-gwCh)
	}
	if need("L") {
		if load, err := meter.LoadAvg(); err == nil {
			vals["L"] = int(load)
		} else {
			vals["L"] = -1
		}
	}
	if need("M") {
		vals["M"] = meter.MemPercent()
	}
	if need("T") {
		vals["T"] = meter.TempC()
	}
	if cpuCh != nil {
		vals["C"], vals["I"] = <-cpuCh, <-cpuCh
	}
	if need("S") {
		vals["S"] = meter.SwapPercent()
	}
	if need("U") {
		vals["U"] = meter.DiskPercent()
	}
	if need("B") {
		vals["B"] = meter.BatteryPercent()
	}
	for i := range rs {
		rs[i].v = vals[rs[i].id] // отсутствует в map → 0 не бывает: need() покрыл все
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

// warnIfNarrow предупреждает один раз, если терминал уже заголовка:
// строка будет переноситься и таблица поедет.
func (u *ui) warnIfNarrow() {
	if !u.tty {
		return
	}
	if cols := termCols(); cols > 0 && len([]rune(u.head)) > cols {
		fmt.Fprintln(os.Stderr, "lmon: терминал уже набора колонок — строки будут переноситься")
	}
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
	var s string
	switch {
	case r.v > 999: // слишком большое — индикатор переполнения, ширина сохраняется
		s = "+++"
	case r.deg && r.v >= 0 && r.v < 100:
		s = fmt.Sprintf("%2d°", r.v) // ровно 3 позиции; при ≥100 ° уже в заголовке
	default:
		s = fmt.Sprintf("%3d", r.v)
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
// В блочном режиме градусы не приписываются: цвет несёт весь смысл.
func (u *ui) blockCell(r reading) string {
	if r.v < 0 { // датчика нет
		return dim + "███" + reset
	}
	return "\x1b[38;5;" + gradient256(r.severity()) + "m███" + reset
}

func stdoutIsTTY() bool {
	fi, err := os.Stdout.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}
