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
	if _, err := parseConfig([]string{"-b", "--hybrid"}); err == nil ||
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
	if _, err := parseConfig([]string{"--cols", "NX"}); err == nil || !strings.Contains(err.Error(), "unknown column") {
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
