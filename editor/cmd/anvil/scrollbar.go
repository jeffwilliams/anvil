package main

import (
	"gioui.org/io/pointer"

	"image"
	"image/color"

	"gioui.org/io/event"
	"gioui.org/layout"
	"gioui.org/unit"
	//"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"github.com/jeffwilliams/anvil/editor/internal/events"
)

type scrollbar struct {
	layouter
	style      scrollbarStyle
	lineHeight int
	dims       layout.Dimensions
	windowBody *Body
	//pressPos     f32.Point
	//window       *Window
	//col          *Col
	//dragging     bool
	pointerState     PointerState
	eventInterceptor *events.EventInterceptor
}

type scrollbarStyle struct {
	FgColor     color.NRGBA
	BgColor     color.NRGBA
	BorderColor color.NRGBA
	GutterWidth unit.Dp
	LineSpacing unit.Dp
	Fonts       []FontStyle
}

func (b *scrollbar) Init(style scrollbarStyle, windowBody *Body) {
	b.style = style
	b.layouter.lineSpacing = style.LineSpacing
	b.layouter.setFontStyles(style.Fonts)
	b.windowBody = windowBody

	b.InitPointerEventHandlers()
}

func (b *scrollbar) InitPointerEventHandlers() {
	b.pointerState.Handler(PointerEventMatch{pointer.Press, pointer.ButtonPrimary}, b.moveBackward)
	b.pointerState.Handler(PointerEventMatch{pointer.Press, pointer.ButtonSecondary}, b.moveForward)

	b.pointerState.Handler(PointerEventMatch{pointer.Press, pointer.ButtonTertiary}, b.setTextposToMouse)
	b.pointerState.Handler(PointerEventMatch{pointer.Drag, pointer.ButtonTertiary}, b.setTextposToMouse)
	//b.pointerState.Handler(PointerEventMatch{pointer.Release, pointer.ButtonPrimary}, b.onPointerRelease)
}

func (b *scrollbar) layout(gtx layout.Context) layout.Dimensions {
	b.handleEvents(gtx)
	b.dims = b.draw(gtx)
	b.listenForEvents(gtx)
	return b.dims
}

func (b *scrollbar) handleEvents(gtx layout.Context) {
	for {
		e, ok := gtx.Event(pointer.Filter{Target: b, Kinds: pointer.Press | pointer.Drag | pointer.Release | pointer.Leave})
		if !ok {
			break
		}

		pe, ok := e.(pointer.Event)
		if !ok {
			log(LogCatgWin, "scrollbar filtered for pointer.Event, but got a %T instead\n", pe)
			continue
		}

		if b.intercept(gtx, &pe) {
			continue
		}

		b.Pointer(gtx, &pe)
	}
}

func (b *scrollbar) intercept(gtx layout.Context, ev *pointer.Event) (processed bool) {
	if b.eventInterceptor == nil {
		return false
	}

	return b.eventInterceptor.Filter(gtx, ev)
}

func (b *scrollbar) Pointer(gtx layout.Context, ev *pointer.Event) {
	b.pointerState.currentPointerEvent.set = false
	b.pointerState.Event(ev, gtx)
	b.pointerState.InvokeHandlers()
}

func (b *scrollbar) moveForward(ps *PointerState) {
	b.move(ps, Down)
}

func (b *scrollbar) moveBackward(ps *PointerState) {
	b.move(ps, Up)
}

func (b *scrollbar) move(ps *PointerState, dir verticalDirection) {
	//l.pressPos = ps.currentPointerEvent.Position
	//l.dragging = false
	h := b.windowBody.heightInLines(ps.gtx)
	linesToScroll := lerp(int(ps.currentPointerEvent.Position.Y), ps.gtx.Constraints.Max.Y, h)
	if linesToScroll < 1 {
		linesToScroll = 1
	}

	for ; linesToScroll > 0; linesToScroll-- {
		b.windowBody.ScrollOneLine(ps.gtx, dir)
	}
}

