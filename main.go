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
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/martiyanov/lmon/internal/meter"
	"github.com/martiyanov/lmon/internal/netping"
)

const version = "1.12.0"

const helpTextRu = `lmon ` + version + ` — минимальный монитор сети и нагрузки.

Заголовок закреплён внизу окна, значения стекаются над ним.
Подписи колонок залиты цветом текущего состояния — статус читается
по «ленте» внизу даже в окне высотой 2 строки. При изменении размера
терминала число строк подстраивается, заголовок остаётся на месте.

Колонки (в скобках — буква для --cols):
  NET (N)  — пинг до внешнего хоста (--target / LMON_TARGET, по умолчанию
             8.8.8.8), мс. Поддерживается IPv6-литерал, в том числе
             link-local с зоной: fe80::1%en0. Качество канала до интернета.
  GTW (G)  — пинг до шлюза (--gw / LMON_GW; по умолчанию определяется
             автоматически: route/netstat/ipconfig на macOS, ip route и
             /proc/net/route на Linux, route print на Windows).
             Не определился — «·».
  RUN (L)  — сколько программ в среднем ждёт процессора (классический
             load average, целая часть). На Windows показателя нет («·»).
  MEM (M)  — занятая память, %.
  TMP (T)  — температура CPU, °C. На macOS нужен smctemp (без него «·»);
             на Linux читается из sysfs; на Windows нет датчика («·»).
  CPU (C)  — загрузка CPU, % (--cpu).
  IOW (I)  — iowait CPU, % (--iowait; только Linux, иначе «·»).
  I/O (K)  — занятость диска, % (--disk-busy; Windows, PDH, без админа).
  SWP (S)  — занятые swap/файл подкачки, % (--swap; Windows читает
             pagefile через PDH, без прав администратора).
  DSK (U)  — занято на системном диске, % (--disk).
  BAT (B)  — заряд батареи, % (--batt; шкала перевёрнута: меньше — хуже).
  GPU (P)  — загрузка GPU, % (--gpu; macOS — ioreg без sudo,
             Linux — nvidia-smi или sysfs, Windows — «·»).
   G° (H)  — температура GPU, °C (--gputemp; Linux — nvidia-smi/hwmon,
             macOS/Windows — «·», датчика без прав администратора нет).

Режимы отображения:
  -b, --blocks  — вместо цифр цветные квадраты ███, градация цвета
                  от зелёного через жёлтый к красному по шкале каждой
                  колонки. Статус видно боковым зрением. Тусклый
                  квадрат — датчика нет.
  --hybrid      — смешанный режим: точные цифры на подложке цвета
                  состояния (как нижняя лента, но в потоке данных).

Набор колонок:
  --cols NG,CSB  — свой набор и порядок (каждая буква — колонка,
                   разделители запятая/пробел/плюс, регистр не важен;
                   LMON_COLS). Флаги --cpu и др. добавляют колонки справа.

  Буквы колонок (все двенадцать):
    N — сеть      G — шлюз       L — очередь CPU  M — память
    T — t°C CPU   C — загрузка CPU (%)
    I — iowait    S — swap       U — диск (можно D)  B — батарея
    K — занятость диска (--disk-busy, Windows)
    P — загрузка GPU (%)         H — t°GPU

  Примеры: только сеть и память — lmon --cols NM
           swap перед памятью  — lmon --cols N,SG,M
  --once         — одна строка значений без заголовка; цвет только на
                   терминале, в пайпе — чистый текст (tmux/скрипты):
                   set -g status-right '#(lmon --once --cols=NGMC)'

Цвета: зелёный — норма, жёлтый — внимание, красный — плохо.
Пороги (зелёный ≤ / жёлтый ≤ / дальше красный):
  NET: 50/150 мс   GTW: 10/50 мс   RUN: по ядрам* MEM: 60/85
  TMP: 60/75       CPU: 50/80      IOW: 5/20    SWP: 30/70
  DSK: 80/90       I/O: 60/85   BAT: наоборот — ≥50 зелёный, ≥20 жёлтый, ниже красный
Значения крупнее 999 показываются как +++.
* RUN: зелёный — пока очередь не длиннее числа ядер,
  жёлтый — до двух ядер очереди, дальше красный.

Диагностика: что значат плохие цифры
  • NET и GTW красные вместе — проблема локальная: Wi-Fi, роутер, кабель.
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
  lmon --interval 5  — период обновления 5 секунд (по умолчанию 1)
  lmon -b            — режим цветных квадратов
  lmon --hybrid      — цифры на цветной подложке
  lmon --cpu --swap  — добавить колонки CPU и swap
  lmon --gpu         — добавить колонку загрузки GPU
  lmon --disk-busy   — занятость диска, % (Windows, PDH)
  lmon --cols TNGC   — свой набор и порядок колонок
  lmon --ping VPN=… --cols NG,VPN,M
                     — пинг-колонку тоже можно поставить в нужное место
  lmon --thresholds NET=100/200/500
                     — свои пороги цвета (например, для мобильного
                       интернета); колонка=зелёный/жёлтый/красный
  lmon --ping VPN=10.8.0.1
                     — своя колонка пинга до любого хоста (имя 2–3
                       символа); VPN не подключён — будет 999.
                       Место в выводе — в --cols (NG,VPN,M).
                       Повторяемо: несколько --ping; LMON_PING
  lmon --once        — одна строка значений; цвет только на терминале
  lmon --no-hints    — без подсказок о недоступных колонках
  lmon help          — эта справка
  lmon --version     — версия

Настройки (флаги и переменные окружения):
  --target    LMON_TARGET     внешний хост: IPv4, DNS-имя или IPv6-литерал
  --gw        LMON_GW         шлюз (по умолчанию — автоопределение;
                              семейство адресов — как у --target)
  --interval    LMON_INTERVAL     период обновления, сек (0 < x ≤ 3600).
                              При таймаутах пинга строка идёт реже:
                              строка = измерение + интервал; бюджет пинга
                              ограничен двумя интервалами
  --thresholds  LMON_THRESHOLDS   свои пороги цвета: КОЛОНКА=з/ж/к через
                              запятую, числа через / — NET=100/200/500,
                              RUN=8/16/32; LMON_THRESHOLDS
  --disk-busy   LMON_DISK_BUSY  занятость диска, % (Windows, PDH; колонка I/O)
  --temp-cmd    LMON_TEMP_CMD  внешняя команда, печатающая температуру CPU
                              в °C (берётся первое число). Нужна на Windows,
                              где нет штатного API: например, LHM через
                              PowerShell.

Примеры:
  lmon --target 1.1.1.1
  lmon -b --cpu --batt 30
  LMON_COLS=NGMC lmon --once

Колонки, которых в системе нет, скрываются автоматически (например,
RUN и C° в Windows, iowait в macOS): вместо датчика не показывается «·».
При старте lmon печатает, что скрыто и чем заменить (--cpu и т. п.).
«·» остаётся только для датчиков, которые бывают, но сейчас недоступны
(TMP без smctemp, GPU без nvidia-smi). 999 — ICMP-ответ не пришёл:
хост, файрвол или сеть. Подсказки не блокируют запуск; отключить:
--no-hints или LMON_NO_HINTS=1.

Первая строка после запуска может быть завышена: Wi-Fi-радио
просыпается. Дальше значения стабилизируются.

Пинг выполняется системной утилитой ping (нет утилиты — «·»).
Интервал отсчитывается от начала цикла, поэтому долгие измерения
не накапливают задержку.

Язык сообщений: lmon --lang ru|en — иначе автоматически по локали
(LANG/LC_ALL). Эта справка: lmon --lang ru help.
`

