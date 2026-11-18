//go:build linux

package meter

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// GPUPowerW — потребление выбранной NVIDIA GPU, Вт (nvidia-smi).
func GPUPowerW() int {
	out, err := nvidiaSMI("-i", strconv.Itoa(GPUIndex()),
		"--query-gpu=power.draw", "--format=csv,noheader,nounits")
	if err != nil {
		return -1
	}
	f := strings.Fields(string(out))
	if len(f) == 0 {
		return -1
	}
	v, err := strconv.ParseFloat(strings.ReplaceAll(f[0], ",", "."), 64)
	if err != nil || v < 0 || v > 1000 {
		return -1
	}
	return int(v + 0.5)
}

// NVMeTempC — максимальная температура NVMe из hwmon (обычно Composite/Sensor).
func NVMeTempC() int {
	hwmons, _ := filepath.Glob("/sys/class/hwmon/hwmon*")
	best := -1
	for _, h := range hwmons {
		if strings.TrimSpace(readString(filepath.Join(h, "name"))) != "nvme" {
			continue
		}
		inputs, _ := filepath.Glob(filepath.Join(h, "temp*_input"))
		for _, in := range inputs {
			if v, ok := readIntFile(in); ok {
				t := v / 1000
				if t > best {
					best = t
				}
			}
		}
	}
	return best
}

var (
	diskMu       sync.Mutex
	diskLastAt   time.Time
	diskLastIO   uint64
	diskLastTime uint64
)

func readDiskTotals() (ios, tm, inflight uint64) {
	stats, _ := filepath.Glob("/sys/block/*/stat")
	for _, p := range stats {
		dev := filepath.Base(filepath.Dir(p))
		if skipBlockDevice(dev) {
			continue
		}
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		f := strings.Fields(string(data))
		if len(f) < 11 {
			continue
		}
		reads, _ := strconv.ParseUint(f[0], 10, 64)
		writes, _ := strconv.ParseUint(f[4], 10, 64)
		inq, _ := strconv.ParseUint(f[8], 10, 64)
		weighted, _ := strconv.ParseUint(f[10], 10, 64)
		ios += reads + writes
		tm += weighted
		inflight += inq
	}
	return ios, tm, inflight
}

// DiskIO — средняя латентность (await, мс) и текущая очередь I/O по
// физическим дискам. Первый вызов делает короткое окно 150 мс, чтобы
// отдать значение и в режиме --once.
func DiskIO() (awaitMs, queue int) {
	ios, tm, inflight := readDiskTotals()
	queue = int(inflight)
	now := time.Now()
	diskMu.Lock()
	if diskLastAt.IsZero() {
		diskLastAt, diskLastIO, diskLastTime = now, ios, tm
		diskMu.Unlock()
		time.Sleep(150 * time.Millisecond)
		ios, tm, inflight = readDiskTotals()
		queue = int(inflight)
		now = time.Now()
		diskMu.Lock()
	}
	defer diskMu.Unlock()
	dIO := ios - diskLastIO
	dTm := tm - diskLastTime
	dt := now.Sub(diskLastAt).Seconds()
	diskLastAt, diskLastIO, diskLastTime = now, ios, tm
	if dt <= 0 || dIO == 0 {
		return 0, queue
	}
	return int(float64(dTm)/float64(dIO) + 0.5), queue
}

func skipBlockDevice(dev string) bool {
	for _, p := range []string{"loop", "ram", "zram", "sr", "dm-", "md", "fd"} {
		if strings.HasPrefix(dev, p) {
			return true
		}
	}
	return false
}

var (
	netMu     sync.Mutex
	netLastAt time.Time
	netRx     uint64
	netTx     uint64
	netErr    uint64
)

func readNetTotals() (rx, tx, errs uint64) {
	data, err := os.ReadFile("/proc/net/dev")
	if err != nil {
		return 0, 0, 0
	}
	for _, line := range strings.Split(string(data), "\n") {
		i := strings.IndexByte(line, ':')
		if i < 0 {
			continue
		}
		iface := strings.TrimSpace(line[:i])
		if iface == "lo" || iface == "" {
			continue
		}
		f := strings.Fields(line[i+1:])
		if len(f) < 12 {
			continue
		}
		rxb, _ := strconv.ParseUint(f[0], 10, 64)
		rxe, _ := strconv.ParseUint(f[2], 10, 64)
		rxd, _ := strconv.ParseUint(f[3], 10, 64)
		txb, _ := strconv.ParseUint(f[8], 10, 64)
		txe, _ := strconv.ParseUint(f[10], 10, 64)
		txd, _ := strconv.ParseUint(f[11], 10, 64)
		rx += rxb
		tx += txb
		errs += rxe + rxd + txe + txd
	}
	return rx, tx, errs
}

// NetRate — трафик RX/TX в КБ/с и ошибки+дропы в секунду (без lo).
// Первый вызов делает короткое окно 150 мс для --once.
func NetRate() (rxKB, txKB, errPerSec int) {
	rx, tx, errs := readNetTotals()
	now := time.Now()
	netMu.Lock()
	if netLastAt.IsZero() {
		netLastAt, netRx, netTx, netErr = now, rx, tx, errs
		netMu.Unlock()
		time.Sleep(150 * time.Millisecond)
		rx, tx, errs = readNetTotals()
		now = time.Now()
		netMu.Lock()
	}
	defer netMu.Unlock()
	dt := now.Sub(netLastAt).Seconds()
	dRx, dTx, dErr := rx-netRx, tx-netTx, errs-netErr
	netLastAt, netRx, netTx, netErr = now, rx, tx, errs
	if dt <= 0 {
		return 0, 0, 0
	}
	return int(float64(dRx)/1024/dt + 0.5), int(float64(dTx)/1024/dt + 0.5), int(float64(dErr)/dt + 0.5)
}

var psiRe = regexp.MustCompile(`some avg10=([0-9.]+)`)

// PSIMemory — давление на память, avg10 % (Linux PSI).
func PSIMemory() int {
	data, err := os.ReadFile("/proc/pressure/memory")
	if err != nil {
		return -1
	}
	m := psiRe.FindStringSubmatch(string(data))
	if len(m) != 2 {
		return -1
	}
	v, err := strconv.ParseFloat(m[1], 64)
	if err != nil || v < 0 {
		return -1
	}
	return int(v + 0.5)
}

var (
	sysMu      sync.Mutex
	failedAt   time.Time
	failedVal  int
	failedOnce bool
)

// FailedUnits — число упавших systemd-юнитов (кэш 5 с).
func FailedUnits() int {
	sysMu.Lock()
	defer sysMu.Unlock()
	if failedOnce && time.Since(failedAt) < 5*time.Second {
		return failedVal
	}
	failedOnce = true
	failedAt = time.Now()
	out, err := runCmd("systemctl", "--failed", "--no-legend", "--plain")
	if err != nil {
		failedVal = -1
		return failedVal
	}
	n := 0
	for _, l := range strings.Split(string(out), "\n") {
		if strings.TrimSpace(l) != "" {
			n++
		}
	}
	failedVal = n
	return n
}

// RebootRequired — 1, если система просит перезагрузку (пакет/ядро).
func RebootRequired() int {
	for _, p := range []string{"/var/run/reboot-required", "/run/reboot-required"} {
		if _, err := os.Stat(p); err == nil {
			return 1
		}
	}
	return 0
}
