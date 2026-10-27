//go:build darwin

package meter

import (
	"encoding/binary"
	"testing"
)

const iostatSample = `              disk0       cpu     load average
    KB/t  tps  MB/s  us sy id   1m   5m   15m
   20.65   64  1.29  11  7 81  2.32 2.42 2.65
   14.36   61  0.85  22  8 70  2.29 2.41 2.64
`

func TestParseIostatCPU(t *testing.T) {
	// вторая строка данных: id = 70 → загрузка 30%
	if got := parseIostatCPU(iostatSample); got != 30 {
		t.Errorf("parseIostatCPU = %d, want 30", got)
	}
	if got := parseIostatCPU("мусор"); got != -1 {
		t.Errorf("parseIostatCPU(мусор) = %d, want -1", got)
	}
}

func TestParseSwapRaw(t *testing.T) {
	// запись 32 байта: total=2GiB, avail, used≈1426MiB → ~69%
	raw := make([]byte, 32)
	binary.LittleEndian.PutUint64(raw[0:], 2147483648)
	binary.LittleEndian.PutUint64(raw[8:], 651411456)
	binary.LittleEndian.PutUint64(raw[16:], 1495511040)
	if got := parseSwapRaw(raw); got != 69 {
		t.Errorf("parseSwapRaw = %d, want 69", got)
	}
	if got := parseSwapRaw([]byte{1, 2, 3}); got != 0 {
		t.Errorf("parseSwapRaw(кривой) = %d, want 0", got)
	}
}

func TestParsePmsetBatt(t *testing.T) {
	out := "Now drawing from 'AC Power'\n -InternalBattery-0 (id=23068771)\t55%; discharging; 2:09 remaining present: true\n"
	if got := parsePmsetBatt(out); got != 55 {
		t.Errorf("parsePmsetBatt = %d, want 55", got)
	}
	if got := parsePmsetBatt("no battery"); got != -1 {
		t.Errorf("parsePmsetBatt(нет батареи) = %d, want -1", got)
	}
}
