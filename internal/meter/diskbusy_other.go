//go:build !windows

package meter

// DiskBusyPercent — занятость диска: пока только Windows (PDH). На
// macOS/Linux честно -1; аналог iowait для них — колонка IOW.
func DiskBusyPercent() int { return -1 }
