package main

import "testing"

func TestGridColumns(t *testing.T) {
	rs := []reading{
		{id: "N", head: "NET"}, {id: "G", head: "GTW"}, {id: "C", head: "CPU"},
		{id: "L", head: "RUN"}, {id: "M", head: "MEM"}, {id: "T", head: " C°"},
		{id: "U", head: "DSK"}, {id: "S", head: "SWP"},
	}
	cases := []struct{ cols, want int }{
		{100, 8}, {80, 8}, {74, 8}, {64, 8}, {63, 8}, {56, 8},
		{55, 8}, {47, 8}, {30, 8}, {24, 8}, {23, 8}, {21, 8},
		{20, 8}, {15, 8}, {5, 5},
	}
	for _, c := range cases {
		if got := gridColumns(c.cols, rs); got != c.want {
			t.Errorf("gridColumns(%d) = %d, want %d", c.cols, got, c.want)
		}
	}
}

func TestResizeLabelsFit(t *testing.T) {
	n := reading{id: "N", head: "NET"}
	if got := resizeLabelFor(n, 6); got != "NET" {
		t.Fatalf("narrow label = %q, want NET", got)
	}
	if got := resizeLabelFor(n, 8); got != "Network" {
		t.Fatalf("wide label = %q, want Network", got)
	}
}

func TestResizeNumericWidth(t *testing.T) {
	u := &ui{}
	r := reading{id: "L", v: 12345}
	if got := u.resizeText(r, 6); got != "12345" {
		t.Fatalf("wide number = %q", got)
	}
	if got := u.resizeText(r, 3); got != "+++" {
		t.Fatalf("narrow number = %q", got)
	}
}

func TestResizeDefaults(t *testing.T) {
	cfg, err := parseConfig(nil)
	if err != nil || !cfg.resizeWindow {
		t.Fatalf("default resize = %+v err=%v", cfg.resizeWindow, err)
	}
	cfg, err = parseConfig([]string{"--once"})
	if err != nil || cfg.resizeWindow {
		t.Fatalf("once resize = %+v err=%v", cfg.resizeWindow, err)
	}
	cfg, err = parseConfig([]string{"--horizontal"})
	if err != nil || cfg.resizeWindow {
		t.Fatalf("horizontal resize = %+v err=%v", cfg.resizeWindow, err)
	}
	cfg, err = parseConfig([]string{"--three-columns"})
	if err != nil || !cfg.resizeWindow || !cfg.threeColumns {
		t.Fatalf("three columns = %+v err=%v", cfg, err)
	}
	cfg, err = parseConfig([]string{"--resize-window=false"})
	if err != nil || cfg.resizeWindow {
		t.Fatalf("explicit off = %+v err=%v", cfg.resizeWindow, err)
	}
	cfg, err = parseConfig([]string{"--compact"})
	if err != nil || cfg.resizeWindow || !cfg.compact {
		t.Fatalf("compact = %+v err=%v", cfg, err)
	}
}

func TestUnitsNeedSpace(t *testing.T) {
	u := &ui{}
	if got := u.resizeText(reading{id: "C", v: 74}, 3); got != "74" {
		t.Fatalf("narrow percent = %q", got)
	}
	if got := u.resizeText(reading{id: "C", v: 74}, 4); got != "74 %" {
		t.Fatalf("wide percent = %q", got)
	}
	if got := rateWide(500, 5); got != "500 K" {
		t.Fatalf("rate = %q", got)
	}
	if got := rateWide(500, 4); got != "500" {
		t.Fatalf("rate narrow = %q", got)
	}
}
