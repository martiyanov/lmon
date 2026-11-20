package main

import (
	"strings"
	"testing"
)

func TestProfileAndAutomationFlags(t *testing.T) {
	cfg, err := parseConfig([]string{"--profile", "server"})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"N", "G", "C", "L", "M", "T", "U", "S", "I", "K"} {
		if !containsString(cfg.cols, id) {
			t.Fatalf("server profile missing %s: %v", id, cfg.cols)
		}
	}
	if _, err := parseConfig([]string{"--profile", "nope"}); err == nil {
		t.Fatal("invalid profile accepted")
	}
	if _, err := parseConfig([]string{"--json"}); err == nil {
		t.Fatal("--json without --once/--top accepted")
	}
	if _, err := parseConfig([]string{"--exit-code"}); err == nil {
		t.Fatal("--exit-code without --once accepted")
	}
	cfg, err = parseConfig([]string{"--once", "--json", "--exit-code"})
	if err != nil || !cfg.jsonOut || !cfg.exitCode {
		t.Fatalf("automation flags: %+v %v", cfg, err)
	}
	cfg, err = parseConfig([]string{"--top", "5", "--json"})
	if err != nil || cfg.topN != 5 || !cfg.jsonOut {
		t.Fatalf("top flags: %+v %v", cfg, err)
	}
}

func TestProfileExplicitColsWins(t *testing.T) {
	cfg, err := parseConfig([]string{"--profile", "server", "--cols", "NG"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(cfg.cols, ",") != "N,G" {
		t.Fatalf("cols = %v", cfg.cols)
	}
}

func containsString(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}
