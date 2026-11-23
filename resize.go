package main

import (
	"fmt"
	"strings"
)

// drawGrid — экспериментальный responsive-режим: карточки индикаторов
// заполняют ширину терминала и перестраиваются при его сужении.
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
	cardW := cols / perRow
	if cardW < 1 {
		cardW = 1
	}
	cardH := 2
	needRows := ((len(rs) + perRow - 1) / perRow) * cardH
	if needRows > rows {
		cardH = 1 // тесный терминал: одна строка на карточку
	}
	for i, r := range rs {
		row := i / perRow
		col := i % perRow
		x := col*cardW + 1
		y := row*cardH + 1
		if y > rows {
			break
		}
		label := resizeLabel(r)
		value := u.dataRow([]reading{r})
		if cardH == 1 {
			fmt.Fprintf(u.w, "\x1b[%d;%dH%s", y, x, clipANSI(label+" "+value, cardW))
			continue
		}
		// Всегда две строки: имя карточки и значение под ним.
		fmt.Fprintf(u.w, "\x1b[%d;%dH%s", y, x, clipANSI(label, cardW))
		if y+1 <= rows {
			fmt.Fprintf(u.w, "\x1b[%d;%dH%s", y+1, x, clipANSI(value, cardW))
		}
	}
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

func (u *ui) gridEnabled() bool { return u.resizeWindow }
