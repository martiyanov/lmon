package main

import "testing"

func TestGridColumns(t *testing.T) {
	cases := []struct{ cols, n, want int }{
		{100, 8, 8}, {60, 8, 6}, {40, 8, 4}, {30, 8, 3},
		{20, 8, 2}, {12, 8, 1}, {5, 8, 1}, {100, 2, 2},
	}
	for _, c := range cases {
		if got := gridColumns(c.cols, c.n); got != c.want {
			t.Errorf("gridColumns(%d,%d) = %d, want %d", c.cols, c.n, got, c.want)
		}
	}
}
