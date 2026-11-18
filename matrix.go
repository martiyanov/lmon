package main

import "strconv"

// matrixCode — зелёная палитра: тёмно-зелёный → светло-зелёный.
func matrixCode(sev float64) int {
	palette := []int{22, 28, 34, 40, 46, 82, 118, 154}
	i := int(sev*float64(len(palette)-1) + 0.5)
	if i < 0 {
		i = 0
	}
	if i >= len(palette) {
		i = len(palette) - 1
	}
	return palette[i]
}

func matrixColor(sev float64) string {
	return "\x1b[38;5;" + strconv.Itoa(matrixCode(sev)) + "m"
}

func matrixBg(sev float64) string {
	return "\x1b[38;5;16;48;5;" + strconv.Itoa(matrixCode(sev)) + "m"
}
