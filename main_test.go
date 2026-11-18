package main

import (
	"bytes"
	"context"
	"errors"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/martiyanov/lmon/internal/netping"
)

func TestCheckHost(t *testing.T) {
	ok := []string{"8.8.8.8", "1.1.1.1", "example.com", "my-host.example.org", "192.168.1.1",
		"2606:4700:4700::1111", "fe80::1%en0", "::1"}
	for _, h := range ok {
		if err := checkHost(h, "en"); err != nil {
			t.Errorf("checkHost(%q) = %v, want nil", h, err)
		}
	}
	bad := map[string]string{
		"":         "пустое",
		"-x":       "дефис",
		"8.8.8.8 ": "пробел",
		"a b":      "пробел",
		"h\x01":    "управляющий",
	}
	for h, why := range bad {
		if err := checkHost(h, "en"); err == nil {
			t.Errorf("checkHost(%q) = nil, want error (%s)", h, why)
		}
	}
	// сообщения локализуются
	if err := checkHost("", "ru"); err == nil || !strings.Contains(err.Error(), "пустое значение") {
		t.Errorf("checkHost ru: err = %v, want русское сообщение", err)
	}
	if err := checkHost("", "en"); err == nil || !strings.Contains(err.Error(), "empty value") {
		t.Errorf("checkHost en: err = %v, want english message", err)
	}
}

func TestLang(t *testing.T) {
	// автоопределение по локали: LC_ALL важнее LANG
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_MESSAGES", "")
	t.Setenv("LANG", "ru_RU.UTF-8")
	if got := langAuto(); got != "ru" {
		t.Errorf("langAuto(ru_RU) = %q, want ru", got)
	}
	t.Setenv("LANG", "en_US.UTF-8")
	if got := langAuto(); got != "en" {
		t.Errorf("langAuto(en_US) = %q, want en", got)
	}
	t.Setenv("LC_ALL", "ru_RU.UTF-8")
	t.Setenv("LANG", "en_US.UTF-8")
	if got := langAuto(); got != "ru" {
		t.Errorf("langAuto(LC_ALL=ru) = %q, want ru", got)
	}

	// --lang перекрывает локаль;help и ошибки соответствуют языку
	t.Setenv("LC_ALL", "en_US.UTF-8")
	_, err := parseConfig([]string{"--lang", "ru", "--target", "2606:4700::1111", "--gw", "192.168.1.1"})
	if err == nil || !strings.Contains(err.Error(), "IPv4 или оба IPv6") {
		t.Errorf("--lang ru: err = %v, want русская ошибка", err)
	}
	cfg, err := parseConfig([]string{"--lang", "ru"})
	if err != nil {
		t.Fatalf("--lang ru (валидный): err = %v", err)
	}
	if cfg.lang != "ru" {
		t.Errorf("cfg.lang = %q, want ru", cfg.lang)
	}
	if _, err := parseConfig([]string{"--target", "2606:4700::1111", "--gw", "192.168.1.1"}); err == nil ||
		!strings.Contains(err.Error(), "both IPv4 or both IPv6") {
		t.Errorf("en по локали: err = %v, want english error", err)
	}
	if _, err := parseConfig([]string{"--lang", "xx"}); err == nil || !strings.Contains(err.Error(), "en or ru") {
		t.Errorf("--lang xx: err = %v, want ошибка языка", err)
	}
	if _, err := parseConfig([]string{"--bars", "--hybrid"}); err == nil ||
		!strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("-b --hybrid: err = %v, want взаимоисключение", err)
	}

	// справки на месте у обоих языков
	if !strings.Contains(helpTextFor("en"), "Column letters") {
		t.Error("helpTextFor(en): нет шпаргалки букв")
	}
	if !strings.Contains(helpTextFor("ru"), "Буквы колонок") {
		t.Error("helpTextFor(ru): нет шпаргалки букв")
	}
}

func TestBatchUI(t *testing.T) {
	// GW стал трёхбуквенным GTW — лента ровная
	var gtw string
	for _, r := range layout(config{}) {
		if r.id == "G" {
			gtw = r.head
		}
	}
	if gtw != "GTW" {
		t.Errorf("head GW = %q, want GTW", gtw)
	}

	// значения температуры — без градуса, он в заголовке C°
	if got := valueText(reading{v: 59}); got != " 59" {
		t.Errorf("valueText(59) = %q, want \" 59\" без градуса", got)
	}

	// RUN красится по числу ядер: зелёный ≤ ядер, жёлтый ≤ 2×ядер
	cores := runtime.NumCPU()
	var l reading
	for _, r := range layout(config{}) {
		if r.id == "L" {
			l = r
		}
	}
	if l.green != cores || l.yellow != 2*cores || l.red != 4*cores {
		t.Errorf("RUN thresholds = %d/%d/%d, want %d/%d/%d (по ядрам)",
			l.green, l.yellow, l.red, cores, 2*cores, 4*cores)
	}

	// явные --thresholds перекрывают умолчания, включая ядра
	// (карта ключуется id-буквой; имена NET/RUN парсер переводит в буквы)
	thr := map[string][3]int{"L": {3, 6, 12}, "N": {100, 200, 500}}
	rs := []reading{colRegistry["L"], colRegistry["N"]}
	applyThresholds(rs, 8, thr)
	if rs[0].green != 3 || rs[0].yellow != 6 || rs[0].red != 12 {
		t.Errorf("пороги L после --thresholds = %d/%d/%d, want 3/6/12", rs[0].green, rs[0].yellow, rs[0].red)
	}
	if rs[1].green != 100 || rs[1].yellow != 200 || rs[1].red != 500 {
		t.Errorf("пороги NET после --thresholds = %d/%d/%d, want 100/200/500", rs[1].green, rs[1].yellow, rs[1].red)
	}
}

