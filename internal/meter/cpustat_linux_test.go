//go:build linux

package meter

import "testing"

func TestParseProcStat(t *testing.T) {
	sample := `cpu  123456 7890 234567 9876543 45678 1234 5678 901 0 0
cpu0 100 10 200 8000 50 1 2 0 0 0
intr 123456789
`
	busy, idle, iowait, err := parseProcStat(sample)
	if err != nil {
		t.Fatal(err)
	}
	// busy = user+nice+system+irq+softirq+steal = 123456+7890+234567+1234+5678+901
	if busy != 373726 {
		t.Errorf("busy = %d, want 373726", busy)
	}
	if idle != 9876543 || iowait != 45678 {
		t.Errorf("idle/iowait = %d/%d, want 9876543/45678", idle, iowait)
	}
	if _, _, _, err := parseProcStat("no cpu line"); err == nil {
		t.Error("parseProcStat: ожидалась ошибка без строки cpu")
	}
}

func TestParseSwapMemInfo(t *testing.T) {
	sample := `MemTotal:       16384000 kB
SwapTotal:       2097148 kB
SwapFree:        1048574 kB
`
	total, free := parseSwapMemInfo(sample)
	if total != 2097148 || free != 1048574 {
		t.Fatalf("parseSwapMemInfo = (%d, %d), want (2097148, 1048574)", total, free)
	}
	if got := (total - free) * 100 / total; got != 50 {
		t.Errorf("SwapPercent = %d, want 50", got)
	}
}
