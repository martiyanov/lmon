package main

import "testing"

func TestGridColumns(t *testing.T) {
	cases := []struct{ cols, n, want int }{
		{100, 8, 8}, {80, 8, 8}, {79, 8, 3}, {60, 8, 3}, {40, 8, 3},
		{30, 8, 3}, {29, 8, 1}, {20, 8, 1}, {12, 8, 1}, {5, 8, 1},
		{100, 2, 2},
	}
	for _, c := range cases {
		if got := gridColumns(c.cols, c.n); got != c.want {
			t.Errorf("gridColumns(%d,%d) = %d, want %d", c.cols, c.n, got, c.want)
		}
	}
}

func TestResizeIgnoresRain(t *testing.T) {
	cfg, err := parseConfig([]string{"--resize-window", "--rain", "--hybrid"})
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.resizeWindow || cfg.rain || !cfg.hybrid {
		t.Fatalf("cfg = %+v", cfg)
	}
}
