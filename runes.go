package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/martiyanov/lmon/internal/meter"
)

// runeLevels — одноклеточные «шифрованные» символы, сгруппированные по
// визуальному весу: от точки до плотного греческого знака. Внутри группы несколько вариантов,
// поэтому одно и то же значение каждый раз выглядит немного иначе.
var runeLevels = [][]rune{
	{'.', ','},
	{':', ';', '!', '?'},
	{'-', '=', '_', '~'},
	{'+', '*', '^'},
	{'#', '%', '&'},
	{'@', '$', '?'},
	{'λ', 'π', 'Σ'},
	{'Ψ', 'Ω', 'Ξ', 'Ѣ', 'Ѧ', 'Ѫ', 'Ѭ', 'Ѳ', 'Ѵ', 'Ѱ', 'Ѯ'},
}

// runeGlyph выбирает руну по уровню 0..1; cycle перебирает варианты
// внутри одного уровня, чтобы повторяющееся значение не застывало.
func runeGlyph(sev float64, cycle int) rune {
	if sev <= 0 {
		return ' '
	}
	idx := int(sev*float64(len(runeLevels)-1) + 0.5)
	if idx < 0 {
		idx = 0
	}
	if idx >= len(runeLevels) {
		idx = len(runeLevels) - 1
	}
	g := runeLevels[idx]
	return g[cycle%len(g)]
}

type gcell struct {
	glyph rune
	sev   float64
	blank bool
}

// gview — падающие заполняющие символы: заголовки сверху, значения вниз.
type gview struct {
	w        io.Writer
	tty      bool
	color    bool
	matrix   bool
	rows     int
	cols     int
	history  map[string][]gcell
	counters map[string]int
}

func newGView(w io.Writer, cfg config, tty bool) *gview {
	v := &gview{
		w:        w,
		tty:      tty,
		color:    tty && os.Getenv("NO_COLOR") == "",
		matrix:   cfg.matrixPalette,
		rows:     termRows(),
		cols:     termCols(),
		history:  map[string][]gcell{},
		counters: map[string]int{},
	}
	if v.rows <= 0 {
		v.rows = 24
	}
	if v.cols <= 0 {
		v.cols = 80
	}
	return v
}

func (v *gview) setup() {
	if v.tty {
		fmt.Fprint(v.w, "\x1b[?25l\x1b[2J\x1b[H")
	}
}
func (v *gview) restore() {
	if v.tty {
		fmt.Fprint(v.w, "\x1b[0m\x1b[?25h\n")
	}
}
func (v *gview) resize() {
	if r := termRows(); r > 0 {
		v.rows = r
	}
	if c := termCols(); c > 0 {
		v.cols = c
	}
}

func (v *gview) visible(rs []reading) []reading {
	out := make([]reading, 0, len(rs))
	for _, r := range rs {
		if r.blank {
			continue
		}
		if len(out) >= v.cols {
			break
		}
		out = append(out, r)
	}
	return out
}

func (v *gview) add(rs []reading) {
	h := v.rows - 1
	if h < 1 {
		h = 1
	}
	for _, r := range v.visible(rs) {
		var c gcell
		switch {
		case r.v < 0:
			c.blank = true
		case r.v == 999:
			c.sev = 1
			c.glyph = runeGlyph(1, v.counters[r.id])
		default:
			c.sev = r.severity()
			if c.sev > 0 {
				c.glyph = runeGlyph(c.sev, v.counters[r.id])
			} else {
				c.blank = true
			}
		}
		v.counters[r.id]++
		hist := append([]gcell{c}, v.history[r.id]...)
		if len(hist) > h {
			hist = hist[:h]
		}
		v.history[r.id] = hist
	}
}

func (v *gview) cell(c gcell) string {
	if c.blank {
		return " "
	}
	if !v.color {
		return string(c.glyph)
	}
	return "\x1b[38;5;46m" + string(c.glyph) + reset
}

func (v *gview) draw(rs []reading) {
	vis := v.visible(rs)
	if len(vis) == 0 {
		return
	}
	if !v.tty {
		var b strings.Builder
		for _, r := range vis {
			b.WriteString(compactLabel(r))
		}
		fmt.Fprintln(v.w, b.String())
		for y := 0; y < v.rows-1; y++ {
			b.Reset()
			for _, r := range vis {
				h := v.history[r.id]
				if y < len(h) {
					b.WriteString(v.cell(h[y]))
				} else {
					b.WriteByte(' ')
				}
			}
			fmt.Fprintln(v.w, b.String())
		}
		return
	}
	fmt.Fprint(v.w, "\x1b[H\x1b[1;1H\x1b[2K")
	if v.color {
		fmt.Fprint(v.w, "\x1b[38;5;46m")
	}
	for _, r := range vis {
		fmt.Fprint(v.w, compactLabel(r))
	}
	if v.color {
		fmt.Fprint(v.w, reset)
	}
	for y := 0; y < v.rows-1; y++ {
		fmt.Fprintf(v.w, "\x1b[%d;1H\x1b[2K", y+2)
		for _, r := range vis {
			h := v.history[r.id]
			if y < len(h) {
				fmt.Fprint(v.w, v.cell(h[y]))
			} else {
				fmt.Fprint(v.w, " ")
			}
		}
	}
	fmt.Fprintf(v.w, "\x1b[%d;1H\x1b[0J", v.rows+1)
}

func monitorRunes(ctx context.Context, cfg config, w io.Writer, dropped []droppedColumn) int {
	termSetup()
	defer termRestore()
	v := newGView(w, cfg, true)
	v.setup()
	defer v.restore()
	first := measure(ctx, cfg)
	if !cfg.noHints {
		if hs := startupHints(cfg, first, runtime.GOOS, dropped); len(hs) > 0 {
			printHints(os.Stderr, hs)
		}
	}
	v.add(first)
	v.draw(first)
	if cfg.iters > 0 && cfg.iters <= 1 {
		return 0
	}
	ticker := time.NewTicker(cfg.interval)
	defer ticker.Stop()
	refreshGW := time.NewTicker(30 * time.Second)
	defer refreshGW.Stop()
	for it := 2; ; it++ {
		select {
		case <-ctx.Done():
			return 130
		default:
		}
		select {
		case <-refreshGW.C:
			refreshGateway(&cfg, meter.DefaultGateway)
		default:
		}
		v.resize()
		rs := measure(ctx, cfg)
		v.add(rs)
		v.draw(rs)
		if cfg.iters > 0 && it >= cfg.iters {
			return 0
		}
		if err := nextTick(ctx, ticker, cfg.interval); err != nil {
			return 130
		}
	}
}
