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
	total, available, ok := parseMemInfo(sample)
	if !ok || total != 16384000 || available != 8000000 {
		t.Fatalf("parseMemInfo = (%d, %d, %v), want (16384000, 8000000, true)", total, available, ok)
	}
	if _, _, ok := parseMemInfo("MemTotal: 100 kB\n"); ok {
		t.Error("parseMemInfo: без MemAvailable ожидался ok=false")
	}
	if got := int((total - available) * 100 / total); got != 51 {
		t.Errorf("MemPercent = %d, want 51", got)
	}
}

func TestParseProcRoute(t *testing.T) {
	sample := `Iface	Destination	Gateway 	Flags	RefCnt	Use	Metric	Mask		MTU	Window	IRTT
wlp3s0	00000000	0101A8C0	0003	0	0	600	00000000	0	0	0
wlp3s0	0001A8C0	00000000	0001	0	0	600	00FFFFFF	0	0	0
docker0	000011AC	00000000	0001	0	0	0	00FFFFFF	0	0	0
`
	gw, err := parseProcRoute(sample)
	if err != nil {
		t.Fatal(err)
	}
	if gw != "192.168.1.1" {
		t.Errorf("parseProcRoute = %q, want 192.168.1.1", gw)
	}
	if _, err := parseProcRoute("Iface\tDestination\tGateway\tFlags\n"); err == nil {
		t.Error("parseProcRoute: ожидалась ошибка при отсутствии маршрута")
	}
}

func TestVia6ZoneAppend(t *testing.T) {
	// ip -6 route get 2001:db8::1 → via fe80::1 dev eth0 → зона из dev
	m := via6Re.FindStringSubmatch("2001:db8::1 from :: via fe80::1 dev eth0 proto kernel src 2001:db8::a:1 metric 1024")
	if m == nil {
		t.Fatal("via6Re не нашёл via/dev")
	}
	if got := m[1] + "%" + m[2]; got != "fe80::1%eth0" {
		t.Errorf("шлюз = %q, want fe80::1%%eth0", got)
	}
}
