package netping

import "testing"

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
