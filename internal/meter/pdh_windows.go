//go:build windows

package meter

import (
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Минимальная обёртка над PDH (Performance Data Helper): счётчики читаются
// без прав администратора. Для русской Windows используем
// PdhAddEnglishCounterW — имена счётчиков в системе локализованы, а
// английские PDH сопоставляет сам.

var (
	pdhDLL                           = windows.NewLazySystemDLL("pdh.dll")
	procPdhOpenQueryW                = pdhDLL.NewProc("PdhOpenQueryW")
	procPdhAddEnglishCounterW        = pdhDLL.NewProc("PdhAddEnglishCounterW")
	procPdhCollectQueryData          = pdhDLL.NewProc("PdhCollectQueryData")
	procPdhGetFormattedCounterValue  = pdhDLL.NewProc("PdhGetFormattedCounterValue")
	procPdhGetFormattedCounterArrayW = pdhDLL.NewProc("PdhGetFormattedCounterArrayW")
	procPdhCloseQuery                = pdhDLL.NewProc("PdhCloseQuery")
)

const (
	pdhFmtDouble = 0x00000200
	pdhMoreData  = 0x800007D2
)

// pdhFmtCounterValue повторяет PDH_FMT_COUNTERVALUE: DWORD + union (8 байт).
type pdhFmtCounterValue struct {
	CStatus     uint32
	DoubleValue float64
}

// pdhFmtCounterValueItem повторяет PDH_FMT_COUNTERVALUE_ITEM_W.
type pdhFmtCounterValueItem struct {
	szName   *uint16
	FmtValue pdhFmtCounterValue
}

// pdhCounter — лениво инициализируемый счётчик: путь задаётся английским
// именем, PDH сам сопоставляет его с локализованным.
type pdhCounter struct {
	path string

	once    sync.Once
	query   uintptr
	counter uintptr
	ok      bool
}

func newPDHCounter(path string) *pdhCounter { return &pdhCounter{path: path} }

func (c *pdhCounter) init() {
	pathPtr, err := windows.UTF16PtrFromString(c.path)
	if err != nil {
		return
	}
	var query, counter uintptr
	if r, _, _ := procPdhOpenQueryW.Call(0, 0, uintptr(unsafe.Pointer(&query))); r != 0 {
		return
	}
	r, _, _ := procPdhAddEnglishCounterW.Call(query,
		uintptr(unsafe.Pointer(pathPtr)), 0, uintptr(unsafe.Pointer(&counter)))
	if r != 0 {
		_, _, _ = procPdhCloseQuery.Call(query)
		return
	}
	c.query, c.counter, c.ok = query, counter, true
}

func (c *pdhCounter) collect() bool {
	c.once.Do(c.init)
	if !c.ok {
		return false
	}
	r, _, _ := procPdhCollectQueryData.Call(c.query)
	return r == 0
}

// value — одно значение счётчика.
func (c *pdhCounter) value() (float64, bool) {
	if !c.collect() {
		return 0, false
	}
	var v pdhFmtCounterValue
	r, _, _ := procPdhGetFormattedCounterValue.Call(c.counter, pdhFmtDouble,
		0, uintptr(unsafe.Pointer(&v)))
	if r != 0 || v.CStatus != 0 {
		return 0, false
	}
	return v.DoubleValue, true
}

// values — значения счётчика с wildcard-инстансами (например,
// «\GPU Engine(*)\Utilization Percentage»).
func (c *pdhCounter) values() ([]float64, bool) {
	if !c.collect() {
		return nil, false
	}
	var size, count uint32
	r, _, _ := procPdhGetFormattedCounterArrayW.Call(c.counter, pdhFmtDouble,
		uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&count)), 0)
	if uint32(r) != pdhMoreData || size == 0 {
		return nil, false
	}
	buf := make([]byte, size)
	r, _, _ = procPdhGetFormattedCounterArrayW.Call(c.counter, pdhFmtDouble,
		uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&count)),
		uintptr(unsafe.Pointer(&buf[0])))
	if r != 0 || count == 0 {
		return nil, false
	}
	items := unsafe.Slice((*pdhFmtCounterValueItem)(unsafe.Pointer(&buf[0])), int(count))
	out := make([]float64, 0, len(items))
	for _, it := range items {
		if it.FmtValue.CStatus == 0 {
			out = append(out, it.FmtValue.DoubleValue)
		}
	}
	return out, len(out) > 0
}
