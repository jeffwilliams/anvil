package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"math"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
)

type Float struct {
	content blockEditable
	Id      int
	x, y    int
	// width and height, if set, constrain the float
	width, height int
	// evoker is the editable that caused this Float to be displayed
	evoker *editable
	style  floatStyle
	opts   FloatDrawOpts
}

type FloatDrawOpts struct {
	// If ShowAllLines is true, when the Float is drawn and x and y are set, they
	// are adjusted so that all the lines in the float are visible. That is, the y is
	// reduced to try and show all lines, and _maybe_ the x is reduced to prevent as
	// much wrapping.
	ShowAllLines bool
}

func NewFloat(style floatStyle, workChan chan Work) *Float {
	f := &Float{
		x: 0, y: 0, width: 0, height: 0, style: style,
	}

	executor := NewCommandExecutor(f)
	//f.Body.Init(style.tagBlockStyle(), style.tagEditableStyle(), style.Syntax, executor, f, workChan)
	scheduler := NewScheduler(workChan)
	f.content.Init(style.blockStyle, style.editableStyle, scheduler)
	f.content.executeOn = &f.content.editable
	f.content.colorizeAnsiEscapes = true
	f.content.SetAdapter(&editableAdapter{
		executor: executor,
		owner:    f,
	})
	f.content.label = fmt.Sprintf("float at (%d, %d)", f.x, f.y)
	f.content.PreventScrolling = true

	return f
}

func (f *Float) SetPos(x, y int) {
	f.x = x
	f.y = y
	if f.x < 0 {
		f.x = 0
	}
	if f.y < 0 {
		f.y = 0
	}
}

func (f *Float) SetEvoker(e *editable) {
	f.evoker = e
}

func (f *Float) draw(gtx layout.Context) {

	origMax := gtx.Constraints.Max

	if f.width > 0 {
		gtx.Constraints.Max.X = f.width
	}
	if f.height > 0 {
		gtx.Constraints.Max.Y = f.height
	}

	x, y := f.x, f.y
	if f.opts.ShowAllLines {
		x, y = f.computeCoordsThatDisplayMostText(gtx, origMax.X, origMax.Y)
	}

	off := op.Offset(image.Point{x, y}).Push(gtx.Ops)
	gtx.Values["offset"].(*OffsetStack).PushWithPurpose(image.Pt(x, y), "float")

	if gtx.Constraints.Max.X > origMax.X-x {
		gtx.Constraints.Max.X = origMax.X - x
	}
	if gtx.Constraints.Max.Y > origMax.Y-y {
		gtx.Constraints.Max.Y = origMax.Y - y
	}

	dims := f.content.layout(gtx)
	off.Pop()
	gtx.Values["offset"].(*OffsetStack).Pop()

	// Draw border
	colr := color.NRGBA(f.style.InnerBorderColor)

	// left
	drawRect(gtx,
		x-gtx.Metric.Dp(f.style.InnerBorderWidth),
		y-gtx.Metric.Dp(f.style.InnerBorderWidth),
		x,
		y+dims.Size.Y+gtx.Metric.Dp(f.style.InnerBorderWidth),
		colr)
	// right
	drawRect(gtx,
		x+dims.Size.X,
		y-gtx.Metric.Dp(f.style.InnerBorderWidth),
		x+dims.Size.X+gtx.Metric.Dp(f.style.InnerBorderWidth),
		y+dims.Size.Y+gtx.Metric.Dp(f.style.InnerBorderWidth),
		colr)
	// top
	drawRect(gtx,
		x,
		y-gtx.Metric.Dp(f.style.InnerBorderWidth),
		x+dims.Size.X,
		y,
		colr)
	// bottom
	drawRect(gtx,
		x,
		y+dims.Size.Y,
		x+dims.Size.X,
		y+dims.Size.Y+gtx.Metric.Dp(f.style.InnerBorderWidth),
		colr)

	colr = color.NRGBA(f.style.OuterBorderColor)
	both := gtx.Metric.Dp(f.style.InnerBorderWidth + f.style.OuterBorderWidth)
	inner := gtx.Metric.Dp(f.style.InnerBorderWidth)

	drawRect(gtx,
		x-both,
		y-both,
		x-inner,
		y+dims.Size.Y+both,
		colr)
	drawRect(gtx,
		x+dims.Size.X+inner,
		y-inner,
		x+dims.Size.X+both,
		y+dims.Size.Y+inner,
		colr)
	drawRect(gtx,
		x-inner,
		y-both,
		x+dims.Size.X+both,
		y-inner,
		colr)
	drawRect(gtx,
		x-inner,
		y+dims.Size.Y+inner,
		x+dims.Size.X+both,
		y+dims.Size.Y+both,
		colr)

}