func TestParseThresholds(t *testing.T) {
	thr, err := parseThresholds("NET=100/200/500,d=90/95/99", "en")
	if err != nil {
		t.Fatalf("parseThresholds: %v", err)
	}
	if thr["N"] != [3]int{100, 200, 500} { // имена переводятся в буквы
		t.Errorf("NET = %v (по ключу N)", thr["N"])
	}
	if _, ok := thr["U"]; !ok { // D — алиас диска
		t.Error("алиас D не сработал")
	}
	if _, err := parseThresholds("NET=100,200", "en"); err == nil {
		t.Error("запятая внутри группы: want ошибка (числа через /)")
	}
	if _, err := parseThresholds("NET=300/200/100", "en"); err == nil {
		t.Error("убывающие числа: want ошибка")
	}
	if _, err := parseThresholds("BAT=60/80/90", "en"); err == nil {
		t.Error("батарея перевёрнута: растущие числа — ошибка")
	}
	if _, err := parseThresholds("XXXX=1/2/3", "en"); err == nil { // 4 символа — не имя пинга
		t.Error("неизвестная колонка: want ошибка")
	}
	cfg, err := parseConfig([]string{"--thresholds", "RUN=2/4/8"})
	if err != nil {
		t.Fatalf("parseConfig --thresholds: %v", err)
	}
	if cfg.thresholds["L"] != [3]int{2, 4, 8} {
		t.Errorf("cfg.thresholds = %v (по ключу L)", cfg.thresholds)
	}
}

func TestParsePings(t *testing.T) {
	cfg, err := parseConfig([]string{"--ping", "vpn=10.8.0.1", "--ping", "NAS=192.168.1.50"})
	if err != nil {
		t.Fatalf("--ping: %v", err)
	}
	if len(cfg.pings) != 2 || cfg.pings[0].name != "VPN" || cfg.pings[0].host != "10.8.0.1" {
		t.Errorf("pings = %+v", cfg.pings)
	}
	// колонка появляется в layout справа с заголовком из 3 символов
	rs := layout(cfg)
	if rs[len(rs)-2].id != "VPN" || rs[len(rs)-2].head != "VPN" || rs[len(rs)-1].id != "NAS" {
		t.Errorf("layout с пингами: последние колонки %+v", rs[len(rs)-2:])
	}
	if rs[len(rs)-2].green != 50 || rs[len(rs)-2].yellow != 150 { // пороги как у NET
		t.Errorf("пороги VPN = %d/%d, want 50/150", rs[len(rs)-2].green, rs[len(rs)-2].yellow)
	}

	t.Setenv("LMON_PING", "DNS=1.1.1.1") // через переменную окружения
	cfg, err = parseConfig(nil)
	if err != nil || len(cfg.pings) != 1 || cfg.pings[0].name != "DNS" {
		t.Errorf("LMON_PING: cfg.pings = %+v, err = %v", cfg.pings, err)
	}

	for _, bad := range [][]string{
		{"--ping", "V=1.1.1.1"},                            // однобуквенное занято встроенными
		{"--ping", "НОС=1.1.1.1"},                          // не латиница/цифры
		{"--ping", "VPN=1.1.1.1", "--ping", "vpn=2.2.2.2"}, // дубль
		{"--ping", "нет-равно"},                            // без =
	} {
		if _, err := parseConfig(bad); err == nil {
			t.Errorf("%v: want ошибка", bad)
		}
	}
	// пороги именованного пинга принимаются --thresholds
	thr, err := parseThresholds("VPN=100/200/500", "en")
	if err != nil || thr["VPN"] != [3]int{100, 200, 500} {
		t.Errorf("thresholds VPN: %v, err = %v", thr, err)
	}
}

func TestPingBudget(t *testing.T) {
	cases := []struct {
		interval time.Duration
		want     time.Duration
	}{
		{500 * time.Millisecond, 1500 * time.Millisecond}, // минимум 1.5с
		{time.Second, 2 * time.Second},                    // два интервала
		{2 * time.Second, 4 * time.Second},                // максимум 4с
		{10 * time.Second, 4 * time.Second},               // дальше не растёт
	}
	for _, c := range cases {
		if got := pingBudget(c.interval); got != c.want {
			t.Errorf("pingBudget(%v) = %v, want %v", c.interval, got, c.want)
		}
	}
}

