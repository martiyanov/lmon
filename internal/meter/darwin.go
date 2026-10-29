//go:build darwin

package meter

import (
	"context"
	"encoding/binary"
	"errors"
	"net/netip"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

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
	"compressed":  regexp.MustCompile(`Pages compressed:\s+(\d+)`),             // старые macOS
	"compressor":  regexp.MustCompile(`Pages occupied by compressor:\s+(\d+)`), // современные macOS
}

// MemPercent возвращает долю занятой памяти в процентах (парсинг vm_stat,
// та же формула, что в zsh-версии: занято = active + wired + compressed).
func MemPercent() int {
	out, err := runCmd("vm_stat")
	if err != nil {
		return -1
	}
	pages := parseVMStat(string(out))
	// compressor (новые macOS) = те же сжатые занятые страницы, что
	// «Pages compressed» в старых: считаем занятыми и включаем в общие
	if pages["compressed"] == 0 {
		pages["compressed"] = pages["compressor"]
	}
	total := pages["free"] + pages["active"] + pages["inactive"] +
		pages["speculative"] + pages["wired"] + pages["compressed"]
	if total == 0 {
		return -1
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
	if isV6(target) {
		return gateway6()
	}
	if gw, err := routeGetDefault(); err == nil {
		return gw, nil
	}
	gw, iface := netstatDefault()
	if gw != "" {
		return gw, nil
	}
	if iface != "" { // default через интерфейс без IP-шлюза (VPN/utun) — берём DHCP-шлюз интерфейса
		if gw, err := dhcpRouter(iface); err == nil {
			return gw, nil
		}
	}
	return "", errors.New("gateway: не найден")
}

func isV6(host string) bool {
	a, err := netip.ParseAddr(host)
	return err == nil && a.Is6() && !a.Is4In6()
}

// gateway6 ищет IPv6-шлюз: route get -inet6, затем netstat -f inet6
// (неполные зоны вроде fe80::%utun0 пропускаются). DHCP-fallback
// не применяется: в IPv6 адрес приносит RA, а не DHCP.
func gateway6() (string, error) {
	if out, err := runCmd("route", "-n", "get", "-inet6", "default"); err == nil {
		if gw, err := parseRouteGateway(string(out)); err == nil {
			if isV6(gw) {
				return gw, nil
			}
		}
	}
	if out, err := runCmd("netstat", "-rn", "-f", "inet6"); err == nil {
		if gw := parseNetstat6(string(out)); gw != "" {
			return gw, nil
		}
	}
	return "", errors.New("gateway: IPv6-шлюз не найден")
}

// parseNetstat6 берёт первый default-маршрут с полным IPv6-шлюзом.
// Строки вида «fe80::%utun0» — мусор без идентификатора интерфейса
// в самом адресе (заканчивается на «::») — пропускаются.
func parseNetstat6(out string) string {
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) < 4 || f[0] != "default" {
			continue
		}
		addr := f[1]
		base := addr
		if i := strings.IndexByte(addr, '%'); i > 0 {
			base = addr[:i]
		}
		if strings.HasSuffix(base, "::") {
			continue // неполный link-local без идентификатора
		}
		if a, err := netip.ParseAddr(addr); err == nil && a.Is6() {
			return addr
		}
	}
	return ""
}

func routeGetDefault() (string, error) {
	out, err := runCmd("route", "-n", "get", "default")
	if err != nil {
		return "", err
	}
	return parseRouteGateway(string(out))
}

// netstatDefault возвращает (IP-шлюз, интерфейс) по выводу netstat -rn.
// Шлюз — первый default-маршрут с настоящим IPv4 next-hop (флаги перед
// интерфейсом не являются адресом); интерфейс — последний столбец первой
// default-строки: он нужен для DHCP-fallback, когда IP-шлюза нет (VPN).
func netstatDefault() (gw, iface string) {
	out, err := runCmd("netstat", "-rn", "-f", "inet")
	if err != nil {
		return "", ""
	}
	return parseNetstat(string(out))
}

func parseNetstat(out string) (gw, iface string) {
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) < 4 || f[0] != "default" {
			continue
		}
		if iface == "" {
			iface = f[len(f)-1]
		}
		if ip, err := netip.ParseAddr(f[1]); err == nil && ip.Is4() {
			return f[1], iface
		}
	}
	return "", iface
}

func dhcpRouter(iface string) (string, error) {
	out, err := runCmd("ipconfig", "getoption", iface, "router")
	if err != nil {
		return "", err
	}
	gw := strings.TrimSpace(string(out))
	if gw == "" || !strings.Contains(gw, ".") {
		return "", errors.New("gateway: DHCP router пуст")
	}
	return gw, nil
}

// runCmd выполняет системную команду с таймаутом, чтобы зависший процесс
// не останавливал цикл монитора.
func runCmd(name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, name, args...).Output()
}

func parseRouteGateway(out string) (string, error) {
	// строка вида "    gateway: 192.168.1.1" или "gateway: fe80::1%en0"
	m := regexp.MustCompile(`(?m)^\s*gateway:\s+(\S+)`).FindStringSubmatch(out)
	if m == nil {
		return "", errors.New("gateway: маршрут по умолчанию не найден")
	}
	if _, err := netip.ParseAddr(m[1]); err != nil && !isV6(m[1]) {
		return "", errors.New("gateway: не IP-адрес")
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
	out, err := runCmd(path, "-c")
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
