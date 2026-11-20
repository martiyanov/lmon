package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

type procRow struct {
	PID     int     `json:"pid"`
	CPU     float64 `json:"cpu"`
	Mem     float64 `json:"mem"`
	RSSKB   int64   `json:"rss_kb"`
	Command string  `json:"command"`
}

func topProcesses(n int) ([]procRow, error) {
	if runtime.GOOS == "windows" {
		return nil, fmt.Errorf("--top is not supported on Windows yet")
	}
	out, err := exec.Command("ps", "-axo", "pid=,pcpu=,pmem=,rss=,comm=").Output()
	if err != nil {
		return nil, fmt.Errorf("ps: %w", err)
	}
	var rows []procRow
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) < 5 {
			continue
		}
		pid, err1 := strconv.Atoi(f[0])
		cpu, err2 := strconv.ParseFloat(f[1], 64)
		mem, err3 := strconv.ParseFloat(f[2], 64)
		rss, err4 := strconv.ParseInt(f[3], 10, 64)
		if err1 != nil || err2 != nil || err3 != nil || err4 != nil {
			continue
		}
		rows = append(rows, procRow{PID: pid, CPU: cpu, Mem: mem, RSSKB: rss, Command: strings.Join(f[4:], " ")})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].CPU == rows[j].CPU {
			return rows[i].RSSKB > rows[j].RSSKB
		}
		return rows[i].CPU > rows[j].CPU
	})
	if n > 0 && len(rows) > n {
		rows = rows[:n]
	}
	return rows, nil
}

func runTop(cfg config, w io.Writer) int {
	rows, err := topProcesses(cfg.topN)
	if err != nil {
		fmt.Fprintf(w, "lmon: %v\n", err)
		return 1
	}
	if cfg.jsonOut {
		enc := json.NewEncoder(w)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(rows); err != nil {
			return 1
		}
		return 0
	}
	fmt.Fprintf(w, "%7s %6s %6s %9s  %s\n", "PID", "CPU%", "MEM%", "RSS", "COMMAND")
	for _, p := range rows {
		fmt.Fprintf(w, "%7d %6.1f %6.1f %7.1fM  %s\n", p.PID, p.CPU, p.Mem, float64(p.RSSKB)/1024, p.Command)
	}
	return 0
}
