//go:build darwin

package meter

import (
	"regexp"
	"strconv"
)

var gpuUtilRe = regexp.MustCompile(`"Device Utilization %"=(\d+)`)

// GPUPercent — загрузка GPU (Apple Silicon/Intel: ioreg IOAccelerator,
// без sudo; берём максимальный из ускорителей). Нет данных — -1.
func GPUPercent() int {
	out, err := runCmd("ioreg", "-r", "-d", "1", "-w", "0", "-c", "IOAccelerator")
	if err != nil {
		return -1
	}
	return parseGPUUtil(string(out))
}

// parseGPUUtil берёт максимальную загрузку среди ускорителей.
func parseGPUUtil(out string) int {
	best := -1
	for _, m := range gpuUtilRe.FindAllStringSubmatch(out, -1) {
		if v, err := strconv.Atoi(m[1]); err == nil && v > best {
			best = v
		}
	}
	return best
}

// GPUTempC — температура GPU на macOS доступна только через powermetrics
// под sudo; честно показываем «нет данных».
func GPUTempC() int { return -1 }
