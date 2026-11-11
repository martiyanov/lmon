package main

import (
	"bytes"
	"strings"
	"testing"
)

func hintsText(hs []startupHint) string {
	var b strings.Builder
	for _, h := range hs {
		b.WriteString(h.title)
		b.WriteString(" · ")
		b.WriteString(h.detail)
		b.WriteString("\n")
	}
	return b.String()
}

func setValues(cols []reading, vals map[string]int) {
	for i := range cols {
		if v, ok := vals[cols[i].id]; ok {
			cols[i].v = v
		}
	}
}

func ids(rs []reading) string {
	var b strings.Builder
	for _, r := range rs {
		b.WriteString(r.id)
	}
	return b.String()
}

func droppedHeads(ds []droppedColumn) string {
	hs := make([]string, 0, len(ds))
	for _, d := range ds {
		hs = append(hs, strings.TrimSpace(d.head))
	}
	return strings.Join(hs, ",")
}

func TestLayoutDropsWindowsUnsupported(t *testing.T) {
	cols, dropped := layoutWithDropped(config{lang: "ru"}, "windows")
	if got := ids(cols); got != "NGM" {
		t.Fatalf("windows layout = %q, want NGM (без RUN и C°)", got)
	}
	if got := droppedHeads(dropped); got != "RUN,C°" {
		t.Fatalf("dropped = %q, want RUN,C°", got)
	}
}

func TestLayoutFallbackWhenEverythingDropped(t *testing.T) {
	cols, dropped := layoutWithDropped(config{cols: []string{"L", "T"}}, "windows")
	if got := ids(cols); got != "M" {
		t.Fatalf("layout = %q, want fallback M", got)
	}
	if got := droppedHeads(dropped); got != "RUN,C°" {
		t.Fatalf("dropped = %q", got)
	}
}

func TestLayoutDarwinDropsOnlyIowaitAndGpuTemp(t *testing.T) {
	cols, dropped := layoutWithDropped(config{iowait: true, gputemp: true}, "darwin")
	got := ids(cols)
	if strings.Contains(got, "I") || strings.Contains(got, "H") {
		t.Fatalf("darwin layout содержит I/H: %q", got)
	}
	if got != "NGLMT" {
		t.Fatalf("darwin layout = %q, want NGLMT", got)
	}
	if droppedHeads(dropped) != "IOW,G°" {
		t.Fatalf("dropped = %q, want IOW,G°", droppedHeads(dropped))
	}
}

func TestStartupHintsWindowsHiddenAndNetwork(t *testing.T) {
	cfg := config{lang: "ru"}
	cols, dropped := layoutWithDropped(cfg, "windows")
	setValues(cols, map[string]int{"N": 999, "G": 999, "M": 31})

	hs := startupHints(cfg, cols, "windows", dropped)
	txt := hintsText(hs)
	if len(hs) != 2 {
		t.Fatalf("hints = %d, want 2 (скрытые + сеть):\n%s", len(hs), txt)
	}
	for _, want := range []string{
		"Скрыты в этой ОС: RUN, C°",
		"NET и GTW = 999", "New-NetFirewallRule", "--cpu",
	} {
		if !strings.Contains(txt, want) {
			t.Errorf("в подсказках нет %q:\n%s", want, txt)
		}
	}
}

func TestStartupHintsAllGood(t *testing.T) {
	cfg := config{lang: "en"}
	cols, dropped := layoutWithDropped(cfg, "darwin")
	setValues(cols, map[string]int{"N": 24, "G": 2, "L": 1, "M": 48, "T": 55})
	if hs := startupHints(cfg, cols, "darwin", dropped); len(hs) != 0 {
		t.Fatalf("ожидалось 0 подсказок, получили %d:\n%s", len(hs), hintsText(hs))
	}
}

func TestStartupHintsOnlyEnabledColumns(t *testing.T) {
	cfg := config{lang: "en", cols: []string{"M"}}
	cols, dropped := layoutWithDropped(cfg, "windows")
	setValues(cols, map[string]int{"M": 48})
	if hs := startupHints(cfg, cols, "windows", dropped); len(hs) != 0 {
		t.Fatalf("подсказки про выключенные колонки: %s", hintsText(hs))
	}
}

func TestStartupHintsGatewayMissing(t *testing.T) {
	cfg := config{lang: "en", gwAuto: true}
	cols, dropped := layoutWithDropped(cfg, "linux")
	setValues(cols, map[string]int{"N": 24, "G": -1, "L": 1, "M": 48, "T": 55})
	hs := startupHints(cfg, cols, "linux", dropped)
	if len(hs) != 1 {
		t.Fatalf("hints = %d, want 1:\n%s", len(hs), hintsText(hs))
	}
	if !strings.Contains(hs[0].detail, "--gw") {
		t.Errorf("нет совета --gw: %+v", hs[0])
	}
}

