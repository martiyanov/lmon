package meter

import "testing"

func TestFanPercentRange(t *testing.T) {
	v := FanPercent()
	if v < -1 || v > 100 {
		t.Fatalf("FanPercent = %d, want -1..100", v)
	}
}