const helpTextEn = `lmon ` + version + ` — a minimal network and system load monitor.

The header ribbon is pinned to the bottom of the window; values stream
above it. Header cells carry the color of the current state, so the
status is readable at a glance even in a 2-line window. On resize the
number of lines adapts; the header stays in place.

Columns (letter in parentheses is for --cols):
  NET (N)  — ping to an external host (--target / LMON_TARGET, default
             8.8.8.8), ms. IPv6 literals supported, including link-local
             with a zone: fe80::1%en0. Overall internet link quality.
  GTW (G)  — ping to the gateway (--gw / LMON_GW; auto-detected by
             default: route/netstat/ipconfig on macOS, ip route and
             /proc/net/route on Linux, route print on Windows).
             Not detected — «·».
  RUN (L)  — average number of programs waiting for the CPU (the classic
             load average, integer part). Not available on Windows («·»).
  MEM (M)  — memory in use, %.
  TMP (T)  — CPU temperature, °C. On macOS requires smctemp (otherwise «·»);
             on Linux read from sysfs; on Windows there is no sensor («·»).
  CPU (C)  — CPU usage, % (--cpu).
  IOW (I)  — CPU iowait, % (--iowait; Linux only, otherwise «·»).
  I/O (K)  — disk busy time, % (--disk-busy; Windows, PDH, no admin).
  SWP (S)  — swap/pagefile in use, % (--swap; on Windows it reads
             pagefile via PDH, no admin rights).
  DSK (U)  — root disk usage, % (--disk).
  BAT (B)  — battery charge, % (--batt; inverted scale: less is worse).
  GPU (P)  — GPU utilization, % (--gpu; on macOS via ioreg without sudo,
             on Linux via nvidia-smi or sysfs, on Windows — «·»).
   G° (H)  — GPU temperature, °C (--gputemp; on Linux via nvidia-smi/hwmon,
             macOS/Windows — «·», no sensor without admin rights).

Display modes:
  -b, --blocks  — colored blocks ███ instead of digits; color grades from
                  green through yellow to red along each column's scale.
                  Status is visible with peripheral vision. A dim block
                  means no sensor.
  --hybrid      — mixed mode: exact digits on the state-colored background
                  (like the bottom ribbon, but in the data stream).

Column set:
  --cols NG,CSB  — your own set and order (each letter is a column;
                   separators comma/space/plus, case-insensitive;
                   LMON_COLS). Flags like --cpu add columns on the right.

  Column letters (all twelve):
    N — network   G — gateway    L — CPU queue  M — memory
    T — CPU °C    C — CPU usage (%)
    I — iowait    S — swap       U — disk (D works too)  B — battery
    K — disk busy (--disk-busy, Windows)
    P — GPU usage (%)            H — GPU °C

  Examples: network and memory only — lmon --cols NM
            swap before memory    — lmon --cols N,SG,M

Colors: green — normal, yellow — attention, red — bad.
Thresholds (green ≤ / yellow ≤ / then red):
  NET: 50/150 ms   GTW: 10/50 ms   RUN: per cores* MEM: 60/85
  TMP: 60/75       CPU: 50/80      IOW: 5/20    SWP: 30/70
  DSK: 80/90       I/O: 60/85   BAT: inverted — ≥50 green, ≥20 yellow, below red
Values above 999 are shown as +++.
* RUN: green while the queue fits within the core count,
  yellow up to twice the cores, red above that.

Diagnostics: what bad numbers mean
  • NET and GTW both red — the problem is local: Wi-Fi, router, cable.
    Only NET red — the route or the ISP.
  • RUN high while CPU is low — the system is waiting, not computing:
    disk, network, locks. High IOW confirms: waiting on the disk (Linux).
  • MEM red and SWP growing — memory is exhausted, everything went to
    swap and will get slow. Close memory hogs or add RAM.
  • CPU ≈ 100 for a long time — a process is eating the CPU; top will
    name the culprit.
  • TMP > 80–90 — overheating/throttling: clogged fans, dust,
    or a sustained peak load.
  • DSK > 90 — the disk is running out of space; things start to break.

Usage:
  lmon               — forever, about once per second
  lmon 20            — 20 lines, then exit
  lmon --interval 5  — refresh period of 5 seconds (default 1)
  lmon -b            — colored blocks mode
  lmon --hybrid      — digits on the state-colored background
  lmon --cpu --swap  — add CPU and swap columns
  lmon --gpu         — add the GPU utilization column
  lmon --disk-busy   — disk busy time %, Windows (PDH)
  lmon --cols TNGC   — your own set and order of columns
  lmon --ping VPN=… --cols NG,VPN,M
                     — ping columns can be placed in the order too
  lmon --thresholds NET=100/200/500
                     — your own color thresholds (e.g. for mobile
                       internet); column=green/yellow/red
  lmon --ping VPN=10.8.0.1
                     — your own ping column to any host (2-3 char
                       name); VPN down — the column shows 999.
                       Position it with --cols (NG,VPN,M).
                       Repeatable; LMON_PING
  lmon --once        — one line; colored only on a terminal
  lmon --no-hints    — start without hints about unavailable columns
  lmon help          — this help
  lmon --version     — version

Settings (flags and environment variables):
  --target    LMON_TARGET     external host: IPv4, DNS name or IPv6 literal
  --gw        LMON_GW         gateway (auto-detected by default; the address
                              family must match --target)
  --interval    LMON_INTERVAL   refresh period, seconds (0 < x ≤ 3600).
                              On ping timeouts rows come slower:
                              a row = measurement + interval; the ping
                              budget is capped at two intervals
  --thresholds  LMON_THRESHOLDS   your own color thresholds:
                              COL=g/y/r, comma-separated groups, numbers
                              slashed — NET=100/200/500, RUN=8/16/32;
                              LMON_THRESHOLDS
  --disk-busy   LMON_DISK_BUSY  disk busy time %, Windows (PDH; column I/O)
  --temp-cmd    LMON_TEMP_CMD  external command that prints CPU temperature
                              in °C (the first number is used). Needed on
                              Windows where no native API exists; e.g. LHM
                              via PowerShell.

Examples:
  lmon --target 1.1.1.1
  lmon -b --cpu --batt 30
  LMON_COLS=NGMC lmon --once

Columns unsupported by the OS are hidden automatically (e.g. RUN and C°
on Windows, iowait on macOS) instead of showing «·». At startup lmon
prints what was hidden and what to use instead (--cpu etc.). «·» stays
only for sensors that usually exist but are unavailable right now
(TMP without smctemp, GPU without nvidia-smi). 999 — no ICMP reply:
host, firewall or network. Hints never block the run; disable them
with --no-hints or LMON_NO_HINTS=1.

The first line after start may be inflated: the Wi-Fi radio is waking
up. Values settle after that.

Ping uses the system ping utility (no utility — «·»).
The interval is counted from the start of the cycle, so slow
measurements do not accumulate delay.

Message language: lmon --lang en|ru — otherwise detected automatically
from the locale (LANG/LC_ALL). This help: lmon --lang en help.
`

