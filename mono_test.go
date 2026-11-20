package main

import (
	"os"
	"strings"
	"testing"
)

func TestMonoFills(t *testing.T) {
	u := &ui{mono: true}
	low := reading{id: "N", v: 10, green: 50, yellow: 150, red: 300}
	if got := u.blockCell(low); got != " . " {
		t.Fatalf("low bar = %q", got)
	}
	bad := reading{id: "N", v: 999, green: 50, yellow: 150, red: 300}
	if got := u.blockCell(bad); got != "!!!" {
		t.Fatalf("bad bar = %q", got)
	}
	if got := u.compactCell(low); got != "·" {
		t.Fatalf("low compact = %q", got)
	}
	if got := u.compactCell(reading{id: "N", v: 199, green: 50, yellow: 150, red: 300}); got != "▓" && got != "█" {
		t.Fatalf("high compact = %q", got)
	}
}

func TestMonoAutoWithoutColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	u := newOnceUI(os.Stdout, config{compact: true}, true)
	if u.color || !u.mono {
		t.Fatalf("compact NO_COLOR: color=%v mono=%v", u.color, u.mono)
	}
	if got := u.compactCell(reading{id: "N", v: 10, green: 50, yellow: 150, red: 300}); strings.Contains(got, "\x1b") || got != "·" {
		t.Fatalf("compact mono = %q", got)
	}
}
