//go:build darwin

package meter

import (
	"encoding/binary"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

// CPUSample возвращает загрузку CPU в процентах за последнюю секунду
// (iostat -c 2 -w 1: вторая строка — скользящее окно в 1 с).
// iowait на macOS ядром не отдаётся — вернёт -1 (нет данных).
func CPUSample() (cpuPercent, iowaitPercent int) {
	out, err := exec.Command("iostat", "-c", "2", "-w", "1").Output()
	if err != nil {
		return -1, -1
	}
	return parseIostatCPU(string(out)), -1
}

// parseIostatCPU берёт колонку id из второй строки данных.
func parseIostatCPU(out string) int {
	lines := strings.Split(out, "\n")
	if len(lines) < 4 {
		return -1
	}
	// строка 1 — шапка: KB/t tps MB/s us sy id 1m 5m 15m (состав дисков различается)
	head := strings.Fields(lines[1])
	data := strings.Fields(lines[3])
	idCol := -1
	for i, h := range head {
		if h == "id" {
			idCol = i
			break
		}
	}
	if idCol < 0 || idCol >= len(data) {
		return -1
	}
	idle, err := strconv.Atoi(data[idCol])
	if err != nil {
		return -1
	}
	return 100 - idle
}

// SwapPercent возвращает долю занятого swap в процентах.
// vm.swapusage — бинарный массив записей struct xsw_usage по 32 байта:
// total, avail, used (uint64, байты), pagesize, encrypted (по 4 байта).
func SwapPercent() int {
	raw, err := unix.SysctlRaw("vm.swapusage")
	if err != nil {
		return 0
	}
	return parseSwapRaw(raw)
}

func parseSwapRaw(raw []byte) int {
	if len(raw) == 0 || len(raw)%32 != 0 {
		return 0
	}
	var total, used uint64
	for i := 0; i+32 <= len(raw); i += 32 {
		total += binary.LittleEndian.Uint64(raw[i:])
		used += binary.LittleEndian.Uint64(raw[i+16:])
	}
	if total == 0 {
		return 0
	}
	return int(used * 100 / total)
}

// BatteryPercent возвращает заряд батареи в процентах; -1, если батареи нет.
func BatteryPercent() int {
	out, err := exec.Command("pmset", "-g", "batt").Output()
	if err != nil {
		return -1
	}
	return parsePmsetBatt(string(out))
}

func parsePmsetBatt(out string) int {
	// "InternalBattery-0 (id=...)	55%; discharging; ..."
	m := regexp.MustCompile(`(\d+)%`).FindStringSubmatch(out)
	if m == nil {
		return -1
	}
	v, err := strconv.Atoi(m[1])
	if err != nil {
		return -1
	}
	return v
}
