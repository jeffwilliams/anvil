package main

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"sort"
	"strings"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"github.com/jeffwilliams/anvil/editor/internal/slice"
)

type Col struct {
	Id      int
	Tag     Tag
	Windows []*Window
	// layedOutColIndex is the index in the visible columns in the layer of this column.
	// Each time the layer is drawn this value is set. Index 0 is the leftmost visible column,
	// 1 is the one to the right of that, and so on.
	layedOutColIndex int

	unpositioned, remove, resized, minimizedExcept, maximize, center []*Window

	// repackItemsBelowLimit is true when on the next layout we should
	// repack the windows so that any not visible because they are below the
	// bottom border of the column are moved up.
	doRepackItemsBelowLimit bool
	// spaceEvenly is true when on the next layout we should
	// space the windows evenly
	spaceEvenly      bool
	windowsMinimized bool
	maximizedWindow  *Window
	layoutBox        layoutBox
	layout           colLayouter
	layer            *Layer

	// vspace is the total vertical space avialable to windows inside this row
	vspace    float32
	Scheduler *Scheduler
	workChan  chan Work
}

type colLayouter struct {
	layouter
	gtx   layout.Context
	col   *Col
	style Style
	width int
	gtxOps
	layoutBoxDims layout.Dimensions
}

func NewCol(style Style) *Col {
	r := &Col{
		layout: colLayouter{
			style: style,
			layouter: layouter{
				fontStyles:  style.Fonts,
				lineSpacing: style.LineSpacing,
			},
		},
	}

	r.Id = application.colIdGenerator.Get()
	r.layoutBox.col = r
	r.layout.col = r
	executor := NewCommandExecutor(r)
	r.Tag.Init(nil, style.tagBlockStyle(), style.tagEditableStyle(), executor, r, r.Scheduler)
	r.Tag.label = "column"
	r.layoutBox.Init(style.layoutBoxStyle())
	return r
}

func (r *Col) NewWindow() *Window {
	w := NewWindow(r, r.layout.style)
	r.AddWindow(w)
	return w
}

func (r *Col) AddWindow(w *Window) {
	if r == nil {
		fmt.Printf("Col.AddWindow called with nil col\n")
	}
	w.col = r
	// Position the new window
	if r.Windows == nil || len(r.Windows) == 0 {
		w.TopY = 0
		r.Windows = append(r.Windows, w)
	} else {
		// TODO: if there is not enough space fail making this window?
		r.unpositioned = append(r.unpositioned, w)
	}
}

func (r *Col) NewWindowDontPosition() *Window {
	w := NewWindow(r, r.layout.style)
	r.Windows = append(r.Windows, w)
	return w
}

func (c *Col) HandleEvents(gtx layout.Context) {
	c.layout.HandleEvents(gtx)
}

func (c *Col) DrawAndListenForEvents(gtx layout.Context) {
	dims := c.layout.DrawAndListenForEvents(gtx)
	gtx.Constraints.Max.X = dims.Size.X

	rowHeaderHeight := float32(dims.Size.Y)
	minimizedWindowHeight := float32(c.layout.lineHeight() + gtx.Metric.Dp(c.layout.style.WinBorderWidth))

	vspaceOnLastLayout := c.vspace
	c.vspace = float32(gtx.Constraints.Max.Y) - rowHeaderHeight

	if vspaceOnLastLayout != 0 && vspaceOnLastLayout != c.vspace {
		c.adjustWindowsOnColumnHeightChange(vspaceOnLastLayout, c.vspace)
	}

	c.positionWindows(rowHeaderHeight)
	c.minimizeOtherWindowsExcept(minimizedWindowHeight)
	c.resizeWindows(minimizedWindowHeight)
	c.maximizeWindows(rowHeaderHeight)
	c.layout.setOffsetAndLayoutWindows(gtx, dims.Size.Y)
	c.removeWindowsMarkedForRemoval()
	c.centerWindowsMarkedForCentering()
	c.repackItemsBelowLimit(rowHeaderHeight)
	c.spaceWindowsEvenly(rowHeaderHeight)
}

func (c *Col) adjustWindowsOnColumnHeightChange(oldHeight, newHeight float32) {
	if c.maximizedWindow != nil {
		c.Maximize(c.maximizedWindow)
		return
	}
	if newHeight < oldHeight {
		c.doRepackItemsBelowLimit = true
	}
}

