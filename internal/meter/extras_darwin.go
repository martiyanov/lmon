//go:build darwin

package meter

import (
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	netMu     sync.Mutex
	netLastAt time.Time
	netRx     uint64
	netTx     uint64
	netErr    uint64
)

// parseNetstatIB суммирует Ibytes/Obytes и ошибки по link-строкам.
// Строки с адресами (IPv4/IPv6) дублируют счётчики, поэтому берём
// только строки с <Link#...>.
func parseNetstatIB(out string) (rx, tx, errs uint64) {
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) < 8 {
			continue
		}
		name := f[0]
		if name == "lo0" || strings.HasPrefix(name, "gif") ||
			strings.HasPrefix(name, "stf") || strings.HasPrefix(name, "utun") ||
			strings.HasPrefix(name, "awdl") || strings.HasPrefix(name, "llw") {
			continue
		}
		link := false
		for _, x := range f {
			if strings.HasPrefix(x, "<Link#") {
				link = true
				break
			}
		}
		if !link || len(f) < 7 {
			continue
		}
		// последние 7 полей: Ipkts Ierrs Ibytes Opkts Oerrs Obytes Coll
		tail := f[len(f)-7:]
		ierrs, e1 := strconv.ParseUint(tail[1], 10, 64)
		ibytes, e2 := strconv.ParseUint(tail[2], 10, 64)
		oerrs, e3 := strconv.ParseUint(tail[4], 10, 64)
		obytes, e4 := strconv.ParseUint(tail[5], 10, 64)
		if e1 != nil || e2 != nil || e3 != nil || e4 != nil {
			continue
		}
		rx += ibytes
		tx += obytes
		errs += ierrs + oerrs
	}
	return rx, tx, errs
}

func readNetTotalsDarwin() (rx, tx, errs uint64) {
	out, err := exec.Command("netstat", "-ib").Output()
	if err != nil {
		return 0, 0, 0
	}
	return parseNetstatIB(string(out))
}

// NetRate — трафик RX/TX в КБ/с и ошибки/дропы в секунду (без lo0).
// Первый вызов делает короткое окно 150 мс для --once.
func NetRate() (rxKB, txKB, errPerSec int) {
	rx, tx, errs := readNetTotalsDarwin()
	now := time.Now()
	netMu.Lock()
	if netLastAt.IsZero() {
		netLastAt, netRx, netTx, netErr = now, rx, tx, errs
		netMu.Unlock()
		time.Sleep(150 * time.Millisecond)
		rx, tx, errs = readNetTotalsDarwin()
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
