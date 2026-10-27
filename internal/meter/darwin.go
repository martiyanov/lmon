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

// TempC возвращает температуру CPU. На macOS нужна внешняя утилита smctemp
// (https://github.com/holtmann/osx/tree/master/smctemp); без неё — 0.
func TempC() int {
	path, err := exec.LookPath("smctemp")
	if err != nil {
		return 0
	}
	out, err := exec.Command(path, "-c").Output()
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(out))
	if len(fields) == 0 {
		return 0
	}
	t, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0
	}
	return int(t)
}