func (r *Col) setConstraintsToWindowHeight(gtx *layout.Context, winIndex int) {
	ps := r.asPackables(r.Windows)
	p := NewPacker(0, r.vspace, ps)
	sz := p.ItemSize(winIndex)

	gtx.Constraints.Max.Y = int(sz)
}

func (l *colLayouter) HandleEvents(gtx layout.Context) {
	gtx.Values["offset"].(*OffsetStack).PushWithPurpose(image.Pt(l.layoutBoxDims.Size.X, 0), fmt.Sprintf("layout box x for col %d", l.col.Id))
	defer gtx.Values["offset"].(*OffsetStack).Pop()

	l.gtx = gtx
	l.gtxOps.gtx = gtx

	l.handleLayoutBoxEvents(l.gtx)

	// We need an accurate width constraint so that we layout text correctly
	w := l.col.layoutBox.width(gtx)
	l.gtx.Constraints.Max.X -= w
	l.col.Tag.HandleEvents(l.gtx)
	l.gtx.Constraints.Max.X += w
}

func (l *colLayouter) DrawAndListenForEvents(gtx layout.Context) layout.Dimensions {
	l.gtx = gtx
	l.gtxOps.gtx = gtx
	l.width = l.gtx.Constraints.Max.X

	l.layoutBoxDims = l.drawLayoutBox(l.gtx)

	// Translate all later draw operations so they are to the right of the layoutBox
	defer l.offset(l.layoutBoxDims.Size.X, 0).Pop()
	l.gtx.Constraints.Max.X -= l.layoutBoxDims.Size.X
	tagDims := l.col.Tag.layout(l.gtx)
	l.gtx.Constraints.Max.X += l.layoutBoxDims.Size.X
	defer l.offset(-l.layoutBoxDims.Size.X, tagDims.Size.Y).Pop()

	// In case the tag takes up multiple lines, color in the part under
	// the layout box
	l.fillUnderLayoutBox(gtx, tagDims.Size.Y-l.layoutBoxDims.Size.Y)

	// Draw a line (border) under the header
	botBorderHeight := l.drawBottomBorder(l.gtx)

	defer l.offset(0, gtx.Metric.Dp(l.style.WinBorderWidth)).Pop()

	l.gtx = layout.Context{}
	l.gtxOps.gtx = layout.Context{}

	return layout.Dimensions{Size: image.Point{X: gtx.Constraints.Max.X, Y: tagDims.Size.Y + botBorderHeight}}
}

func (l *colLayouter) setOffsetAndLayoutWindows(gtx layout.Context, startY int) {
	l.gtx = gtx
	l.gtxOps.gtx = gtx
	l.width = l.gtx.Constraints.Max.X

	borderw := gtx.Metric.Dp(l.style.WinBorderWidth)

	st := l.offset(l.gtx.Constraints.Max.X-borderw, 0)
	l.drawRightBorder(l.gtx)
	st.Pop()
	l.gtx.Constraints.Max.X -= borderw

	defer l.offset(0, startY).Pop()
	gtx.Values["offset"].(*OffsetStack).PushWithPurpose(image.Pt(0, startY), fmt.Sprintf("window starty in col %d", l.col.Id))
	defer gtx.Values["offset"].(*OffsetStack).Pop()

	if len(l.col.Windows) > 0 {
		l.layoutWindows()
	}
}

func (l *colLayouter) handleLayoutBoxEvents(gtx layout.Context) {
	l.col.layoutBox.handleEvents(gtx)
}

func (l *colLayouter) drawLayoutBox(gtx layout.Context) layout.Dimensions {
	l.col.layoutBox.dims = l.col.layoutBox.draw(gtx)
	l.col.layoutBox.listenForEvents(gtx)
	return l.col.layoutBox.dims
}

func (l *colLayouter) fillUnderLayoutBox(gtx layout.Context, height int) {
	st := l.offset(0, -height)
	cst := clip.Rect{Max: image.Pt(gtx.Metric.Dp(l.style.GutterWidth), int(height))}.Push(gtx.Ops)
	paint.ColorOp{Color: color.NRGBA(l.style.BodyBgColor)}.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)
	cst.Pop()
	st.Pop()
}

func (l *colLayouter) layoutWindows() {
	lastWindowY := -10000
	for i, w := range l.col.Windows {
		if w.TopY < lastWindowY {
			log(LogCatgCol, "col %p: The windows stored in the row are not sorted in ascending Y coordinate. About to panic. The windows are:\n", l.col)
			l.col.printWindowPositions()
		}
		lastWindowY = w.TopY

		l.col.setConstraintsToWindowHeight(&l.gtx, i)
		w.Layout(l.gtx)
	}
}