func TestStartupHintsTempPerOS(t *testing.T) {
	cfg := config{lang: "en"}
	darwinCols, darwinDropped := layoutWithDropped(cfg, "darwin")
	setValues(darwinCols, map[string]int{"N": 24, "G": 2, "L": 1, "M": 48, "T": -1})
	if txt := hintsText(startupHints(cfg, darwinCols, "darwin", darwinDropped)); !strings.Contains(txt, "smctemp") {
		t.Errorf("darwin: нет smctemp в %q", txt)
	}

	winCols, winDropped := layoutWithDropped(cfg, "windows")
	setValues(winCols, map[string]int{"N": 24, "G": 2, "M": 48})
	winTxt := hintsText(startupHints(cfg, winCols, "windows", winDropped))
	if strings.Contains(winTxt, "smctemp") || !strings.Contains(winTxt, "C°") {
		t.Errorf("windows: подсказка про температуру неверная: %q", winTxt)
	}
}

func TestStartupHintsLanguage(t *testing.T) {
	cfg := config{lang: "ru", gwAuto: true}
	cols, dropped := layoutWithDropped(cfg, "linux")
	setValues(cols, map[string]int{"N": 24, "G": -1, "L": 1, "M": 48, "T": 55})
	if ru := hintsText(startupHints(cfg, cols, "linux", dropped)); !strings.Contains(ru, "шлюз") {
		t.Errorf("ru: %q", ru)
	}
	cfg.lang = "en"
	if en := hintsText(startupHints(cfg, cols, "linux", dropped)); !strings.Contains(en, "gateway") || strings.Contains(en, "шлюз") {
		t.Errorf("en: %q", en)
	}
}

func TestStartupHintsNamedPing(t *testing.T) {
	cfg := config{lang: "en", pings: []pingTarget{{name: "VPN", host: "10.0.0.1"}}}
	cols, dropped := layoutWithDropped(cfg, "linux")
	setValues(cols, map[string]int{"N": 24, "G": 2, "L": 1, "M": 48, "T": 55, "VPN": 999})
	hs := startupHints(cfg, cols, "linux", dropped)
	txt := hintsText(hs)
	if len(hs) != 1 || !strings.Contains(txt, "VPN = 999") || !strings.Contains(txt, "10.0.0.1") {
		t.Fatalf("named ping hint = %d:\n%s", len(hs), txt)
	}
}

func TestStartupHintsNoBatteryNoise(t *testing.T) {
	cfg := config{lang: "en", cols: []string{"B"}}
	cols, dropped := layoutWithDropped(cfg, "linux")
	setValues(cols, map[string]int{"B": -1})
	if hs := startupHints(cfg, cols, "linux", dropped); len(hs) != 0 {
		t.Fatalf("battery hint noise: %s", hintsText(hs))
	}
}

func TestPrintHintsFormat(t *testing.T) {
	var b bytes.Buffer
	printHints(&b, []startupHint{{title: "RUN: нет данных", detail: "используйте --cpu"}})
	want := "lmon: RUN: нет данных\n      используйте --cpu\n"
	if b.String() != want {
		t.Errorf("printHints = %q, want %q", b.String(), want)
	}
}

func TestNoHintsFlagAndEnv(t *testing.T) {
	for _, k := range []string{"LMON_TARGET", "LMON_GW", "LMON_COLS", "LMON_PING", "LMON_THRESHOLDS", "LMON_NO_HINTS"} {
		t.Setenv(k, "")
	}
	cfg, err := parseConfig([]string{"--no-hints"})
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.noHints {
		t.Error("--no-hints не установил флаг")
	}
	t.Setenv("LMON_NO_HINTS", "1")
	cfg, err = parseConfig(nil)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.noHints {
		t.Error("LMON_NO_HINTS=1 не установил флаг")
	}
}

func TestStartupHintsWindowsSensors(t *testing.T) {
	cfg := config{lang: "en", swap: true, gpu: true, gputemp: true}
	cols, dropped := layoutWithDropped(cfg, "windows")
	setValues(cols, map[string]int{"N": 24, "G": 2, "L": -1, "M": 48, "T": -1, "S": -1, "P": -1, "H": -1})
	txt := hintsText(startupHints(cfg, cols, "windows", dropped))
	for _, want := range []string{"page file", "GPU", "G°"} {
		if !strings.Contains(txt, want) {
			t.Errorf("в подсказках нет %q:\n%s", want, txt)
		}
	}
}

func TestStartupHintsTempCmdFailure(t *testing.T) {
	cfg := config{lang: "en", cols: []string{"T"}, tempCmd: "exit 1"}
	cols, dropped := layoutWithDropped(cfg, "windows")
	setValues(cols, map[string]int{"T": -1})
	txt := hintsText(startupHints(cfg, cols, "windows", dropped))
	if !strings.Contains(txt, "--temp-cmd") {
		t.Errorf("нет подсказки про --temp-cmd:\n%s", txt)
	}
}
