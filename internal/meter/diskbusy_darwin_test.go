//go:build darwin

package meter

import "testing"

func TestParseIOBlockStorageTotal(t *testing.T) {
	out := `+-o IOBlockStorageDriver
  "Statistics" = {"Operations (Write)"=10,"Total Time (Read)"=1500,"Total Time (Write)"=2500,"Bytes (Read)"=1}
+-o IOBlockStorageDriver
  "Statistics" = {"Operations (Write)"=1,"Total Time (Read)"=1000,"Total Time (Write)"=500,"Bytes (Read)"=2}
`
	total, ok := parseIOBlockStorageTotal(out)
	if !ok || total != 5500 {
		t.Fatalf("parse = %d, %v; want 5500, true", total, ok)
	}
	if _, ok := parseIOBlockStorageTotal("no statistics here"); ok {
		t.Error("ожидался ok=false без Total Time")
	}
}
