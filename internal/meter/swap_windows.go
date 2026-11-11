//go:build windows

package meter

import (
	"math"
	"unsafe"

	"golang.org/x/sys/windows"
)

// pagefileUsage — «\Paging File(_Total)\% Usage»: процент занятого файла
// подкачки. Читается без прав администратора; на русской Windows
// английское имя сопоставляет сам PDH.
var pagefileUsage = newPDHCounter(`\Paging File(_Total)\% Usage`)

// SwapPercent — занятость файла подкачки, %. Это ближайший аналог
// Linux-swap: Windows не выгружает процессы целиком, а коммитит страницы
// и держит часть сжатой памяти в RAM. -1 — файла подкачки нет или он
// не читается (тогда колонка показывает «·»).
func SwapPercent() int {
	if v, ok := pagefileUsage.value(); ok {
		return clampPercent(v)
	}
	return pagefileFromPerfInfo()
}

// GetPerformanceInfo: резервный путь, если PDH-счётчик недоступен.
var (
	psapiDLL               = windows.NewLazySystemDLL("psapi.dll")
	procGetPerformanceInfo = psapiDLL.NewProc("GetPerformanceInfo")
)

// performanceInformation повторяет PERFORMANCE_INFORMATION (psapi.h).
type performanceInformation struct {
	cb                uint32
	CommitTotal       uintptr
	CommitLimit       uintptr
	CommitPeak        uintptr
	PhysicalTotal     uintptr
	PhysicalAvailable uintptr
	SystemCache       uintptr
	KernelTotal       uintptr
	KernelPaged       uintptr
	KernelNonpaged    uintptr
	PageSize          uintptr
	HandleCount       uint32
	ProcessCount      uint32
	ThreadCount       uint32
}

// pagefileFromPerfInfo оценивает занятость файла подкачки как «коммит
// сверх физической памяти»: commit limit = RAM + pagefile, поэтому
// (CommitTotal-PhysicalTotal)/(CommitLimit-PhysicalTotal) — приближение.
func pagefileFromPerfInfo() int {
	var pi performanceInformation
	pi.cb = uint32(unsafe.Sizeof(pi))
	r, _, _ := procGetPerformanceInfo.Call(uintptr(unsafe.Pointer(&pi)), uintptr(pi.cb))
	if r == 0 {
		return -1
	}
	if pi.CommitLimit <= pi.PhysicalTotal {
		return -1 // файл подкачки отключён
	}
	size := float64(pi.CommitLimit - pi.PhysicalTotal)
	used := float64(pi.CommitTotal) - float64(pi.PhysicalTotal)
	if used < 0 {
		used = 0
	}
	return clampPercent(used * 100 / size)
}

func clampPercent(v float64) int {
	if math.IsNaN(v) {
		return -1
	}
	n := int(math.Round(v))
	if n < 0 {
		return 0
	}
	if n > 100 {
		return 100
	}
	return n
}