// helpTextFor выбирает справку по языку сообщений (en — международный
// по умолчанию, ru — русский).
func helpTextFor(lang string) string {
	if lang == "ru" {
		return helpTextRu
	}
	return helpTextEn
}

const (
	reset = "\x1b[0m"
	grn   = "\x1b[1;32m"
	yel   = "\x1b[1;33m"
	red   = "\x1b[1;31m"
	dim   = "\x1b[2m"
)

type config struct {
	target     string
	gw         string
	gwAuto     bool // шлюз определён автоматически — можно переопределить позже
	interval   time.Duration
	iters      int
	once       bool
	blocks     bool
	hybrid     bool
	cpu        bool
	iowait     bool
	swap       bool
	disk       bool
	diskBusy   bool // --disk-busy: занятость диска (Windows, PDH)
	batt       bool
	gpu        bool
	gputemp    bool
	cols       []string
	lang       string            // язык help и сообщений: "en" (умолчание) или "ru"
	thresholds map[string][3]int // --thresholds: пороги цвета по колонкам
	pings      []pingTarget      // --ping NAME=host: именованные пинги
	noHints    bool              // --no-hints / LMON_NO_HINTS: без стартовых подсказок
	tempCmd    string            // --temp-cmd: внешняя команда температуры (°C)
}

// pingTarget — именованная пинг-колонка (--ping VPN=10.8.0.1):
// имя из 2–3 символов становится заголовком колонки.
type pingTarget struct {
	name string
	host string
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout))
}

