package meter

import (
	"context"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// CmdTemp — opt-in источник температуры через внешнюю команду: на Windows
// нет официального API температуры CPU, но пользователь может взять её из
// LibreHardwareMonitor/HWiNFO/ACPI и отдать lmon числом. Команда
// выполняется не чаще interval, результат кэшируется.
type CmdTemp struct {
	spec     string
	interval time.Duration

	mu   sync.Mutex
	last time.Time
	val  int
}

// numRe — первое число в выводе команды, с точкой или запятой.
var numRe = regexp.MustCompile(`-?[0-9]+(?:[.,][0-9]+)?`)

// NewCmdTemp создаёт источник; spec выполняется через `cmd /c` на Windows
// и `/bin/sh -c` на остальных ОС (можно использовать pipe/фильтры).
func NewCmdTemp(spec string, interval time.Duration) *CmdTemp {
	if interval <= 0 {
		interval = 2 * time.Second
	}
	return &CmdTemp{spec: spec, interval: interval}
}

// TempC возвращает температуру из команды или -1.
func (c *CmdTemp) TempC() int {
	if c == nil || strings.TrimSpace(c.spec) == "" {
		return -1
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.last.IsZero() && time.Since(c.last) < c.interval {
		return c.val
	}
	c.last = time.Now()
	c.val = runTempCmd(c.spec)
	return c.val
}

func runTempCmd(spec string) int {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/c", spec)
	} else {
		cmd = exec.CommandContext(ctx, "/bin/sh", "-c", spec)
	}
	out, err := cmd.Output()
	if err != nil {
		return -1
	}
	m := numRe.FindString(string(out))
	if m == "" {
		return -1
	}
	v, err := strconv.ParseFloat(strings.ReplaceAll(m, ",", "."), 64)
	if err != nil {
		return -1
	}
	n := int(v)             // отбрасываем дробную часть, как у TempC
	if n < -50 || n > 200 { // защита от «команда вернула мусор»
		return -1
	}
	return n
}
