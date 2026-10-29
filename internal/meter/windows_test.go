//go:build windows

package meter

import (
	"strconv"
	"strings"
	"testing"
)

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

const routePrint6Sample = `===========================================================================
Active Routes:
  If Metric Network Destination      Gateway        Interface Metric
          1    306 ::/0                 fe80::1%12     ...        306
          1    306 ::/0                 fe80::9%7      ...        500
===========================================================================`

func TestGateway6Windows(t *testing.T) {
	best, bestMetric := "", -1
	for _, m := range route6Re.FindAllStringSubmatch(routePrint6Sample, -1) {
		metric, _ := strconv.Atoi(strings.TrimSpace(m[2]))
		if bestMetric < 0 || metric < bestMetric {
			best, bestMetric = m[1], metric
		}
	}
	if best != "fe80::1%12" {
		t.Errorf("лучший v6-шлюз = %q, want fe80::1%%12 (минимальная метрика)", best)
	}
}
