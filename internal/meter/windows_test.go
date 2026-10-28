//go:build windows

package meter

import "testing"

func TestCPUSample(t *testing.T) {
	if c, i := CPUSample(); c != -1 || i != -1 {
		t.Errorf("первый вызов = (%d, %d), want (-1, -1)", c, i)
	}
	if _, i := CPUSample(); i != -1 {
		t.Errorf("iowait = %d, want -1", i)
	}
}

func TestSensors(t *testing.T) {
	if m := MemPercent(); m < 0 || m > 100 {
		t.Errorf("MemPercent = %d", m)
	}
	if s := SwapPercent(); s != -1 {
		t.Errorf("SwapPercent = %d, want -1 (реальный swap-счётчик недоступен)", s)
	}
	if b := BatteryPercent(); b != -1 && (b < 0 || b > 100) {
		t.Errorf("BatteryPercent = %d", b)
	}
	if d := DiskPercent(); d < 0 || d > 100 {
		t.Errorf("DiskPercent = %d", d)
	}
	if TempC() != -1 {
		t.Error("TempC: на Windows ожидается -1")
	}
	if _, err := LoadAvg(); err == nil {
		t.Error("LoadAvg: на Windows ожидается ошибка")
	}
}
