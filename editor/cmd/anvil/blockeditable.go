package main

import (
	"bytes"
	"fmt"
	"image"
	"io"
	"io/ioutil"
	"path/filepath"
	"strings"

	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"

	"gioui.org/f32"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/io/transfer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
)

// blockEditable is an editable that is displayed as a block with a background
type blockEditable struct {
	editable
	dims     layout.Dimensions
	style    blockStyle
	maximize bool
	bgcolor  color.NRGBA
	bgimage  backgroundImage
}

type blockStyle struct {
	StandardBgColor              color.NRGBA
	ErrorBgColor                 color.NRGBA
	ErrorFlashBgColor            color.NRGBA
	StandardFgColor              color.NRGBA
	ErrorFgColor                 color.NRGBA
	ErrorFlashFgColor            color.NRGBA
	PathBasenameColor            color.NRGBA
	ErrorsPathBasenameColor      color.NRGBA
	ErrorsFlashPathBasenameColor color.NRGBA
}

func (t *blockEditable) Init(style blockStyle, editableStyle editableStyle, scheduler *Scheduler) {
	t.style = style
	t.editable.Init(editableStyle)
	t.editable.tag = t
	t.editable.Scheduler = scheduler
	t.bgcolor = style.StandardBgColor
}

func (t *blockEditable) layout(gtx layout.Context) layout.Dimensions {
	t.HandleEvents(gtx)
	return t.dims
}

func (t *blockEditable) HandleEvents(gtx layout.Context) {
	t.prepareForLayout()
	drawn := false

	for {
		ev, ok := t.nextEvent(gtx)

		if !ok {
			break
		}

		t.handleEvent(gtx, ev)

		t.DrawAndListenForEvents(gtx)
		drawn = true
	}

	if !drawn {
		t.DrawAndListenForEvents(gtx)
	}
}

func (t *blockEditable) nextEvent(gtx layout.Context) (ev event.Event, ok bool) {
	pf := pointer.Filter{
		Target:  t,
		Kinds:   pointer.Press | pointer.Drag | pointer.Release | pointer.Scroll,
		ScrollX: pointer.ScrollRange{-100, -100},
		ScrollY: pointer.ScrollRange{100, 100},
	}

	// Since no keys are specified, this matches events for all keys (a catch-all)
	kf := key.Filter{Focus: t, Optional: key.ModCtrl | key.ModShift | key.ModAlt | key.ModCommand}
	// Tab is a special key in GIO used to switch focus between widgets. It is not matched by catch-all
	// filters, so we specifically request it.
	tabf := key.Filter{Focus: t, Name: key.NameTab, Optional: key.ModCtrl | key.ModShift | key.ModAlt | key.ModCommand}

	// This matches events for EditEvents
	ff := key.FocusFilter{Target: t}

	// For clipboard
	tf := transfer.TargetFilter{Target: t, Type: "application/text"}

	ev, ok = gtx.Event(pf, kf, ff, tabf, tf)
	return
}

func (t *blockEditable) handleEvent(gtx layout.Context, ev event.Event) {
	switch e := ev.(type) {
	case pointer.Event:
		t.Pointer(gtx, &e)
	case key.Event:
		t.Key(gtx, &e)
	case key.EditEvent:
		t.InsertTextAndHandleKeys(gtx, e.Text)
	case key.FocusEvent:
		/*action := "set to"
		  if !e.Focus {
		    action = "cleared from"
		  }
		  log(LogCatgEd,"blockEditable.handleEvents: focus %s %p\n", action, t)*/
		t.FocusChanged(gtx, &e)
	case transfer.DataEvent:
		// Clipboard
		data := e.Open()
		t.readTextFromClipboard(data)
		data.Close()
	}
}

func (t *blockEditable) readTextFromClipboard(data io.ReadCloser) {
	// Clipboard
	b, err := ioutil.ReadAll(data)
	if err != nil {
		log(LogCatgEd, "blockEditable.readTextFromClipboard: error reading from clipboard: %v", err)
		return
	}

	var buf bytes.Buffer
	for _, t := range editor.LastSelectionsWrittenToClipboard() {
		buf.WriteString(t)
	}

	text := string(b)
	if t.attemptBlockPaste(text) {
		return
	}

	t.InsertTextAndSelect(fixLineEndings(text))

}

func (t *blockEditable) DrawAndListenForEvents(gtx layout.Context) layout.Dimensions {
	t.relayout(gtx)
	t.dims = t.draw(gtx)
	t.listenForEvents(gtx)
	return t.dims
}

func (t *blockEditable) listenForEvents(gtx layout.Context) {
	r := image.Rectangle{Max: t.dims.Size}
	stack := clip.Rect(r).Push(gtx.Ops)
	defer stack.Pop()

	event.Op(gtx.Ops, t)
}

