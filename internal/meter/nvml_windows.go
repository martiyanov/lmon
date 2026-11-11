//go:build windows

package meter

import (
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// NVML (NVIDIA Management Library) даёт загрузку, температуру, память и
// вентилятор GPU без прав администратора. Библиотека ставится с драйвером
// NVIDIA. Если её нет — вызывающий пробует nvidia-smi/PDH.
type nvmlProcs struct {
	dll    *windows.LazyDLL
	init   *windows.LazyProc
	count  *windows.LazyProc
	handle *windows.LazyProc
	temp   *windows.LazyProc
	util   *windows.LazyProc
	ok     bool
}

var (
	nvmlOnce sync.Once
	nvmlLib  nvmlProcs
)

// loadNVML лениво загружает nvml.dll и инициализирует библиотеку.
func loadNVML() *nvmlProcs {
	nvmlOnce.Do(func() {
		dll := windows.NewLazyDLL("nvml.dll")
		if err := dll.Load(); err != nil {
			return
		}
		n := nvmlProcs{
			dll:    dll,
			init:   dll.NewProc("nvmlInit_v2"),
			count:  dll.NewProc("nvmlDeviceGetCount_v2"),
			handle: dll.NewProc("nvmlDeviceGetHandleByIndex_v2"),
			temp:   dll.NewProc("nvmlDeviceGetTemperature"),
			util:   dll.NewProc("nvmlDeviceGetUtilizationRates"),
		}
		if r, _, _ := n.init.Call(); uint32(r) != 0 {
			return
		}
		n.ok = true
		nvmlLib = n
	})
	if !nvmlLib.ok {
		return nil
	}
	return &nvmlLib
}

type nvmlUtilization struct {
	Gpu    uint32
	Memory uint32
}

// nvmlProbe возвращает загрузку и температуру первого GPU.
// ok=false — NVML недоступна (нет NVIDIA или драйвера).
func nvmlProbe() (util, temp int, ok bool) {
	n := loadNVML()
	if n == nil {
		return -1, -1, false
	}
	var count uint32
	if r, _, _ := n.count.Call(uintptr(unsafe.Pointer(&count))); uint32(r) != 0 || count == 0 {
		return -1, -1, false
	}
	var device uintptr
	if r, _, _ := n.handle.Call(0, uintptr(unsafe.Pointer(&device))); uint32(r) != 0 || device == 0 {
		return -1, -1, false
	}
	util, temp = -1, -1
	var u nvmlUtilization
	if r, _, _ := n.util.Call(device, uintptr(unsafe.Pointer(&u))); uint32(r) == 0 {
		util = int(u.Gpu)
	}
	var t uint32
	// NVML_TEMPERATURE_GPU = 0
	if r, _, _ := n.temp.Call(device, 0, uintptr(unsafe.Pointer(&t))); uint32(r) == 0 {
		temp = int(t)
	}
	return util, temp, true
}
