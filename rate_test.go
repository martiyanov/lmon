package main

import "testing"

func TestRateText(t *testing.T) {
	cases := map[int]string{
		0: "  0", 52: " 52", 999: "999",
		1000: " 1M", 1500: " 1M", 25000: "25M",
		99999: "99M", 100000: ".1G", 150000: ".2G",
		999999: " 1G", 1000000: " 1G", 1500000: " 1G",
		10000000: "10G", 999999999: " 1T", 1000000000: "+++",
	}
	for v, want := range cases {
		got := rateText(v)
		if got != want {
			t.Errorf("rateText(%d) = %q, want %q", v, got, want)
		}
		if len([]rune(got)) != 3 {
			t.Errorf("rateText(%d) width = %d, want 3 (%q)", v, len([]rune(got)), got)
		}
	}
}

func TestDiskAndIOPSFormat(t *testing.T) {
	cases := map[int]string{0: "  0", 999: "999", 1000: " 1G", 2500: " 2G", 250000: ".3T", 1000000: " 1T", 1000000000: "+++"}
	for v, want := range cases {
		if got := diskRateText(v); got != want {
			t.Errorf("diskRateText(%d) = %q, want %q", v, got, want)
		}
	}
	iops := map[int]string{0: "  0", 999: "999", 1000: " 1K", 25000: "25K", 250000: ".3M", 1000000: " 1M", 1000000000: "+++"}
	for v, want := range iops {
		if got := iopsText(v); got != want {
			t.Errorf("iopsText(%d) = %q, want %q", v, got, want)
		}
	}
}

func TestRateColumnsStayAligned(t *testing.T) {
	u := &ui{}
	rs := []reading{
		{id: "R", head: " RX", v: 1500},
		{id: "J", head: " TX", v: 1_000_000},
	}
	row := u.dataRow(rs)
	if row != " 1M  1G" {
		t.Fatalf("rate row = %q, want %q", row, " 1M  1G")
	}
	if len([]rune(row)) != 7 {
		t.Fatalf("row width = %d, want 7", len([]rune(row)))
	}
}
