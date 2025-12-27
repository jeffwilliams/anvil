package main

import (
	"image/color"
	"math"
	"sort"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op/paint"
	"github.com/jeffwilliams/anvil/editor/internal/slice"
)

// Layer has no tag. It contains columns.

type Layer struct {
	Name string
	Cols                 []*Col
	layout               layerLayouter
	unpositioned, remove []*Col
	hspace               float32
	hspaceLastLayout     float32
	Scheduler            *Scheduler
	workChan             chan Work
}

func NewLayer(style Style) *Layer {
	l := &Layer{
		layout: layerLayouter{
			layouter: layouter{
				lineSpacing: style.LineSpacing,
				fontStyles:  style.Fonts,
			},
			style: style,
		},
	}
	l.layout.layer = l
	return l
}

type layerLayouter struct {
	layouter
	gtx   layout.Context
	layer *Layer
	style Style
}

func (l *Layer) NewCol() *Col {
	col := l.newCol()
	l.AddCol(col)
	return col
}

func (l *Layer) AddCol(col *Col) {
	col.layer = l
	if len(l.Cols) == 0 {
		col.LeftX = 0
		l.Cols = append(l.Cols, col)
	} else {
		l.unpositioned = append(l.unpositioned, col)
	}
}

// NewColDontPosition creates a new column like NewCol, but the caller is expected
// to manually position it.
func (l *Layer) NewColDontPosition() *Col {
	col := l.newCol()
	l.Cols = append(l.Cols, col)
	return col
}

func (l *Layer) NewWindow(col *Col) *Window {
	if len(l.Cols) == 0 {
		return nil
	}

	log(LogCatgLayer, "Layer.NewWindow: col is %p\n", col)
	if col != nil {
		return col.NewWindow()
	}

	cols := l.VisibleCols()
	if len(cols) == 0 {
		return nil
	}
	leastPopulated := cols[0]
	count := math.MaxInt
	for _, c := range cols {
		if len(c.Windows) < count {
			leastPopulated = c
			count = len(c.Windows)
		}
	}

	w := leastPopulated.NewWindow()
	return w
}

func (l *Layer) newCol() *Col {
	col := NewCol(l.layout.style)
	col.layer = l
	col.Scheduler = l.Scheduler
	col.workChan = l.workChan
	return col
}

// Layout handles events and draws the editor.
func (l *Layer) Layout(gtx layout.Context) {
	l.hspaceLastLayout = l.hspace
	l.hspace = float32(gtx.Constraints.Max.X)

	l.positionCols()

	l.layout.layout(gtx)
	l.removeColsMarkedForRemoval()
}

func (l *Layer) setConstraintsToColWidth(gtx *layout.Context, colIndex int) {
	sz := l.colWidth(colIndex)

	gtx.Constraints.Max.X = int(sz)
}

func (l *Layer) colWidth(colIndex int) float32 {
	cols := l.VisibleCols()
	ps := l.asPackables(cols)
	p := NewPacker(0, l.hspace, ps)
	sz := p.ItemSize(colIndex)

	return sz
}

func (l *layerLayouter) layout(gtx layout.Context) {
	l.gtx = gtx

	l.fillBackground(gtx)

	// Already saves stack state
	l.layoutCols()

	l.gtx = layout.Context{}
}

func (l *layerLayouter) fillBackground(gtx layout.Context) {
	paint.ColorOp{Color: color.NRGBA(l.style.BodyBgColor)}.Add(gtx.Ops)
	st := drawFilledBox(gtx, float32(gtx.Constraints.Max.X), float32(gtx.Constraints.Max.Y))
	paint.PaintOp{}.Add(gtx.Ops)
	st.Pop()

}

func (l *layerLayouter) layoutCols() {

	processEvents := func() (retry bool) {
		lastColX := -10000
		cols := l.layer.VisibleCols()
		for i, c := range cols {
			if c.LeftX < lastColX {
				log(LogCatgLayer, "The cols are not sorted in ascending X coordinate")
				retry = true
				return
			}

			lastColX = c.LeftX
			l.layer.setConstraintsToColWidth(&l.gtx, i)
			c.HandleEvents(l.gtx)
		}
		return
	}

	success := false
	for i := 0; i < 3; i++ {
		retry := processEvents()
		// Processing events might re-arrange the columns. In that case
		// try the layout again from the start.
		if !retry {
			success = true
			break
		}
	}

	if !success {

		cols := l.layer.VisibleCols()
		for i, c := range cols {
			log(LogCatgLayer, "col %d: left is %d", i, c.LeftX)
		}

		panic("The cols are not sorted in ascending X coordinate")
	}

	cols := l.layer.VisibleCols()

	// The event handling may have
	// changed the position of one of the columns, so we need to
	// first process those events, and then only later
	// draw the columns. We can't "layout" (handle events and draw) each column
	// in order because we could draw some of the columns then a later one changes
	// position and affects the width of the previously drawn columns.
	for i, c := range cols {
		l.layer.setConstraintsToColWidth(&l.gtx, i)
		c.DrawAndListenForEvents(l.gtx)
	}

}

