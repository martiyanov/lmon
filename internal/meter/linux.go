//go:build linux

package meter

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// LoadAvg возвращает load average за минуту из /proc/loadavg.
func LoadAvg() (float64, error) {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return 0, err
	}
	return parseLoadAvg(string(data))
}

func parseLoadAvg(s string) (float64, error) {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return 0, errors.New("loadavg: пустой файл")
	}
	return strconv.ParseFloat(fields[0], 64)
}

// MemPercent возвращает долю занятой памяти в процентах: занято всё,
// что не попало в MemAvailable (то же определение, что у free(1)).
func MemPercent() int {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0
	}
	total, available := parseMemInfo(string(data))
	if total == 0 {
		return 0
	}
	return int((total - available) * 100 / total)
}

func parseMemInfo(s string) (total, available int64) {
	for _, line := range strings.Split(s, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		v, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			continue
		}
		switch fields[0] {
		case "MemTotal:":
			total = v
		case "MemAvailable:":
			available = v
		}
	}
	return total, available
}

// TempC возвращает температуру CPU из sysfs: сначала сенсоры hwmon
// (coretemp/k10temp/...), затем thermal_zone с CPU-типом; 0, если сенсоров нет.
func TempC() int {
	if t, ok := tempFromHwmon(); ok {
		return t
	}
	if t, ok := tempFromThermalZones(); ok {
		return t
	}
	return 0
}

func tempFromHwmon() (int, bool) {
	hwmons, _ := filepath.Glob("/sys/class/hwmon/hwmon*")
	for _, h := range hwmons {
		name, _ := os.ReadFile(filepath.Join(h, "name"))
		switch strings.TrimSpace(string(name)) {
		case "coretemp", "k10temp", "k8temp", "zenpower", "cpu_thermal":
		default:
			continue
		}
		inputs, _ := filepath.Glob(filepath.Join(h, "temp*_input"))
		for _, in := range inputs {
			if t, ok := readMilliDegrees(in); ok {
				return t, true
			}
		}
	}
	return 0, false
}

func tempFromThermalZones() (int, bool) {
	zones, _ := filepath.Glob("/sys/class/thermal/thermal_zone*")
	for _, z := range zones {
		typ, _ := os.ReadFile(filepath.Join(z, "type"))
		t := strings.ToLower(string(typ))
		if !strings.Contains(t, "pkg") && !strings.Contains(t, "cpu") && !strings.Contains(t, "x86") {
			continue
		}
		if v, ok := readMilliDegrees(filepath.Join(z, "temp")); ok {
			return v, true
		}
	}
	// зоны с CPU-меткой нет — берём первую читаемую
	for _, z := range zones {
		if v, ok := readMilliDegrees(filepath.Join(z, "temp")); ok {
			return v, true
		}
	}
	return 0, false
}

func readMilliDegrees(path string) (int, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	milli, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
	if err != nil || milli <= 0 || milli > 150000 {
		return 0, false
	}
	return int(milli / 1000), true
}
