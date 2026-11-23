package main

import (
	"strings"
	"unicode/utf8"
)

// clipANSI обрезает строку до width видимых символов, не ломая SGR-цвета.
// lmon использует одноклеточные глифы, поэтому считаем по рунам.
func clipANSI(s string, width int) string {
	if width <= 0 {
		return s
	}
	var b strings.Builder
	visible := 0
	for i := 0; i < len(s); {
		if s[i] == '\x1b' {
			// сохраняем CSI-последовательность целиком (SGR, erase line и т.п.)
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
			} else {
				for j < len(s) && (s[j] < 0x40 || s[j] > 0x7e) {
					j++
				}
				if j < len(s) {
					j++
				}
			}
			b.WriteString(s[i:j])
			i = j
			continue
		}
		if visible >= width {
			break
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		b.WriteRune(r)
		visible++
		i += size
	}
	if visible >= width {
		b.WriteString(reset)
	}
	return b.String()
}

// ansiWidth — число видимых символов без ANSI-последовательностей.
func ansiWidth(s string) int {
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
			} else {
				for j < len(s) && (s[j] < 0x40 || s[j] > 0x7e) {
					j++
				}
				if j < len(s) {
					j++
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
