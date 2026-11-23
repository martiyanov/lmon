package main

import (
	"fmt"
	"strings"
)

// drawGrid — таблица во всю ширину: все заголовки всегда в одной строке.
// Обычный режим: шапка снизу, значения идут вверх.
// --rain: шапка сверху, значения идут вниз.
func (u *ui) drawGrid(rs []reading) {
	rows := termRows()
	cols := termCols()
	if rows < 1 {
		rows = 24
	}
	if cols < 1 {
		cols = 80
	}
	u.rows, u.cols = rows, cols
	fmt.Fprint(u.w, "\x1b[H\x1b[2J")
	if len(rs) == 0 {
		fmt.Fprintf(u.w, "\x1b[1;1H%s", clipANSI(u.head, cols))
		return
	}
	perRow := gridColumns(cols, rs)
	shown := rs[:perRow]
	cellW := cols / perRow
	if cellW < 1 {
		cellW = 1
	}
	widths := make([]int, perRow)
	extra := cols - cellW*perRow
	if extra < 0 {
		extra = 0
	}
	for i := range widths {
		widths[i] = cellW
		if i < extra {
			widths[i]++
		}
	}
	if u.horizontal {
		u.drawResizeHorizontal(shown, widths, rows)
		return
	}
	if u.rain {
		u.drawResizeRain(shown, widths, rows)
		return
	}
	u.drawResizeStream(shown, widths, rows)
}

func (u *ui) drawResizeStream(rs []reading, widths []int, rows int) {
	// шапка внизу, новые значения над ней
	headerY := rows
	x := 1
	for i, r := range rs {
		w := widths[i]
		fmt.Fprintf(u.w, "\x1b[%d;%dH%s", headerY, x, u.resizeHeaderCell(r, w))
		x += w
	}
	start := len(u.gridRows) - 1
	for n := 0; n < rows-1 && start-n >= 0; n++ {
		y := rows - 1 - n
		row := u.gridRows[start-n]
		x = 1
		for i, r := range row {
			if i >= len(rs) {
				break
			}
			w := widths[i]
			fmt.Fprintf(u.w, "\x1b[%d;%dH%s", y, x, u.resizeValueCell(r, w))
			x += w
		}
	}
}

func (u *ui) drawResizeRain(rs []reading, widths []int, rows int) {
	// шапка сверху, новые значения под ней, старые уходят вниз
	x := 1
	for i, r := range rs {
		w := widths[i]
		fmt.Fprintf(u.w, "\x1b[1;%dH%s", x, u.resizeHeaderCell(r, w))
		x += w
	}
	start := len(u.gridRows) - 1
	for n := 0; n < rows-2 && start-n >= 0; n++ {
		y := 2 + n
		row := u.gridRows[start-n]
		x = 1
		for i, r := range row {
			if i >= len(rs) {
				break
			}
			w := widths[i]
			fmt.Fprintf(u.w, "\x1b[%d;%dH%s", y, x, u.resizeValueCell(r, w))
			x += w
		}
	}
}

func (u *ui) drawResizeHorizontal(rs []reading, widths []int, rows int) {
	// шапка сверху; внутри каждой колонки — горизонтальная лента значений
	x := 1
	for i, r := range rs {
		w := widths[i]
		fmt.Fprintf(u.w, "\x1b[1;%dH%s", x, u.resizeHeaderCell(r, w))
		x += w
	}
	slots := 1
	minCell := widths[0]
	for _, w := range widths {
		if w < minCell {
			minCell = w
		}
	}
	slots = (minCell + 1) / 4
	if slots < 1 {
		slots = 1
	}
	for i, r := range rs {
		_ = r
		x := 1
		for j := 0; j < i; j++ {
			x += widths[j]
		}
		// последние slots значений этой колонки, слева направо
		var samples []reading
		for n := len(u.gridRows) - 1; n >= 0 && len(samples) < slots; n-- {
			if i < len(u.gridRows[n]) {
				samples = append([]reading{u.gridRows[n][i]}, samples...)
			}
		}
		if len(samples) == 0 {
			samples = []reading{r}
		}
		pos := x
		for _, sample := range samples {
			w := 3
			if pos+3 > x+widths[i] {
				break
			}
			fmt.Fprintf(u.w, "\x1b[2;%dH%s", pos, u.resizeValueCell(sample, w))
			pos += 4
		}
	}
}

