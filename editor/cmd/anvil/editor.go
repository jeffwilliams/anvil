package main

import (
	"bytes"
	"container/list"
	"fmt"
	"image"
	"image/color"
	"math"
	"sort"
	"strings"
	"unicode/utf8"

	"gioui.org/f32"
	"gioui.org/io/clipboard"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"github.com/jeffwilliams/anvil/editor/internal/slice"
	"github.com/jeffwilliams/anvil/editor/internal/words"
)

type Editor struct {
	Tag                                    Tag
	Cols                                   []*Col
	layout                                 editorLayouter
	hspace                                 float32
	hspaceLastLayout                       float32
	unpositioned, remove                   []*Col
	lastSelection                          globalSelection
	focusedEditable                        *editable
	focusedWindow                          *Window
	jobs                                   []Job
	work                                   chan Work
	recentFiles                            *LRUCache
	completer                              *words.Completer
	Marks                                  Marks
	opsForNextLayout                       OpsForNextLayout
	redrawRequired                         bool
	editableWhereTertiaryButtonHoldStarted *editable
	showBasenamesOnlyInTags                bool
	insertWhenTabPressed                   string
	lastSelectionsWrittenToClipboard       []string
}

type Job interface {
	Kill()
	Name() string
}

type StartNexter interface {
	// build and add the next job to the editor
	StartNext()
}

func NewEditor(style Style) *Editor {
	e := &Editor{
		layout: editorLayouter{
			layouter: layouter{
				lineSpacing: style.LineSpacing,
				fontStyles:  style.Fonts,
			},
			style: style,
		},
		recentFiles: NewLRUCache(100),
	}

	e.insertWhenTabPressed = "\t"
	e.work = make(chan Work)
	e.layout.ed = e
	executor := NewCommandExecutor(e)
	scheduler := NewScheduler(e.WorkChan())
	e.Tag.Init(nil, style.tagBlockStyle(), style.tagEditableStyle(), executor, e, scheduler)
	e.Tag.label = "editor"
	e.setInitialTag()
	e.completer = words.NewCompleter()
	return e
}

func (e *Editor) NewCol() *Col {
	col := e.newCol()

	if len(e.Cols) == 0 {
		col.LeftX = 0
		e.Cols = append(e.Cols, col)
	} else {
		e.unpositioned = append(e.unpositioned, col)
	}

	return col
}

// NewColDontPosition creates a new column like NewCol, but the caller is expected
// to manually position it.
func (e *Editor) NewColDontPosition() *Col {
	col := e.newCol()
	e.Cols = append(e.Cols, col)
	return col
}

func (e *Editor) newCol() *Col {
	col := NewCol(e.layout.style)
	col.ed = e
	col.Scheduler = NewScheduler(e.WorkChan())
	col.workChan = e.WorkChan()
	return col
}

func (e *Editor) removeColumn(col *Col) {
	col.Clear()

	match := func(i int) bool {
		log(LogCatgEditor, "Editor.Delcol: compare %p to needle %p\n", e.Cols[i], col)
		return e.Cols[i] == col
	}
	e.Cols = slice.RemoveFirstMatchFromSlicePreserveOrder(e.Cols, match).([]*Col)
}

func (e *Editor) RepositionCol(col *Col) {
	match := func(i int) bool {
		return e.Cols[i] == col
	}

	e.Cols = slice.RemoveFirstMatchFromSlicePreserveOrder(e.Cols, match).([]*Col)
	e.unpositioned = append(e.unpositioned, col)
}

func (e *Editor) Clear() {
	e.Cols = nil
}

