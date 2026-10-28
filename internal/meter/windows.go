//go:build windows

package meter

import (
	"errors"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Часть WinAPI не обёрнута в используемой версии x/sys — зовём напрямую.
var (
	modkernel32              = windows.NewLazySystemDLL("kernel32.dll")
	procGlobalMemoryStatusEx = modkernel32.NewProc("GlobalMemoryStatusEx")
	procGetSystemTimes       = modkernel32.NewProc("GetSystemTimes")
	procGetSystemPowerStatus = modkernel32.NewProc("GetSystemPowerStatus")
)

// LoadAvg — показателя load average на Windows нет; колонка L выведется как «·».
func LoadAvg() (float64, error) {
	return 0, errors.New("load average: на Windows отсутствует")
}

// CPUSample возвращает загрузку CPU дельтой GetSystemTimes
// (kernel time включает idle). iowait на Windows отсутствует.
func CPUSample() (cpuPercent, iowaitPercent int) {
	var idle, kernel, user windows.Filetime
	r1, _, _ := procGetSystemTimes.Call(
		uintptr(unsafe.Pointer(&idle)),
		uintptr(unsafe.Pointer(&kernel)),
		uintptr(unsafe.Pointer(&user)))
	if r1 == 0 {
		return -1, -1
	}
	ft := func(f windows.Filetime) uint64 { return uint64(f.HighDateTime)<<32 | uint64(f.LowDateTime) }
	busy := (ft(kernel) - ft(idle)) + ft(user)
	total := ft(kernel) + ft(user)
	if !havePrevCPU {
		prevBusyCPU, prevTotalCPU, havePrevCPU = busy, total, true
		return -1, -1
	}
	if total > prevTotalCPU {
		cpuPercent = int((busy - prevBusyCPU) * 100 / (total - prevTotalCPU))
	}
	prevBusyCPU, prevTotalCPU = busy, total
	return cpuPercent, -1
}

var prevBusyCPU, prevTotalCPU uint64
var havePrevCPU bool

// memoryStatusEx повторяет MEMORYSTATUSEX.
type memoryStatusEx struct {
	Length                       uint32
	MemoryLoad                   uint32
	TotalPhys, AvailPhys         uint64
	TotalPageFile, AvailPageFile uint64
	TotalVirtual, AvailVirtual   uint64
	AvailExtendedVirtual         uint64
}

func queryMemory() (physLoad, pageLoad int) {
	var ms memoryStatusEx
	ms.Length = uint32(unsafe.Sizeof(ms))
	r1, _, _ := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&ms)))
	if r1 == 0 || ms.TotalPhys == 0 {
		return -1, -1
	}
	if ms.TotalPhys != 0 {
		physLoad = int((ms.TotalPhys - ms.AvailPhys) * 100 / ms.TotalPhys)
	}
	if ms.TotalPageFile != 0 {
		pageLoad = int((ms.TotalPageFile - ms.AvailPageFile) * 100 / ms.TotalPageFile)
	}
	return physLoad, pageLoad
}

// MemPercent — занятая физическая память (как в диспетчере задач).
func MemPercent() int { p, _ := queryMemory(); return p }

// SwapPercent — реальная статистика файла подкачка через WinAPI недоступна
// без счётчиков производительности; MEMORYSTATUSEX даёт committed memory,
// а не swap. Показываем честное «нет данных».
func SwapPercent() int { return -1 }

// TempC — надёжного датчика температуры через WinAPI нет
// (WMI-счётчик ненадёжен и требует прав администратора).
func TempC() int { return -1 }

// DiskPercent — занятость системного диска (GetDiskFreeSpaceEx).
func DiskPercent() int {
	drive := os.Getenv("SystemDrive")
	if drive == "" {
		drive = "C:"
	}
	p, err := windows.UTF16PtrFromString(drive + `\`)
	if err != nil {
		return -1
	}
	var free, total uint64
	if err := windows.GetDiskFreeSpaceEx(p, &free, &total, new(uint64)); err != nil || total == 0 {
		return -1
	}
	return int((total - free) * 100 / total)
}

// systemPowerStatus повторяет SYSTEM_POWER_STATUS.
type systemPowerStatus struct {
	ACLineStatus        uint8
	BatteryFlag         uint8
	BatteryLifePercent  uint8
	Reserved1           uint8
	BatteryLifeTime     uint32
	BatteryFullLifeTime uint32
}

// BatteryPercent — заряд батареи; -1, если батареи нет.
func BatteryPercent() int {
	var ps systemPowerStatus
	r1, _, _ := procGetSystemPowerStatus.Call(uintptr(unsafe.Pointer(&ps)))
	if r1 == 0 || ps.BatteryLifePercent > 100 || ps.BatteryFlag&0x80 != 0 {
		return -1
	}
	return int(ps.BatteryLifePercent)
}

var routeRe = regexp.MustCompile(`(?m)^\s*0\.0\.0\.0\s+0\.0\.0\.0\s+(\d{1,3}(?:\.\d{1,3}){3})\s+\S+\s+(\d+)`)

// DefaultGateway разбирает вывод route print (таблица маршрутов не локализована).
// Из нескольких default-маршрутов берётся маршрут с наименьшей метрикой;
// записи с шлюзом «On-link» (без IP next-hop) пропускаются.
func DefaultGateway(target string) (string, error) {
	out, err := exec.Command("route", "print", "-4", "0.0.0.0").Output()
	if err != nil {
		return "", err
	}
	best, bestMetric := "", -1
	for _, m := range routeRe.FindAllStringSubmatch(string(out), -1) {
		metric, err := strconv.Atoi(strings.TrimSpace(m[2]))
		if err != nil || metric < 0 {
			metric = 999999
		}
		if bestMetric < 0 || metric < bestMetric {
			best, bestMetric = m[1], metric
		}
	}
	if best == "" {
		return "", errors.New("gateway: маршрут по умолчанию не найден")
	}
	return best, nil
}
