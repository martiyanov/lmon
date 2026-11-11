package netping

import (
	"strings"
	"testing"
	"time"
)

const macPingOut = `PING 8.8.8.8 (8.8.8.8): 56 data bytes
64 bytes from 8.8.8.8: icmp_seq=0 ttl=117 time=94.939 ms
64 bytes from 8.8.8.8: icmp_seq=1 ttl=117 time=51.210 ms

--- 8.8.8.8 ping statistics ---
2 packets transmitted, 2 packets received, 0.0% packet loss
round-trip min/avg/max/stddev = 51.210/73.075/94.939/21.865 ms
`

const linuxPingOut = `PING 8.8.8.8 (8.8.8.8) 56(84) bytes of data.
64 bytes from 8.8.8.8: icmp_seq=1 ttl=57 time=15.3 ms
64 bytes from 8.8.8.8: icmp_seq=2 ttl=57 time=14.8 ms

--- 8.8.8.8 ping statistics ---
2 packets transmitted, 2 received, 0% packet loss, time 1002ms
rtt min/avg/max/mdev = 14.812/15.057/15.303/0.245 ms
`

func TestParseTimesMac(t *testing.T) {
	times := parseTimes(macPingOut)
	if len(times) != 2 { // строка stddev не содержит "time="
		t.Fatalf("len = %d, want 2", len(times))
	}
	if last := times[len(times)-1]; last != 51.210 {
		t.Errorf("last time = %v, want 51.210", last)
	}
	if got := int(times[1]); got != 51 {
		t.Errorf("times[1] = %d, want 51", got)
	}
}

func TestParseTimesLinux(t *testing.T) {
	times := parseTimes(linuxPingOut)
	if len(times) != 2 {
		t.Fatalf("len = %d, want 2", len(times))
	}
	if got := int(times[1]); got != 14 {
		t.Errorf("times[1] = %d, want 14", got)
	}
}

func TestParseTimesSubMillisecond(t *testing.T) {
	times := parseTimes("64 bytes from 192.168.0.1: icmp_seq=1 ttl=64 time<1 ms\n")
	if len(times) != 1 || times[0] != 1 {
		t.Errorf("parseTimes(time<1) = %v, want [1]", times)
	}
}

func TestParseTimesEmpty(t *testing.T) {
	if times := parseTimes("PING 8.8.8.8: 56 data bytes\n\nRequest timeout for icmp_seq 0\n"); len(times) != 0 {
		t.Errorf("parseTimes(timeout) = %v, want empty", times)
	}
}

func TestPingCmdsMatrix(t *testing.T) {
	cases := []struct {
		goos, host string
		want       []string // начало primary-команды
		fallback   bool
	}{
		{"darwin", "8.8.8.8", []string{"ping", "-c", "2", "-i", "0.1", "-W", "1000", "8.8.8.8"}, false},
		{"darwin", "fe80::1%en0", []string{"ping6", "-c", "2", "-i", "0.1", "fe80::1%en0"}, false},
		{"linux", "8.8.8.8", []string{"ping", "-c", "2", "-i", "0.2", "-W", "1", "8.8.8.8"}, false},
		{"linux", "2606:4700::1111", []string{"ping", "-6", "-c", "2", "-i", "0.2", "-W", "1", "2606:4700::1111"}, true},
		{"windows", "8.8.8.8", []string{"ping", "-n", "2", "-w", "1000", "8.8.8.8"}, false},
		{"windows", "2606::1", []string{"ping", "-6", "-n", "2", "-w", "1000", "2606::1"}, false},
	}
	for _, c := range cases {
		primary, fallback := pingCmds(c.goos, c.host, "2")
		if strings.Join(primary, " ") != strings.Join(c.want, " ") {
			t.Errorf("pingCmds(%s,%s) = %v, want %v", c.goos, c.host, primary, c.want)
		}
		if (fallback != nil) != c.fallback {
			t.Errorf("pingCmds(%s,%s) fallback = %v, want наличие=%v", c.goos, c.host, fallback, c.fallback)
		}
	}
	// ретрай — одиночным пакетом
	retry, _ := pingCmds("darwin", "fe80::1%en0", "1")
	if retry[2] != "1" {
		t.Errorf("ретрай darwin v6 = %v, want -c 1", retry)
	}
}

func TestIsV6(t *testing.T) {
	v6 := []string{"2606:4700:4700::1111", "fe80::1%en0", "::1"}
	v4 := []string{"8.8.8.8", "192.168.1.1", "example.com", "::ffff:1.2.3.4"}
	for _, h := range v6 {
		if !isV6(h) {
			t.Errorf("isV6(%q) = false, want true", h)
		}
	}
	for _, h := range v4 {
		if isV6(h) {
			t.Errorf("isV6(%q) = true, want false", h)
		}
	}
}

func TestRetryAllowed(t *testing.T) {
	// ретрай разрешён, только когда в бюджете есть ~1с на ещё один таймаут
	if !retryAllowed(1050*time.Millisecond, 2050*time.Millisecond) {
		t.Error("retryAllowed(1.05с, 2.05с) = false, want true")
	}
	if retryAllowed(1050*time.Millisecond, 2*time.Second) {
		t.Error("retryAllowed(1.05с, 2с) = true, want false — ретрай не уложится")
	}
}

func TestParseTimesWindowsOEM(t *testing.T) {
	// Русская Windows: ping.exe при перенаправлении вывода пишет в OEM
	// кодировке (CP866). Байты «время» не UTF-8, но «TTL» и цифры — ASCII.
	// Строка: «Ответ от 8.8.8.8: число байт=32 время=33мс TTL=117»
	line := []byte{
		0x8e, 0xe2, 0xa2, 0xa5, 0xe2, ' ', 0xae, 0xe2, ' ', // Ответ от
		'8', '.', '8', '.', '8', '.', '8', ':', ' ',
		0xe7, 0xa8, 0xe1, 0xab, 0xae, ' ', // число
		0xa1, 0xa0, 0xa9, 0xe2, '=', '3', '2', ' ', // байт=32
		0xa2, 0xe0, 0xa5, 0xac, 0xef, '=', '3', '3', 0xac, 0xe1, ' ', // время=33мс
		'T', 'T', 'L', '=', '1', '1', '7', '\n',
	}
	times := parseTimes(string(line))
	if len(times) != 1 || times[0] != 33 {
		t.Fatalf("parseTimes(OEM) = %v, want [33]", times)
	}
}

func TestParseTimesWindowsEnglish(t *testing.T) {
	// Английская Windows: Reply from 8.8.8.8: bytes=32 time=33ms TTL=117
	out := "Reply from 8.8.8.8: bytes=32 time=33ms TTL=117\n" +
		"Reply from 8.8.8.8: bytes=32 time<1ms TTL=117\n"
	times := parseTimes(out)
	if len(times) != 2 || times[0] != 33 || times[1] != 1 {
		t.Fatalf("parseTimes(EN) = %v, want [33 1]", times)
	}
}
