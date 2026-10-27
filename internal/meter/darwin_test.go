//go:build darwin

package meter

import "testing"

const vmStatSample = `Mach Virtual Memory Statistics: (page size of 16384 bytes)
Pages free:                               123456.
Pages speculative:                        10000.
Pages throttled:                          0.
Pages wired down:                         200000.
Pages purgeable:                          500.
Pages occupied by compressor:             60000.
Pages anonymous:                          300000
Pages stored in compressor:               30000.
Pages inactive:                           150000.
Pages active:                             400000.
Pages swapped into pages:                 0.
Pages swapped out of pages:               0.
`

func TestParseVMStat(t *testing.T) {
	pages := parseVMStat(vmStatSample)
	want := map[string]int64{
		"free":        123456,
		"speculative": 10000,
		"wired":       200000,
		"inactive":    150000,
		"active":      400000,
		"compressed":  0, // на современных macOS строки Pages compressed нет
	}
	for name, w := range want {
		if pages[name] != w {
			t.Errorf("%s = %d, want %d", name, pages[name], w)
		}
	}

	total := pages["free"] + pages["active"] + pages["inactive"] +
		pages["speculative"] + pages["wired"] + pages["compressed"]
	used := pages["active"] + pages["wired"] + pages["compressed"]
	if got := int(used * 100 / total); got != 67 {
		t.Errorf("MemPercent = %d, want 67", got)
	}
}

func TestParseVMStatOldFormat(t *testing.T) {
	old := "Pages free: 100.\nPages active: 200.\nPages compressed: 100.\n"
	pages := parseVMStat(old)
	if pages["compressed"] != 100 {
		t.Errorf("compressed = %d, want 100", pages["compressed"])
	}
}

func TestParseRouteGateway(t *testing.T) {
	out := `   route to: default
destination: default
       mask: default
    gateway: 192.168.1.1
  interface: en0
      flags: <UP,GATEWAY,DONE,STATIC>`
	gw, err := parseRouteGateway(out)
	if err != nil {
		t.Fatal(err)
	}
	if gw != "192.168.1.1" {
		t.Errorf("parseRouteGateway = %q, want 192.168.1.1", gw)
	}
	if _, err := parseRouteGateway("route: writing to routing socket: not in table\n"); err == nil {
		t.Error("parseRouteGateway: ожидалась ошибка при отсутствии маршрута")
	}
}
