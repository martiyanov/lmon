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
		pages["speculative"] + pages["wired"] + pages["compressor"]
	used := pages["active"] + pages["wired"] + pages["compressor"]
	if got := int(used * 100 / total); got != 69 {
		t.Errorf("MemPercent = %d, want 69", got)
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

const netstatSample = `Routing tables

Internet:
Destination        Gateway            Flags               Netif Expire
default            link#16            UCSI                utun8
default            192.168.1.1        UGScg                 en0
127                127.0.0.1          UCS                    lo0
`

func TestNetstatDefaultVPN(t *testing.T) {
	// default через utun без IP-шлюза + обычный через en0:
	// берём IP-шлюз, интерфейс — последний столбец строки
	gw, iface := parseNetstat(netstatSample)
	if gw != "192.168.1.1" {
		t.Errorf("netstatDefault gw = %q, want 192.168.1.1", gw)
	}
	if iface != "utun8" && iface != "en0" {
		t.Errorf("netstatDefault iface = %q", iface)
	}

	only := `Internet:
Destination        Gateway            Flags               Netif Expire
default            link#16            UCSI                utun8
`
	gw2, iface2 := parseNetstat(only)
	if gw2 != "" {
		t.Errorf("только utun: gw = %q, want пусто", gw2)
	}
	if iface2 != "utun8" {
		t.Errorf("только utun: iface = %q, want utun8", iface2)
	}
}

const vmStatNew = `Mach Virtual Memory Statistics: (page size of 16384 bytes)
Pages free:                               100000.
Pages speculative:                        10000.
Pages wired down:                         200000.
Pages occupied by compressor:             60000.
Pages inactive:                           150000.
Pages active:                             400000.
`

func TestParseVMStatCompressor(t *testing.T) {
	// compressor новых macOS учитывается как сжатые занятые страницы
	pages := parseVMStat(vmStatNew)
	if pages["compressor"] != 60000 {
		t.Fatalf("compressor = %d, want 60000", pages["compressor"])
	}
	total := pages["free"] + pages["active"] + pages["inactive"] +
		pages["speculative"] + pages["wired"] + pages["compressor"]
	used := pages["active"] + pages["wired"] + pages["compressor"]
	if got := int(used * 100 / total); got != 71 {
		t.Errorf("MEM с compressor = %d, want 71", got)
	}
}

const netstat6Sample = `Routing tables

Internet6:
Destination                             Gateway                         Flags               Netif Expire
default                                 fe80::%utun0                     UGcIg               utun0
default                                 fe80::1%en0                      UGcg                  en0
::1                                     ::1                             UHL                   lo0
`

func TestParseNetstat6(t *testing.T) {
	// неполный fe80::%utun0 (без идентификатора в адресе) пропускается
	if gw := parseNetstat6(netstat6Sample); gw != "fe80::1%en0" {
		t.Errorf("parseNetstat6 = %q, want fe80::1%%en0", gw)
	}
	if gw := parseNetstat6("default                                 fe80::%utun0                            UGcIg               utun0"); gw != "" {
		t.Errorf("parseNetstat6 с одним мусорным = %q, want пусто", gw)
	}
}

func TestParseRouteGatewayV6(t *testing.T) {
	out := `   route to: default
destination: default
       mask: default
    gateway: fe80::1%en0
  interface: en0`
	gw, err := parseRouteGateway(out)
	if err != nil || gw != "fe80::1%en0" {
		t.Errorf("parseRouteGateway v6 = (%q, %v)", gw, err)
	}
}