func (f *Float) computeCoordsThatDisplayMostText(gtx layout.Context, maxX, maxY int) (x, y int) {
	x = f.x

	//If we can't display at least 10 characters per line or so, then shift left.
	w, _, err := f.content.RuneSizeInCurrentFont('X')
	if err == nil {
		if (maxX-x)/w.Round() < 10 {
			have := (maxX - x) / w.Round()
			more := 10 - have

			x -= more * w.Round()
			if x < 0 {
				x = 0
			}
		}
	}

	// Constrain the max x so that we can properly deterimine how many lines wrap
	savedMaxX := gtx.Constraints.Max.X
	if maxX > x+f.width {
		gtx.Constraints.Max.X = x + f.width
	}
	c := f.content.wrappedLineCount(gtx)
	gtx.Constraints.Max.X = savedMaxX

	lh := f.content.lineHeight()
	heightInLines := int(math.Floor(float64(maxY) / float64(lh)))

	idealY := (heightInLines - c) * lh
	if idealY < 0 {
		idealY = 0
	}
	if f.y < idealY {
		return x, f.y
	}
	return x, idealY
}

func (f *Float) SetFocus(gtx layout.Context) {
	f.content.AddOpForNextLayout(func(gtx layout.Context) {
		f.content.SetFocus(gtx)
	})
}

type OffsetStack struct {
	offsets  []image.Point
	combined image.Point
	purpose  []string
}

func (s *OffsetStack) Push(pt image.Point) {
	if s.offsets == nil {
		s.offsets = make([]image.Point, 0, 10)
	}
	s.offsets = append(s.offsets, pt)
	s.combined = image.Point{}
}

func (s *OffsetStack) PushWithPurpose(pt image.Point, purpose string) {
	for len(s.purpose) < len(s.offsets) {
		s.purpose = append(s.purpose, "")
	}
	s.Push(pt)
	s.purpose = append(s.purpose, purpose)
}

func (s *OffsetStack) Pop() {
	if len(s.offsets) == 0 {
		return
	}

	s.offsets = s.offsets[:len(s.offsets)-1]
	s.combined = image.Point{}
	if len(s.purpose) > len(s.offsets) {
		s.purpose = s.purpose[:len(s.offsets)]
	}
}

func (s *OffsetStack) Offset() image.Point {
	var zero image.Point
	if s.combined == zero {
		for _, pt := range s.offsets {
			s.combined = s.combined.Add(pt)
		}
	}

	return s.combined
}

func (s *OffsetStack) String() string {
	var buf bytes.Buffer

	fmt.Fprintf(&buf, "stack: ")
	for i, pt := range s.offsets {
		fmt.Fprintf(&buf, "%s ", pt)
		if s.purpose != nil && len(s.purpose) >= i {
			fmt.Fprintf(&buf, "[%s] ", s.purpose[i])
		}
	}

	fmt.Fprintf(&buf, "combined: ")
	fmt.Fprintf(&buf, "%s ", s.combined)
	return buf.String()
}

type floatStyle struct {
	InnerBorderColor Color
	InnerBorderWidth unit.Dp
	OuterBorderColor Color
	OuterBorderWidth unit.Dp
	blockStyle
	editableStyle
}

func (f *Float) SetStyle(style Style) {
	f.style = style.trayStyle()
	f.content.SetStyle(f.style.blockStyle, f.style.editableStyle)
}
