package main

import "testing"

func TestRateText(t *testing.T) {
	cases := map[int]string{
		0: "  0", 52: " 52", 999: "999",
		1000: "  1M", 1500: "  1M", 25000: " 25M",
		999999: "999M", 1000000: "  1G", 1500000: "  1G",
		999999999: "999G", 1000000000: "+++",
	}
	for v, want := range cases {
		if got := rateText(v); got != want {
			t.Errorf("rateText(%d) = %q, want %q", v, got, want)
		}
	}
}

func TestDiskAndIOPSFormat(t *testing.T) {
	cases := map[int]string{0: "  0", 999: "999", 1000: "  1G", 2500: "  2G", 1000000: "  1T", 1000000000: "+++"}
	for v, want := range cases {
		if got := diskRateText(v); got != want {
			t.Errorf("diskRateText(%d) = %q, want %q", v, got, want)
		}
	}
	iops := map[int]string{0: "  0", 999: "999", 1000: "  1K", 25000: " 25K", 1000000: "  1G", 1000000000: "+++"}
	for v, want := range iops {
		if got := iopsText(v); got != want {
			t.Errorf("iopsText(%d) = %q, want %q", v, got, want)
		}
	}
}
