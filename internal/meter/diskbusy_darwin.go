//go:build darwin

package meter

import (
	"regexp"
	"strconv"
	"sync"
	"time"
)

// macOS не отдаёт iowait. Ближайший аналог «диск занят» — дельта
// Total Time (Read/Write) из IOKit IOBlockStorageDriver: это время
// обслуживания I/O в наносекундах, без прав администратора.
var ioTotalRe = regexp.MustCompile(`"Total Time \((?:Read|Write)\)"=(\d+)`)

// parseIOBlockStorageTotal суммирует время чтения и записи по всем
// IOBlockStorageDriver в выводе `ioreg -c IOBlockStorageDriver`.
func parseIOBlockStorageTotal(out string) (uint64, bool) {
	var total uint64
	n := 0
	for _, m := range ioTotalRe.FindAllStringSubmatch(out, -1) {
		v, err := strconv.ParseUint(m[1], 10, 64)
		if err != nil {
			continue
		}
		total += v
		n++
	}
	return total, n > 0
}

var (
	diskMu        sync.Mutex
	lastDiskTotal uint64
	lastDiskAt    time.Time
)

// DiskBusyPercent — доля времени, когда диск был занят I/O, % (0..100).
// Первый вызов возвращает 0: нужна дельта между двумя замерами.
func DiskBusyPercent() int {
	out, err := runCmd("ioreg", "-r", "-c", "IOBlockStorageDriver", "-k", "Statistics", "-l")
	if err != nil {
		return -1
	}
	total, ok := parseIOBlockStorageTotal(string(out))
	if !ok {
		return -1
	}
	now := time.Now()
	diskMu.Lock()
	defer diskMu.Unlock()
	if lastDiskAt.IsZero() || now.Sub(lastDiskAt) < 100*time.Millisecond {
		lastDiskTotal, lastDiskAt = total, now
		return 0
	}
	dt := now.Sub(lastDiskAt).Seconds()
	delta := float64(total-lastDiskTotal) / 1e9
	lastDiskTotal, lastDiskAt = total, now
	if dt <= 0 || delta < 0 {
		return 0
	}
	return clampPercent(delta / dt * 100)
}
