//go:build linux

package meter

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// GPUPercent: nvidia-smi, затем sysfs gpu_busy_percent (amdgpu/i915).
func GPUPercent() int {
	if out, err := runCmd("nvidia-smi", "--query-gpu=utilization.gpu",
		"--format=csv,noheader,nounits"); err == nil {
		if v, ok := firstInt(string(out)); ok {
			return v
		}
	}
	if v, ok := gpuBusySysfs(); ok {
		return v
	}
	return -1
}

// GPUTempC: nvidia-smi, затем hwmon GPU-драйверов (amdgpu/i915/nouveau/radeon).
func GPUTempC() int {
	if out, err := runCmd("nvidia-smi", "--query-gpu=temperature.gpu",
		"--format=csv,noheader,nounits"); err == nil {
		if v, ok := firstInt(string(out)); ok {
			return v
		}
	}
	if t, ok := gpuTempHwmon(); ok {
		return t
	}
	return -1
}

func firstInt(s string) (int, bool) {
	f := strings.Fields(s)
	if len(f) == 0 {
		return 0, false
	}
	v, err := strconv.Atoi(f[0])
	return v, err == nil
}

func gpuBusySysfs() (int, bool) {
	for _, pat := range []string{
		"/sys/class/drm/card*/device/gpu_busy_percent",
		"/sys/class/drm/card*/gt/gt*/busy_percent",
	} {
		paths, _ := filepath.Glob(pat)
		for _, p := range paths {
			data, err := os.ReadFile(p)
			if err != nil {
				continue
			}
			if v, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil && v >= 0 && v <= 100 {
				return v, true
			}
		}
	}
	return 0, false
}

func gpuTempHwmon() (int, bool) {
	hwmons, _ := filepath.Glob("/sys/class/hwmon/hwmon*")
	for _, h := range hwmons {
		name, _ := os.ReadFile(filepath.Join(h, "name"))
		switch strings.TrimSpace(string(name)) {
		case "amdgpu", "i915", "nouveau", "radeon":
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