func run(args []string, w io.Writer) int {
	// help/version ловим в любом месте аргументов: и «help», и «--lang ru help»
	for _, a := range args {
		switch a {
		case "help", "-h", "--help":
			// язык справки: токен ru/en среди аргументов (--lang ru help,
			// help ru), иначе — автоопределение по локали
			lang := langFromArgs(args)
			io.WriteString(w, helpTextFor(lang)) // не fmt: в тексте есть %en0, vet примет за формат-директиву
			return 0
		case "--version":
			fmt.Fprintf(w, "lmon %s\n", version)
			return 0
		}
	}

	cfg, err := parseConfig(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "lmon: %v %s\n", err,
			lmsg(langFromArgs(args), "(see lmon help)", "(см. lmon help)"))
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
	var blocks, hybrid bool
	fs.BoolVar(&blocks, "blocks", false, "colored blocks instead of digits")
	fs.BoolVar(&blocks, "b", false, "shorthand for --blocks")
	fs.BoolVar(&hybrid, "hybrid", false, "digits on the state-colored background")
	lang := fs.String("lang", langAuto(), "message language: en or ru")
	var cpu, iowait, swap, disk, diskBusy, batt, gpu, gputemp bool
	fs.BoolVar(&cpu, "cpu", false, "add CPU usage column")
	fs.BoolVar(&iowait, "iowait", false, "add iowait column (Linux only)")
	fs.BoolVar(&swap, "swap", false, "add swap usage column")
	fs.BoolVar(&disk, "disk", false, "add root disk usage column")
	fs.BoolVar(&diskBusy, "disk-busy", envFlag("LMON_DISK_BUSY"), "add disk busy column (Windows, PDH)")
	fs.BoolVar(&batt, "batt", false, "add battery charge column")
	fs.BoolVar(&gpu, "gpu", false, "add GPU utilization column")
	fs.BoolVar(&gputemp, "gputemp", false, "add GPU temperature column")
	colsFlag := fs.String("cols", os.Getenv("LMON_COLS"), "base column set and order, e.g. NG,CSB; --ping names can be placed too (NG,VPN,M)")
	thrFlag := fs.String("thresholds", os.Getenv("LMON_THRESHOLDS"), "per-column color thresholds COL=green/yellow/red (e.g. NET=100/200/500)")
	// --ping повторяемый: копим сырые строки, валидируем после разбора
	var pingsRaw rawList
	if env := os.Getenv("LMON_PING"); env != "" {
		for _, part := range strings.Split(env, ",") {
			_ = pingsRaw.Set(part)
		}
	}
	fs.Var(&pingsRaw, "ping", "named ping column NAME=host, repeatable (e.g. --ping VPN=10.8.0.1)")
	var once bool
	fs.BoolVar(&once, "once", false, "print one line of values and exit (for tmux/status bars)")
	noHints := fs.Bool("no-hints", envFlag("LMON_NO_HINTS"), "do not print startup hints about unavailable columns")
	tempCmd := fs.String("temp-cmd", envStr("LMON_TEMP_CMD", ""), "external command that prints CPU temperature (°C)")

	if err := fs.Parse(args); err != nil {
		return config{}, err
	}

	lg := *lang
	if lg != "en" && lg != "ru" {
		return config{}, fmt.Errorf(lmsg(lg, "--lang=%q: expected en or ru", "--lang=%q: ожидается en или ru"), lg)
	}
	if blocks && hybrid {
		return config{}, errors.New(lmsg(lg, "--blocks and --hybrid are mutually exclusive", "--blocks и --hybrid взаимоисключающи"))
	}

	var iters int
	if fs.NArg() > 1 {
		return config{}, errors.New(lmsg(lg, "expected at most one positional argument (line count)", "ожидается не более одного позиционного аргумента (число строк)"))
	}
	if fs.NArg() == 1 {
		n, err := strconv.Atoi(fs.Arg(0))
		if err != nil || n < 0 {
			return config{}, fmt.Errorf(lmsg(lg, "argument %q: expected a line count", "аргумент %q: ожидается число строк"), fs.Arg(0))
		}
		iters = n
	}

	if math.IsNaN(*interval) || math.IsInf(*interval, 0) || *interval <= 0 || *interval > 3600 {
		return config{}, fmt.Errorf(lmsg(lg, "--interval=%v: expected seconds in (0; 3600]", "--interval=%v: нужно число секунд в (0; 3600]"), *interval)
	}
	if err := checkHost(*target, lg); err != nil {
		return config{}, fmt.Errorf("--target: %w", err)
	}

	// gwAuto — режим автоопределения (шлюз не задан вручную), а не факт
	// удачного обнаружения: сеть может появиться и позже.
	gwHost := *gw
	gwAuto := gwHost == ""
	if !gwAuto {
		if err := checkHost(gwHost, lg); err != nil {
			return config{}, fmt.Errorf("--gw: %w", err)
		}
		// NET и GW работают в одном семействе адресов
		if isV6(*target) != isV6(gwHost) {
			return config{}, errors.New(lmsg(lg, "--target and --gw must be both IPv4 or both IPv6", "--target и --gw должны быть оба IPv4 или оба IPv6"))
		}
	}

	// --thresholds / LMON_THRESHOLDS: свои пороги цвета, «NET=100,200,500».
	// Для обычных колонок числа растут (зелёный < жёлтый < красный),
	// для перевёрнутых (батарея) — убывают.
	thr, err := parseThresholds(*thrFlag, lg)
	if err != nil {
		return config{}, err
	}

	// --ping / LMON_PING: именованные пинг-колонки, «VPN=10.8.0.1».
	pings, err := parsePings(pingsRaw, lg)
	if err != nil {
		return config{}, err
	}

	// --cols / LMON_COLS: набор и порядок колонок; сюда же можно ставить
	// именованные пинг-колонки из --ping (VPN, DNS…).
	cols, err := parseCols(*colsFlag, pings, lg)
	if err != nil {
		return config{}, err
	}

	d := time.Duration(*interval * float64(time.Second))
	if d <= 0 {
		d = time.Second
	}
	return config{target: *target, gw: gwHost, gwAuto: gwAuto, interval: d, iters: iters, cols: cols, once: once,
		blocks: blocks, hybrid: hybrid, cpu: cpu, iowait: iowait, swap: swap, disk: disk, diskBusy: diskBusy, batt: batt,
		gpu: gpu, gputemp: gputemp, lang: lg, thresholds: thr, pings: pings,
		noHints: *noHints, tempCmd: *tempCmd}, nil
}

// rawList — повторяемый флаг: каждое вхождение добавляет строку.
type rawList []string

func (r *rawList) String() string { return strings.Join(*r, ",") }
func (r *rawList) Set(v string) error {
	*r = append(*r, v)
	return nil
}

// isPingName — подходит ли строка как имя именованного пинга:
// 2–3 заглавные буквы/цифры (однобуквенные заняты встроенными колонками).
func isPingName(s string) bool {
	if n := len(s); n < 2 || n > 3 {
		return false
	}
	for _, c := range s {
		if !(c >= 'A' && c <= 'Z' || c >= '0' && c <= '9') {
			return false
		}
	}
	return true
}

// parsePings разбирает «VPN=10.8.0.1»: имя 2–3 символа (однобуквенные
// заняты встроенными колонками), хост — как у --target.
func parsePings(raw rawList, lang string) ([]pingTarget, error) {
	var pings []pingTarget
	seen := map[string]bool{}
	for _, item := range raw {
		kv := strings.SplitN(strings.TrimSpace(item), "=", 2)
		if len(kv) != 2 {
			return nil, fmt.Errorf(lmsg(lang,
				"--ping: expected NAME=host, got %q",
				"--ping: ожидается ИМЯ=хост, получено %q"), item)
		}
		name := strings.ToUpper(strings.TrimSpace(kv[0]))
		host := strings.TrimSpace(kv[1])
		if !isPingName(name) {
			return nil, fmt.Errorf(lmsg(lang,
				"--ping: name %q must be 2-3 letters/digits (single letters are built-in columns)",
				"--ping: имя %q должно быть из 2–3 букв/цифр (однобуквенные заняты встроенными колонками)"), kv[0])
		}
		if seen[name] {
			return nil, fmt.Errorf(lmsg(lang,
				"--ping: name %q given twice",
				"--ping: имя %q указано дважды"), name)
		}
		if err := checkHost(host, lang); err != nil {
			return nil, fmt.Errorf(lmsg(lang, "--ping %s: %v", "--ping %s: %v"), name, err)
		}
		seen[name] = true
		pings = append(pings, pingTarget{name: name, host: host})
	}
	return pings, nil
}

