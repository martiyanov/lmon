package main

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/martiyanov/lmon/internal/meter"
)

// runeLevels — одноклеточные «шифрованные» символы, сгруппированные по
// визуальному весу: от точки до плотного греческого знака. Внутри группы несколько вариантов,
// поэтому одно и то же значение каждый раз выглядит немного иначе.
var runeLevels = [][]rune{
	{'.', ',', '·', '\'', '`'},
	{':', ';', '!', '?', 'i', 'l', 'I', '|'},
	{'-', '=', '_', '~', '+', '×', '÷'},
	{'+', '*', '^', 'x', 'X', 'v', '±'},
	{'#', '%', '&', '$', '@', '§', '¤'},
	{'λ', 'π', 'Σ', 'µ', 'σ', 'φ', 'γ', 'δ'},
	{'Ψ', 'Ω', 'Ξ', 'Φ', 'Θ', 'Δ', 'Λ', 'Π'},
	{'W', 'M', '@', '%', '&', '∞', '∑', '√'},
}

// runeGlyph выбирает символ нужного уровня. Внутри уровня варианты
// перемешиваются и расходуются без повторов; когда мешок исчерпан,
// он перемешивается заново (первый символ не совпадает с предыдущим).
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
	return nextRune(idx)
}

// severityLevel — уровень 0..len(runeLevels)-1 по «плохости».
func severityLevel(sev float64) int {
	if sev <= 0 {
		return 0
	}
	l := int(sev*float64(len(runeLevels)-1) + 0.5)
	if l < 0 {
		return 0
	}
	if l >= len(runeLevels) {
		return len(runeLevels) - 1
	}
	return l
}

// matrixGlyphFor — один символ для compact/horizontal/rain.
func matrixGlyphFor(sev float64, used map[rune]bool) rune {
	if severityLevel(sev) == 0 {
		return '·'
	}
	return runeForRow(sev, used)
}

// runeForRow выбирает символ, которого ещё нет в текущей строке.
// Сначала берёт целевой уровень, затем соседние — чтобы даже при
// одинаковых значениях символы в одной строке не повторялись.
func runeForRow(sev float64, used map[rune]bool) rune {
	if sev <= 0 {
		return ' '
	}
	level := int(sev*float64(len(runeLevels)-1) + 0.5)
	if level < 0 {
		level = 0
	}
	if level >= len(runeLevels) {
		level = len(runeLevels) - 1
	}
	try := func(lv int) (rune, bool) {
		if lv < 0 || lv >= len(runeLevels) {
			return 0, false
		}
		g := runeLevels[lv]
		start := rand.Intn(len(g))
		for i := 0; i < len(g); i++ {
			c := g[(start+i)%len(g)]
			if !used[c] {
				used[c] = true
				return c, true
			}
		}
		return 0, false
	}
	for d := 0; d < len(runeLevels); d++ {
		if c, ok := try(level - d); ok {
			return c
		}
		if d > 0 {
			if c, ok := try(level + d); ok {
				return c
			}
		}
	}
	// Строка больше всего набора — повторяем, иначе нечего выводить.
	c := runeGlyph(sev, 0)
	used[c] = true
	return c
}

var (
	runeMu   sync.Mutex
	runeBags = map[int][]rune{}
	runePos  = map[int]int{}
	runeLast = map[int]rune{}
)

func nextRune(level int) rune {
	runeMu.Lock()
	defer runeMu.Unlock()
	bag := runeBags[level]
	pos := runePos[level]
	if len(bag) == 0 || pos >= len(bag) {
		bag = append([]rune(nil), runeLevels[level]...)
		rand.Shuffle(len(bag), func(i, j int) { bag[i], bag[j] = bag[j], bag[i] })
		if len(bag) > 1 && bag[0] == runeLast[level] {
			bag = append(bag[1:], bag[0])
		}
		runeBags[level] = bag
		pos = 0
	}
	g := bag[pos]
	runePos[level] = pos + 1
	runeLast[level] = g
	return g
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
	used := map[rune]bool{}
	for _, r := range v.visible(rs) {
		var c gcell
		switch {
		case r.v < 0:
			c.blank = true
		case r.v == 999:
			c.sev = 1
			c.glyph = matrixGlyphFor(1, used)
		default:
			c.sev = r.severity()
			if severityLevel(c.sev) == 0 {
				c.glyph = '·'
			} else {
				c.glyph = matrixGlyphFor(c.sev, used)
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
