package main

import (
	"context"
	"fmt"
	"io"
	"math/bits"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/martiyanov/lmon/internal/meter"
)

// matrixRamp — 256 Braille-символов, отсортированных по числу точек:
// 0 — пустая клетка, 255 — максимально заполненная.
func matrixRamp() []rune {
	idx := make([]int, 256)
	for i := range idx {
		idx[i] = i
	}
	sort.Slice(idx, func(a, b int) bool {
		pa, pb := bits.OnesCount8(uint8(idx[a])), bits.OnesCount8(uint8(idx[b]))
		if pa != pb {
			return pa < pb
		}
		return idx[a] < idx[b]
	})
	out := make([]rune, len(idx))
	for i, v := range idx {
		out[i] = rune(0x2800 + v) // Unicode Braille Patterns
	}
	return out
}

var matrixGlyphs = matrixRamp()

// matrixColor — зелёная гамма: тёмно-зелёный → светло-зелёный.
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

// mview — матричный режим: подписи сверху, значения падают вниз.
type mview struct {
	w       io.Writer
	tty     bool
	color   bool
	matrix  bool
	rows    int
	cols    int
	history map[string][]hcell
}

func newMView(w io.Writer, cfg config, tty bool) *mview {
	v := &mview{
		w:       w,
		tty:     tty,
		color:   tty && os.Getenv("NO_COLOR") == "",
		matrix:  cfg.matrixPalette,
		rows:    termRows(),
		cols:    termCols(),
		history: map[string][]hcell{},
	}
	if v.rows <= 0 {
		v.rows = 24
	}
	if v.cols <= 0 {
		v.cols = 80
	}
	return v
}

func (v *mview) setup() {
	if v.tty {
		fmt.Fprint(v.w, "\x1b[?25l\x1b[2J\x1b[H")
	}
}

func (v *mview) restore() {
	if v.tty {
		fmt.Fprint(v.w, "\x1b[0m\x1b[?25h\n")
	}
}

func (v *mview) resize() {
	if r := termRows(); r > 0 {
		v.rows = r
	}
	if c := termCols(); c > 0 {
		v.cols = c
	}
}

func (v *mview) visible(rs []reading) []reading {
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

// add кладёт новый замер под заголовки; старые уезжают вниз.
func (v *mview) add(rs []reading) {
	h := v.rows - 1
	if h < 1 {
		h = 1
	}
	for _, r := range v.visible(rs) {
		c := hcell{}
		switch {
		case r.v < 0:
			c.state = -1
		case r.v == 999:
			c.state = 999
			c.sev = 1
		default:
			c.sev = r.severity()
		}
		hist := append([]hcell{c}, v.history[r.id]...)
		if len(hist) > h {
			hist = hist[:h]
		}
		v.history[r.id] = hist
	}
}

func (v *mview) cell(c hcell) string {
	if c.state == -1 {
		return " "
	}
	sev := c.sev
	if c.state == 999 {
		sev = 1
	}
	idx := int(sev*255 + 0.5)
	if idx < 0 {
		idx = 0
	}
	if idx > 255 {
		idx = 255
	}
	g := matrixGlyphs[idx]
	if !v.color {
		return string(g)
	}
	if v.matrix {
		return matrixColor(sev) + string(g) + reset
	}
	if c.state == 999 {
		return redBright + string(g) + reset
	}
	return fgSev(sev, false) + string(g) + reset
}

func (v *mview) draw(rs []reading) {
	vis := v.visible(rs)
	if !v.tty {
		// Вне TTY — просто строки, без управления курсором.
		if len(vis) == 0 {
			return
		}
		var labels strings.Builder
		for _, r := range vis {
			labels.WriteString(compactLabel(r))
		}
		fmt.Fprintln(v.w, labels.String())
		h := v.rows - 1
		for y := 0; y < h; y++ {
			var line strings.Builder
			for _, r := range vis {
				hist := v.history[r.id]
				if y < len(hist) {
					line.WriteString(v.cell(hist[y]))
				} else {
					line.WriteByte(' ')
				}
			}
			fmt.Fprintln(v.w, line.String())
		}
		return
	}
	fmt.Fprint(v.w, "\x1b[H")
	// заголовки
	fmt.Fprint(v.w, "\x1b[1;1H\x1b[2K")
	if v.color {
		if v.matrix {
			fmt.Fprint(v.w, "\x1b[38;5;46m")
		} else {
			fmt.Fprint(v.w, grn)
		}
	}
	for _, r := range vis {
		fmt.Fprint(v.w, compactLabel(r))
	}
	if v.color {
		fmt.Fprint(v.w, reset)
	}
	h := v.rows - 1
	for y := 0; y < h; y++ {
		fmt.Fprintf(v.w, "\x1b[%d;1H\x1b[2K", y+2)
		for _, r := range vis {
			hist := v.history[r.id]
			if y < len(hist) {
				fmt.Fprint(v.w, v.cell(hist[y]))
			} else {
				fmt.Fprint(v.w, " ")
			}
		}
	}
	fmt.Fprintf(v.w, "\x1b[%d;1H\x1b[0J", h+2)
}

func monitorRain(ctx context.Context, cfg config, w io.Writer, dropped []droppedColumn) int {
	termSetup()
	defer termRestore()
	v := newMView(w, cfg, true)
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
