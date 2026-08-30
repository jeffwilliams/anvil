package draw

import (
	"image/color"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
)

func Run(gtx layout.Context, ops []Op, consts map[Const]float32) {
	var path clip.Path

	setColor := func(op *Op) {
		r := uint8(op.Args[0].(float32))
		g := uint8(op.Args[1].(float32))
		b := uint8(op.Args[2].(float32))
		a := uint8(op.Args[3].(float32))
		colr := color.NRGBA{r, g, b, a}
		paint.ColorOp{Color: colr}.Add(gtx.Ops)
	}

	pt := func(x, y float32) f32.Point {
		xi := gtx.Metric.Dp(unit.Dp(int(x)))
		yi := gtx.Metric.Dp(unit.Dp(int(y)))
		return f32.Pt(float32(xi), float32(yi))
	}

	move := func(op *Op) {
		path.Move(pt(op.Args[0].(float32), op.Args[1].(float32)))
	}

	line := func(op *Op) {
		path.Line(pt(op.Args[0].(float32), op.Args[1].(float32)))
	}

	clos := func(op *Op) {
		path.Close()
		stack := clip.Outline{Path: path.End()}.Op().Push(gtx.Ops)
		paint.PaintOp{}.Add(gtx.Ops)
		stack.Pop()
	}

	for _, op := range ops {
		op.ResolveConsts(consts)

		switch op.Code {
		case Color:
			setColor(&op)
		case Begin:
			path.Begin(gtx.Ops)
		case Move:
			move(&op)
		case Line:
			line(&op)
		case Close:
			clos(&op)
		}
	}
}
