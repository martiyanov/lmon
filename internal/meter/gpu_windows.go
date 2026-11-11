//go:build windows

package meter

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// GPUPercent — загрузка GPU, %.
// Порядок: NVML (NVIDIA, без прав) → nvidia-smi → PDH GPU Engine
// (AMD/Intel, WDDM, без прав). Нет источника — -1.
func GPUPercent() int {
	if u, _, ok := nvmlProbe(); ok && u >= 0 {
		return u
	}
	if u, _, ok := smiProbe(); ok && u >= 0 {
		return u
	}
	if u, ok := pdhGPUUtil(); ok {
		return u
	}
	return -1
}

// GPUTempC — температура GPU, °C. NVML (NVIDIA) или nvidia-smi;
// для AMD/Intel без вендорской библиотеки честно -1.
func GPUTempC() int {
	if _, t, ok := nvmlProbe(); ok && t >= 0 {
		return t
	}
	if _, t, ok := smiProbe(); ok && t >= 0 {
		return t
	}
	return -1
}

// smiProbe запускает nvidia-smi и кэширует результат ~1 с: measure()
// спрашивает загрузку и температуру отдельно, лишний процесс не нужен.
var (
	smiMu   sync.Mutex
	smiAt   time.Time
	smiUtil int
	smiTemp int
	smiOK   bool
)

func smiProbe() (util, temp int, ok bool) {
	smiMu.Lock()
	defer smiMu.Unlock()
	if time.Since(smiAt) < 900*time.Millisecond {
		return smiUtil, smiTemp, smiOK
	}
	smiAt = time.Now()
	smiUtil, smiTemp, smiOK = -1, -1, false
	path := nvidiaSmiPath()
	if path == "" {
		return smiUtil, smiTemp, smiOK
	}
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, "--query-gpu=utilization.gpu,temperature.gpu",
		"--format=csv,noheader,nounits").Output()
	if err != nil {
		return smiUtil, smiTemp, smiOK
	}
	line := strings.SplitN(strings.TrimSpace(string(out)), "\n", 2)[0]
	parts := strings.Split(line, ",")
	if len(parts) < 2 {
		return smiUtil, smiTemp, smiOK
	}
	if v, err := strconv.Atoi(strings.TrimSpace(parts[0])); err == nil && v >= 0 && v <= 100 {
		smiUtil = v
	}
	if v, err := strconv.Atoi(strings.TrimSpace(parts[1])); err == nil && v >= -50 && v <= 150 {
		smiTemp = v
	}
	smiOK = smiUtil >= 0 || smiTemp >= 0
	return smiUtil, smiTemp, smiOK
}

// nvidiaSmiPath ищет nvidia-smi: PATH, System32 (новые драйверы) и
// исторический каталог NVSMI.
func nvidiaSmiPath() string {
	if p, err := exec.LookPath("nvidia-smi.exe"); err == nil {
		return p
	}
	candidates := []string{
		filepath.Join(os.Getenv("SystemRoot"), "System32", "nvidia-smi.exe"),
		filepath.Join(os.Getenv("ProgramFiles"), "NVIDIA Corporation", "NVSMI", "nvidia-smi.exe"),
	}
	for _, p := range candidates {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}

// pdhGPUUtil — «\GPU Engine(*)\Utilization Percentage»: работает для
// любых вендоров через WDDM, английское имя не зависит от локали.
var gpuEngines = newPDHCounter(`\GPU Engine(*)\Utilization Percentage`)

func pdhGPUUtil() (int, bool) {
	vals, ok := gpuEngines.values()
	if !ok {
		return 0, false
	}
	best := -1.0
	for _, v := range vals {
		if v > best {
			best = v
		}
	}
	if best < 0 {
		return 0, false
	}
	return clampPercent(best), true
}
