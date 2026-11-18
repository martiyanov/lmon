//go:build linux

package meter

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// nvidiaSMI запускает nvidia-smi, пробуя обычные места установки драйвера:
// на серверах PATH иногда не содержит /usr/bin, а на некоторых сборках
// утилита лежит в /usr/local/bin.
func nvidiaSMI(args ...string) ([]byte, error) {
	var lastErr error
	for _, bin := range []string{"nvidia-smi", "/usr/bin/nvidia-smi", "/usr/local/bin/nvidia-smi"} {
		out, err := runCmd(bin, args...)
		if err == nil {
			return out, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

// GPUPercent: nvidia-smi для выбранной карты, затем sysfs gpu_busy_percent.
func GPUPercentAt(index int) int {
	if out, err := nvidiaSMI("-i", strconv.Itoa(index),
		"--query-gpu=utilization.gpu", "--format=csv,noheader,nounits"); err == nil {
		if v, ok := firstInt(string(out)); ok {
			return v
		}
	}
	if v, ok := gpuBusySysfs(index); ok {
		return v
	}
	return -1
}

// GPUTempC: nvidia-smi, затем hwmon GPU-драйверов (amdgpu/i915/nouveau/radeon).
func GPUTempCAt(index int) int {
	if out, err := nvidiaSMI("-i", strconv.Itoa(index),
		"--query-gpu=temperature.gpu", "--format=csv,noheader,nounits"); err == nil {
		if v, ok := firstInt(string(out)); ok {
			return v
		}
	}
	if t, ok := gpuTempHwmon(index); ok {
		return t
	}
	return -1
}

// GPUList возвращает список GPU для --gpu-list: сначала пробует nvidia-smi,
// затем перечисляет DRM-устройства (AMD/Intel) из sysfs.
func GPUList() []string {
	if out, err := nvidiaSMI("--query-gpu=index,name", "--format=csv,noheader"); err == nil {
		var lines []string
		for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if strings.TrimSpace(l) != "" {
				lines = append(lines, l)
			}
		}
		if len(lines) > 0 {
			return lines
		}
	}
	return drmGPUList()
}

// drmGPUList перечисляет физические DRM-карты без NVIDIA: индекс, драйвер, PCI ID.
func drmGPUList() []string {
	uevents, _ := filepath.Glob("/sys/class/drm/card*/device/uevent")
	sort.Strings(uevents)
	var lines []string
	for _, ue := range uevents {
		data, err := os.ReadFile(ue)
		if err != nil {
			continue
		}
		var driver, pci string
		for _, l := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(l, "DRIVER=") {
				driver = strings.TrimPrefix(l, "DRIVER=")
			}
			if strings.HasPrefix(l, "PCI_ID=") {
				pci = strings.TrimPrefix(l, "PCI_ID=")
			}
		}
		if driver == "" && pci == "" {
			continue // виртуальный/непривязанный DRM-узел, не GPU
		}
		if driver == "nvidia" || driver == "nvidia-drm" {
			continue // NVIDIA уже показан через nvidia-smi
		}
		card := filepath.Base(filepath.Dir(filepath.Dir(ue))) // .../cardN/device/uevent → cardN
		desc := strings.TrimSpace(driver + " " + pci)
		lines = append(lines, strconv.Itoa(len(lines))+": "+desc+" ("+card+")")
	}
	return lines
}

func firstInt(s string) (int, bool) {
	f := strings.Fields(s)
	if len(f) == 0 {
		return 0, false
	}
	v, err := strconv.Atoi(f[0])
	return v, err == nil
}

func gpuBusySysfs(index int) (int, bool) {
	var paths []string
	for _, pat := range []string{
		"/sys/class/drm/card*/device/gpu_busy_percent",
		"/sys/class/drm/card*/gt/gt*/busy_percent",
	} {
		p, _ := filepath.Glob(pat)
		paths = append(paths, p...)
	}
	sort.Strings(paths)
	if index < 0 || index >= len(paths) {
		return 0, false
	}
	data, err := os.ReadFile(paths[index])
	if err != nil {
		return 0, false
	}
	if v, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil && v >= 0 && v <= 100 {
		return v, true
	}
	return 0, false
}

func gpuTempHwmon(index int) (int, bool) {
	hwmons, _ := filepath.Glob("/sys/class/hwmon/hwmon*")
	sort.Strings(hwmons)
	var gpuHwmons []string
	for _, h := range hwmons {
		name, _ := os.ReadFile(filepath.Join(h, "name"))
		switch strings.TrimSpace(string(name)) {
		case "amdgpu", "i915", "nouveau", "radeon":
			gpuHwmons = append(gpuHwmons, h)
		}
	}
	if index < 0 || index >= len(gpuHwmons) {
		return 0, false
	}
	inputs, _ := filepath.Glob(filepath.Join(gpuHwmons[index], "temp*_input"))
	for _, in := range inputs {
		if t, ok := readMilliDegrees(in); ok {
			return t, true
		}
	}
	return 0, false
}
