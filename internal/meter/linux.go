//go:build linux

package meter

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// LoadAvg возвращает load average за минуту из /proc/loadavg.
func LoadAvg() (float64, error) {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return 0, err
	}
	return parseLoadAvg(string(data))
}

func parseLoadAvg(s string) (float64, error) {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return 0, errors.New("loadavg: пустой файл")
	}
	return strconv.ParseFloat(fields[0], 64)
}

// MemPercent возвращает долю занятой памяти в процентах: занято всё,
// что не попало в MemAvailable (то же определение, что у free(1)).
func MemPercent() int {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return -1
	}
	total, available, ok := parseMemInfo(string(data))
	if !ok || total <= 0 || available < 0 || available > total {
		return -1 // ошибка чтения/полей — «·», не ноль
	}
	return int((total - available) * 100 / total)
}

func parseMemInfo(s string) (total, available int64, ok bool) {
	var haveTotal, haveAvail bool
	for _, line := range strings.Split(s, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		v, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			continue
		}
		switch fields[0] {
		case "MemTotal:":
			total, haveTotal = v, true
		case "MemAvailable:":
			available, haveAvail = v, true
		}
	}
	return total, available, haveTotal && haveAvail
}

// DefaultGateway ищет шлюз по умолчанию. Цепочка:
//  1. ip -4 route get <target> — маршрут, который ядро выбрало для цели;
//  2. ip -4 route show default;
//  3. /proc/net/route (учитывается флаг шлюза RTF_GATEWAY).
func DefaultGateway(target string) (string, error) {
	if gw, err := ipRouteGet(target); err == nil {
		return gw, nil
	}
	if gw, err := ipRouteDefault(); err == nil {
		return gw, nil
	}
	if gw, err := procRouteDefault(); err == nil {
		return gw, nil
	}
	return "", errors.New("gateway: не найден")
}

var viaRe = regexp.MustCompile(`\bvia\s+(\d{1,3}(?:\.\d{1,3}){3})\b`)

// runCmd выполняет системную команду с таймаутом, чтобы зависший процесс
// не останавливал цикл монитора.
func runCmd(name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, name, args...).Output()
}

func ipRouteGet(target string) (string, error) {
	out, err := runCmd("ip", "-4", "route", "get", target)
	if err != nil {
		return "", err
	}
	if m := viaRe.FindStringSubmatch(string(out)); m != nil {
		return m[1], nil
	}
	return "", errors.New("gateway: прямой маршрут без шлюза")
}

func ipRouteDefault() (string, error) {
	out, err := runCmd("ip", "-4", "route", "show", "default")
	if err != nil {
		return "", err
	}
	if m := viaRe.FindStringSubmatch(string(out)); m != nil {
		return m[1], nil
	}
	return "", errors.New("gateway: default без шлюза")
}

func procRouteDefault() (string, error) {
	data, err := os.ReadFile("/proc/net/route")
	if err != nil {
		return "", err
	}
	return parseProcRoute(string(data))
}

func parseProcRoute(s string) (string, error) {
	for _, line := range strings.Split(s, "\n")[1:] {
		f := strings.Fields(line)
		if len(f) < 4 || f[1] != "00000000" {
			continue
		}
		flags, err := strconv.ParseUint(f[3], 16, 32)
		if err != nil || flags&0x2 == 0 { // RTF_GATEWAY
			continue
		}
		b, err := hex.DecodeString(f[2])
		if err != nil || len(b) != 4 {
			continue
		}
		return fmt.Sprintf("%d.%d.%d.%d", b[3], b[2], b[1], b[0]), nil // little-endian
	}
	return "", errors.New("gateway: маршрут по умолчанию не найден")
}

// TempC возвращает температуру CPU из sysfs: сначала сенсоры hwmon
// (coretemp/k10temp/...), затем thermal_zone с CPU-типом; 0, если сенсоров нет.
func TempC() int {
	if t, ok := tempFromHwmon(); ok {
		return t
	}
	if t, ok := tempFromThermalZones(); ok {
		return t
	}
	return -1 // датчика нет — не выдумываем ноль
}

func tempFromHwmon() (int, bool) {
	hwmons, _ := filepath.Glob("/sys/class/hwmon/hwmon*")
	for _, h := range hwmons {
		name, _ := os.ReadFile(filepath.Join(h, "name"))
		switch strings.TrimSpace(string(name)) {
		case "coretemp", "k10temp", "k8temp", "zenpower", "cpu_thermal":
		default:
			continue
		}
		inputs, _ := filepath.Glob(filepath.Join(h, "temp*_input"))
		for _, in := range inputs {
			if t, ok := readMilliDegrees(in); ok {
				return t, true
			}
		}
	}
	return 0, false
}

func tempFromThermalZones() (int, bool) {
	zones, _ := filepath.Glob("/sys/class/thermal/thermal_zone*")
	for _, z := range zones {
		typ, _ := os.ReadFile(filepath.Join(z, "type"))
		t := strings.ToLower(string(typ))
		if !strings.Contains(t, "pkg") && !strings.Contains(t, "cpu") && !strings.Contains(t, "x86") {
			continue
		}
		if v, ok := readMilliDegrees(filepath.Join(z, "temp")); ok {
			return v, true
		}
	}
	// зоны с CPU-меткой нет — честно «нет данных»: первая попавшаяся
	// зона может измерять что угодно (аккумулятор, NVMe, кожух)
	return 0, false
}

func readMilliDegrees(path string) (int, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	milli, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
	if err != nil || milli <= 0 || milli > 150000 {
		return 0, false
	}
	return int(milli / 1000), true
}
