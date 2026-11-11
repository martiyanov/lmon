//go:build windows

package meter

// diskBusy — «\PhysicalDisk(_Total)\% Disk Time»: доля времени, когда диск
// был занят запросами. Это не Linux-iowait (в Windows его нет), но лучший
// беcадминный индикатор I/O-давления. Счётчик может превышать 100% на
// некоторых драйверах — значение ограничиваем.
var diskBusy = newPDHCounter(`\PhysicalDisk(_Total)\% Disk Time`)

// DiskBusyPercent — занятость диска, % (0..100), без прав администратора.
func DiskBusyPercent() int {
	if v, ok := diskBusy.value(); ok {
		return clampPercent(v)
	}
	return -1
}
