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
	"unsafe"
)

// GPUPercent — загрузка GPU, %.
// Порядок: NVML (NVIDIA, без прав) → nvidia-smi → PDH GPU Engine
// (AMD/Intel, WDDM, без прав). Нет источника — -1.
func GPUPercentAt(index int) int {
	if u, _, ok := nvmlProbe(index); ok && u >= 0 {
		return u
	}
	if u, _, ok := smiProbe(index); ok && u >= 0 {
		return u
	}
	if u, ok := pdhGPUUtil(index); ok {
		return u
	}
	return -1
}

// GPUTempC — температура GPU, °C. NVML (NVIDIA) или nvidia-smi;
// для AMD/Intel без вендорской библиотеки честно -1.
func GPUTempCAt(index int) int {
	if _, t, ok := nvmlProbe(index); ok && t >= 0 {
		return t
	}
	if _, t, ok := smiProbe(index); ok && t >= 0 {
		return t
	}
	return -1
}

// smiProbe запускает nvidia-smi и кэширует результат ~1 с: measure()
// спрашивает загрузку и температуру отдельно, лишний процесс не нужен.
type smiResult struct {
	util, temp int
	ok         bool
	at         time.Time
}

var (
	smiMu    sync.Mutex
	smiCache = map[int]smiResult{}
)

func smiProbe(index int) (util, temp int, ok bool) {
	smiMu.Lock()
	defer smiMu.Unlock()
	if r, ok := smiCache[index]; ok && time.Since(r.at) < 900*time.Millisecond {
		return r.util, r.temp, r.ok
	}
	res := smiResult{util: -1, temp: -1, at: time.Now()}
	path := nvidiaSmiPath()
	if path == "" {
		smiCache[index] = res
		return res.util, res.temp, res.ok
	}
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, "-i", strconv.Itoa(index),
		"--query-gpu=utilization.gpu,temperature.gpu",
		"--format=csv,noheader,nounits").Output()
	if err != nil {
		smiCache[index] = res
		return res.util, res.temp, res.ok
	}
	line := strings.SplitN(strings.TrimSpace(string(out)), "\n", 2)[0]
	parts := strings.Split(line, ",")
	if len(parts) >= 2 {
		if v, err := strconv.Atoi(strings.TrimSpace(parts[0])); err == nil && v >= 0 && v <= 100 {
			res.util = v
		}
		if v, err := strconv.Atoi(strings.TrimSpace(parts[1])); err == nil && v >= -50 && v <= 150 {
			res.temp = v
		}
		res.ok = res.util >= 0 || res.temp >= 0
	}
	smiCache[index] = res
	return res.util, res.temp, res.ok
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

func pdhGPUUtil(index int) (int, bool) {
	if index != 0 {
		return 0, false // PDH-счётчики агрегированы, выбрать конкретную GPU нельзя
	}
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

// GPUList — список видеокарт для --gpu-list: nvidia-smi, затем NVML.
func GPUList() []string {
	if path := nvidiaSmiPath(); path != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
		defer cancel()
		out, err := exec.CommandContext(ctx, path, "--query-gpu=index,name",
			"--format=csv,noheader").Output()
		if err == nil {
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
	}
	if n := loadNVML(); n != nil {
		var count uint32
		if r, _, _ := n.count.Call(uintptr(unsafe.Pointer(&count))); uint32(r) == 0 && count > 0 {
			lines := make([]string, 0, count)
			for i := uint32(0); i < count; i++ {
				lines = append(lines, strconv.Itoa(int(i))+": NVIDIA GPU")
			}
			return lines
		}
	}
	return nil
}
