//go:build linux

package meter

import (
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// FanPercent — максимальная скорость вентиляторов, %:
// PWM-выходы hwmon (0..255 → 0..100%) плюс вентилятор выбранной NVIDIA GPU.
// PWM — это управляющий сигнал, а не точные обороты, но он показывает
// «насколько раскручены» вентиляторы и меняется вместе с шумом.
func FanPercent() int {
	best := -1
	paths, _ := filepath.Glob("/sys/class/hwmon/hwmon*/pwm[0-9]*")
	for _, p := range paths {
		if !isPWMPath(p) {
			continue
		}
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		v, err := strconv.Atoi(strings.TrimSpace(string(data)))
		if err != nil || v < 0 || v > 255 {
			continue
		}
		pct := int(math.Round(float64(v) * 100 / 255))
		if pct > best {
			best = pct
		}
	}
	if gpu, ok := nvidiaFanPercent(); ok && gpu > best {
		best = gpu
	}
	return best
}

func isPWMPath(p string) bool {
	base := filepath.Base(p)
	rest := strings.TrimPrefix(base, "pwm")
	if rest == "" || rest == base {
		return false
	}
	for _, r := range rest {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// nvidiaFanPercent — fan.speed выбранной GPU (0..100) через nvidia-smi.
func nvidiaFanPercent() (int, bool) {
	out, err := nvidiaSMI("-i", strconv.Itoa(GPUIndex()),
		"--query-gpu=fan.speed", "--format=csv,noheader,nounits")
	if err != nil {
		return 0, false
	}
	v, ok := firstInt(string(out))
	if !ok || v < 0 || v > 100 {
		return 0, false
	}
	return v, true
}
