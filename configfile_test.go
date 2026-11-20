package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadConfigArgs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	body := "# test\ninterval=2\ncols=NG\nbars=true\ngpu=false\nping=VPN=10.8.0.1\nthresholds=NET=10/20/30\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	args, err := loadConfigArgs(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := parseConfig(args)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.interval != 2*time.Second || strings.Join(cfg.cols, ",") != "N,G" || !cfg.blocks || cfg.gpu {
		t.Fatalf("config = %+v", cfg)
	}
	if len(cfg.pings) != 1 || cfg.pings[0].name != "VPN" || cfg.pings[0].host != "10.8.0.1" {
		t.Fatalf("pings = %+v", cfg.pings)
	}
	if cfg.thresholds["N"] != [3]int{10, 20, 30} {
		t.Fatalf("thresholds = %+v", cfg.thresholds)
	}
}

func TestWriteConfigRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lmon", "config")
	cfg := config{
		target: "1.1.1.1", gw: "192.168.1.1", interval: 3 * time.Second,
		cols: []string{"N", "G", "M"}, blocks: true, rain: true, matrixPalette: true,
		pings:      []pingTarget{{name: "VPN", host: "10.8.0.1"}},
		thresholds: map[string][3]int{"NET": {10, 20, 30}},
	}
	if err := writeConfigFile(cfg, path); err != nil {
		t.Fatal(err)
	}
	args, err := loadConfigArgs(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := parseConfig(args)
	if err != nil {
		t.Fatal(err)
	}
	if got.target != cfg.target || got.gw != cfg.gw || got.interval != cfg.interval {
		t.Fatalf("round trip = %+v", got)
	}
	if strings.Join(got.cols, ",") != "N,G,M" || !got.blocks || !got.rain || !got.matrixPalette {
		t.Fatalf("modes = %+v", got)
	}
	if len(got.pings) != 1 || got.thresholds["N"] != [3]int{10, 20, 30} {
		t.Fatalf("pings/thresholds = %+v %+v", got.pings, got.thresholds)
	}
}
