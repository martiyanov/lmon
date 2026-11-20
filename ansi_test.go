package main

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestClipANSI(t *testing.T) {
	in := "\x1b[38;5;46mNET\x1b[39m \x1b[1;31mGTW\x1b[0m"
	got := clipANSI(in, 7)
	if !strings.Contains(got, "NET") || !strings.Contains(got, "GTW") {
		t.Fatalf("clip lost content: %q", got)
	}
	if strings.Contains(got, "X") {
		t.Fatalf("unexpected")
	}
	// 5 visible cells: "NET" + space + "G", не больше
	if visibleLen(got) != 7 {
		t.Fatalf("visible width = %d, want 7 (%q)", visibleLen(got), got)
	}
	if clipANSI("abc", 10) != "abc" {
		t.Fatalf("short string changed")
	}
	if !strings.Contains(clipANSI(in, 3), "\x1b[0m") {
		t.Fatalf("truncated colored string must reset")
	}
}

func visibleLen(s string) int {
	n := 0
	for i := 0; i < len(s); {
		if s[i] == '\x1b' {
			j := i + 1
			if j < len(s) && s[j] == '[' {
				j++
				for j < len(s) {
					c := s[j]
					j++
					if c >= 0x40 && c <= 0x7e {
						break
					}
				}
			}
			i = j
			continue
		}
		_, size := utf8.DecodeRuneInString(s[i:])
		n++
		i += size
	}
	return n
}