// parseCols разбирает --cols / LMON_COLS. Токены разделяются запятой,
// пробелом или плюсом. Токен-имя пинга (--ping VPN=…) ставится в нужное
// место; одиночные буквы — обычные колонки. Компактная запись без
// разделителей (NGM) работает как раньше; чтобы вставить пинг, отделяй
// его разделителем: NG,VPN,M.
func parseCols(spec string, pings []pingTarget, lang string) ([]string, error) {
	if spec == "" {
		return nil, nil
	}
	pingByName := make(map[string]string, len(pings))
	names := make([]string, 0, len(pings))
	for _, p := range pings {
		pingByName[p.name] = p.name
		names = append(names, p.name)
	}
	seen := map[string]bool{}
	var cols []string
	add := func(id string) error {
		if seen[id] {
			return fmt.Errorf(lmsg(lang, "--cols: column %q listed twice", "--cols: колонка %q указана дважды"), id)
		}
		seen[id] = true
		cols = append(cols, id)
		return nil
	}
	for _, tok := range strings.FieldsFunc(spec, func(r rune) bool {
		return r == ',' || r == ' ' || r == '+' || r == '\t'
	}) {
		up := strings.ToUpper(tok)
		if name, ok := pingByName[up]; ok {
			if err := add(name); err != nil {
				return nil, err
			}
			continue
		}
		// Токен из нескольких символов, который не имя пинга: это либо
		// компактный набор букв (NGM), либо опечатка/незаданный пинг.
		if len(up) > 1 {
			onlyLetters := true
			for _, r := range up {
				if r == 'D' {
					continue
				}
				if _, ok := colRegistry[string(r)]; !ok {
					onlyLetters = false
					break
				}
			}
			if !onlyLetters {
				hint := "N G L M T C I S U B P H K"
				return nil, fmt.Errorf(lmsg(lang,
					"--cols: unknown column or ping name %q (columns: %s; define a ping first: --ping %s=host)",
					"--cols: неизвестная колонка или имя пинга %q (колонки: %s; пинг нужно сначала задать: --ping %s=host)"), up, hint, up)
			}
		}
		for _, r := range up {
			if r == 'D' { // D — понятная буква для диска, U — историческая
				r = 'U'
			}
			id := string(r)
			if _, ok := colRegistry[id]; !ok {
				hint := "N G L M T C I S U B P H K"
				if len(names) > 0 {
					hint += ", " + strings.Join(names, " ")
				}
				return nil, fmt.Errorf(lmsg(lang,
					"--cols: unknown column %q (available: %s; ping names only after --ping)",
					"--cols: неизвестная колонка %q (есть: %s; имена пингов — только после --ping)"), id, hint)
			}
			if err := add(id); err != nil {
				return nil, err
			}
		}
	}
	if len(cols) == 0 {
		return nil, errors.New(lmsg(lang, "--cols: empty column set", "--cols: пустой набор колонок"))
	}
	return cols, nil
}

// parseThresholds разбирает «NET=100,200,500,RUN=8,16,32»: колонка и три
// порога — зелёный, жёлтый, красный. D принимается как алиас U (диск).
func parseThresholds(spec, lang string) (map[string][3]int, error) {
	none := map[string][3]int{}
	if spec == "" {
		return none, nil
	}
	thr := map[string][3]int{}
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		eq := strings.SplitN(part, "=", 2)
		if len(eq) != 2 {
			return none, fmt.Errorf(lmsg(lang,
				"--thresholds: expected COL=green,yellow,red, got %q",
				"--thresholds: ожидается КОЛОНКА=зелёный,жёлтый,красный, получено %q"), part)
		}
		id := strings.ToUpper(strings.TrimSpace(eq[0]))
		if id == "D" {
			id = "U"
		}
		if _, ok := colRegistry[id]; !ok {
			// можно и имя колонки (NET, GTW, RUN…), не только букву
			for k, r := range colRegistry {
				if strings.ToUpper(strings.TrimSpace(r.head)) == id {
					id = k
					break
				}
			}
		}
		base, ok := colRegistry[id]
		if !ok && !isPingName(id) {
			// не буква, не имя колонки и не имя именованного пинга
			return none, fmt.Errorf(lmsg(lang,
				"--thresholds: unknown column %q (available: N G L M T C I S U B P H K)",
				"--thresholds: неизвестная колонка %q (есть: N G L M T C I S U B P H K)"), eq[0])
		}
		var t [3]int
		ns := strings.Split(eq[1], "/")
		if len(ns) != 3 {
			return none, fmt.Errorf(lmsg(lang,
				"--thresholds: %s: expected three numbers green/yellow/red",
				"--thresholds: %s: ожидается три числа через / — зелёный/жёлтый/красный"), id)
		}
		for i, sv := range ns {
			n, err := strconv.Atoi(strings.TrimSpace(sv))
			if err != nil || n < 0 {
				return none, fmt.Errorf(lmsg(lang,
					"--thresholds: %s: %q is not a non-negative number",
					"--thresholds: %s: %q — не неотрицательное число"), id, sv)
			}
			t[i] = n
		}
		if base.invert {
			if !(t[0] > t[1] && t[1] > t[2]) {
				return none, fmt.Errorf(lmsg(lang,
					"--thresholds: %s is inverted (battery): numbers must decrease",
					"--thresholds: %s перевёрнута (батарея): числа должны убывать"), id)
			}
		} else if !(t[0] < t[1] && t[1] < t[2]) {
			return none, fmt.Errorf(lmsg(lang,
				"--thresholds: %s: numbers must increase (green < yellow < red)",
				"--thresholds: %s: числа должны расти (зелёный < жёлтый < красный)"), id)
		}
		thr[id] = t
	}
	return thr, nil
}

// checkHost допускает IPv4-адрес или DNS-имя; остальное (пустая строка,
// аргумент, начинающийся с «-», пробелы, управляющие символы) — ошибка:
// значение попадает в argv системного ping.
func checkHost(h, lang string) error {
	if h == "" {
		return errors.New(lmsg(lang, "empty value", "пустое значение"))
	}
	if strings.HasPrefix(h, "-") {
		return fmt.Errorf(lmsg(lang, "%q starts with a hyphen", "%q начинается с дефиса"), h)
	}
	if ip, err := netip.ParseAddr(h); err == nil {
		switch {
		case ip.Is4():
			return nil
		case ip.Is4In6():
			return errors.New(lmsg(lang, "IPv4-mapped IPv6 is not supported — use a plain IPv4", "IPv4-mapped IPv6 не поддерживается — укажи обычный IPv4"))
		}
		return nil // IPv6-литерал, в том числе с зоной: fe80::1%en0
	}
	for _, r := range h {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '.' || r == '-' || r == '_':
		default:
			return fmt.Errorf(lmsg(lang, "invalid character %q", "недопустимый символ %q"), r)
		}
	}
	return nil
}

// langAuto определяет язык сообщений по локали пользователя
// (LC_ALL → LC_MESSAGES → LANG): начинается с "ru" — русский,
// иначе английский.
func langAuto() string {
	for _, k := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := os.Getenv(k); v != "" {
			if strings.HasPrefix(strings.ToLower(v), "ru") {
				return "ru"
			}
			return "en"
		}
	}
	return "en"
}

