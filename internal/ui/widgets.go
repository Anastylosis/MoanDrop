package ui

import (
	"fmt"
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/Anastylosis/MoanDrop/internal/core"
)

const frameGap = 3

// frameProgress is the row of cells that fill in pink, one per sampled
// phash frame.
type frameProgress struct {
	widget.BaseWidget
	total, done int
}

func newFrameProgress(total int) *frameProgress {
	f := &frameProgress{total: total}
	f.ExtendBaseWidget(f)
	return f
}

func (f *frameProgress) SetDone(n int) {
	n = max(0, min(n, f.total))
	if n == f.done {
		return
	}
	f.done = n
	f.Refresh()
}

func (f *frameProgress) Done() int { return f.done }

func (f *frameProgress) CreateRenderer() fyne.WidgetRenderer {
	r := &frameRenderer{f: f}
	for range f.total {
		c := canvas.NewRectangle(screenCell)
		c.CornerRadius = 2
		r.cells = append(r.cells, c)
	}
	return r
}

type frameRenderer struct {
	f     *frameProgress
	cells []*canvas.Rectangle
}

func (r *frameRenderer) Layout(size fyne.Size) {
	n := float32(len(r.cells))
	w := (size.Width - (n-1)*frameGap) / n
	h := min(w*4/3, size.Height)
	y := (size.Height - h) / 2
	for i, c := range r.cells {
		c.Resize(fyne.NewSize(w, h))
		c.Move(fyne.NewPos(float32(i)*(w+frameGap), y))
	}
}

func (r *frameRenderer) MinSize() fyne.Size {
	return fyne.NewSize(float32(len(r.cells))*8+float32(len(r.cells)-1)*frameGap, 18)
}

func (r *frameRenderer) Refresh() {
	on := theme.Color(theme.ColorNamePrimary)
	for i, c := range r.cells {
		if i < r.f.done {
			c.FillColor = on
		} else {
			c.FillColor = screenCell
		}
		c.Refresh()
	}
}

func (r *frameRenderer) Objects() []fyne.CanvasObject {
	objs := make([]fyne.CanvasObject, len(r.cells))
	for i, c := range r.cells {
		objs[i] = c
	}
	return objs
}

func (r *frameRenderer) Destroy() {}

// dropZone is the player-black drop target: caption, frame cells, and a
// mono line under them.
type dropZone struct {
	*fyne.Container
	cells *frameProgress
	line  *canvas.Text
}

const dropHint = "or use File > Open Video..."

func newDropZone() *dropZone {
	bg := canvas.NewRectangle(screenBlack)
	bg.CornerRadius = 8
	caption := canvas.NewText("Drop a video or a folder here.", white)
	caption.TextStyle = fyne.TextStyle{Bold: true}
	caption.TextSize = theme.TextSize() * 1.4
	caption.Alignment = fyne.TextAlignCenter
	z := &dropZone{cells: newFrameProgress(core.PhashFrames)}
	z.line = canvas.NewText(dropHint, screenMuted)
	z.line.TextStyle = fyne.TextStyle{Monospace: true}
	z.line.TextSize = theme.TextSize() * 0.9
	z.line.Alignment = fyne.TextAlignCenter
	z.Container = container.NewStack(bg, container.NewPadded(container.NewVBox(caption, z.cells, z.line)))
	return z
}

// SetFrames shows N of total frames sampled; a negative n returns to idle.
func (z *dropZone) SetFrames(n int) {
	if n < 0 {
		z.cells.SetDone(0)
		z.line.Text = dropHint
	} else {
		z.cells.SetDone(n)
		z.line.Text = fmt.Sprintf("Fingerprinting: %d of %d frames sampled", n, core.PhashFrames)
	}
	z.line.Refresh()
}

// chip is a small pill: muted on surface, or pink text on a faint pink
// fill for the AI declaration.
type chip struct {
	widget.BaseWidget
	Text string
	ai   bool
}

func newChip(text string, ai bool) *chip {
	c := &chip{Text: text, ai: ai}
	c.ExtendBaseWidget(c)
	return c
}

func (c *chip) CreateRenderer() fyne.WidgetRenderer {
	bg := canvas.NewRectangle(color.Transparent)
	bg.CornerRadius = 100
	txt := canvas.NewText(c.Text, color.Transparent)
	txt.TextStyle = fyne.TextStyle{Monospace: true}
	r := &chipRenderer{c: c, bg: bg, txt: txt}
	r.Refresh()
	return r
}

type chipRenderer struct {
	c   *chip
	bg  *canvas.Rectangle
	txt *canvas.Text
}

func (r *chipRenderer) Layout(size fyne.Size) {
	r.bg.Resize(size)
	ts := r.txt.MinSize()
	r.txt.Resize(ts)
	r.txt.Move(fyne.NewPos((size.Width-ts.Width)/2, (size.Height-ts.Height)/2))
}

func (r *chipRenderer) MinSize() fyne.Size {
	ts := r.txt.MinSize()
	return fyne.NewSize(ts.Width+14, ts.Height+2)
}

func (r *chipRenderer) Refresh() {
	r.txt.Text = r.c.Text
	r.txt.TextSize = theme.TextSize() * 0.82
	if r.c.ai {
		pink := theme.Color(theme.ColorNamePrimary)
		r.txt.Color = pink
		r.bg.FillColor = softPink(pink)
	} else {
		r.txt.Color = theme.Color(theme.ColorNamePlaceHolder)
		r.bg.FillColor = theme.Color(theme.ColorNameInputBackground)
	}
	r.bg.Refresh()
	r.txt.Refresh()
}

func softPink(c color.Color) color.Color {
	n := color.NRGBAModel.Convert(c).(color.NRGBA)
	return withAlpha(n, 0x38)
}

func (r *chipRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.bg, r.txt}
}

func (r *chipRenderer) Destroy() {}

// monoLabel is a muted, monospace label for hashes, timecodes and counts.
func monoLabel(text string) *widget.Label {
	l := widget.NewLabelWithStyle(text, fyne.TextAlignLeading, fyne.TextStyle{Monospace: true})
	l.Importance = widget.LowImportance
	return l
}
