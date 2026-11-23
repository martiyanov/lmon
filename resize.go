package main

import (
	"fmt"
	"strings"
)

// drawGrid — адаптивная таблица: широкие колонки, затем 3 колонки, затем 1.
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
	perRow := gridColumns(cols, len(rs))
	cellW := cols / perRow
	if cellW < 1 {
		cellW = 1
	}
	// Если все колонки помещаются в один ряд — сохраняем обычный поток:
	// новые значения снизу, заголовок закреплён внизу.
	if perRow == len(rs) && len(u.gridRows) > 0 && cols >= len(rs)*10 {
		u.drawResizeStream(rs, cellW, rows)
		return
	}
	// header + value + пустая строка между группами
	groupH := 3
	groups := (len(rs) + perRow - 1) / perRow
	if groups*groupH > rows {
		groupH = 2
	}
	for i, r := range rs {
		group := i / perRow
		col := i % perRow
		x := col*cellW + 1
		y := group*groupH + 1
		if y+1 > rows {
			break
		}
		fmt.Fprintf(u.w, "\x1b[%d;%dH%s", y, x, u.resizeHeaderCell(r, cellW))
		fmt.Fprintf(u.w, "\x1b[%d;%dH%s", y+1, x, u.resizeValueCell(r, cellW))
	}
}

func (u *ui) drawResizeStream(rs []reading, cellW, rows int) {
	// header внизу, как у обычного TUI
	headerY := rows
	for i, r := range rs {
		x := i*cellW + 1
		fmt.Fprintf(u.w, "\x1b[%d;%dH%s", headerY, x, u.resizeHeaderCell(r, cellW))
	}
	start := len(u.gridRows) - 1
	for n := 0; n < rows-1 && start-n >= 0; n++ {
		y := rows - 1 - n
		row := u.gridRows[start-n]
		for i, r := range row {
			if i >= len(rs) {
				break
			}
			x := i*cellW + 1
			fmt.Fprintf(u.w, "\x1b[%d;%dH%s", y, x, u.resizeValueCell(r, cellW))
		}
	}
}

func (u *ui) resizeHeaderCell(r reading, width int) string {
	label := resizeLabel(r)
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
	// hybrid: заливаем цветом всю ширину ячейки, не только три цифры.
	if u.hybrid && u.color && !u.matrix {
		text := u.displayText(r)
		if r.v < 0 {
			return dim + centerPlain(text, width) + reset
		}
		return bgSev(r.severity(), false) + centerPlain(text, width) + reset
	}
	return centerANSI(u.dataRow([]reading{r}), width)
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

// gridColumns — сколько карточек помещается в ряд.
// 10 — минимальная комфортная ширина карточки: имя + значение.
func gridColumns(cols, n int) int {
	if n < 1 {
		return 1
	}
	perRow := cols / 10
	if perRow < 1 {
		perRow = 1
	}
	if perRow > n {
		perRow = n
	}
	return perRow
}
