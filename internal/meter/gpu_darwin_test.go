//go:build darwin

package meter

import "testing"

func TestParseGPUUtil(t *testing.T) {
	sample := `+-o IOAccelerator  <class IOAccelerator, id 0x1000002d3, registered, matched, active, busy 0 (0 ms), retain 10>
    "PerformanceStatistics"={"Device Utilization %"=37,"Model":"Apple M2","In Use":"No"}
+-o IOAccelerator  <class IOAccelerator, ...>
    "PerformanceStatistics"={"Device Utilization %"=64}`
	if got := parseGPUUtil(sample); got != 64 {
		t.Errorf("parseGPUUtil = %d, want 64 (максимум из ускорителей)", got)
	}
	if got := parseGPUUtil("no statistics here"); got != -1 {
		t.Errorf("parseGPUUtil без данных = %d, want -1", got)
	}
}

func TestGPUPercentLive(t *testing.T) {
	if v := GPUPercent(); v != -1 && (v < 0 || v > 100) {
		t.Errorf("GPUPercent = %d", v)
	}
	if GPUTempC() != -1 {
		t.Error("GPUTempC на macOS обязан быть -1 (только powermetrics под sudo)")
	}
}
