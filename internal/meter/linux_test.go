//go:build linux

package meter

import "testing"

func TestParseLoadAvg(t *testing.T) {
	got, err := parseLoadAvg("0.52 0.58 0.57 1/812 2345\n")
	if err != nil {
		t.Fatal(err)
	}
	if got != 0.52 {
		t.Errorf("parseLoadAvg = %v, want 0.52", got)
	}
}

func TestParseMemInfo(t *testing.T) {
	sample := `MemTotal:       16384000 kB
MemFree:        1000000 kB
MemAvailable:   8000000 kB
Buffers:          100000 kB
SwapTotal:             0 kB
`
	total, available := parseMemInfo(sample)
	if total != 16384000 || available != 8000000 {
		t.Fatalf("parseMemInfo = (%d, %d), want (16384000, 8000000)", total, available)
	}
	if got := int((total - available) * 100 / total); got != 51 {
		t.Errorf("MemPercent = %d, want 51", got)
	}
}
