package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/martiyanov/lmon/internal/netping"
)

func TestCheckHost(t *testing.T) {
	ok := []string{"8.8.8.8", "1.1.1.1", "example.com", "my-host.example.org", "192.168.1.1"}
	for _, h := range ok {
		if err := checkHost(h); err != nil {
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
		if err := checkHost(h); err == nil {
			t.Errorf("checkHost(%q) = nil, want error (%s)", h, why)
		}
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
		9:    " 9°",
		75:   "75°",
		100:  "100",
		1500: "+++",
	}
	for v, want := range cases {
		if got := u.digitCell(reading{id: "T", head: "T°", deg: true, v: v}); got != want {
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
	rs := []reading{{head: "NET"}, {head: "MEM"}, {head: "TMP"}}
	head, sep := headerOf(rs)
	if head != "NET MEM TMP" {
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
	row := u.sampleRow(context.Background(), config{cols: []string{"M"}})
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