// lmsg выбирает сообщение по языку: en — международное умолчание, ru — русское.
func lmsg(lang, en, ru string) string {
	if lang == "ru" {
		return ru
	}
	return en
}

// langFromArgs ищет язык до разбора флагов: --lang=en | --lang en | голый
// токен ru/en («help ru»), чтобы help и сообщения об ошибках звучали
// на выбранном пользователем языке.
func langFromArgs(args []string) string {
	bare := ""
	for i, a := range args {
		v := strings.TrimLeft(a, "-")
		if eq := strings.TrimPrefix(v, "lang="); eq != v {
			if eq == "ru" || eq == "en" {
				return eq
			}
		}
		if v == "lang" && i+1 < len(args) && (args[i+1] == "ru" || args[i+1] == "en") {
			return args[i+1]
		}
		if (a == "ru" || a == "en") && bare == "" {
			bare = a
		}
	}
	if bare != "" {
		return bare
	}
	return langAuto()
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

// envFlag читает булеву настройку из окружения: 1/true/yes/on — включено.
func envFlag(name string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
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
	cols, dropped := layoutWithDropped(cfg, runtime.GOOS)
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
	// без ANSI, без заголовка, без изменения режима терминала.
	// Подсказки — в stderr, чтобы не засорять машиночитаемый stdout.
	if cfg.once {
		rs := measure(ctx, cfg)
		if !cfg.noHints {
			printHints(os.Stderr, startupHints(cfg, rs, runtime.GOOS, dropped))
		}
		// Цвет — как у обычного вывода: включаем только на терминале.
		// В пайп/tmux строка остаётся чистым текстом (скрипты не ломаются).
		tty := stdoutIsTTY() && w == io.Writer(os.Stdout)
		if tty {
			termSetup()
			defer termRestore()
		}
		fmt.Fprintln(w, newOnceUI(w, cfg, tty).dataRow(rs))
		return 0
	}

	termSetup()
	defer termRestore()

	tty := stdoutIsTTY()
	u := &ui{w: w, tty: tty, color: tty && os.Getenv("NO_COLOR") == "", blocks: cfg.blocks, hybrid: cfg.hybrid, lang: cfg.lang}
	u.head, u.sep = headerOf(cols)

	defer u.restore()
	u.setup()
	u.warnIfNarrow()

	// Первый замер — он же основа стартовых подсказок: какая колонка не
	// дала данных, почему и что сделать. Подсказки печатаются один раз,
	// не блокируют запуск и не мешают закреплённому заголовку.
	first := measure(ctx, cfg)
	if !cfg.noHints {
		if hs := startupHints(cfg, first, runtime.GOOS, dropped); len(hs) > 0 {
			if tty {
				u.note(hs)
			} else {
				printHints(os.Stderr, hs)
			}
		}
	}
	u.emit(first)
	if cfg.iters > 0 && cfg.iters <= 1 {
		return 0
	}

	// интервал — от строки к строке (nextTick): долгое измерение замедляет
	// каденс, но «задолженные» тики не отдаются пачкой — каденс ровный
	ticker := time.NewTicker(cfg.interval)
	defer ticker.Stop()

	// сеть меняется (Wi-Fi↔Ethernet, VPN) — переопределяем шлюз, но не чаще раза в 30 с
	refreshGW := time.NewTicker(30 * time.Second)
	defer refreshGW.Stop()

	for it := 2; ; it++ {
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
		u.emit(measure(ctx, cfg))
		if cfg.iters > 0 && it >= cfg.iters {
			return 0
		}
		if err := nextTick(ctx, ticker, cfg.interval); err != nil {
			return 130
		}
	}
}

// newOnceUI собирает ui для --once: tty передаётся явно, чтобы цвет
// включался ровно на терминале и не зависел от os.Stdout в тестах.
func newOnceUI(w io.Writer, cfg config, tty bool) *ui {
	return &ui{
		w:      w,
		tty:    tty,
		color:  tty && os.Getenv("NO_COLOR") == "",
		blocks: cfg.blocks,
		hybrid: cfg.hybrid,
		lang:   cfg.lang,
	}
}

// printHints печатает стартовые подсказки вне TUI: в --once, при выводе в
// пайп и в любой ситуации, когда stdout должен остаться машиночитаемым.
func printHints(w io.Writer, hints []startupHint) {
	for _, h := range hints {
		fmt.Fprintf(w, "lmon: %s\n", h.title)
		if h.detail != "" {
			fmt.Fprintf(w, "      %s\n", h.detail)
		}
	}
}

// nextTick перезапускает тикер после выведенной строки: интервал
// отсчитывается от строки к строке. За долгое измерение (таймаут пинга)
// тикер успевает натикать «задолженные» тики — без перезапуска они
// отдаются сразу, и строки догоняют друг друга пачками.
func nextTick(ctx context.Context, t *time.Ticker, d time.Duration) error {
	t.Reset(d)
	// Reset не выбрасывает тик, накопившийся в буфере канала за время
	// долгого измерения (поймано тестом TestNextTickSpacing) — выкидываем сами
	select {
	case <-t.C:
	default:
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// pingBudget ограничивает одно измерение пинга в зависимости от периода
// обновления: удвоенный интервал, но не меньше 1.5с и не больше 4с
// (4с — внешний предел exec-контекста в netping). При --interval 5
// и больше ограничение фактически не действует.
func pingBudget(interval time.Duration) time.Duration {
	b := 2 * interval
	if b < 1500*time.Millisecond {
		b = 1500 * time.Millisecond
	}
	return min(b, 4*time.Second)
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
	invert             bool // больше — лучше (батарея)
}

// colRegistry — все известные колонки; ключ — однобуквенный id из --cols.
var colRegistry = map[string]reading{
	"N": {id: "N", head: "NET", green: 50, yellow: 150, red: 300},
	"G": {id: "G", head: "GTW", green: 10, yellow: 50, red: 100},
	"L": {id: "L", head: "RUN", green: 4, yellow: 8, red: 16},
	"M": {id: "M", head: "MEM", green: 60, yellow: 85, red: 97},
	"T": {id: "T", head: " C°", green: 60, yellow: 75, red: 90},
	"C": {id: "C", head: "CPU", green: 50, yellow: 80, red: 100},
	"I": {id: "I", head: "IOW", green: 5, yellow: 20, red: 50},
	"S": {id: "S", head: "SWP", green: 30, yellow: 70, red: 100},
	"U": {id: "U", head: "DSK", green: 80, yellow: 90, red: 100},
	"B": {id: "B", head: "BAT", green: 50, yellow: 20, red: 0, invert: true},
	"P": {id: "P", head: "GPU", green: 50, yellow: 85, red: 100},
	"H": {id: "H", head: " G°", green: 60, yellow: 80, red: 90},
	"K": {id: "K", head: "I/O", green: 60, yellow: 85, red: 100}, // диск занят (Windows, PDH)
}

// layout возвращает колонки в порядке вывода; значения заполнит sampleRow.
// --cols задаёт набор и порядок целиком; без него — базовые колонки
// плюс включённые флагами.
func requestedLayout(cfg config) []reading {
	base := []string{"N", "G", "L", "M", "T"}
	if len(cfg.cols) > 0 {
		base = cfg.cols
	}
	rs := make([]reading, 0, len(base)+5)
	for _, id := range base {
		r, ok := columnByID(id, cfg.pings)
		if !ok {
			continue // не должно случаться: --cols проверен парсером
		}
		rs = append(rs, r)
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
	add("K", cfg.diskBusy)
	add("B", cfg.batt)
	add("P", cfg.gpu)
	add("H", cfg.gputemp)
	// именованные пинги, не указанные в --cols, — справа, порогами как у NET
	for _, p := range cfg.pings {
		r := pingReading(p)
		exists := false
		for _, x := range rs {
			if x.id == r.id {
				exists = true
				break
			}
		}
		if !exists {
			rs = append(rs, r)
		}
	}
	applyThresholds(rs, runtime.NumCPU(), cfg.thresholds)
	return rs
}

// columnByID находит колонку по букве или имени пинга.
func columnByID(id string, pings []pingTarget) (reading, bool) {
	if r, ok := colRegistry[id]; ok {
		return r, true
	}
	for _, p := range pings {
		if p.name == id {
			return pingReading(p), true
		}
	}
	return reading{}, false
}

// pingReading — колонка именованного пинга; пороги как у NET.
func pingReading(p pingTarget) reading {
	return reading{id: p.name, head: fmt.Sprintf("%3s", p.name), green: 50, yellow: 150, red: 300}
}

// tempValue берёт температуру из внешней команды (--temp-cmd), если она
// задана, иначе из штатного датчика ОС.
var (
	extTempMu   sync.Mutex
	extTemp     *meter.CmdTemp
	extTempSpec string
)

func tempValue(cfg config) int {
	if cfg.tempCmd == "" {
		return meter.TempC()
	}
	extTempMu.Lock()
	if extTemp == nil || extTempSpec != cfg.tempCmd {
		extTemp = meter.NewCmdTemp(cfg.tempCmd, 2*time.Second)
		extTempSpec = cfg.tempCmd
	}
	inst := extTemp
	extTempMu.Unlock()
	return inst.TempC()
}

// layout — колонки с учётом возможностей ОС: то, что в системе не
// измерить (RUN/C° в Windows и т. п.), не показывается вовсе.
func layout(cfg config) []reading {
	rs, _ := layoutWithDropped(cfg, runtime.GOOS)
	return rs
}

// layoutWithDropped возвращает видимые колонки и список скрытых: скрытые
// объясняются стартовой подсказкой, чтобы «·» не выглядел поломкой.
func layoutWithDropped(cfg config, goos string) ([]reading, []droppedColumn) {
	rs := requestedLayout(cfg)
	out := make([]reading, 0, len(rs))
	var dropped []droppedColumn
	for _, r := range rs {
		supported := columnSupportedByOS(r.id, goos)
		if r.id == "T" && cfg.tempCmd != "" {
			supported = true // температура из внешней команды
		}
		if supported {
			out = append(out, r)
			continue
		}
		dropped = append(dropped, droppedColumn{id: r.id, head: r.head, reason: supportedReason(r.id)})
	}
	if len(out) == 0 {
		// все запрошенные колонки недоступны — не оставляем пустой экран
		out = append(out, colRegistry["M"])
	}
	return out, dropped
}

// applyThresholds подгоняет пороги цвета под конкретную машину и владельца:
//   - RUN (load average) красится по числу ядер: load — это длина очереди,
//     поэтому «все ядра заняты» — ещё норма (практика мониторинга:
//     предупреждают при 0.7–1.0×ядер, критично при 1.5–2×ядер;
//     мы берём зелёный ≤ ядер, жёлтый ≤ 2×ядер, красный выше);
//   - явное --thresholds перекрывает любые умолчания, включая ядра.
func applyThresholds(rs []reading, cores int, thr map[string][3]int) {
	if cores < 1 {
		cores = 1
	}
	for i := range rs {
		if t, ok := thr[rs[i].id]; ok {
			rs[i].green, rs[i].yellow, rs[i].red = t[0], t[1], t[2]
			continue
		}
		if rs[i].id == "L" {
			rs[i].green = cores
			rs[i].yellow = 2 * cores
			rs[i].red = 4 * cores
		}
	}
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

func measure(ctx context.Context, cfg config) []reading {
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
	// Бюджет пинга привязан к периоду обновления: при коротком интервале
	// мёртвый хост быстрее помечается 999, строка не ждёт полных таймаутов.
	budget := pingBudget(cfg.interval)
	vals := make(map[string]int, len(rs))
	var netCh, gwCh chan netping.Result
	if need("N") {
		netCh = make(chan netping.Result, 1)
		go func() { netCh <- netping.Latency(ctx, cfg.target, budget) }()
	}
	if need("G") {
		if cfg.gw == "" { // шлюз не определился — датчика нет
			vals["G"] = -1
		} else {
			gwCh = make(chan netping.Result, 1)
			go func() { gwCh <- netping.Latency(ctx, cfg.gw, budget) }()
		}
	}
	// именованные пинги (--ping VPN=10.8.0.1): каждый — своя колонка и канал
	pingChs := make(map[string]chan netping.Result, len(cfg.pings))
	for _, p := range cfg.pings {
		if need(p.name) {
			ch := make(chan netping.Result, 1)
			pingChs[p.name] = ch
			go func(ch chan netping.Result, host string) { ch <- netping.Latency(ctx, host, budget) }(ch, p.host)
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
	for name, ch := range pingChs {
		vals[name] = pingValue(<-ch)
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
		vals["T"] = tempValue(cfg)
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
	if need("K") {
		vals["K"] = meter.DiskBusyPercent()
	}
	if need("B") {
		vals["B"] = meter.BatteryPercent()
	}
	if need("P") {
		vals["P"] = meter.GPUPercent()
	}
	if need("H") {
		vals["H"] = meter.GPUTempC()
	}
	for i := range rs {
		rs[i].v = vals[rs[i].id] // отсутствует в map → 0 не бывает: need() покрыл все
	}

	return rs
}

// dataRow — строка значений (цифры или квадраты) без служебных символов.
func (u *ui) dataRow(rs []reading) string {
	cells := make([]string, len(rs))
	for j, r := range rs {
		switch {
		case u.hybrid && u.color: // цифры на подложке цвета состояния
			cells[j] = u.hybridCell(r)
		case u.blocks && u.color:
			cells[j] = u.blockCell(r)
		default:
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
	hybrid bool
	lang   string
	rows   int
	head   string
	sep    string
	lastRS []reading
}

func (u *ui) setup() {
	if !u.tty {
		// вне терминала заголовок обычный, сверху
		fmt.Fprintf(u.w, "%s\n%s\n", u.head, u.sep)
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
		fmt.Fprintln(os.Stderr, "lmon: "+lmsg(u.lang, "terminal is narrower than the column set — lines will wrap", "терминал уже набора колонок — строки будут переноситься"))
	}
}

func (u *ui) maybeResize() {
	if u.tty {
		// -1 = размер неизвестен (ошибка ioctl) — не трогаем экран,
		// иначе фейковое «24» вызывало полную перерисовку и мелькание
		if r := termRows(); r > 0 && r != u.rows {
			u.resize(r)
		}
	}
}

// resize меняет область и перекрашивает заголовок БЕЗ очистки экрана:
// содержимое остаётся, никакой вспышки.
func (u *ui) resize(rows int) {
	if rows < 2 {
		rows = 2
	}
	fmt.Fprint(u.w, "\x1b[r") // сброс прежней области прокрутки
	fmt.Fprintf(u.w, "\x1b[1;%dr", rows-1)
	u.paintHeader(rows)
	fmt.Fprintf(u.w, "\x1b[%d;1H", rows-1)
	u.rows = rows
}

func (u *ui) redraw(rows int) {
	if rows < 2 {
		rows = 2 // минимум: одна строка данных + заголовок
	}
	fmt.Fprint(u.w, "\x1b[2J\x1b[H")
	// прокрутка — всё окно, кроме нижней строки заголовка:
	// заголовок закреплён снизу и виден даже в окне высотой 2 строки
	fmt.Fprintf(u.w, "\x1b[1;%dr", rows-1)
	u.paintHeader(rows)
	fmt.Fprintf(u.w, "\x1b[%d;1H", rows-1)
	u.rows = rows
}

// paintHeader перекрашивает нижний заголовок текущими состояниями колонок.
func (u *ui) paintHeader(rows int) {
	fmt.Fprintf(u.w, "\x1b[%d;1H", rows)
	fmt.Fprint(u.w, u.headerRow(u.lastRS))
}

// headerOf строит строки заголовка и разделителя из списка колонок.
func headerOf(rs []reading) (head, sep string) {
	hs := make([]string, len(rs))
	ss := make([]string, len(rs))
	for i, r := range rs {
		hs[i] = r.head // головы в реестре уже выровнены до 3 символов
		ss[i] = "---"
	}
	return strings.Join(hs, " "), strings.Join(ss, " ")
}

// emit выводит строку значений и обновляет цвет нижнего заголовка.
func (u *ui) emit(rs []reading) {
	u.lastRS = rs
	if !u.tty {
		fmt.Fprintln(u.w, u.dataRow(rs))
		return
	}
	fmt.Fprint(u.w, "\r\n") // прокрутка области данных
	fmt.Fprint(u.w, u.dataRow(rs))
	u.paintHeader(u.rows)
	fmt.Fprintf(u.w, "\x1b[%d;1H", u.rows-1)
}

// headerRow — подписи колонок на подложке цвета текущего состояния:
// даже при окне в одну строку данных видно, какая колонка каким цветом.
func (u *ui) headerRow(rs []reading) string {
	if len(rs) == 0 {
		return u.head // до первого измерения — просто подписи
	}
	cells := make([]string, len(rs))
	for i, r := range rs {
		label := r.head
		if r.v < 0 { // датчика нет — нейтральная тусклая подпись
			cells[i] = dim + label + reset
			continue
		}
		cells[i] = "\x1b[38;5;16;48;5;" + gradient256(r.severity()) + "m" + label + reset
	}
	return strings.Join(cells, " ")
}

// restore возвращает нормальный режим терминала (в т.ч. после Ctrl+C).
func (u *ui) restore() {
	if u.tty {
		fmt.Fprint(u.w, "\x1b[r\x1b[?25h\n")
	}
}

// valueText — текст значения фиксированной ширины 3 (без цвета).
func valueText(r reading) string {
	if r.v < 0 { // датчика нет
		return fmt.Sprintf("%3s", "·")
	}
	switch {
	case r.v > 999: // слишком большое — индикатор переполнения, ширина сохраняется
		return "+++"
	}
	return fmt.Sprintf("%3d", r.v) // ° живёт в заголовке колонки, не в значениях
}

// digitCell — классическое цифровое представление с трёхступенчатым цветом.
func (u *ui) digitCell(r reading) string {
	if !u.color {
		return valueText(r)
	}
	if r.v < 0 {
		return dim + valueText(r) + reset
	}
	s := valueText(r)
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

// blockCell — квадрат-индикатор с непррерывной градацией цвета.
// В блочном режиме градусы не приписываются: цвет несёт весь смысл.
func (u *ui) blockCell(r reading) string {
	if r.v < 0 { // датчика нет
		return dim + "███" + reset
	}
	return "\x1b[38;5;" + gradient256(r.severity()) + "m███" + reset
}

// hybridCell — смешанный режим: точные цифры на подложке цвета состояния
// (как нижняя лента заголовка, но в потоке данных).
func (u *ui) hybridCell(r reading) string {
	if r.v < 0 {
		return dim + valueText(r) + reset
	}
	return "\x1b[38;5;16;48;5;" + gradient256(r.severity()) + "m" + valueText(r) + reset
}

func stdoutIsTTY() bool {
	fi, err := os.Stdout.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}