func (l *colLayouter) drawBottomBorder(gtx layout.Context) (height int) {
	height = gtx.Metric.Dp(l.style.WinBorderWidth)
	paint.ColorOp{Color: color.NRGBA(l.style.WinBorderColor)}.Add(gtx.Ops)
	st := drawFilledBox(gtx, float32(gtx.Constraints.Max.X), float32(height))
	paint.PaintOp{}.Add(gtx.Ops)
	st.Pop()
	return
}

func (l *colLayouter) drawRightBorder(gtx layout.Context) {
	paint.ColorOp{Color: color.NRGBA(l.style.WinBorderColor)}.Add(gtx.Ops)
	st := drawFilledBox(gtx, float32(gtx.Metric.Dp(l.style.WinBorderWidth)), float32(gtx.Constraints.Max.Y))
	paint.PaintOp{}.Add(gtx.Ops)
	st.Pop()
}

func (r *Col) positionWindows(rowHeaderHeight float32) {
	if len(r.unpositioned) == 0 {
		return
	}

	for _, w := range r.Windows {
		w.centerBodyOnFirstCursorOrPrimarySelection()
	}

	ps := r.asPackables(r.Windows)
	unp := r.asPackables(r.unpositioned)

	p := NewPacker(rowHeaderHeight, r.vspace, ps)

	if r.maximizedWindow != nil {
		// Unmaximize all the windows so that this newly added window is packed in with the rest.
		ps = p.MinimizeAllExcept(r.maximizedWindow)
	}
	ps = p.Pack(unp)

	r.setWindowsTo(ps)

	r.unpositioned = nil
}

func (r *Col) resizeWindows(rowHeaderHeight float32) {
	if len(r.resized) == 0 {
		return
	}

	ps := r.asPackables(r.Windows)
	res := r.asPackables(r.resized)
	toCenter := r.copyWindows()

	p := NewPacker(rowHeaderHeight, r.vspace, ps)

	amt := r.layout.lineHeight() * 10
	for _, r := range res {
		ps = p.Grow(r, float32(amt))
	}

	r.setWindowsTo(ps)

	for _, w := range toCenter {
		w.centerBodyOnFirstCursorOrPrimarySelection()
	}

	r.resized = nil
}

func (r *Col) copyWindows() []*Window {
	rc := make([]*Window, len(r.Windows))
	copy(rc, r.Windows)
	return rc
}

func (r *Col) minimizeOtherWindowsExcept(rowHeaderHeight float32) {
	if len(r.minimizedExcept) == 0 {
		return
	}

	ps := r.asPackables(r.Windows)
	res := r.asPackables(r.minimizedExcept)
	toCenter := r.copyWindows()

	p := NewPacker(rowHeaderHeight, r.vspace, ps)

	for _, r := range res {
		ps = p.MinimizeAllExcept(r)
	}

	r.setWindowsTo(ps)

	for _, w := range toCenter {
		w.centerBodyOnFirstCursorOrPrimarySelection()
	}

	r.minimizedExcept = nil
	r.maximizedWindow = nil
}

func (r *Col) maximizeWindows(rowHeaderHeight float32) {
	if len(r.maximize) == 0 {
		return
	}

	ps := r.asPackables(r.Windows)
	res := r.asPackables(r.maximize)

	p := NewPacker(rowHeaderHeight, r.vspace, ps)

	for _, w := range res {
		ps = p.Maximize(w)
		if win, ok := w.(*Window); ok {
			r.maximizedWindow = (*Window)(win)
		}
	}

	r.setWindowsTo(ps)

	r.maximize = nil
}

func (r *Col) repackItemsBelowLimit(rowHeaderHeight float32) {
	if !r.doRepackItemsBelowLimit {
		return
	}

	ps := r.asPackables(r.Windows)
	p := NewPacker(rowHeaderHeight, r.vspace, ps)
	p.RepackItemsBelowLimit()
	r.setWindowsTo(ps)
	r.doRepackItemsBelowLimit = false
}

func (r *Col) spaceWindowsEvenly(rowHeaderHeight float32) {
	if !r.spaceEvenly {
		return
	}

	ps := r.asPackables(r.Windows)
	p := NewPacker(rowHeaderHeight, r.vspace, ps)
	p.SpaceEvenly()
	r.setWindowsTo(ps)
	r.spaceEvenly = false
}