func (t *blockEditable) draw(gtx layout.Context) layout.Dimensions {
	if t.maximize {
		return t.drawMaximized(gtx)
	} else {
		return t.drawTight(gtx)
	}
}

func (t *blockEditable) drawMaximized(gtx layout.Context) layout.Dimensions {
	r := clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, gtx.Constraints.Max.Y)}
	stack := r.Push(gtx.Ops)
	defer stack.Pop()
	t.drawBackground(gtx)

	t.editable.draw(gtx)
	return layout.Dimensions{Size: image.Point{X: gtx.Constraints.Max.X, Y: gtx.Constraints.Max.Y}}
}

func (t *blockEditable) drawBackground(gtx layout.Context) {
	if t.bgimage.img == nil {
		paint.ColorOp{Color: t.bgcolor}.Add(gtx.Ops)
		paint.PaintOp{}.Add(gtx.Ops)
		return
	}

	if t.bgimage.scalingType != dontScale {
		var scale f32.Affine2D

		switch t.bgimage.scalingType {
		case scaleToPercent:
			scale = scale.Scale(f32.Point{0, 0}, f32.Point{t.bgimage.fraction, t.bgimage.fraction})
		case scaleToFitWindow:
			f := float32(gtx.Constraints.Max.X) / float32(t.bgimage.img.Bounds().Size().X)
			scale = scale.Scale(f32.Point{0, 0}, f32.Point{f, f})
		}

		tform := op.Affine(scale)
		//tform.Add(gtx.Ops)
		stack := tform.Push(gtx.Ops)
		defer stack.Pop()
	}

	imageOp := paint.NewImageOp(t.bgimage.img)
	imageOp.Filter = paint.FilterNearest
	imageOp.Add(gtx.Ops)

	paint.PaintOp{}.Add(gtx.Ops)
}

func (t *blockEditable) drawTight(gtx layout.Context) layout.Dimensions {
	// We don't know how many lines the editable is (how big it is) until we draw it, but we also
	// want to fill the background before drawing the editable. To fill the background we need to know
	// how big it is. So we draw it but record the drawing operations into a macro instead of performing
	// them, then we fill the background and replay the macro.
	macro := op.Record(gtx.Ops)
	dims := t.editable.draw(gtx)

	minHeight := t.editable.layouter.lineHeight()
	if minHeight > 0 && dims.Size.Y < minHeight {
		dims.Size.Y = minHeight
	}
	c := macro.Stop()

	//log(LogCatgEd,"blockEditable.drawTight: dimensions for %s are computed to be %#v\n", t.editable.label, tagDimensions)

	r := clip.Rect{Max: image.Pt(dims.Size.X, dims.Size.Y)}
	stack := r.Push(gtx.Ops)
	defer stack.Pop()
	t.drawBackground(gtx)

	c.Add(gtx.Ops)

	return layout.Dimensions{Size: image.Point{X: gtx.Constraints.Max.X, Y: dims.Size.Y}}
}

func fixLineEndings(s string) string {
	return strings.ReplaceAll(s, "\r\n", "\n")
}

func (t *blockEditable) SetStyle(style blockStyle, editableStyle editableStyle) {
	t.style = style
	t.editable.SetStyle(editableStyle)
	t.bgcolor = style.StandardBgColor
}

type backgroundImage struct {
	img         image.Image
	filename    string
	scalingType scalingType
	// Fraction is the amount to scale in both dimentions. 1.0 is no scaling, 0.5 is 50%, etc.
	fraction float32
}

type scalingType int

const (
	dontScale scalingType = iota
	scaleToFitWindow
	scaleToPercent
)

func (b *backgroundImage) Load(path string) error {
	var ldr FileLoader
	contents, _, err := ldr.Load(path)

	if err != nil {
		return fmt.Errorf("Reading image failed: %v", err.Error())
	}

	if contents == nil {
		return fmt.Errorf("Reading image resulted in no data; is it a directory?")
	}

	var buf bytes.Buffer
	buf.Write(contents)
	var img image.Image
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".png":
		img, err = png.Decode(&buf)
	case ".jpg", ".jpeg":
		img, err = jpeg.Decode(&buf)
	case ".gif":
		img, err = gif.Decode(&buf)
	// TODO: Add .svg support using oksvg and rasterx
	default:
		return fmt.Errorf("Can't load images of type %s", ext)
	}

	if err != nil {
		return fmt.Errorf("Decoding image failed: %v", err.Error())
	}

	b.img = img
	b.filename = path
	b.scalingType = dontScale
	b.fraction = 1.0
	return nil
}
