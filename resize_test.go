package main

import "testing"

func TestGridColumns(t *testing.T) {
	rs := []reading{
		{id: "N", head: "NET"}, {id: "G", head: "GTW"}, {id: "C", head: "CPU"},
		{id: "L", head: "RUN"}, {id: "M", head: "MEM"}, {id: "T", head: " C°"},
		{id: "U", head: "DSK"}, {id: "S", head: "SWP"},
	}
	cases := []struct{ cols, want int }{
		{100, 8}, {80, 8}, {74, 8}, {64, 8}, {63, 8}, {56, 8},
		{55, 8}, {47, 8}, {30, 8}, {24, 8}, {23, 8}, {21, 8},
		{20, 8}, {15, 8}, {5, 5},
	}
	for _, c := range cases {
		if got := gridColumns(c.cols, rs); got != c.want {
			t.Errorf("gridColumns(%d) = %d, want %d", c.cols, got, c.want)
		}
	}
}

func TestResizeLabelsFit(t *testing.T) {
	n := reading{id: "N", head: "NET"}
	if got := resizeLabelFor(n, 6); got != "NET" {
		t.Fatalf("narrow label = %q, want NET", got)
	}
	if got := resizeLabelFor(n, 8); got != "Network" {
		t.Fatalf("wide label = %q, want Network", got)
	}
}