func (r *Col) asPackables(a []*Window) []Packable {
	ps := make([]Packable, len(a))
	for i := 0; i < len(a); i++ {
		ps[i] = a[i]
	}
	return ps
}

func (r *Col) setWindowsTo(ps []Packable) {
	for len(r.Windows) < len(ps) {
		r.Windows = append(r.Windows, nil)
	}

	for i := 0; i < len(ps); i++ {
		r.Windows[i] = ps[i].(*Window)
	}
}

func round(f float32) float32 {
	return float32(math.Round(float64(f)))
}

func (r *Col) printWindowPositions() {
	for _, w := range r.Windows {
		log(LogCatgCol, "%p: %d\n", w, w.TopY)
	}
}

func (c *Col) moveWindowBy(w *Window, off f32.Point) {

	x := c.layer.leftXOfVisibleColumn(c.layedOutColIndex)
	if x < 0 {
		return
	}

	absX := off.X + float32(x)
	c2 := c.layer.bestColForXCoord(int(absX))
	if c2 != c {
		c.moveWindowToNewCol(w, c, c2, off)
		return
	}

	ps := c.asPackables(c.Windows)
	p := NewPacker(float32(w.headerHeight()), c.vspace, ps)
	ps = p.MoveTo(w, float32(w.TopY)+off.Y)

	c.setWindowsTo(ps)
	c.printWindowPositions()
	c.markAllWindowsForCentering()
}

func (c *Col) moveWindowToNewCol(w *Window, from, to *Col, off f32.Point) {
	if from == to {
		return
	}

	fromX := c.layer.leftXOfVisibleColumn(from.layedOutColIndex)
	if fromX < 0 {
		return
	}
	toX := c.layer.leftXOfVisibleColumn(to.layedOutColIndex)
	if toX < 0 {
		return
	}

	from.markForRemoval(w)
	w.col = to
	xDiff := float32(toX - fromX)
	to.moveWindowBy(w, off.Sub(f32.Pt(xDiff, 0)))

	c.adjustWindowPathsWhenMovedBetweenCols(w, from, to)

	return

}

func (c *Col) adjustWindowPathsWhenMovedBetweenCols(w *Window, from, to *Col) {
	// Update display path based on new window
	toPath, toPathSet := to.Path()
	if toPathSet {
		// Moved to a column with a path. Make display path relative
		p := w.LoadPath().MakeRelativeUsingPrefix(toPath)
		if p != nil {
			w.SetDisplayPath(p)
			w.SetTagFromDisplayPath()
		}
	}

	_, fromPathSet := from.Path()
	if fromPathSet && !toPathSet {
		// Moved from a column with a path to one without.
		// Make display path absolute
		w.SetDisplayPath(w.LoadPath())
		w.SetTagFromDisplayPath()
	}

}

func (c *Col) markForRemoval(w *Window) {
	c.appendWindowIfNotPresent(&c.remove, w)
}

func (r *Col) removeWindowsMarkedForRemoval() {
	if r.remove == nil || len(r.remove) == 0 {
		return
	}

	for _, w := range r.remove {
		r.removeWindow(w)
	}
	r.remove = nil
	if len(r.Windows) > 0 {
		r.Windows[0].TopY = 0
	}
}

func (r *Col) removeWindow(w *Window) {
	match := func(i int) bool {
		return r.unpositioned[i] == w
	}
	r.unpositioned = slice.RemoveFirstMatchFromSlicePreserveOrder(r.unpositioned, match).([]*Window)

	match2 := func(i int) bool {
		return r.Windows[i] == w
	}
	r.Windows = slice.RemoveFirstMatchFromSlicePreserveOrder(r.Windows, match2).([]*Window)

	w.removeFromAllClones()

	if w == r.maximizedWindow {
		r.maximizedWindow = nil
	}

	editor.Completer().DeleteAllFromSource(w.Body.completionSource)
	editor.AddRecentFile(w.loadPath.String())
}

func (c *Col) markForCentering(w *Window) {
	c.appendWindowIfNotPresent(&c.center, w)
}

func (c *Col) markAllWindowsForCentering() {
	for _, w := range c.Windows {
		c.markForCentering(w)
	}
}

func (c *Col) centerWindowsMarkedForCentering() {
	for _, w := range c.center {
		w.centerBodyOnFirstCursorOrPrimarySelection()
	}
	c.center = c.center[:0]
}