func (b *scrollbar) setTextposToMouse(ps *PointerState) {
	log(LogCatgWin, "drag on scrollbar at %s\n", ps.currentPointerEvent.Position)

	bdy := b.windowBody
	textLen := len(bdy.Bytes())

	targetTextPos := lerp(int(ps.currentPointerEvent.Position.Y), ps.gtx.Constraints.Max.Y, textLen)

	if targetTextPos < 0 {
		targetTextPos = 0
	}

	if targetTextPos > textLen {
		targetTextPos = textLen
	}

	b.windowBody.SetTopLeft(targetTextPos)

	return
}

func (b *scrollbar) draw(gtx layout.Context) layout.Dimensions {
	// Draw a thick bar, then a thin right column
	st := clip.Rect{
		Min: image.Pt(0, 0),
		Max: image.Pt(gtx.Metric.Dp(b.style.GutterWidth-1), gtx.Constraints.Max.Y)}.Push(gtx.Ops)
	paint.ColorOp{Color: b.style.BgColor}.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)
	st.Pop()

	st = clip.Rect{
		Min: image.Pt(gtx.Metric.Dp(b.style.GutterWidth-1), 0),
		Max: image.Pt(gtx.Metric.Dp(b.style.GutterWidth), gtx.Constraints.Max.Y)}.Push(gtx.Ops)
	paint.ColorOp{Color: b.style.BorderColor}.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)
	st.Pop()

	// Draw the button
	top, bot := b.buttonPositions(gtx)
	st = clip.Rect{
		Min: image.Pt(0, top),
		Max: image.Pt(gtx.Metric.Dp(b.style.GutterWidth-1), bot)}.Push(gtx.Ops)
	paint.ColorOp{Color: b.style.FgColor}.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)
	st.Pop()

	return layout.Dimensions{Size: image.Point{X: gtx.Metric.Dp(b.style.GutterWidth), Y: gtx.Constraints.Max.Y}}
}

func (b scrollbar) buttonPositions(gtx layout.Context) (top, bottom int) {
	bdy := b.windowBody
	textLen := len(bdy.Bytes())
	r := bdy.TopLeftIndex

	lh := int(b.lineHeight)

	top = lerp(r, textLen, gtx.Constraints.Max.Y)

	disp, err := b.lenOfDisplayedBodyTextInBytes(gtx)
	if err != nil {
		disp = lh
	}

	bottom = lerp(r+disp, textLen, gtx.Constraints.Max.Y)

	if bottom-top < 2 {
		bottom = top + 2
	}

	return
}

func (b scrollbar) lenOfDisplayedBodyTextInBytes(gtx layout.Context) (int, error) {
	// When we call LenOfDisplayedTextInBytes on the body of the window, it lays out the
	// text according to the constraints in gtx. At the time of this call, the constraints
	// are set to the size of the entire window; not to the inset remaining portion that is
	// left after rendering the scrollbar. Thus we must set gtx temporarily to the correct width
	gw := gtx.Metric.Dp(b.style.GutterWidth)
	gtx.Constraints.Max.X -= gw
	bdy := b.windowBody
	disp, err := bdy.LenOfDisplayedTextInBytes(gtx)
	gtx.Constraints.Max.X += gw
	return disp, err

}

// Linear interpolation. Finds the percentage that x is of tot1, and finds that
// percentage of tot2.
func lerp(x, tot1, tot2 int) int {
	if tot1 > 0 {
		return tot2 * x / tot1
	}
	return 0
}

func (b *scrollbar) listenForEvents(gtx layout.Context) {
	r := image.Rectangle{Max: b.dims.Size}
	st := clip.Rect(r).Push(gtx.Ops)

	event.Op(gtx.Ops, b)

	st.Pop()
}

func (b *scrollbar) SetStyle(style scrollbarStyle) {
	b.style = style
}
