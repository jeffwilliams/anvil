package main

import (
	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"image"
)

// drawBox draws the outline of a rectangular box into gtx
func drawBox(gtx layout.Context, w, h, strokewidth float32) clip.Stack {

	// Clipping paths drawn clockwise fall inside, counterclockwise outside.
	var path clip.Path
	path.Begin(gtx.Ops)
	path.Line(f32.Pt(w, 0))
	path.Line(f32.Pt(0, h))
	path.Line(f32.Pt(-w, 0))
	path.Line(f32.Pt(0, -h))

	path.Move(f32.Pt(strokewidth, strokewidth))

	w -= 2 * strokewidth
	h -= 2 * strokewidth

	path.Line(f32.Pt(0, h))
	path.Line(f32.Pt(w, 0))
	path.Line(f32.Pt(0, -h))
	path.Line(f32.Pt(-w, 0))
	return clip.Outline{Path: path.End()}.Op().Push(gtx.Ops)
}

// drawBox draws a filled box into gtx
func drawFilledBox(gtx layout.Context, w, h float32) clip.Stack {
	var path clip.Path

	path.Begin(gtx.Ops)
	path.Line(f32.Pt(w, 0))
	path.Line(f32.Pt(0, h))
	path.Line(f32.Pt(-w, 0))
	path.Line(f32.Pt(0, -h))

	return clip.Outline{Path: path.End()}.Op().Push(gtx.Ops)
}

type gtxOps struct {
	gtx layout.Context
	pt  f32.Point
}

// Pushes an offset to the GIO stack
func (s gtxOps) offset(x, y int) op.TransformStack {
	return op.Offset(image.Point{x, y}).Push(s.gtx.Ops)
}
