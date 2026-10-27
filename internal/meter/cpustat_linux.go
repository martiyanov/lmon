//go:build linux

package meter

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

var (
	prevCPU  [3]uint64 // busy, idle, iowait — прошлый замер тиков
	prevHave bool
)

// CPUSample возвращает загрузку CPU и iowait в процентах как дельту
// тиков /proc/stat между вызовами (первый вызов — с момента загрузки,
// чтобы не выдавать «·» в первой строке).
func CPUSample() (cpuPercent, iowaitPercent int) {
	busy, idle, iowait, err := cpuTicks()
	if err != nil {
		return -1, -1
	}
	if !prevHave {
		prevCPU = [3]uint64{busy, idle, iowait}
		prevHave = true
	}
	dBusy := busy - prevCPU[0]
	dIdle := idle - prevCPU[1]
	dIowait := iowait - prevCPU[2]
	dTotal := dBusy + dIdle + dIowait
	prevCPU = [3]uint64{busy, idle, iowait}
	if dTotal == 0 {
		return -1, -1
	}
	return int(dBusy * 100 / dTotal), int(dIowait * 100 / dTotal)
}

// cpuTicks читает первую строку /proc/stat.
func cpuTicks() (busy, idle, iowait uint64, err error) {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 0, 0, 0, err
	}
	return parseProcStat(string(data))
}

func parseProcStat(s string) (busy, idle, iowait uint64, err error) {
	line := ""
	for _, l := range strings.Split(s, "\n") {
		if strings.HasPrefix(l, "cpu ") {
			line = l
			break
		}
	}
	if line == "" {
		return 0, 0, 0, errors.New("procstat: строка cpu не найдена")
	}
	f := strings.Fields(line)[1:]
	get := func(i int) uint64 {
		if i >= len(f) {
			return 0
		}
		v, _ := strconv.ParseUint(f[i], 10, 64)
		return v
	}
	// user nice system idle iowait irq softirq steal ...
	idle = get(3)
	iowait = get(4)
	busy = get(0) + get(1) + get(2) + get(5) + get(6) + get(7)
	return busy, idle, iowait, nil
}

// SwapPercent возвращает долю занятого swap в процентах (/proc/meminfo).
func SwapPercent() int {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0
	}
	total, free := parseSwapMemInfo(string(data))
	if total == 0 {
		return 0
	}
	return int((total - free) * 100 / total)
}

func parseSwapMemInfo(s string) (total, free int64) {
	for _, line := range strings.Split(s, "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		v, err := strconv.ParseInt(f[1], 10, 64)
		if err != nil {
			continue
		}
		switch f[0] {
		case "SwapTotal:":
			total = v
		case "SwapFree:":
			free = v
		}
	}
	return total, free
}

// BatteryPercent возвращает заряд батареи в процентах; -1, если батареи нет.
func BatteryPercent() int {
	caps, _ := filepath.Glob("/sys/class/power_supply/BAT*/capacity")
	for _, c := range caps {
		data, err := os.ReadFile(c)
		if err != nil {
			continue
		}
		v, err := strconv.Atoi(strings.TrimSpace(string(data)))
		if err == nil {
			return v
		}
	}
	return -1
}