func (e *Editor) NewWindow(col *Col) *Window {
	if len(e.Cols) == 0 {
		return nil
	}

	log(LogCatgEditor, "Editor.NewWindow: col is %p\n", col)
	if col != nil {
		return col.NewWindow()
	}

	cols := e.VisibleCols()
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

func (e *Editor) AppendError(dir string, msg string) {
	fname := e.ErrorsFileNameOf(dir)

	if msg == "" {
		return
	}

	if msg[len(msg)-1] != '\n' {
		msg = msg + "\n"
	}

	w := e.FindOrCreateWindow(fname)

	if w != nil {
		w.Append([]byte(msg))
		w.GrowIfBodyTooSmall()
		w.Body.AddOpForNextLayout(func(gtx layout.Context) {
			w.Body.moveToEndOfDoc(gtx)
			// This is to force a redraw
			w.Body.invalidateLayedoutText()
			e.SetOnlyFlashedWindow(w)
		})
	}
}

func (e *Editor) ClearErrors(dir string) {
	n := editor.ErrorsFileNameOf(dir)
	win, _ := editor.FindWindowForFile(n)
	if win != nil {
		win.Body.SetText([]byte{})
		win.Body.ClearManualHighlights()
	}
}

func (e *Editor) ErrorsFileNameOf(dir string) string {
	if strings.HasSuffix(dir, "/") || strings.HasSuffix(dir, "\\") {
		dir = dir[:len(dir)-1]
	}
	return fmt.Sprintf("%s+Errors", dir)
}

func (e *Editor) FindOrCreateWindow(fname string) *Window {
	log(LogCatgEditor, "FindOrCreateWindow: Looking for window '%s'\n", fname)
	w := e.FindWindowForFileAndDisplay(fname)
	if w != nil {
		return w
	}

	// TODO: This helps somewhat with putting +Errors windows in the right column.
	// It can still be wrong, though, if you type in column 1, then click a command
	// in column 2. Instead perhaps we can put the column in FileLoadOpts
	w = editor.NewWindow(e.focusedColumn())
	if w == nil {
		log(LogCatgEditor, "FindOrCreateWindow: failed to create window\n")
		return nil
	}

	log(LogCatgEditor, "FindOrCreateWindow: Creating window '%s'\n", fname)
	gpath := NewGlobalPath(fname, GlobalPathIsFile)
	w.SetPathsAndTag(gpath, gpath)
	e.notifyFileOpened(w)
	return w
}

func (e *Editor) focusedColumn() *Col {
	if e.focusedEditable == nil {
		return nil
	}

	ad, ok := e.focusedEditable.adapter.(*editableAdapter)
	if !ok {
		return nil
	}

	switch item := ad.owner.(type) {
	case *Window:
		return item.col
	case *Col:
		return item
	}

	return nil
}

type LoadFileOpts struct {
	GoTo              seek
	SelectBehaviour   selectBehaviour
	InCol             *Col
	GrowBodyBehaviour growBodyBehaviour
	Tail              bool
}

func (e *Editor) LoadFile(displayPath, loadPath *GlobalPath) *Window {
	return e.LoadFileOpts(displayPath, loadPath, LoadFileOpts{GrowBodyBehaviour: growBodyIfTooSmall})
}

func (e *Editor) LoadFileOpts(displayPath, loadPath *GlobalPath, opts LoadFileOpts) *Window {
	w := e.FindWindowForFileAndDisplay(loadPath.String())
	if w != nil {
		w.showIfHidden()

		w.GrowIfBodyTooSmall()
		// TODO: Warp pointer to here
		w.Body.AddOpForNextLayout(func(gtx layout.Context) {
			w.Body.moveCursorTo(gtx, opts.GoTo, opts.SelectBehaviour)
		})
		return w
	}

	w = editor.NewWindow(opts.InCol)
	if w == nil {
		log(LogCatgEditor, "Editor.LoadFile: failed to create window\n")
		return nil
	}

	w.SetPathsAndTag(displayPath, loadPath)
	err := w.LoadFileOpts(displayPath, loadPath, opts)
	if err != nil {
		log(LogCatgEditor, "Editor.LoadFile: Error loading window. Will mark for removal\n")
		w.col.markForRemoval(w)
		e.AppendError("", err.Error())
		return nil
	}
	e.notifyFileOpened(w)
	return w
}

func (e *Editor) FindWindowForFileAndDisplay(path string) *Window {
	win, _ := e.FindWindowForFile(path)

	if win != nil && win.col != nil {
		win.col.SetVisible(true)
	}

	return win
}

func (e *Editor) FindWindowForFile(path string) (win *Window, count int) {
	for _, c := range e.Cols {
		for _, w := range c.Windows {
			if e.windowFilesAreSame(w.loadPath.String(), path) {
				win = w
				count++
			}
		}
		for _, w := range c.unpositioned {
			if e.windowFilesAreSame(w.loadPath.String(), path) {
				win = w
				count++
			}
		}
	}
	return
}

func (e *Editor) DelWindow(w *Window) {
	if w.col == nil {
		return
	}

	_, count := e.FindWindowForFile(w.loadPath.String())
	if count == 1 {
		log(LogCatgEditor, "Editor.DelWindow: sending file closed notification\n")
		e.notifyFileClosed(w)
	}

	application.WinIdGenerator().Free(w.Id)
	w.col.markForRemoval(w)
	e.SignalRedrawRequired()

}

func (e *Editor) notifyFileClosed(w *Window) {
	n := ApiNotification{
		WinId: w.Id,
		Op:    ApiNotificationOpFileClosed,
	}

	addApiNotificationToAllSessions(n)
}

func (e *Editor) notifyFileOpened(w *Window) {
	n := ApiNotification{
		WinId: w.Id,
		Op:    ApiNotificationOpFileOpened,
	}

	addApiNotificationToAllSessions(n)
}

func (e *Editor) windowFilesAreSame(a, b string) bool {
	for len(a) > 0 && (a[len(a)-1] == '/' || a[len(a)-1] == '\\') {
		a = a[:len(a)-1]
	}
	for len(b) > 0 && (b[len(b)-1] == '/' || b[len(b)-1] == '\\') {
		b = b[:len(b)-1]
	}

	return a == b
}

func (e *Editor) Windows() []*Window {
	r := []*Window{}
	for _, c := range e.Cols {
		for _, w := range c.Windows {
			r = append(r, w)
		}
	}
	return r
}

func (e *Editor) FindWindowForId(id int) *Window {
	for _, c := range e.Cols {
		for _, w := range c.Windows {
			if w.Id == id {
				return w
			}
		}
		for _, w := range c.unpositioned {
			if w.Id == id {
				return w
			}
		}
	}
	return nil
}

type editorLayouter struct {
	layouter
	gtx   layout.Context
	ed    *Editor
	style Style
}

// Layout handles events and draws the editor.
func (e *Editor) Layout(gtx layout.Context) {
	e.redrawRequired = false
	e.hspaceLastLayout = e.hspace
	e.hspace = float32(gtx.Constraints.Max.X)

	e.positionCols()

	e.layout.layout(gtx)
	e.removeColsMarkedForRemoval()
	e.opsForNextLayout.Perform(gtx)

	if e.redrawRequired {
		gtx.Execute(op.InvalidateCmd{})
	}
}

func (e *Editor) SignalRedrawRequired() {
	e.redrawRequired = true
}

func (e *Editor) setConstraintsToColWidth(gtx *layout.Context, colIndex int) {
	sz := e.colWidth(colIndex)

	gtx.Constraints.Max.X = int(sz)
}

func (e *Editor) colWidth(colIndex int) float32 {
	cols := e.VisibleCols()
	ps := e.asPackables(cols)
	p := NewPacker(0, e.hspace, ps)
	sz := p.ItemSize(colIndex)

	return sz
}

func (l *editorLayouter) layout(gtx layout.Context) {
	l.gtx = gtx

	// Already saves stack state
	tagDims := l.ed.Tag.layout(gtx)

	st := l.offset(0, tagDims.Size.Y)
	l.drawBottomBorder(gtx)
	st2 := l.offset(0, gtx.Metric.Dp(l.style.WinBorderWidth))

	l.gtx.Constraints.Max.Y -= tagDims.Size.Y

	// Already saves stack state
	l.layoutCols()

	st2.Pop()
	st.Pop()

	l.gtx = layout.Context{}
}

func (l *editorLayouter) offset(x, y int) op.TransformStack {
	return op.Offset(image.Point{x, y}).Push(l.gtx.Ops)
}

func (l *editorLayouter) layoutCols() {

	processEvents := func() (retry bool) {
		lastColX := -10000
		cols := l.ed.VisibleCols()
		for i, c := range cols {
			if c.LeftX < lastColX {
				log(LogCatgEditor, "The cols are not sorted in ascending X coordinate")
				retry = true
				return
			}

			lastColX = c.LeftX
			l.ed.setConstraintsToColWidth(&l.gtx, i)
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

		cols := l.ed.VisibleCols()
		for i, c := range cols {
			log(LogCatgEditor, "col %d: left is %d", i, c.LeftX)
		}

		panic("The cols are not sorted in ascending X coordinate")
	}

	cols := l.ed.VisibleCols()

	// The event handling may have
	// changed the position of one of the columns, so we need to
	// first process those events, and then only later
	// draw the columns. We can't "layout" (handle events and draw) each column
	// in order because we could draw some of the columns then a later one changes
	// position and affects the width of the previously drawn columns.
	for i, c := range cols {
		l.ed.setConstraintsToColWidth(&l.gtx, i)
		c.DrawAndListenForEvents(l.gtx)
	}

}

func (e *editorLayouter) drawBottomBorder(gtx layout.Context) {
	paint.ColorOp{Color: color.NRGBA(e.style.WinBorderColor)}.Add(gtx.Ops)
	w := float32(gtx.Metric.Dp(e.style.WinBorderWidth))
	st := drawFilledBox(gtx, float32(gtx.Constraints.Max.X), w)
	paint.PaintOp{}.Add(gtx.Ops)
	st.Pop()
}

func (e *Editor) positionCols() {
	e.packNewCols()
	e.spaceColsEvenlyIfWidthChanged()
}

func (e *Editor) packNewCols() {
	if len(e.unpositioned) == 0 {
		return
	}

	log(LogCatgEditor, "editor: Positioning columns\n")

	ps := e.asPackables(e.Cols)
	unp := e.asPackables(e.unpositioned)

	p := NewPacker(0, e.hspace, ps)
	ps = p.Pack(unp)

	e.setColsTo(ps)

	e.unpositioned = nil
}

func (e *Editor) spaceColsEvenlyIfWidthChanged() {
	if e.hspaceLastLayout == e.hspace {
		return
	}

	if e.hspace < e.hspaceLastLayout {
		ps := e.asPackables(e.Cols)
		p := NewPacker(0, e.hspace, ps)
		p.SpaceEvenly()
	}
}

func (e *Editor) asPackables(a []*Col) []Packable {
	ps := make([]Packable, len(a))
	for i := 0; i < len(a); i++ {
		ps[i] = a[i]
	}
	sort.SliceStable(ps, func(i, j int) bool {
		return ps[i].PackingCoord() < ps[j].PackingCoord()
	})
	return ps
}

func (e *Editor) setColsTo(ps []Packable) {
	for len(e.Cols) < len(ps) {
		e.Cols = append(e.Cols, nil)
	}

	for i := 0; i < len(ps); i++ {
		e.Cols[i] = ps[i].(*Col)
	}
}

func (e *Editor) bestColForXCoord(absoluteX int) *Col {
	cols := e.VisibleCols()
	for i, c := range cols {
		d := 0
		if i < len(cols)-1 {
			d = cols[i+1].LeftX
		}
		log(LogCatgEditor, "Editor.bestColForXCoord: absoluteX=%d, col %d %p ends at %d\n", absoluteX, i, c, d)
		if i >= len(cols)-1 || absoluteX < cols[i+1].LeftX {
			return c
		}
	}
	return cols[len(cols)-1]
}

func (e *Editor) markForRemoval(c *Col) {
	e.remove = append(e.remove, c)
}

func (e *Editor) removeColsMarkedForRemoval() {
	if e.remove == nil || len(e.remove) == 0 {
		return
	}

	for _, c := range e.remove {
		e.removeColumn(c)
	}
	e.remove = nil

	e.ensureFirstVisibleColIsLeftJustified()
}

func (e *Editor) ensureFirstVisibleColIsLeftJustified() {
	if len(e.Cols) > 0 {
		for _, c := range e.Cols {
			if c.Visible() {
				c.LeftX = 0
				return
			}
		}
	}
}

func (r *Editor) moveWindowBy(w *Window, off f32.Point, absoluteX float32) {
	// This is meant to find the right column the window has been moved to.
	cols := r.VisibleCols()
	for i, c := range cols {
		if i >= len(cols) || absoluteX < float32(cols[i+1].LeftX) {
			c.moveWindowBy(w, off)
			break
		}
	}
}

func (e *Editor) moveColBy(c *Col, off f32.Point) {
	ps := e.asPackables(e.VisibleCols())
	p := NewPacker(0, e.hspace, ps)
	movedPs := p.MoveTo(c, float32(c.LeftX)+off.X)

	newCols := make([]*Col, 0, len(e.Cols))
	for _, c := range e.Cols {
		if !c.Visible() {
			newCols = append(newCols, c)
		}
	}
	for _, c := range movedPs {
		newCols = append(newCols, c.(*Col))
	}
	e.Cols = newCols
}

func (e *Editor) setLastSelection(ed *editable, sel *selection) {
	e.lastSelection.editable = ed
	e.lastSelection.sel = sel
	e.lastSelection.isSet = true
}

func (e *Editor) clearLastSelection() {
	e.lastSelection.isSet = false
}

func (e *Editor) clearLastSelectionIfOwnedBy(ed *editable) {
	if e.lastSelection.editable == ed {
		e.clearLastSelection()
	}
}

func (e *Editor) lastSelectionSet() bool {
	return e.lastSelection.isSet
}

func (e *Editor) getLastSelection() *globalSelection {
	return &e.lastSelection
}

func (e *Editor) cutLastSelection(gtx layout.Context) {
	log(LogCatgEditor, "Editor.cutLastSelection: lastSelectionSet: %v\n", e.lastSelectionSet())
	if e.lastSelectionSet() {
		e.lastSelection.editable.cutText(gtx, e.lastSelection.sel)
	}
}

func (e *Editor) copyLastSelection(gtx layout.Context) {
	if e.lastSelectionSet() {
		e.lastSelection.editable.copyText(gtx, e.lastSelection.sel)
	}
}

func (e *Editor) textOfLastSelection() string {
	sel := editor.lastSelection
	if sel.isSet && sel.editable != nil {
		return sel.editable.textOfSelection(sel.sel)
	}
	return ""
}

func (e *Editor) pasteToFocusedEditable(gtx layout.Context) {
	if e.focusedEditable == nil {
		log(LogCatgEditor, "editor.pasteToFocusedEditable: no editable is focused. Not pasting.\n")
		return
	}
	tag := editor.focusedEditable.Tag()
	log(LogCatgEditor, "editor.pasteToFocusedEditable: pasting to editable: %s\n", editor.focusedEditable.label)
	cmd := clipboard.ReadCmd{Tag: tag}
	gtx.Execute(cmd)
}

func (e *Editor) cutAllSelectionsFromLastSelectedEditable(gtx layout.Context) {
	if e.lastSelectionSet() {
		e.lastSelection.editable.cutAllSelectedText(gtx)
	}
}

func (e *Editor) copyAllSelectionsFromLastSelectedEditable(gtx layout.Context) {
	if e.lastSelectionSet() {
		e.lastSelection.editable.copyAllSelectedText(gtx)
	}
}

func (e *Editor) setFocusedEditable(ed *editable, owningWindow *Window) {
	e.focusedEditable = ed
	e.focusedWindow = owningWindow
	// Clear any windows that are flashed
	e.SetOnlyFlashedWindow(nil)
	e.clearAllRecentlyTypedText()
}

func (e *Editor) getFocusedEditable() *editable {
	return e.focusedEditable
}

func (e *Editor) clearFocusedEditable() {
	e.focusedEditable = nil
	e.focusedWindow = nil
}

func (e *Editor) setEditableWhereTertiaryButtonHoldStarted(ed *editable) {
	e.editableWhereTertiaryButtonHoldStarted = ed
}

func (e *Editor) getEditableWhereTertiaryButtonHoldStarted() *editable {
	return e.editableWhereTertiaryButtonHoldStarted
}

func (e *Editor) clearEditableWhereTertiaryButtonHoldStarted() {
	e.editableWhereTertiaryButtonHoldStarted = nil
}

type globalSelection struct {
	editable *editable
	sel      *selection
	isSet    bool
}

func (e *Editor) AddJob(j Job) {
	if j == nil {
		return
	}
	log(LogCatgEditor, "editor.AddJob called for job %s\n", j.Name())

	e.jobs = append(e.jobs, j)
	e.prependJobToTag(j)
}

func (e *Editor) RemoveJob(job Job) {
	if job == nil {
		return
	}

	var keep []Job
	var found bool
	for _, j := range e.jobs {

		if j == job {
			found = true
			continue
		}

		keep = append(keep, j)
	}

	e.jobs = keep
	if found {
		e.removeJobFromTag(job)
	}
}

func (e *Editor) Jobs() []Job {
	r := []Job{}
	for _, j := range e.jobs {
		r = append(r, j)
	}
	return r
}

func (e *Editor) removeJobFromTag(job Job) {
	_, startOfChange, lenOfChange := removeJobFromTagString(job.Name(), e.Tag.String())
	e.Tag.deleteFromPieceTable(startOfChange, lenOfChange)
}

func removeJobFromTagString(job, tag string) (newTag string, startOfChange, lengthOfChange int) {
	/* We manage the tag the same way acme does: basically just remove the first instance of this tag name.
	   We need to handle cases where the name of a job is a subtring of another job.

		Case 1: The tag entirely consists of only the job. Clear the tag.
		Case 2: Job is first in the tag. Then the tag must begin with the job name followed by a space. If this is the
		  case, delete the initial part of the tag.
		Case 3: Job is neither first nor last in the tag. Then the tag must contain the jobname preceeded by and followed by a space. Remove that portion.
		Case 4: Job is the last item in the tag. Then it is only preceeded by a space.
	*/
	if tag == job {
		return "", 0, utf8.RuneCountInString(job)
	}

	joblen := utf8.RuneCountInString(job)
	taglen := utf8.RuneCountInString(tag)

	if strings.HasPrefix(tag, job+" ") {
		newTag = strings.Replace(tag, job+" ", "", 1)
		startOfChange = 0
		lengthOfChange = joblen + 1
		return
	}

	if strings.HasSuffix(tag, " "+job+" ") {
		newTag = tag[:len(tag)-(len(job)+2)]
		startOfChange = taglen - joblen - 2
		lengthOfChange = joblen + 2
		return
	}

	if strings.HasSuffix(tag, " "+job) {
		newTag = tag[:len(tag)-(len(job)+1)]
		startOfChange = taglen - joblen - 1
		lengthOfChange = joblen + 1
		return
	}

	i := strings.Index(tag, " "+job+" ")
	if i >= 0 {
		newTag = strings.Replace(tag, " "+job+" ", " ", 1)
		startOfChange = utf8.RuneCountInString(tag[:i])
		lengthOfChange = joblen + 1
		return
	}

	newTag = tag
	return
}

func (e *Editor) prependJobToTag(job Job) {
	s := fmt.Sprintf("%s ", job.Name())
	e.Tag.insertToPieceTable(0, s)
}

func (e *Editor) KillJob(name string) {
	if name == "" {
		e.killLatestJob()
		return
	}

	for _, j := range e.jobs {
		if j.Name() == name {
			j.Kill()
			break
		}
	}
}

// killLatestJob kills the job that was started the latest.
func (e *Editor) killLatestJob() {
	if len(e.jobs) > 0 {
		e.jobs[len(e.jobs)-1].Kill()
	}
}

func (e *Editor) WorkChan() chan Work {
	return e.work
}

// setInitialTag is needed instead of using setTag when initializing to avoid an initialization
// loop, when the global editor variable is being initialized and it refers back to itself when
// the Tag editable tries to clear it's selections (and notify the main editor)
func (e *Editor) setInitialTag() {
	s := fmt.Sprintf(settings.Layout.EditorTag)
	e.Tag.SetTextStringNoReset(s)
}

func (e *Editor) jobList() string {
	var buf bytes.Buffer

	for i, j := range e.jobs {
		if i > 0 {
			fmt.Fprintf(&buf, " ")
		}
		fmt.Fprintf(&buf, "%s", j.Name())
	}

	return buf.String()
}

func (e *Editor) Putall() {
	for _, c := range e.Cols {
		for _, w := range c.Windows {
			if w.fileType == typeFile && !w.IsErrorsWindow() {
				w.Put()
			}
		}
	}
}

func (e *Editor) Completer() *words.Completer {
	return e.completer
}

func (e *Editor) AddRecentFile(f string) {
	if strings.HasSuffix(f, "+Errors") {
		return
	}
	e.recentFiles.Add(f)
}

func (e *Editor) RecentFiles() []string {
	return e.recentFiles.AllSorted()
}

func (e *Editor) SetStyle(style Style) {
	e.layout.style = style
	e.layout.setFontStyles(style.Fonts)
	log(LogCatgEditor, "Editor.SetStyle: fonts: %#v\n", style.Fonts)
	log(LogCatgEditor, "Editor.SetStyle: global VariableFont: %#v\n", VariableFont)
	e.Tag.SetStyle(style.tagBlockStyle(), style.tagEditableStyle())

	for _, c := range e.Cols {
		c.SetStyle(style)
		for _, w := range c.Windows {
			w.SetStyle(style)
		}
	}
}

func (e *Editor) Execute(cmd string, args []string) {
	e.Tag.AddOpForNextLayout(func(gtx layout.Context) {
		e.Tag.adapter.execute(&e.Tag.blockEditable.editable, gtx, cmd, args)
	})
}

func (e *Editor) SetOnlyFlashedWindow(win *Window) {
	for _, c := range e.Cols {
		for _, w := range c.Windows {
			w.setFlash(w == win)
		}
		for _, w := range c.unpositioned {
			w.setFlash(w == win)
		}
	}
}

func (e *Editor) clearAllRecentlyTypedText() {
	for _, c := range e.Cols {
		for _, w := range c.Windows {
			w.Tag.ClearRecentlyTypedText()
			w.Body.ClearRecentlyTypedText()
		}
		for _, w := range c.unpositioned {
			w.Tag.ClearRecentlyTypedText()
			w.Body.ClearRecentlyTypedText()
		}
	}
}

func (e *Editor) VisibleCols() []*Col {
	r := make([]*Col, 0, len(e.Cols))
	for _, c := range e.Cols {
		if c.Visible() {
			r = append(r, c)
		}
	}
	return r
}

func (e *Editor) NumVisibleCols() int {
	i := 0
	for _, c := range e.Cols {
		if c.Visible() {
			i++
		}
	}
	return i
}

type LRUCache struct {
	entries  map[string]struct{}
	sequence list.List
	max      int
}

func NewLRUCache(max int) *LRUCache {
	return &LRUCache{
		entries: make(map[string]struct{}),
		max:     max,
	}
}

func (c *LRUCache) Add(s string) {
	_, ok := c.entries[s]
	if ok {
		return
	}

	c.evict()
	c.add(s)
}

func (c *LRUCache) evict() {
	if len(c.entries) < c.max {
		return
	}

	s := c.sequence.Remove(c.sequence.Front()).(string)
	delete(c.entries, s)
}

func (c *LRUCache) add(s string) {
	c.entries[s] = struct{}{}
	c.sequence.PushBack(s)
}

func (c *LRUCache) AllSorted() []string {
	var r []string
	for s := range c.entries {
		r = append(r, s)
	}

	sort.Strings(r)

	return r
}

func (c *LRUCache) All() []string {
	var r []string
	for e := c.sequence.Front(); e != nil; e = e.Next() {
		r = append(r, e.Value.(string))
	}
	return r
}

func (e *Editor) SetColVisible(colName string) {
	for _, c := range e.Cols {
		if c.Name() == colName {
			c.SetVisible(true)
		}
	}
}

func (e *Editor) SetFirstHiddenColVisible() {
	for _, c := range e.Cols {
		if !c.Visible() {
			c.SetVisible(true)
			break
		}
	}
}

func (e *Editor) ListCols(includeFiles, includeShowCommand bool) string {
	var buf bytes.Buffer
	for _, c := range e.Cols {
		buf.WriteString(c.Name())
		if !c.Visible() {
			buf.WriteString(" (hidden)")
		}
		if includeShowCommand {
			if !c.Visible() {
				fmt.Fprintf(&buf, " ◊Showcol %s◊", c.Name())
			} else {
				fmt.Fprintf(&buf, " ◊Hidecol %s◊", c.Name())
			}
		}
		buf.WriteRune('\n')

		if includeFiles {
			for _, w := range c.Windows {
				file := w.displayPath.String()
				if file == "" {
					file = "(unnamed)"
				}
				fmt.Fprintf(&buf, "  %s\n", file)
			}
		}
	}

	return buf.String()
}

func (e *Editor) AddOpForNextLayout(op LayoutOp) {
	e.opsForNextLayout.Add(op)
}

func (e *Editor) setInsertWhenTabPressed(s string) {
	e.insertWhenTabPressed = s
}

func (e *Editor) getInsertWhenTabPressed() string {
	return e.insertWhenTabPressed
}

func (e *Editor) SetLastSelectionsWrittenToClipboard(t []string) {
	e.lastSelectionsWrittenToClipboard = t
}

func (e *Editor) LastSelectionsWrittenToClipboard() []string {
	return e.lastSelectionsWrittenToClipboard
}
