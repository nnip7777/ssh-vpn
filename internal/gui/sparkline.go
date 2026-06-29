package gui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"
)

type SparkLine struct {
	widget.BaseWidget
	data      []float64
	warnings  []bool
	cap       int
	head      int
	count     int
	color     color.Color
	warnColor color.Color
	max       float64
	bg        *canvas.Rectangle
	barPool   []*canvas.Rectangle
}

func NewSparkLine(clr color.Color, size int) *SparkLine {
	s := &SparkLine{
		data:      make([]float64, size),
		warnings:  make([]bool, size),
		cap:       size,
		color:     clr,
		warnColor: color.NRGBA{R: 160, G: 60, B: 60, A: 255},
		max:       1,
	}
	s.ExtendBaseWidget(s)
	return s
}

func (s *SparkLine) PushWithWarning(val float64, warn bool) {
	s.data[s.head] = val
	s.warnings[s.head] = warn
	s.head = (s.head + 1) % s.cap
	if s.count < s.cap {
		s.count++
	}
	s.Refresh()
}

func (s *SparkLine) Push(val float64) {
	s.PushWithWarning(val, false)
}

func (s *SparkLine) SetMax(maxVal float64) {
	if maxVal > 0 {
		s.max = maxVal
	} else {
		s.max = 1
	}
}

func (s *SparkLine) CreateRenderer() fyne.WidgetRenderer {
	s.bg = canvas.NewRectangle(darkBg)
	s.barPool = make([]*canvas.Rectangle, s.cap)
	for i := range s.barPool {
		s.barPool[i] = canvas.NewRectangle(s.color)
	}
	return &sparkRenderer{parent: s}
}

type sparkRenderer struct {
	parent *SparkLine
}

func (r *sparkRenderer) Destroy() {}

func (r *sparkRenderer) Layout(size fyne.Size) {
	r.parent.bg.Resize(size)
	n := r.parent.count
	if n == 0 {
		for _, b := range r.parent.barPool {
			b.Resize(fyne.NewSize(0, 0))
		}
		return
	}

	gap := float64(1)
	barW := (float64(size.Width) - gap*float64(n-1)) / float64(n)
	if barW < 1 {
		barW = 1
	}

	start := 0
	if r.parent.count >= r.parent.cap {
		start = r.parent.head
	}

	for i := 0; i < n; i++ {
		dataIdx := (start + i) % r.parent.cap
		val := r.parent.data[dataIdx]
		h := float64(size.Height) * val / r.parent.max
		if h < 1 && val > 0 {
			h = 1
		}
		bar := r.parent.barPool[i]
		if r.parent.warnings[dataIdx] {
			bar.FillColor = r.parent.warnColor
		} else {
			bar.FillColor = r.parent.color
		}
		bar.Resize(fyne.NewSize(float32(barW), float32(h)))
		bar.Move(fyne.NewPos(float32(float64(i)*(barW+gap)), float32(size.Height)-float32(h)))
		bar.Refresh()
	}

	for i := n; i < len(r.parent.barPool); i++ {
		r.parent.barPool[i].Resize(fyne.NewSize(0, 0))
	}
}

func (r *sparkRenderer) MinSize() fyne.Size {
	return fyne.NewSize(100, 24)
}

func (r *sparkRenderer) Objects() []fyne.CanvasObject {
	objs := make([]fyne.CanvasObject, 0, len(r.parent.barPool)+1)
	objs = append(objs, r.parent.bg)
	for _, b := range r.parent.barPool {
		objs = append(objs, b)
	}
	return objs
}

func (r *sparkRenderer) Refresh() {
	r.Layout(r.parent.Size())
}
