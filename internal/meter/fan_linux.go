//go:build linux

package meter

import (
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// FanList — все доступные вентиляторы: hwmon (fan*_input + pwm*) и GPU.
// Порядок стабильный: сначала hwmon в порядке путей, затем GPU.
func FanList() []FanInfo {
	var infos []FanInfo
	hwmons, _ := filepath.Glob("/sys/class/hwmon/hwmon*")
	sort.Strings(hwmons)
	for _, h := range hwmons {
		hwName := strings.TrimSpace(readString(filepath.Join(h, "name")))
		fans, _ := filepath.Glob(filepath.Join(h, "fan*_input"))
		sort.Strings(fans)
		for _, fp := range fans {
			base := filepath.Base(fp) // fan1_input
			idx := strings.TrimSuffix(strings.TrimPrefix(base, "fan"), "_input")
			if idx == "" {
				continue
			}
			rpm, _ := readIntFile(fp)
			pct := -1
			if v, ok := readIntFile(filepath.Join(h, "pwm"+idx)); ok && v >= 0 && v <= 255 {
				pct = int(math.Round(float64(v) * 100 / 255))
			}
			if rpm < 0 && pct < 0 {
				continue
			}
			name := strings.TrimSpace(readString(filepath.Join(h, "fan"+idx+"_label")))
			if name == "" {
				name = hwName + " fan" + idx
			}
			kind := "rpm"
			if rpm < 0 {
				kind = "pct"
			}
			infos = append(infos, FanInfo{Name: name, RPM: rpm, Percent: pct, Kind: kind})
		}
	}
	if pct, name, ok := nvidiaFanInfo(); ok {
		infos = append(infos, FanInfo{Name: "GPU " + name, RPM: -1, Percent: pct, Kind: "pct"})
	}
	return infos
}

// FanValues — значения для колонок: RPM/10 для вентиляторов с оборотами,
// проценты для GPU/PWM-only. Порядок совпадает с FanList.
func FanValues() []int {
	infos := FanList()
	out := make([]int, 0, len(infos))
	for _, fi := range infos {
		out = append(out, fanValue(fi))
	}
	return out
}

func fanValue(fi FanInfo) int {
	if fi.Kind == "rpm" {
		if fi.RPM < 0 {
			return -1
		}
		return fi.RPM / 10
	}
	return fi.Percent
}

// FanPercent — максимальная скорость вентиляторов, % (PWM или GPU fan).
func FanPercent() int {
	best := -1
	for _, fi := range FanList() {
		if fi.Percent > best {
			best = fi.Percent
		}
	}
	return best
}

func nvidiaFanInfo() (pct int, name string, ok bool) {
	out, err := nvidiaSMI("-i", strconv.Itoa(GPUIndex()),
		"--query-gpu=name,fan.speed", "--format=csv,noheader,nounits")
	if err != nil {
		return 0, "", false
	}
	line := strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
	parts := strings.Split(line, ",")
	if len(parts) < 2 {
		return 0, "", false
	}
	v, err := strconv.Atoi(strings.TrimSpace(parts[len(parts)-1]))
	if err != nil || v < 0 || v > 100 {
		return 0, "", false
	}
	return v, strings.TrimSpace(strings.Join(parts[:len(parts)-1], ",")), true
}

func readString(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(b)
}

func readIntFile(path string) (int, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	v, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil {
		return 0, false
	}
	return v, true
}