func (u *ui) resizeHeaderCell(r reading, width int) string {
	label := resizeLabelFor(r, width)
	if !u.color {
		return centerPlain(label, width)
	}
	switch {
	case r.blank:
		return centerPlain("   ", width)
	case r.v < 0:
		if u.matrix {
			return bgSev(0, true) + centerPlain(label, width) + reset
		}
		return purpleBg + centerPlain(label, width) + reset
	case r.v == 999:
		if u.matrix {
			return bgSev(1, true) + centerPlain(label, width) + reset
		}
		return redBg + centerPlain(label, width) + reset
	default:
		return bgSev(r.severity(), u.matrix) + centerPlain(label, width) + reset
	}
}

func (u *ui) resizeValueCell(r reading, width int) string {
	if u.blocks && (u.color || u.mono) {
		return u.resizeBlockCell(r, width)
	}
	if u.hybrid && u.color && !u.matrix {
		text := strings.TrimSpace(u.displayText(r))
		if r.v < 0 {
			return dim + centerPlain(text, width) + reset
		}
		return bgSev(r.severity(), false) + centerPlain(text, width) + reset
	}
	return centerANSI(strings.TrimSpace(u.dataRow([]reading{r})), width)
}

func (u *ui) resizeBlockCell(r reading, width int) string {
	if u.mono {
		if r.blank {
			return strings.Repeat(" ", width)
		}
		if r.v < 0 {
			return strings.Repeat("?", width)
		}
		if r.v == 999 {
			return strings.Repeat("!", width)
		}
		ramp := []rune{'░', '▒', '▓', '█'}
		n := int(r.severity()*3 + 0.5)
		if n < 0 {
			n = 0
		}
		if n > 3 {
			n = 3
		}
		return strings.Repeat(string(ramp[n]), width)
	}
	if r.blank {
		return strings.Repeat(" ", width)
	}
	var c string
	switch {
	case r.v < 0:
		if u.matrix {
			c = bgSev(0, true)
		} else {
			c = purpleBg
		}
	case r.v == 999:
		if u.matrix {
			c = bgSev(1, true)
		} else {
			c = redBg
		}
	default:
		c = bgSev(r.severity(), u.matrix)
	}
	return c + strings.Repeat(" ", width) + reset
}

func centerPlain(s string, width int) string {
	rs := []rune(s)
	if len(rs) >= width {
		return string(rs[:width])
	}
	left := (width - len(rs)) / 2
	right := width - len(rs) - left
	return strings.Repeat(" ", left) + s + strings.Repeat(" ", right)
}

func centerANSI(s string, width int) string {
	n := ansiWidth(s)
	if n >= width {
		return clipANSI(s, width)
	}
	left := (width - n) / 2
	right := width - n - left
	return strings.Repeat(" ", left) + s + strings.Repeat(" ", right)
}

// resizeLabelFor — полное, короткое или однобуквенное имя по ширине.
func resizeLabelFor(r reading, width int) string {
	full := resizeLabel(r)
	if ansiWidth(full) < width {
		return full
	}
	short := strings.TrimSpace(r.head)
	if ansiWidth(short) < width {
		return short
	}
	return compactLabel(r)
}

// resizeLabel — человеческие имена для широкого режима.
func resizeLabel(r reading) string {
	switch r.id {
	case "N":
		return "Network"
	case "G":
		return "Gateway"
	case "C":
		return "CPU"
	case "L":
		return "Load"
	case "M":
		return "Memory"
	case "T":
		return "Temp"
	case "U":
		return "Disk"
	case "S":
		return "Swap"
	case "I":
		return "IO wait"
	case "K":
		return "Disk busy"
	case "B":
		return "Battery"
	case "P":
		return "GPU"
	case "H":
		return "GPU temp"
	case "R":
		return "Net RX"
	case "J":
		return "Net TX"
	case "E":
		return "Net err"
	case "DR":
		return "Disk MB/s"
	case "DI":
		return "Disk IOPS"
	case "MP":
		return "Pressure"
	}
	if strings.TrimSpace(r.head) == "" {
		return r.id
	}
	return strings.TrimSpace(r.head)
}

// gridColumns — все колонки всегда в одном ряду; ширина только сжимается.
func gridColumns(cols int, rs []reading) int {
	if len(rs) == 0 {
		return 1
	}
	if cols < len(rs) {
		return cols
	}
	return len(rs)
}