func (r *Col) Clear() {
	for _, w := range r.Windows {
		r.removeWindow(w)
	}
}

func (c *Col) Grow(w *Window) {
	if c.maximizedWindow != nil && len(c.minimizedExcept) == 0 {
		// Allow Growing a window if the user has a pending request that the column
		// show windows minimized except one. This handles a case where the user
		// tried to acquire a window that was invisible while another in the column
		// is maximized
		return
	}

	c.appendWindowIfNotPresent(&c.resized, w)
}

func (c *Col) MinimizeAllExcept(w *Window) {
	c.appendWindowIfNotPresent(&c.minimizedExcept, w)
}

func (c *Col) Maximize(w *Window) {
	c.appendWindowIfNotPresent(&c.maximize, w)
}

func (c *Col) MaximizedWindow() *Window {
	return c.maximizedWindow
}

func (r *Col) Optimize() bool {
	if r.maximizedWindow == nil {
		return false
	}

	r.MinimizeAllExcept(r.maximizedWindow)
	return true
}

func (c *Col) SpaceEvenly() {
	c.spaceEvenly = true
}

func (c *Col) SetStyle(style Style) {
	c.layout.style = style
	c.layout.setFontStyles(style.Fonts)
	c.layout.layouter.lineSpacing = style.LineSpacing
	c.Tag.SetStyle(style.tagBlockStyle(), style.tagEditableStyle())
	c.layoutBox.SetStyle(style.layoutBoxStyle())
}

func (c *Col) Visible() bool {
	return c.layer.ColVisible(c)
}

func (c *Col) Name() string {
	if c.hasNoUserSetName() {
		return fmt.Sprintf("Col %d", c.Id)
	}

	t := c.Tag.String()
	parts := strings.Split(t, " ")
	return parts[0]
}

// Path returns the column filesystem path, if the columns name
// looks like an absolute or remote path. If the columns
// name is empty, or is not an absolute path, ok is false.
func (c *Col) Path() (path *GlobalPath, ok bool) {
	cn := c.Name()
	if cn == "" {
		return
	}

	path = NewGlobalPath(cn, GlobalPathIsDir)

	if !path.IsRemote() && !path.IsAbsolute() {
		return
	}

	ok = true
	return
}

func (c *Col) hasNoUserSetName() bool {
	return strings.HasPrefix(c.Tag.String(), "New")
}

// The visible column to the left of the given column.
// May return nil.
func (c *Col) left() *Col {
	return c.layer.visibleColLeftOf(c.layedOutColIndex)
}

// The column to the right of the given column.
// May return nil.
func (c *Col) right() *Col {
	return c.layer.visibleColRightOf(c.layedOutColIndex)
}

// The window in this column at approximately this position.
// May return nil if the column has no windows.
func (c *Col) windowAt(y int) *Window {
	var win *Window
	top := -1
	for _, otherWin := range c.Windows {
		// Use <= because the other window might be at the exact same Y.
		if otherWin.TopY <= y &&
			(top == -1 || otherWin.TopY > top) {
			win = otherWin
			top = otherWin.TopY
		}
	}
	return win
}

func (c *Col) setWindowsOnlyShowBasenamesInTag(only bool) {
	for _, w := range c.Windows {
		w.setOnlyShowBasenamesInTag(only)
		w.SetTagFromDisplayPath()
	}
}

func (c *Col) Sort() {
	if len(c.Windows) < 2 {
		return
	}

	type winPos struct {
		index int
		path  string
	}

	coords := make([]float32, len(c.Windows))

	p := make([]winPos, len(c.Windows))
	for i, w := range c.Windows {
		p[i].index = i
		p[i].path = w.displayPath.String()
		coords[i] = w.PackingCoord()
	}

	sort.Slice(coords, func(i, j int) bool {
		return coords[i] < coords[j]
	})

	sort.Slice(p, func(i, j int) bool {
		return p[i].path < p[j].path
	})

	for i, e := range p {
		c.Windows[e.index].SetPackingCoord(coords[i])
	}

	sort.Slice(c.Windows, func(i, j int) bool {
		return c.Windows[i].PackingCoord() < c.Windows[j].PackingCoord()
	})

	editor.SignalRedrawRequired()
}

func (c *Col) appendWindowIfNotPresent(list *[]*Window, win *Window) {
	for _, e := range *list {
		if e == win {
			return
		}
	}
	*list = append(*list, win)
}
