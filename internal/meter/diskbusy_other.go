//go:build !windows && !darwin

package meter

// DiskBusyPercent — занятость диска: на Linux пока нет реализации
// (для I/O там есть колонка IOW), на прочих ОС — «нет данных».
func DiskBusyPercent() int { return -1 }
