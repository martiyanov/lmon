//go:build darwin

package meter

import (
	"encoding/binary"
	"errors"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

// LoadAvg возвращает load average за минуту (sysctl vm.loadavg).
func LoadAvg() (float64, error) {
	raw, err := unix.SysctlRaw("vm.loadavg")
	if err != nil {
		return 0, err
	}
	if len(raw) < 24 {
		return 0, errors.New("vm.loadavg: короткий ответ")
	}
	// struct loadavg { fixpt_t ldavg[3]; long fscale; }, fixpt_t = uint32
	ldavg := binary.LittleEndian.Uint32(raw[0:4])
	fscale := binary.LittleEndian.Uint64(raw[16:24])
	if fscale == 0 {
		return 0, errors.New("vm.loadavg: fscale = 0")
	}
	return float64(ldavg) / float64(fscale), nil
}

var pageRe = map[string]*regexp.Regexp{
	"free":        regexp.MustCompile(`Pages free:\s+(\d+)`),
	"active":      regexp.MustCompile(`Pages active:\s+(\d+)`),
	"inactive":    regexp.MustCompile(`Pages inactive:\s+(\d+)`),
	"speculative": regexp.MustCompile(`Pages speculative:\s+(\d+)`),
	"wired":       regexp.MustCompile(`Pages wired down:\s+(\d+)`),
	"compressed":  regexp.MustCompile(`Pages compressed:\s+(\d+)`), // старые macOS
}

// MemPercent возвращает долю занятой памяти в процентах (парсинг vm_stat,
// та же формула, что в zsh-версии: занято = active + wired + compressed).
func MemPercent() int {
	out, err := exec.Command("vm_stat").Output()
	if err != nil {
		return 0
	}
	pages := parseVMStat(string(out))
	total := pages["free"] + pages["active"] + pages["inactive"] +
		pages["speculative"] + pages["wired"] + pages["compressed"]
	if total == 0 {
		return 0
	}
	used := pages["active"] + pages["wired"] + pages["compressed"]
	return int(used * 100 / total)
}

func parseVMStat(out string) map[string]int64 {
	pages := make(map[string]int64, len(pageRe))
	for name, re := range pageRe {
		if m := re.FindStringSubmatch(out); m != nil {
			if v, err := strconv.ParseInt(m[1], 10, 64); err == nil {
				pages[name] = v
			}
		}
	}
	return pages
}

// DefaultGateway ищет шлюз по умолчанию. Цепочка (в духе честности:
// лучше вернуть ошибку, чем выдуманный адрес):
//  1. route -n get default — основной путь;
//  2. netstat -rn -f inet — маршрут default через физический интерфейс
//     (актуально при VPN: route get может вернуть utun без IP шлюза);
//  3. ipconfig getoption <iface> router — DHCP-шлюз интерфейса, даже
//     если default-маршрут перекрыт VPN.
func DefaultGateway(target string) (string, error) {
	if gw, err := routeGetDefault(); err == nil {
		return gw, nil
	}
	if gw, iface, err := netstatDefault(); err == nil {
		return gw, nil
	} else if iface != "" {
		if gw, err := dhcpRouter(iface); err == nil {
			return gw, nil
		}
	}
	return "", errors.New("gateway: не найден")
}

func routeGetDefault() (string, error) {
	out, err := exec.Command("route", "-n", "get", "default").Output()
	if err != nil {
		return "", err
	}
	return parseRouteGateway(string(out))
}

var netstatDefaultRe = regexp.MustCompile(`(?m)^default\s+(\d{1,3}(?:\.\d{1,3}){3})\s+(\S+)`)

// netstatDefault возвращает шлюз и интерфейс первого default-маршрута
// с настоящим IP-шлюзом (маршруты utun point-to-point его не имеют).
func netstatDefault() (gw, iface string, err error) {
	out, err := exec.Command("netstat", "-rn", "-f", "inet").Output()
	if err != nil {
		return "", "", err
	}
	m := netstatDefaultRe.FindStringSubmatch(string(out))
	if m == nil {
		return "", "", errors.New("gateway: default-маршрут с IP-шлюзом не найден")
	}
	return m[1], m[2], nil
}

func dhcpRouter(iface string) (string, error) {
	out, err := exec.Command("ipconfig", "getoption", iface, "router").Output()
	if err != nil {
		return "", err
	}
	gw := strings.TrimSpace(string(out))
	if gw == "" || !strings.Contains(gw, ".") {
		return "", errors.New("gateway: DHCP router пуст")
	}
	return gw, nil
}

func parseRouteGateway(out string) (string, error) {
	// строка вида "    gateway: 192.168.1.1"
	m := regexp.MustCompile(`(?m)^\s*gateway:\s+([0-9.]+)`).FindStringSubmatch(out)
	if m == nil {
		return "", errors.New("gateway: маршрут по умолчанию не найден")
	}
	return m[1], nil
}

// TempC возвращает температуру CPU. На macOS нужна внешняя утилита smctemp
// (https://github.com/holtmann/osx/tree/master/smctemp). Нет утилиты или
// данных — честное -1 (колонка покажет «·»), а не выдуманный ноль.
func TempC() int {
	path, err := exec.LookPath("smctemp")
	if err != nil {
		return -1
	}
	out, err := exec.Command(path, "-c").Output()
	if err != nil {
		return -1
	}
	fields := strings.Fields(string(out))
	if len(fields) == 0 {
		return -1
	}
	t, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return -1
	}
	return int(t)
}