func TestNextTickSpacing(t *testing.T) {
	// пока «измерение» спало, тикер натикал с запасом; nextTick обязан
	// перезапустить интервал от текущего момента, а не вернуть накопленный
	// тик мгновенно — иначе строки идут пачками
	tk := time.NewTicker(100 * time.Millisecond)
	defer tk.Stop()
	time.Sleep(250 * time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	start := time.Now()
	if err := nextTick(ctx, tk, 100*time.Millisecond); err != nil {
		t.Fatalf("nextTick: %v", err)
	}
	if el := time.Since(start); el < 50*time.Millisecond {
		t.Errorf("nextTick вернулся через %v — отдал накопленный тик, будет пачка строк", el)
	}
}

func TestLayoutAdditiveCols(t *testing.T) {
	// --cols задаёт базовый набор, флаги добавляют справа без дублей
	cfg := config{cols: []string{"T", "N", "G"}, cpu: true, swap: true}
	got := ""
	for _, r := range layout(cfg) {
		got += r.id
	}
	if got != "TNGCS" {
		t.Errorf("layout = %q, want TNGCS", got)
	}

	cfg2 := config{cols: []string{"N", "G", "C"}, cpu: true}
	got2 := ""
	for _, r := range layout(cfg2) {
		got2 += r.id
	}
	if got2 != "NGC" {
		t.Errorf("layout(дубль) = %q, want NGC", got2)
	}

	cfg3 := config{}
	got3 := ""
	for _, r := range layout(cfg3) {
		got3 += r.id
	}
	if got3 != "NGLMT" {
		t.Errorf("layout(дефолт) = %q, want NGLMT", got3)
	}
}

func TestDigitCellWidth(t *testing.T) {
	u := &ui{color: false}
	cases := map[int]string{
		9:    "  9",
		75:   " 75",
		100:  "100",
		1500: "+++",
	}
	for v, want := range cases {
		if got := u.digitCell(reading{id: "T", head: " C°", v: v}); got != want {
			t.Errorf("digitCell(T,%d) = %q, want %q", v, got, want)
		}
	}
	if got := u.digitCell(reading{id: "N", v: 1500}); got != "+++" {
		t.Errorf("digitCell(N,1500) = %q, want +++", got)
	}
	if got := u.digitCell(reading{id: "N", v: -1}); got != "  ·" {
		t.Errorf("digitCell(N,-1) = %q, want \"  ·\"", got)
	}
}

func TestHeaderOf(t *testing.T) {
	rs := []reading{{head: "NET"}, {head: "MEM"}, {head: " C°"}}
	head, sep := headerOf(rs)
	if head != "NET MEM  C°" {
		t.Errorf("headerOf head = %q", head)
	}
	if sep != "--- --- ---" {
		t.Errorf("headerOf sep = %q", sep)
	}
}

func TestGatewayLifecycle(t *testing.T) {
	detect := func(gw string, fail bool) func(string) (string, error) {
		return func(string) (string, error) {
			if fail {
				return "", errors.New("нет сети")
			}
			return gw, nil
		}
	}
	manual := config{gw: "10.0.0.138"}
	refreshGateway(&manual, detect("192.168.1.1", false))
	if manual.gw != "10.0.0.138" {
		t.Errorf("ручной --gw изменился: %q", manual.gw)
	}

	auto := config{gwAuto: true} // шлюза нет при старте
	refreshGateway(&auto, detect("", true))
	if auto.gw != "" {
		t.Errorf("без сети gw = %q, want пусто", auto.gw)
	}
	refreshGateway(&auto, detect("192.168.1.1", false)) // сеть появилась
	if auto.gw != "192.168.1.1" {
		t.Errorf("сеть появилась, gw = %q, want 192.168.1.1", auto.gw)
	}
	refreshGateway(&auto, detect("10.1.1.1", false)) // шлюз сменился
	if auto.gw != "10.1.1.1" {
		t.Errorf("смена шлюза: gw = %q, want 10.1.1.1", auto.gw)
	}
	refreshGateway(&auto, detect("", true)) // шлюз исчез
	if auto.gw != "" {
		t.Errorf("шлюз исчез, gw = %q, want пусто", auto.gw)
	}
}

func TestOnceNoANSI(t *testing.T) {
	// --cols M — только память: без пингов, мгновенно
	var b bytes.Buffer
	u := &ui{w: &b}
	row := u.dataRow(measure(context.Background(), config{cols: []string{"M"}}))
	if strings.ContainsRune(row, '\x1b') {
		t.Errorf("вывод содержит ESC: %q", row)
	}
	if strings.TrimSpace(row) == "" || strings.Contains(row, "·") && row != "  ·" {
		t.Logf("row=%q (память может быть недоступна)", row)
	}
}

func TestPingValue(t *testing.T) {
	if got := pingValue(netping.Result{ValueMS: 999, State: netping.StateValid}); got != 999 {
		t.Errorf("измеренные 999 мс обязаны остаться 999, got %d", got)
	}
	if got := pingValue(netping.Result{State: netping.StateUnreachable}); got != 999 {
		t.Errorf("нет ответа = 999, got %d", got)
	}
	if got := pingValue(netping.Result{State: netping.StateUnavailable}); got != -1 {
		t.Errorf("нет ping = -1, got %d", got)
	}
	if got := pingValue(netping.Result{ValueMS: 42, State: netping.StateValid}); got != 42 {
		t.Errorf("valid = значение, got %d", got)
	}
}

func TestFamilyMismatch(t *testing.T) {
	_, err := parseConfig([]string{"--lang", "ru", "--target", "2606:4700::1111", "--gw", "192.168.1.1"})
	if err == nil || !strings.Contains(err.Error(), "IPv4 или оба IPv6") {
		t.Errorf("смешение семейств: err = %v, want ошибка семейства", err)
	}
	if _, err := parseConfig([]string{"--target", "2606:4700::1111"}); err != nil {
		t.Errorf("v6-таргет без gw: err = %v, want nil", err)
	}
}

func TestColsLetters(t *testing.T) {
	// D — понятный алиас диска (историческая буква U); регистр не важен.
	cfg, err := parseConfig([]string{"--cols", "n,d"})
	if err != nil {
		t.Fatalf("--cols n,d: err = %v", err)
	}
	if got := strings.Join(cfg.cols, ""); got != "NU" {
		t.Errorf("--cols n,d = %q, want NU", got)
	}
	if _, err := parseConfig([]string{"--cols", "N1"}); err == nil || !strings.Contains(err.Error(), "unknown column") {
		t.Errorf("--cols NX: err = %v, want ошибка с перечнем букв", err)
	}
	// в шпаргалке help перечислены все буквы реестра — в обоих языках
	for _, id := range []string{"N", "G", "L", "M", "T", "C", "I", "S", "U", "B", "P", "H"} {
		if !strings.Contains(helpTextRu, id+" —") || !strings.Contains(helpTextEn, id+" —") {
			t.Errorf("help: буква %s не упомянута в шпаргалке", id)
		}
	}
}

func TestOnceUIColorsOnlyOnTTY(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	cfg := config{hybrid: true}
	rs := []reading{{id: "N", head: "NET", v: 20, green: 50, yellow: 150, red: 300}}

	var piped bytes.Buffer
	if row := newOnceUI(&piped, cfg, false).dataRow(rs); strings.ContainsRune(row, '\x1b') {
		t.Errorf("--once в пайп содержит ANSI: %q", row)
	}

	var tty bytes.Buffer
	if row := newOnceUI(&tty, cfg, true).dataRow(rs); !strings.ContainsRune(row, '\x1b') {
		t.Errorf("--once на терминале без цвета: %q", row)
	}

	// NO_COLOR отключает цвет и на терминале
	t.Setenv("NO_COLOR", "1")
	if row := newOnceUI(&tty, cfg, true).dataRow(rs); strings.ContainsRune(row, '\x1b') {
		t.Errorf("NO_COLOR не отключил цвет в --once: %q", row)
	}
}

func TestHeaderAndHybridUseExplicitBlack(t *testing.T) {
	// Windows Terminal «жирный чёрный» подсвечивает в серый — на цветной
	// плашке подпись становится нечитаемой. Должен быть явный чёрный
	// 256-цветной палитры (16) и без bold.
	r := reading{id: "N", head: "NET", v: 20, green: 50, yellow: 150, red: 300}
	u := &ui{color: true}
	for name, s := range map[string]string{
		"header": u.headerRow([]reading{r}),
		"hybrid": u.hybridCell(r),
	} {
		if !strings.Contains(s, "38;5;16;48;5;") {
			t.Errorf("%s: нет явного чёрного: %q", name, s)
		}
		if strings.Contains(s, "1;30") {
			t.Errorf("%s: остался жирный чёрный: %q", name, s)
		}
	}
}

func TestLayoutWindowsSensorColumns(t *testing.T) {
	// SWP (pagefile), GPU и G° на Windows теперь поддержаны: источники —
	// PDH/NVML/nvidia-smi, без прав администратора.
	cols, _ := layoutWithDropped(config{swap: true, gpu: true, gputemp: true}, "windows")
	got := ids(cols)
	for _, want := range []string{"S", "P", "H"} {
		if !strings.Contains(got, want) {
			t.Errorf("windows layout = %q, нет колонки %s", got, want)
		}
	}
}

func TestTempCmdKeepsTemperatureColumn(t *testing.T) {
	cfg := config{cols: []string{"T"}, tempCmd: "echo 50"}
	cols, dropped := layoutWithDropped(cfg, "windows")
	if got := ids(cols); got != "T" {
		t.Fatalf("layout = %q, want T (--temp-cmd включает C° на Windows)", got)
	}
	if len(dropped) != 0 {
		t.Fatalf("dropped = %v, want пусто", dropped)
	}
}

func TestParseTempCmdFlag(t *testing.T) {
	t.Setenv("LMON_TEMP_CMD", "")
	cfg, err := parseConfig([]string{"--temp-cmd", "echo 50"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.tempCmd != "echo 50" {
		t.Errorf("tempCmd = %q", cfg.tempCmd)
	}
}

func TestDiskBusyColumn(t *testing.T) {
	for _, goos := range []string{"windows", "darwin"} {
		if !columnSupportedByOS("K", goos) {
			t.Errorf("K должна поддерживаться на %s", goos)
		}
	}
	if columnSupportedByOS("K", "linux") {
		t.Error("K пока не поддерживается на Linux")
	}
	for _, goos := range []string{"windows", "darwin"} {
		cols, _ := layoutWithDropped(config{diskBusy: true}, goos)
		if got := ids(cols); !strings.Contains(got, "K") {
			t.Errorf("%s layout = %q, нет K", goos, got)
		}
	}
	cols, dropped := layoutWithDropped(config{cols: []string{"K"}}, "linux")
	if got := ids(cols); got != "M" {
		t.Errorf("linux layout = %q, want fallback M", got)
	}
	if got := droppedHeads(dropped); got != "I/O" {
		t.Errorf("dropped = %q, want I/O", got)
	}
}

func TestParseDiskBusyFlag(t *testing.T) {
	t.Setenv("LMON_DISK_BUSY", "")
	cfg, err := parseConfig([]string{"--disk-busy"})
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.diskBusy {
		t.Error("--disk-busy не установил флаг")
	}
	t.Setenv("LMON_DISK_BUSY", "1")
	cfg, err = parseConfig(nil)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.diskBusy {
		t.Error("LMON_DISK_BUSY=1 не установил флаг")
	}
}

func TestColsWithPingNames(t *testing.T) {
	for _, k := range []string{"LMON_COLS", "LMON_PING", "LMON_THRESHOLDS"} {
		t.Setenv(k, "")
	}
	cfg, err := parseConfig([]string{"--ping", "VPN=192.0.2.1", "--cols", "NG,VPN,M"})
	if err != nil {
		t.Fatalf("--cols с пингом: %v", err)
	}
	if got := strings.Join(cfg.cols, ""); got != "NGVPNM" {
		t.Fatalf("cols = %q, want NGVPNM", got)
	}
	// пинг уже стоит в --cols — справа повторно не добавляется
	if got := ids(layout(cfg)); got != "NGVPNM" {
		t.Errorf("layout = %q, want NGVPNM", got)
	}

	// без --cols пинг по-прежнему справа
	cfg2, err := parseConfig([]string{"--ping", "VPN=192.0.2.1"})
	if err != nil {
		t.Fatal(err)
	}
	if got := ids(layout(cfg2)); got != "NGLMTVPN" {
		t.Errorf("layout без cols = %q, want NGLMTVPN", got)
	}
}

func TestColsPingErrors(t *testing.T) {
	for _, k := range []string{"LMON_COLS", "LMON_PING", "LMON_THRESHOLDS"} {
		t.Setenv(k, "")
	}
	if _, err := parseConfig([]string{"--cols", "NG,VPN"}); err == nil || !strings.Contains(err.Error(), "ping") {
		t.Errorf("незаданный пинг: err = %v, want подсказку про --ping", err)
	}
	if _, err := parseConfig([]string{"--ping", "VPN=192.0.2.1", "--cols", "VPN,VPN"}); err == nil || !strings.Contains(err.Error(), "twice") {
		t.Errorf("дубль пинга: err = %v, want twice", err)
	}
	// компактная запись без разделителей остаётся рабочим набором букв
	cfg, err := parseConfig([]string{"--cols", "NGM"})
	if err != nil {
		t.Fatalf("--cols NGM: %v", err)
	}
	if got := strings.Join(cfg.cols, ""); got != "NGM" {
		t.Errorf("--cols NGM = %q", got)
	}
}

func TestUnreachableSymbols(t *testing.T) {
	r := reading{id: "N", head: "NET", v: 999, green: 50, yellow: 150, red: 300}
	plain := &ui{}
	if got := plain.dataRow([]reading{r}); got != "999" {
		t.Errorf("без --symbols: %q, want 999", got)
	}
	sym := &ui{symbols: true}
	if got := sym.dataRow([]reading{r}); got != "..." {
		t.Errorf("с символами: %q, want ...", got)
	}
	hyb := &ui{symbols: true, color: true, hybrid: true}
	got := hyb.dataRow([]reading{r})
	if !strings.Contains(got, "...") || strings.Contains(got, "999") {
		t.Errorf("hybrid: %q, want ... на цветной подложке", got)
	}
	// в блочном режиме «ответа нет» — красный блок, без текстовых символов
	blk := &ui{symbols: true, color: true, blocks: true}
	if got := blk.dataRow([]reading{r}); !strings.Contains(got, "███") || !strings.Contains(got, "38;5;196") {
		t.Errorf("blocks: %q, want red block", got)
	}
}

func TestSymbolsFlagAndEnv(t *testing.T) {
	t.Setenv("LMON_SYMBOLS", "")
	cfg, err := parseConfig([]string{"--symbols"})
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.symbols {
		t.Error("--symbols не установил флаг")
	}
	cfg, err = parseConfig([]string{"--once", "--symbols"})
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.symbols || !cfg.once {
		t.Errorf("--once --symbols: symbols=%v once=%v", cfg.symbols, cfg.once)
	}
	t.Setenv("LMON_SYMBOLS", "1")
	cfg, err = parseConfig(nil)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.symbols {
		t.Error("LMON_SYMBOLS=1 не установил флаг")
	}
}

func TestParseGPUFlags(t *testing.T) {
	for _, k := range []string{"LMON_GPU_INDEX", "LMON_GPU_LIST"} {
		t.Setenv(k, "")
	}
	cfg, err := parseConfig([]string{"--gpu", "--gputemp", "--gpu-index", "1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.gpuIdx) != 1 || cfg.gpuIdx[0] != 1 || !cfg.gpu || !cfg.gputemp {
		t.Errorf("gpu flags: idx=%v gpu=%v gputemp=%v", cfg.gpuIdx, cfg.gpu, cfg.gputemp)
	}
	if _, err := parseConfig([]string{"--gpu-index", "-1"}); err == nil {
		t.Error("--gpu-index -1: ожидалась ошибка")
	}
	cfg, err = parseConfig([]string{"--gpu", "--gputemp", "--gpu-index", "0,1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.gpuIdx) != 2 || cfg.gpuIdx[0] != 0 || cfg.gpuIdx[1] != 1 {
		t.Errorf("multi gpu idx = %v, want [0 1]", cfg.gpuIdx)
	}
	if _, err := parseConfig([]string{"--gpu-index", "0,1"}); err == nil {
		t.Error("несколько GPU без --gpu/--gputemp: ожидалась ошибка")
	}
	cfg, err = parseConfig([]string{"--gpu-list"})
	if err != nil || !cfg.gpuList {
		t.Errorf("--gpu-list: cfg=%+v err=%v", cfg.gpuList, err)
	}
	t.Setenv("LMON_GPU_INDEX", "2")
	t.Setenv("LMON_GPU_LIST", "1")
	cfg, err = parseConfig(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.gpuIdx) != 1 || cfg.gpuIdx[0] != 2 || !cfg.gpuList {
		t.Errorf("env GPU: idx=%v list=%v", cfg.gpuIdx, cfg.gpuList)
	}
}

func TestLayoutMultiGPU(t *testing.T) {
	cfg := config{gpu: true, gputemp: true, gpuIdx: []int{0, 1}}
	cols, _ := layoutWithDropped(cfg, "windows")
	got := ids(cols)
	for _, want := range []string{"G0", "G1", "T0", "T1"} {
		if !strings.Contains(got, want) {
			t.Errorf("multi-GPU layout = %q, нет %s", got, want)
		}
	}
	// одна карта — прежние P/H
	one, _ := layoutWithDropped(config{gpu: true, gputemp: true, gpuIdx: []int{1}}, "windows")
	if got := ids(one); !strings.Contains(got, "P") || !strings.Contains(got, "H") {
		t.Errorf("single-GPU layout = %q, want P/H", got)
	}
}

func TestAllFlag(t *testing.T) {
	for _, k := range []string{"LMON_ALL", "LMON_COLS", "LMON_GPU_INDEX"} {
		t.Setenv(k, "")
	}
	cfg, err := parseConfig([]string{"--all"})
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.all || !cfg.cpu || !cfg.iowait || !cfg.swap || !cfg.disk ||
		!cfg.diskBusy || !cfg.batt || !cfg.gpu || !cfg.gputemp {
		t.Errorf("--all не включил все колонки: %+v", cfg)
	}
	cols, _ := layoutWithDropped(cfg, "windows")
	got := ids(cols)
	for _, want := range []string{"N", "G", "M", "C", "S", "U", "B", "P", "H", "K"} {
		if !strings.Contains(got, want) {
			t.Errorf("--all layout = %q, нет %s", got, want)
		}
	}
	if strings.Contains(got, "L") || strings.Contains(got, "T") || strings.Contains(got, "I") {
		t.Errorf("--all layout = %q, на Windows L/T/I должны быть скрыты", got)
	}
	if _, err := parseConfig([]string{"--all", "--cols", "NG"}); err == nil {
		t.Error("--all + --cols: ожидалась ошибка")
	}
	cfg, err = parseConfig([]string{"--all", "--gpu-index", "0,1"})
	if err != nil {
		t.Fatal(err)
	}
	multi, _ := layoutWithDropped(cfg, "windows")
	mgot := ids(multi)
	for _, want := range []string{"G0", "G1", "T0", "T1"} {
		if !strings.Contains(mgot, want) {
			t.Errorf("--all multi layout = %q, нет %s", mgot, want)
		}
	}
	if strings.Contains(mgot, "P") || strings.Contains(mgot, "H") {
		t.Errorf("--all multi layout = %q, не должно быть P/H", mgot)
	}
}

func TestFansFlag(t *testing.T) {
	for _, k := range []string{"LMON_FANS", "LMON_ALL"} {
		t.Setenv(k, "")
	}
	cfg, err := parseConfig([]string{"--fans"})
	if err != nil || !cfg.fans {
		t.Fatalf("--fans: cfg=%+v err=%v", cfg.fans, err)
	}
	// FAN есть только на Linux; на Windows/macOS колонка скрыта
	if cols, _ := layoutWithDropped(config{fans: true}, "linux"); !strings.Contains(ids(cols), "F") {
		t.Errorf("linux layout без F: %q", ids(cols))
	}
	if cols, _ := layoutWithDropped(config{fans: true}, "windows"); strings.Contains(ids(cols), "F") {
		t.Errorf("windows layout не должен содержать F: %q", ids(cols))
	}
	if cols, _ := layoutWithDropped(config{fans: true}, "darwin"); strings.Contains(ids(cols), "F") {
		t.Errorf("darwin layout не должен содержать F: %q", ids(cols))
	}
}

func TestExtrasFlags(t *testing.T) {
	for _, k := range []string{"LMON_GPU_POWER", "LMON_NVME", "LMON_DISK_IO", "LMON_NET_RATE", "LMON_PSI", "LMON_SYSTEM"} {
		t.Setenv(k, "")
	}
	cfg, err := parseConfig([]string{"--gpu-power", "--nvme", "--disk-io", "--net-rate", "--psi", "--system"})
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.gpuPower || !cfg.nvme || !cfg.diskIO || !cfg.netRate || !cfg.psi || !cfg.system {
		t.Errorf("extras flags: %+v", cfg)
	}
	cols, _ := layoutWithDropped(cfg, "linux")
	got := ids(cols)
	for _, want := range []string{"W", "Y", "A", "Q", "R", "J", "E", "Z", "O", "X"} {
		if !strings.Contains(got, want) {
			t.Errorf("linux extras layout = %q, нет %s", got, want)
		}
	}
	w, _ := layoutWithDropped(cfg, "windows")
	if wgot := ids(w); strings.ContainsAny(wgot, "WVAQRJEZOX") {
		t.Errorf("windows layout не должен содержать extras: %q", wgot)
	}
	// колонки можно выбрать и через --cols
	cfg2, err := parseConfig([]string{"--cols", "NAQ"})
	if err != nil || strings.Join(cfg2.cols, "") != "NAQ" {
		t.Errorf("--cols NAQ: %v %v", cfg2.cols, err)
	}
}

func TestFansListAllFlags(t *testing.T) {
	for _, k := range []string{"LMON_FANS", "LMON_FANS_ALL", "LMON_FANS_LIST", "LMON_ALL"} {
		t.Setenv(k, "")
	}
	cfg, err := parseConfig([]string{"--fans-all", "--fans-list"})
	if err != nil || !cfg.fansAll || !cfg.fansList {
		t.Errorf("fans flags: %+v %v", cfg, err)
	}
}

func TestRootMetricsFlags(t *testing.T) {
	for _, k := range []string{"LMON_CPU_POWER", "LMON_NVME_SMART"} {
		t.Setenv(k, "")
	}
	cfg, err := parseConfig([]string{"--cpu-power", "--nvme-smart"})
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.cpuPower || !cfg.nvmeSmart {
		t.Errorf("root flags: %+v", cfg)
	}
	cols, _ := layoutWithDropped(cfg, "linux")
	got := ids(cols)
	if !strings.Contains(got, "PW") || !strings.Contains(got, "WR") {
		t.Errorf("linux root layout = %q, нет PW/WR", got)
	}
	w, _ := layoutWithDropped(cfg, "windows")
	if wgot := ids(w); strings.Contains(wgot, "PW") || strings.Contains(wgot, "WR") {
		t.Errorf("windows layout не должен содержать root-метрики: %q", wgot)
	}
	cfg2, err := parseConfig([]string{"--cols", "PW,WR"})
	if err != nil || strings.Join(cfg2.cols, ",") != "PW,WR" {
		t.Errorf("--cols PW,WR: %v %v", cfg2.cols, err)
	}
}

func TestCompactMode(t *testing.T) {
	for _, k := range []string{"LMON_COMPACT"} {
		t.Setenv(k, "")
	}
	cfg, err := parseConfig([]string{"--compact"})
	if err != nil || !cfg.compact {
		t.Fatalf("--compact: cfg=%+v err=%v", cfg.compact, err)
	}
	if _, err := parseConfig([]string{"--compact", "--bars"}); err == nil {
		t.Error("--compact + --bars: ожидалась ошибка")
	}
	rs := []reading{
		{id: "N", head: "NET", v: 10},
		{id: "M", head: "MEM", v: 50},
		{id: "VPN", head: "VPN", v: 1},
	}
	head, sep := headerOfCompact(rs)
	if head != "NMV" || sep != "" {
		t.Errorf("headerOfCompact = %q/%q, want NMV/empty", head, sep)
	}
	u := &ui{compact: true, symbols: true}
	row := u.dataRow(rs)
	if got := len([]rune(row)); got != len(rs) {
		t.Errorf("compact row = %q (%d runes), want %d", row, got, len(rs))
	}
	if strings.ContainsRune(row, ' ') {
		t.Errorf("compact row contains space: %q", row)
	}
	if got := u.headerRow(rs); got != "NMV" {
		t.Errorf("compact headerRow = %q, want NMV", got)
	}
}

func TestBlockSpecialColors(t *testing.T) {
	u := &ui{color: true, blocks: true, symbols: true}
	if s := u.blockCell(reading{v: -1}); !strings.Contains(s, "38;5;135") || !strings.Contains(s, "███") || strings.Contains(s, "·") {
		t.Errorf("block no-sensor = %q", s)
	}
	if s := u.blockCell(reading{v: 999}); !strings.Contains(s, "38;5;196") || !strings.Contains(s, "███") || strings.Contains(s, "...") {
		t.Errorf("block no-reply = %q", s)
	}
	c := &ui{color: true, compact: true, symbols: true}
	if s := c.compactCell(reading{v: -1}); !strings.Contains(s, "38;5;135") || !strings.Contains(s, "█") || strings.Contains(s, "·") {
		t.Errorf("compact no-sensor = %q", s)
	}
	if s := c.compactCell(reading{v: 999}); !strings.Contains(s, "38;5;196") || !strings.Contains(s, "█") || strings.Contains(s, "!") {
		t.Errorf("compact no-reply = %q", s)
	}
	if h := u.headerRow([]reading{{id: "N", head: "NET", v: -1}}); !strings.Contains(h, "48;5;135") {
		t.Errorf("header no-sensor = %q", h)
	}
	if h := u.headerRow([]reading{{id: "N", head: "NET", v: 999}}); !strings.Contains(h, "48;5;196") {
		t.Errorf("header no-reply = %q", h)
	}
}

func TestColsBlankColumn(t *testing.T) {
	for _, k := range []string{"LMON_COLS", "LMON_PING"} {
		t.Setenv(k, "")
	}
	cfg, err := parseConfig([]string{"--cols", "NG,,M"})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(cfg.cols, ","); got != "N,G,_,M" {
		t.Fatalf("--cols NG,,M = %q", got)
	}
	rs := layout(cfg)
	if len(rs) != 4 || !rs[2].blank {
		t.Fatalf("layout NG,,M = %+v", ids(rs))
	}
	if rs[2].head != "   " {
		t.Errorf("blank head = %q", rs[2].head)
	}
	if got := len([]rune(rs[2].head)); got != 3 {
		t.Errorf("blank width = %d", got)
	}
	// два подряд пустых
	cfg2, err := parseConfig([]string{"--cols", "NG,,,M"})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(cfg2.cols, ","); got != "N,G,_,_,M" {
		t.Errorf("--cols NG,,,M = %q", got)
	}
	// по краям пустые не добавляются
	cfg3, err := parseConfig([]string{"--cols", ",NG,"})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(cfg3.cols, ","); got != "N,G" {
		t.Errorf("--cols ,NG, = %q", got)
	}
	// в compact пустой столбик — один пробел
	u := &ui{compact: true, symbols: true}
	row := u.dataRow([]reading{{id: "N", v: 10}, {id: "_", blank: true}, {id: "M", v: 20}})
	if got := len([]rune(row)); got != 3 {
		t.Errorf("compact blank row = %q (%d runes)", row, got)
	}
}

func TestEmitDelaysDataByOneSample(t *testing.T) {
	var b bytes.Buffer
	u := &ui{w: &b, tty: true, rows: 4, lang: "en"}
	u.emit([]reading{{id: "N", head: "NET", v: 1}})
	if strings.Contains(b.String(), "  1") {
		t.Errorf("первый замер напечатан сразу: %q", b.String())
	}
	b.Reset()
	u.emit([]reading{{id: "N", head: "NET", v: 2}})
	if !strings.Contains(b.String(), "  1") {
		t.Errorf("нет предыдущего замера: %q", b.String())
	}
	if strings.Contains(b.String(), "  2") {
		t.Errorf("текущий замер напечатан слишком рано: %q", b.String())
	}
	b.Reset()
	u.emit([]reading{{id: "N", head: "NET", v: 3}})
	if !strings.Contains(b.String(), "  2") {
		t.Errorf("нет второго замера: %q", b.String())
	}
}

func TestHorizontalView(t *testing.T) {
	var b bytes.Buffer
	v := &hview{w: &b, rows: 3, cols: 20, history: map[string][]hcell{}, lang: "en"}
	rs := []reading{
		{id: "N", head: "NET", v: 10},
		{id: "M", head: "MEM", v: 95, green: 60, yellow: 85, red: 97},
		{id: "T", head: " C°", v: -1},
		{id: "C", head: "CPU", v: 20},
	}
	v.add(rs)
	v.draw(rs)
	out := b.String()
	for _, want := range []string{"N ", "M ", "T "} {
		if !strings.Contains(out, want) {
			t.Errorf("horizontal output = %q, no %q", out, want)
		}
	}
	if strings.Contains(out, "CPU") {
		t.Errorf("показана строка, не поместившаяся по высоте: %q", out)
	}
	if !strings.Contains(out, "█") {
		t.Errorf("нет цветных блоков: %q", out)
	}
	if hLabel(reading{id: "N"}, true) != "Network" {
		t.Errorf("full label N = %q", hLabel(reading{id: "N"}, true))
	}
	if hLabel(reading{id: "M"}, false) != "M" {
		t.Errorf("short label M = %q", hLabel(reading{id: "M"}, false))
	}
}

func TestHorizontalFlags(t *testing.T) {
	for _, k := range []string{"LMON_HORIZONTAL", "LMON_LABELS"} {
		t.Setenv(k, "")
	}
	cfg, err := parseConfig([]string{"--horizontal", "--labels", "full"})
	if err != nil || !cfg.horizontal || !cfg.labelsFull {
		t.Fatalf("horizontal flags: %+v %v", cfg, err)
	}
	if _, err := parseConfig([]string{"--horizontal", "--bars"}); err == nil {
		t.Error("--horizontal + --bars: ожидалась ошибка")
	}
	if _, err := parseConfig([]string{"--horizontal", "--once"}); err == nil {
		t.Error("--horizontal + --once: ожидалась ошибка")
	}
	if _, err := parseConfig([]string{"--labels", "wide"}); err == nil {
		t.Error("--labels wide: ожидалась ошибка")
	}
}

func TestRainDirection(t *testing.T) {
	var b bytes.Buffer
	u := &ui{w: &b, tty: true, rain: true, rows: 5, color: false}
	r1 := []reading{{id: "N", head: "NET", v: 1}}
	r2 := []reading{{id: "N", head: "NET", v: 2}}
	u.emit(r1)
	if strings.Contains(b.String(), "  1") {
		t.Errorf("первый замер сразу стал данными: %q", b.String())
	}
	b.Reset()
	u.emit(r2)
	if !strings.Contains(b.String(), "  1") {
		t.Errorf("нет предыдущего замера: %q", b.String())
	}
	if strings.Contains(b.String(), "  2") {
		t.Errorf("текущий замер слишком рано в данных: %q", b.String())
	}
	b.Reset()
	r3 := []reading{{id: "N", head: "NET", v: 3}}
	u.emit(r3)
	if strings.Index(b.String(), "  2") < 0 || strings.Index(b.String(), "  1") < 0 {
		t.Errorf("rain rows = %q", b.String())
	}
	if strings.Index(b.String(), "  2") > strings.Index(b.String(), "  1") {
		t.Errorf("новые данные должны быть выше старых: %q", b.String())
	}

	cfg, err := parseConfig([]string{"--rain", "--bars"})
	if err != nil || !cfg.rain || !cfg.blocks {
		t.Errorf("--rain --bars: %+v %v", cfg, err)
	}
	if _, err := parseConfig([]string{"--rain", "--horizontal"}); err == nil {
		t.Error("--rain + --horizontal: ожидалась ошибка")
	}
	if _, err := parseConfig([]string{"--rain", "--once"}); err == nil {
		t.Error("--rain + --once: ожидалась ошибка")
	}
}

func TestMatrixPaletteWithModes(t *testing.T) {
	u := &ui{color: true, blocks: true, matrix: true}
	s := u.blockCell(reading{id: "M", v: 95, green: 60, yellow: 85, red: 97})
	if !strings.Contains(s, "38;5;118") || strings.Contains(s, "196") {
		t.Errorf("bars+matrix block = %q, want green palette", s)
	}
	h := u.headerRow([]reading{{id: "M", head: "MEM", v: 95, green: 60, yellow: 85, red: 97}})
	if !strings.Contains(h, "48;5;118") {
		t.Errorf("bars+matrix header = %q", h)
	}
	cfg, err := parseConfig([]string{"--matrix", "--once"})
	if err != nil || !cfg.matrixPalette || !cfg.once {
		t.Errorf("--matrix --once: %+v %v", cfg, err)
	}
}

func TestRainUsesModeSymbols(t *testing.T) {
	var b bytes.Buffer
	u := &ui{w: &b, tty: true, rain: true, blocks: true, color: true, rows: 4}
	u.emit([]reading{{id: "N", head: "NET", v: 1}})
	b.Reset()
	u.emit([]reading{{id: "N", head: "NET", v: 2}})
	if !strings.Contains(b.String(), "███") {
		t.Errorf("rain+bars data = %q, want blocks", b.String())
	}
}

func TestMatrixFillGlyphs(t *testing.T) {
	if runeGlyph(0, 0) != ' ' {
		t.Error("нулевое значение должно быть пустым")
	}
	a := runeGlyph(0.5, 0)
	b := runeGlyph(0.5, 1)
	if a == b {
		t.Errorf("одинаковое значение не меняет символ: %q", a)
	}
	if runeGlyph(1, 0) == ' ' {
		t.Error("максимум не должен быть пустым")
	}
	v := &gview{rows: 4, cols: 3, color: true, history: map[string][]gcell{}, counters: map[string]int{}}
	r := []reading{{id: "N", head: "NET", v: 200, green: 50, yellow: 150, red: 300}}
	v.add(r)
	v.add(r)
	h := v.history["N"]
	if len(h) < 2 || h[0].glyph == h[1].glyph {
		t.Errorf("повтор значения не перебирает символы: %+v", h)
	}
	if !strings.Contains(v.cell(h[0]), "38;5;46") {
		t.Errorf("символ не одним зелёным цветом: %q", v.cell(h[0]))
	}

	cfg, err := parseConfig([]string{"--rain", "--matrix"})
	if err != nil || !cfg.rain || !cfg.matrixPalette {
		t.Errorf("--rain --matrix: %+v %v", cfg, err)
	}
	if _, err := parseConfig([]string{"--rain", "--bars"}); err != nil {
		t.Errorf("--rain --bars: %v", err)
	}
}

func TestMatrixStyleReplacesDigits(t *testing.T) {
	u := &ui{matrix: true, color: true}
	rs := []reading{{id: "N", head: "NET", v: 120, green: 50, yellow: 150, red: 300}}
	out := u.dataRow(rs)
	plain := strings.NewReplacer("\x1b[38;5;46m", "", "\x1b[0m", "").Replace(out)
	if strings.ContainsAny(plain, "0123456789") {
		t.Errorf("matrix row contains digits: %q", out)
	}
	if !strings.Contains(out, "38;5;46") {
		t.Errorf("matrix row is not green: %q", out)
	}
	if got := len([]rune(strings.ReplaceAll(out, "\x1b[0m", ""))); got < 3 {
		t.Errorf("matrix row too short: %q", out)
	}
}
