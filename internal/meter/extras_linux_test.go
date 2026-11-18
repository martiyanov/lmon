//go:build linux

package meter

import "testing"

func TestParseWear(t *testing.T) {
	cases := map[string]int{
		"Percentage Used: 3%":                3,
		"percentage_used : 12%":              12,
		"Percentage Used:                0%": 0,
		"no wear here":                       -1,
	}
	for in, want := range cases {
		if got := parseWear(in); got != want {
			t.Errorf("parseWear(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestIsPermDenied(t *testing.T) {
	for _, s := range []string{"Permission denied", "requires root", "must be root"} {
		if !isPermDenied(s) {
			t.Errorf("isPermDenied(%q) = false", s)
		}
	}
	if isPermDenied("all good") {
		t.Error("isPermDenied(all good) = true")
	}
}
