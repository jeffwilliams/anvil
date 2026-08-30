package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"math"
	"sort"

	"gioui.org/f32"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"github.com/jeffwilliams/anvil/editor/internal/slice"
)

// Layer has no tag. It contains columns.

type Layer struct {
	Name string
	Cols []*Col
	// visibleCols is the X position of the left of each visible column
	visibleCols          []colPosition
	leftVisibleCol       int
	layout               layerLayouter
	unpositioned, remove []*Col
	unpositionedHint     positionHint
	hspace               float32
	hspaceLastLayout     float32
	Scheduler            *Scheduler
	workChan             chan Work
}

type positionHint int

const (
	positionHintNone positionHint = iota
	positionHintEnd
)

type colPosition struct {
	leftX int
	col   *Col
}

func (p colPosition) PackingCoord() float32 {
	return float32(p.leftX)
}

func (p *colPosition) SetPackingCoord(x float32) {
	p.leftX = int(x)
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
	gtxOps
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

func (l *Layer) NewColWithHint(hint positionHint) *Col {
	col := l.newCol()
	l.AddColWithHint(col, hint)
	return col
}

func (l *Layer) AddCol(col *Col) {
	col.layer = l
	if len(l.Cols) == 0 {
		l.visibleCols = []colPosition{{0, col}}
		l.Cols = append(l.Cols, col)
	} else {
		l.unpositioned = append(l.unpositioned, col)
	}
}

func (l *Layer) AddColWithHint(col *Col, hint positionHint) {
	l.unpositionedHint = hint
	l.AddCol(col)
}

// NewColDontPosition creates a new column like NewCol, but the caller is expected
// to manually position it.
func (l *Layer) NewColDontPosition() *Col {
	col := l.newCol()
	l.Cols = append(l.Cols, col)
	return col
}

func (l *Layer) NewWindow(col *Col) *Window {
	c := l.bestColumnForNewWindow(col)
	return c.NewWindow()
}

func (l *Layer) bestColumnForNewWindow(hint *Col) *Col {
	if len(l.Cols) == 0 {
		return nil
	}

	if hint != nil {
		return hint
	}

	cols := l.visibleCols
	if len(cols) == 0 {
		return nil
	}
	leastPopulated := cols[0]
	count := math.MaxInt
	for _, c := range cols {
		if len(c.col.Windows) < count {
			leastPopulated = c
			count = len(c.col.Windows)
		}
	}

	return leastPopulated.col
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
	// TODO: cache the result of asPackables (or cache it within the function itself)
	// it generates a lot of garbage
	ps := l.asPackables(l.visibleCols)
	p := NewPacker(0, l.hspace, ps)
	sz := p.ItemSize(colIndex)

	return sz
}

func (l *layerLayouter) layout(gtx layout.Context) {
	l.gtx = gtx
	l.gtxOps.gtx = gtx
	l.fillBackground(gtx)

	l.handleEvents(gtx)
	l.listenForEvents(gtx)
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

func (l *layerLayouter) handleEvents(gtx layout.Context) {
	for {
		e, ok := gtx.Event(pointer.Filter{
			Target:  l,
			Kinds:   pointer.Scroll,
			ScrollX: pointer.ScrollRange{-100, 100},
			ScrollY: pointer.ScrollRange{-100, 100},
		})

		if !ok {
			break
		}

		l.handleEvent(gtx, e)
	}
}

func (l *layerLayouter) handleEvent(gtx layout.Context, ev event.Event) {
	switch e := ev.(type) {
	case pointer.Event:
		l.handlePointerEvent(gtx, &e)
	}
}

func (l *layerLayouter) handlePointerEvent(gtx layout.Context, ev *pointer.Event) {
	// The layer handles scroll events the same as how editables do, for scrolling the layer or
	// columns. This is needed in both places in case you try and scroll the layers or
	// columns when the mouse is over an empty column. Since there is no editable to
	// handle the event, the layer handles it instead.

	if ev.Kind != pointer.Scroll {
		return
	}

	if ev.Modifiers.Contain(key.ModCtrl) && ev.Modifiers.Contain(key.ModShift) {
		fmt.Printf("l scroll event: %v\n", ev)
		if ev.Scroll.Y <= 0 {
			l.layer.WidenView()
		} else {
			l.layer.NarrowView()
		}
		return
	}

	if ev.Modifiers.Contain(key.ModCtrl) {
		amt := -1
		if ev.Scroll.Y <= 0 {
			amt = 1
		}
		editor.ActivateLayerRelativeToCurrent(amt)
		editor.SignalRedrawRequired()
		return
	}

	if ev.Modifiers.Contain(key.ModShift) {
		amt := -1
		if ev.Scroll.X >= 0 {
			amt = 1
		}
		editor.ScrollColsInActiveLayer(amt)
		editor.SignalRedrawRequired()
		return
	}
}

func (l *layerLayouter) listenForEvents(gtx layout.Context) {
	r := image.Rectangle{Max: gtx.Constraints.Max}
	st := clip.Rect(r).Push(gtx.Ops)
	event.Op(gtx.Ops, l)
	st.Pop()
}

func (l *layerLayouter) layoutCols() {
	log(LogCatgLayer, "layerLayouter: layoutCols called\n")

	processEvents := func() (retry bool) {
		lastColX := -10000
		visibleCols := l.layer.visibleCols
		for i, x := range visibleCols {
			if x.leftX < lastColX {
				log(LogCatgLayer, "The cols are not sorted in ascending X coordinate")
				retry = true
				return
			}

			lastColX = x.leftX
			l.layer.setConstraintsToColWidth(&l.gtx, i)

			if l.layer.leftVisibleCol+i >= len(l.layer.Cols) {
				break
			}

			col := l.layer.Cols[l.layer.leftVisibleCol+i]

			l.gtx.Values["offset"].(*OffsetStack).PushWithPurpose(image.Pt(int(x.leftX), 0), fmt.Sprintf("column %d left", col.Id))
			col.HandleEvents(l.gtx)
			l.gtx.Values["offset"].(*OffsetStack).Pop()
		}
		return
	}

	for i, _ := range l.layer.visibleCols {
		if l.layer.leftVisibleCol+i < len(l.layer.Cols) {
			col := l.layer.Cols[l.layer.leftVisibleCol+i]
			col.layedOutColIndex = i
		}
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
		for i, x := range l.layer.visibleCols {
			log(LogCatgLayer, "visible col %d: left is %d", i, x.leftX)
		}

		panic("The cols are not sorted in ascending X coordinate")
	}

	if len(l.layer.visibleCols) == 0 {
		return
	}

	// The event handling may have
	// changed the position of one of the columns, so we need to
	// first process those events, and then only later
	// draw the columns. We can't "layout" (handle events and draw) each column
	// in order because we could draw some of the columns then a later one changes
	// position and affects the width of the previously drawn columns.
	for i, x := range l.layer.visibleCols {
		l.layer.setConstraintsToColWidth(&l.gtx, i)
		if l.layer.leftVisibleCol+i < len(l.layer.Cols) {
			col := l.layer.Cols[l.layer.leftVisibleCol+i]
			st := l.offset(int(x.leftX), 0)
			l.gtx.Values["offset"].(*OffsetStack).PushWithPurpose(image.Pt(int(x.leftX), 0), fmt.Sprintf("column %d left", col.Id))
			col.DrawAndListenForEvents(l.gtx)
			l.gtx.Values["offset"].(*OffsetStack).Pop()
			st.Pop()
		}
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

	log(LogCatgLayer, "layer: Positioning columns. %d existing, %d unpositioned\n", len(l.visibleCols), len(l.unpositioned))

	ps := l.asPackables(l.visibleCols)

	switch l.unpositionedHint {
	case positionHintEnd:
		for _, c := range l.unpositioned {
			ps = append(ps, &colPosition{0, c})
		}
		p := NewPacker(0, l.hspace, ps)
		ps = p.SpaceEvenly()
	default:
		unp := make([]Packable, len(l.unpositioned))
		for i, c := range l.unpositioned {
			unp[i] = &colPosition{0, c}
		}

		p := NewPacker(0, l.hspace, ps)
		ps = p.Pack(unp)
	}

	l.Cols = l.injectDisplayColsIntoAllCols(ps)
	l.setVisibleCols(ps)
	l.unpositioned = nil
	l.unpositionedHint = positionHintNone
}

func (l *Layer) injectDisplayColsIntoAllCols(displayCols []Packable) []*Col {
	ln := len(l.Cols) + len(l.unpositioned)
	newCols := make([]*Col, ln)

	for i, j, k := 0, 0, 0; i < len(newCols); {
		if i < l.leftVisibleCol {
			newCols[i] = l.Cols[j]
			i++
			j++
		} else if k < len(displayCols) {
			newCols[i] = displayCols[k].(*colPosition).col
			i++
			k++
		} else {
			newCols[i] = l.Cols[j+len(l.visibleCols)]
			i++
			j++
		}
	}

	return newCols
}

func (l *Layer) spaceColsEvenlyIfWidthChanged() {
	if l.hspaceLastLayout == l.hspace {
		return
	}

	if l.hspace < l.hspaceLastLayout {
		ps := l.asPackables(l.visibleCols)
		p := NewPacker(0, l.hspace, ps)
		p.SpaceEvenly()
	}
}

func (l *Layer) asPackables(a []colPosition) []Packable {
	ps := make([]Packable, len(a))
	for i := 0; i < len(a); i++ {
		ps[i] = &a[i]
	}
	sort.SliceStable(ps, func(i, j int) bool {
		return ps[i].PackingCoord() < ps[j].PackingCoord()
	})
	return ps
}

func (l *Layer) setVisibleCols(ps []Packable) {
	l.visibleCols = make([]colPosition, len(ps))
	for i := 0; i < len(ps); i++ {
		l.visibleCols[i] = *(ps[i].(*colPosition))
	}
}

func (l *Layer) bestColForXCoord(absoluteX int) *Col {
	for i, pos := range l.visibleCols {
		d := 0
		if i < len(l.visibleCols)-1 {
			d = l.visibleCols[i+1].leftX
		}
		log(LogCatgLayer, "Editor.bestColForXCoord: absoluteX=%d, col %d %p ends at %d\n", absoluteX, i, pos.col, d)
		if i >= len(l.visibleCols)-1 || absoluteX < l.visibleCols[i+1].leftX {
			return pos.col
		}
	}
	return l.visibleCols[len(l.visibleCols)-1].col
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
	if len(l.visibleCols) == 0 {
		return
	}

	l.visibleCols[0].leftX = 0
}

func (l *Layer) moveWindowBy(w *Window, off f32.Point, absoluteX float32) {
	// This is meant to find the right column the window has been moved to.
	for i, pos := range l.visibleCols {
		if i >= len(l.visibleCols) || absoluteX < float32(l.visibleCols[i+1].leftX) {
			pos.col.moveWindowBy(w, off)
			break
		}
	}
}

func (l *Layer) moveColBy(c *Col, off f32.Point) {
	posOfCol := &l.visibleCols[c.layedOutColIndex]
	ps := l.asPackables(l.visibleCols)

	newPos := float32(posOfCol.leftX) + off.X

	if newPos <= 0 && len(l.visibleCols) > 1 {
		l.moveColOffscreen(c, Left)
		return
	}

	tolerance := float32(l.layout.gtx.Metric.Dp(l.layout.style.GutterWidth))
	if newPos+tolerance >= l.hspace && len(l.visibleCols) > 1 {
		// Moved offscreen
		l.moveColOffscreen(c, Right)
		return
	}

	p := NewPacker(0, l.hspace, ps)
	movedPs := p.MoveTo(posOfCol, newPos)

	l.setVisibleCols(movedPs)

	j := 0
	for i := l.leftVisibleCol; j < len(movedPs); i++ {
		l.Cols[i] = movedPs[j].(*colPosition).col
		j++
	}
}

func listCols(cols []*Col) string {
	var buf bytes.Buffer
	for _, c := range cols {
		buf.WriteString("  ")
		buf.WriteString(c.Name())

		if !c.Visible() {
			buf.WriteString(" (hidden)")
		}
		buf.WriteRune('\n')
	}
	return buf.String()
}

func listColsFromPos(cols []colPosition) string {
	var buf bytes.Buffer
	for _, c := range cols {
		buf.WriteString("  ")
		buf.WriteString(c.col.Name())

		if !c.col.Visible() {
			buf.WriteString(" (hidden)")
		}
		buf.WriteRune('\n')
	}
	return buf.String()
}

func (l *Layer) moveColOffscreen(c *Col, direction horizontalDirection) {
	// Take the column out of the displayed columns and move it
	// to the new position in the columns list just left of the displayed columns
	l.visibleCols = slice.RemoveFirstMatchFromSlicePreserveOrder(l.visibleCols, func(i int) bool {
		return i == c.layedOutColIndex
	}).([]colPosition)

	oldIndex := l.leftVisibleCol + c.layedOutColIndex
	newIndex := l.leftVisibleCol

	if direction == Right {
		newIndex = l.leftVisibleCol + len(l.visibleCols)
	}

	if direction == Left {
		for d := newIndex + 1; d <= oldIndex; d++ {
			l.Cols[d] = l.Cols[d-1]
		}
	} else {
		for d := oldIndex; d < newIndex; d++ {
			l.Cols[d] = l.Cols[d+1]
		}
	}

	for i, c := range l.visibleCols {
		c.col.layedOutColIndex = i
	}

	l.Cols[newIndex] = c

	if direction == Left {
		l.leftVisibleCol++
	}

	c.layedOutColIndex = -1

	l.ensureFirstVisibleColIsLeftJustified()
}

func (l *Layer) removeColumn(col *Col) {
	col.Clear()

	match := func(i int) bool {
		log(LogCatgLayer, "Layer.Delcol: compare %p to needle %p\n", l.Cols[i], col)
		return l.Cols[i] == col
	}
	l.Cols = slice.RemoveFirstMatchFromSlicePreserveOrder(l.Cols, match).([]*Col)

	matchPos := func(i int) bool {
		return l.visibleCols[i].col == col
	}
	l.visibleCols = slice.RemoveFirstMatchFromSlicePreserveOrder(l.visibleCols, matchPos).([]colPosition)
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
	l.visibleCols = nil
}

func (l *Layer) leftXOfVisibleColumn(i int) int {
	if i >= 0 && i < len(l.visibleCols) {
		return l.visibleCols[i].leftX
	}
	return -1
}

func (l *Layer) visibleColLeftOf(layedOutColIndex int) *Col {
	i := layedOutColIndex - 1
	if i < 0 || i >= len(l.visibleCols) {
		return nil
	}
	return l.visibleCols[i].col
}

func (l *Layer) visibleColRightOf(layedOutColIndex int) *Col {
	i := layedOutColIndex + 1
	if i < 0 || i >= len(l.visibleCols) {
		return nil
	}
	return l.visibleCols[i].col
}

// scrollCols scrolls the view of visible columns. If n > 0 the view is scrolled right (columns to the right
// that are not visible will become visible). If n < 0 the view is scrolled left.
func (l *Layer) scrollCols(n int) {
	// Seems like left scrolls right, and right scrolls left. Debug.
	l.leftVisibleCol += n

	if l.leftVisibleCol < 0 {
		l.leftVisibleCol = 0
	}

	if l.leftVisibleCol+len(l.visibleCols) > len(l.Cols) {
		l.leftVisibleCol = len(l.Cols) - len(l.visibleCols)
	}

	for i := range l.visibleCols {
		l.visibleCols[i].col = l.Cols[l.leftVisibleCol+i]
	}
}

func (l *Layer) ColVisible(col *Col) bool {
	for _, c := range l.visibleCols {
		if c.col == col {
			return true
		}
	}
	return false
}

func (l *Layer) scrollUntilVisible(col *Col) {
	if l.ColVisible(col) {
		return
	}

	colIndex := -1
	for i, c := range l.Cols {
		if c == col {
			colIndex = i
			break
		}
	}
	if colIndex < 0 {
		return
	}

	delta := 0
	if colIndex < l.leftVisibleCol {
		delta = colIndex - l.leftVisibleCol
	} else {
		delta = colIndex - (l.leftVisibleCol + len(l.visibleCols) - 1)
	}
	l.scrollCols(delta)
}

func (l *Layer) AnyHiddenCol() *Col {
	if l.leftVisibleCol+len(l.visibleCols) < len(l.Cols) {
		return l.Cols[l.leftVisibleCol+len(l.visibleCols)]
	}

	if l.leftVisibleCol > 0 {
		return l.Cols[l.leftVisibleCol-1]
	}

	return nil
}

// ExpandView makes one more column visible. If there are hidden columns, the next one to the right or left is displayed along with the existing ones. If there are no hidden columns, a new column is created to the right.
func (l *Layer) WidenView() {
	if len(l.visibleCols) == len(l.Cols) {
		col := l.NewColWithHint(positionHintEnd)
		col.Tag.SetTextStringNoUndo(settings.Layout.ColumnTag)
		editor.SignalRedrawRequired()
		return
	}

	var ps []Packable
	if l.leftVisibleCol+len(l.visibleCols) < len(l.Cols) {
		// Make first hidden column to the right visible
		c := l.Cols[l.leftVisibleCol+len(l.visibleCols)]
		ps = l.asPackables(l.visibleCols)
		ps = append(ps, &colPosition{0, c})
	} else if l.leftVisibleCol > 0 {
		// Make first hidden column to the left visible
		c := l.Cols[l.leftVisibleCol-1]
		ps2 := l.asPackables(l.visibleCols)
		ps = make([]Packable, len(ps2)+1)
		ps[0] = &colPosition{0, c}
		copy(ps[1:], ps2)
		l.leftVisibleCol--
	}

	p := NewPacker(0, l.hspace, ps)
	ps = p.SpaceEvenly()
	l.setVisibleCols(ps)
}

func (l *Layer) NarrowView() {
	if len(l.visibleCols) <= 1 {
		return
	}

	l.visibleCols = l.visibleCols[:len(l.visibleCols)-1]
	ps := l.asPackables(l.visibleCols)
	p := NewPacker(0, l.hspace, ps)
	ps = p.SpaceEvenly()
	l.setVisibleCols(ps)
}