func (l *Layer) positionCols() {
	l.packNewCols()
	l.spaceColsEvenlyIfWidthChanged()
}

func (l *Layer) packNewCols() {
	if len(l.unpositioned) == 0 {
		return
	}

	log(LogCatgLayer, "editor: Positioning columns\n")

	ps := l.asPackables(l.Cols)
	unp := l.asPackables(l.unpositioned)

	p := NewPacker(0, l.hspace, ps)
	ps = p.Pack(unp)

	l.setColsTo(ps)

	l.unpositioned = nil
}

func (l *Layer) spaceColsEvenlyIfWidthChanged() {
	if l.hspaceLastLayout == l.hspace {
		return
	}

	if l.hspace < l.hspaceLastLayout {
		ps := l.asPackables(l.Cols)
		p := NewPacker(0, l.hspace, ps)
		p.SpaceEvenly()
	}
}

func (l *Layer) asPackables(a []*Col) []Packable {
	ps := make([]Packable, len(a))
	for i := 0; i < len(a); i++ {
		ps[i] = a[i]
	}
	sort.SliceStable(ps, func(i, j int) bool {
		return ps[i].PackingCoord() < ps[j].PackingCoord()
	})
	return ps
}

func (l *Layer) setColsTo(ps []Packable) {
	for len(l.Cols) < len(ps) {
		l.Cols = append(l.Cols, nil)
	}

	for i := 0; i < len(ps); i++ {
		l.Cols[i] = ps[i].(*Col)
	}
}

func (l *Layer) bestColForXCoord(absoluteX int) *Col {
	cols := l.VisibleCols()
	for i, c := range cols {
		d := 0
		if i < len(cols)-1 {
			d = cols[i+1].LeftX
		}
		log(LogCatgLayer, "Editor.bestColForXCoord: absoluteX=%d, col %d %p ends at %d\n", absoluteX, i, c, d)
		if i >= len(cols)-1 || absoluteX < cols[i+1].LeftX {
			return c
		}
	}
	return cols[len(cols)-1]
}

func (l *Layer) markForRemoval(c *Col) {
	l.remove = append(l.remove, c)
}

func (l *Layer) removeColsMarkedForRemoval() {
	if l.remove == nil || len(l.remove) == 0 {
		return
	}

	for _, c := range l.remove {
		l.removeColumn(c)
	}
	l.remove = nil

	l.ensureFirstVisibleColIsLeftJustified()
}

func (l *Layer) ensureFirstVisibleColIsLeftJustified() {
	if len(l.Cols) > 0 {
		for _, c := range l.Cols {
			if c.Visible() {
				c.LeftX = 0
				return
			}
		}
	}
}

func (l *Layer) moveWindowBy(w *Window, off f32.Point, absoluteX float32) {
	// This is meant to find the right column the window has been moved to.
	cols := l.VisibleCols()
	for i, c := range cols {
		if i >= len(cols) || absoluteX < float32(cols[i+1].LeftX) {
			c.moveWindowBy(w, off)
			break
		}
	}
}

func (l *Layer) moveColBy(c *Col, off f32.Point) {
	ps := l.asPackables(l.VisibleCols())
	p := NewPacker(0, l.hspace, ps)
	movedPs := p.MoveTo(c, float32(c.LeftX)+off.X)

	newCols := make([]*Col, 0, len(l.Cols))
	for _, c := range l.Cols {
		if !c.Visible() {
			newCols = append(newCols, c)
		}
	}
	for _, c := range movedPs {
		newCols = append(newCols, c.(*Col))
	}
	l.Cols = newCols
}

func (l *Layer) VisibleCols() []*Col {
	r := make([]*Col, 0, len(l.Cols))
	for _, c := range l.Cols {
		if c.Visible() {
			r = append(r, c)
		}
	}
	return r
}

func (l *Layer) NumVisibleCols() int {
	i := 0
	for _, c := range l.Cols {
		if c.Visible() {
			i++
		}
	}
	return i
}

func (l *Layer) removeColumn(col *Col) {
	col.Clear()

	match := func(i int) bool {
		log(LogCatgLayer, "Layer.Delcol: compare %p to needle %p\n", l.Cols[i], col)
		return l.Cols[i] == col
	}
	l.Cols = slice.RemoveFirstMatchFromSlicePreserveOrder(l.Cols, match).([]*Col)
}

func (l *Layer) RepositionCol(col *Col) {
	match := func(i int) bool {
		return l.Cols[i] == col
	}

	l.Cols = slice.RemoveFirstMatchFromSlicePreserveOrder(l.Cols, match).([]*Col)
	l.unpositioned = append(l.unpositioned, col)
}

func (l *Layer) Clear() {
	l.Cols = nil
}

func (l *Layer) SetFirstHiddenColVisible() {
	for _, c := range l.Cols {
		if !c.Visible() {
			c.SetVisible(true)
			break
		}
	}
}
