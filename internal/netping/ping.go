// Package netping измеряет ICMP-задержку через системную утилиту ping,
// поэтому прав root не нужны.
package netping

import (
	"context"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"time"
)

// unreachable показывается, когда хост не отвечает.
const unreachable = 999

var timeRe = regexp.MustCompile(`time[=<]([0-9]+(?:\.[0-9]+)?)`)

// Latency возвращает время отклика до host в миллисекундах (усечённое до
// целого). Отправляются два пакета: первый часто только «будит» Wi-Fi-радио,
// поэтому честной задержкой считается последний ответ.
func Latency(ctx context.Context, host string) int {
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()

	out, _ := exec.CommandContext(ctx, "ping", args(host)...).Output()
	if times := parseTimes(string(out)); len(times) > 0 {
		return int(times[len(times)-1])
	}
	return unreachable
}

func args(host string) []string {
	if runtime.GOOS == "darwin" {
		// на macOS -W в миллисекундах; интервал меньше 0.2 с разрешён
		return []string{"-c", "2", "-i", "0.1", "-W", "1000", host}
	}
	// на Linux -W в секундах, а интервал меньше 0.2 с требует root
	return []string{"-c", "2", "-i", "0.2", "-W", "1", host}
}

func parseTimes(out string) []float64 {
	ms := timeRe.FindAllStringSubmatch(out, -1)
	times := make([]float64, 0, len(ms))
	for _, m := range ms {
		if v, err := strconv.ParseFloat(m[1], 64); err == nil {
			times = append(times, v)
		}
	}
	return times
}
