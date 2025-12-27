package main

import (
	"bytes"
	"container/list"
	"fmt"
	"image"
	"image/color"
	"sort"
	"strings"
	"unicode/utf8"

	"gioui.org/io/clipboard"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"github.com/jeffwilliams/anvil/editor/internal/slice"
	"github.com/jeffwilliams/anvil/editor/internal/words"
	"github.com/jeffwilliams/anvil/editor/internal/runes"
)

type Editor struct {
	Tag                                    Tag
	Layers                                 []*Layer
	activeLayerIndex                       int
	layout                                 editorLayouter
	hspace                                 float32
	hspaceLastLayout                       float32
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
	insertWhenTabPressed                   string
	lastSelectionsWrittenToClipboard       []string
	// keyHandlingCoordination coordinates handlers. See the comment
	// at the top of Editable.InsertTextAndHandleKeys for more info
	keyHandlingCoordination keyHandlingCoordination
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

func (e *Editor) NewLayer() *Layer {
	layer := NewLayer(e.layout.style)
	layer.Scheduler = e.Tag.Scheduler
	layer.workChan = e.work
	return layer
}

func (e *Editor) activeLayer() *Layer {
	if len(e.Layers) == 0 {
		log(LogCatgEditor, "Editor.activeLayer: no layer is active\n")
		return nil
	}

	return e.Layers[e.activeLayerIndex]
}

func (e *Editor) Clear() {
	e.Layers = nil
}

func (e *Editor) NewCol() *Col {
	l := e.activeLayer()
	if l == nil {
		l = e.AddLayer()
	}

	return l.NewCol()
}

func (e *Editor) AddLayer() *Layer {
	l := e.NewLayer()
	e.Layers = append(e.Layers, l)
	return l
}

func (e *Editor) ActivateLayerRelativeToCurrent(delta int) {
	e.ActivateLayer(e.activeLayerIndex + delta)
}

func (e *Editor) ActivateLayer(index int) {
	oldIndex := e.activeLayerIndex
	e.activeLayerIndex = index
	e.clampLayerIndex()

	if oldIndex == e.activeLayerIndex {
		return
	}

	e.ReplaceLayerNameInTag()

	e.Tag.AddOpForNextLayout(func(gtx layout.Context) {
		e.Tag.SetFocus(gtx)
	})
}

// ReplaceLayerNameInTag looks for the Lyrname command in the editor tag
// and tries to update its arguments to be the name of the current layer, if 
// applicable.
func (e *Editor) ReplaceLayerNameInTag() {
	log(LogCatgEditor, "ReplaceLayerNameInTag: called\n")
	if len(e.Layers) == 0  || e.activeLayerIndex < 0 || e.activeLayerIndex >= len(e.Layers) {
		log(LogCatgEditor, "ReplaceLayerNameInTag: layers are messed up\n")
		return
	}

	activeLayer := e.activeLayer()
	if activeLayer == nil {
		log(LogCatgEditor, "no active layer\n")
		return
	}

	tag := e.Tag.Bytes()
	tagStr := string(tag)

	cmdNameStart := strings.Index(tagStr, "Lyrname ")
	if cmdNameStart < 0 {
		log(LogCatgEditor, "ReplaceLayerNameInTag: Can't find 'Lyrname '\n")
		return
	}

	cmdArgStart := cmdNameStart + len("Lyrname ")

	newTag, succeeded := e.replaceLayerNameInTagLozengeDelimited(tag, tagStr, cmdNameStart, cmdArgStart, activeLayer.Name)

	if !succeeded {
		newTag, succeeded = e.replaceLayerNameInTagNonLozengeDelimited(tag, tagStr, cmdNameStart, cmdArgStart, activeLayer.Name)
	}

	if !succeeded {
		return
	}
	
	e.Tag.SetTextStringNoReset(newTag)
}

func (e *Editor) replaceLayerNameInTagLozengeDelimited(tag []byte, tagStr string, cmdNameStart, cmdArgStart int, activeLayerName string) (newTag string, succeeded bool) {
	if cmdNameStart == 0 {
		log(LogCatgEditor, "ReplaceLayerNameInTag.replaceLayerNameInTagLozengeDelimited: cmdNameStart is 0\n")
		return
	}

	walker := runes.NewWalker(tag)
	walker.SetBytePos(cmdNameStart)

	walker.Backward(1)
	if walker.Rune() != '◊' {
		log(LogCatgEditor, "ReplaceLayerNameInTag.replaceLayerNameInTagLozengeDelimited: previous rune is not ◊\n")
		return
	}

	var cmdEnd int
	for walker.Forward(1); !walker.AtEnd(); walker.Forward(1) {
		if walker.Rune() == '◊' {
			cmdEnd = walker.BytePos()
		}
	}
	
	if cmdEnd == 0 {
		log(LogCatgEditor, "ReplaceLayerNameInTag.replaceLayerNameInTagLozengeDelimited: no end\n")
		return
	}
	
	newTag = tagStr[0:cmdArgStart] + activeLayerName + tagStr[cmdEnd:] 
	succeeded = true
	return 
}

func (e *Editor) replaceLayerNameInTagNonLozengeDelimited(tag []byte, tagStr string, cmdNameStart, cmdArgStart int, activeLayerName string) (newTag string, succeeded bool) {

	if activeLayerName == "" {
		return
	}

	toReplace := "" 
	for i, l := range e.Layers {
		if i == e.activeLayerIndex {
			log(LogCatgEditor, "ReplaceLayerNameInTag.replaceLayerNameInTagNonLozengeDelimited: skip active layer\n")
			continue
		}
		
		if l.Name == "" {
			log(LogCatgEditor, "ReplaceLayerNameInTag.replaceLayerNameInTagNonLozengeDelimited: skip unnamed layer\n")
			continue
		}
		
		if strings.HasPrefix(tagStr[cmdArgStart:], l.Name) {
			toReplace = l.Name
			break
		}
	}

	if toReplace == "" {
		return
	}

	newTag = tagStr[0:cmdArgStart] + activeLayerName + tagStr[cmdArgStart + len(toReplace):] 
	succeeded = true
	return	
}

func (e *Editor) ActivateHighestLayer() {
	e.ActivateLayer(len(e.Layers) - 1)
}

func (e *Editor) clampLayerIndex() {
	e.activeLayerIndex = e.clampedLayerIndex(e.activeLayerIndex)
}

func (e *Editor) clampedLayerIndex(index int) int {
	if index >= len(e.Layers) {
		index = len(e.Layers) - 1
	}
	if index < 0 {
		index = 0
	}
	return index
}

func (e *Editor) DelActiveLayer() {
	if len(e.Layers) <= 1 {
		return
	}

	for i := e.activeLayerIndex; i < len(e.Layers)-1; i++ {
		e.Layers[i] = e.Layers[i+1]
	}
	e.Layers = e.Layers[:len(e.Layers)-1]
	e.clampLayerIndex()
}

func (e *Editor) MoveColToLayerRelativeToCurrent(col *Col, delta int) {
	activeLayer := e.activeLayer()
	if activeLayer == nil {
		log(LogCatgEditor, "No active layer\n")
		return
	}

	i := e.clampedLayerIndex(e.activeLayerIndex + delta)
	newLayer := e.Layers[i]
	if newLayer == col.layer {
		return
	}

	match := func(i int) bool {
		return col.layer.Cols[i] == col
	}
	col.layer.Cols = slice.RemoveFirstMatchFromSlicePreserveOrder(col.layer.Cols, match).([]*Col)
	col.layer = newLayer
	newLayer.AddCol(col)
}

func (e *Editor) SetActiveLayerName(name string)  {
	activeLayer := e.activeLayer()
	if activeLayer == nil {
		return
	}
	activeLayer.Name = name
}

// NewColDontPosition creates a new column like NewCol, but the caller is expected
// to manually position it.
func (e *Editor) NewColDontPosition() *Col {
	l := e.activeLayer()
	if l == nil {
		return nil
	}

	return l.NewColDontPosition()
}

func (e *Editor) NewWindow(col *Col) *Window {
	l := e.activeLayer()
	if l == nil {
		return nil
	}

	return l.NewWindow(col)
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
		e.EnsureWindowIsInCurrentLayer(w)
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

func (e *Editor) EnsureWindowIsInCurrentLayer(w *Window) {
	if len(e.Layers) <= 1 {
		return
	}

	activeLayer := e.activeLayer()
	if activeLayer == nil {
		return
	}

	if len(activeLayer.Cols) == 0 {
		return
	}

	if w.col.layer == activeLayer {
		return
	}

	from := w.col
	to := e.focusedColumn()
	if to == nil {
		to = activeLayer.Cols[0]
	}
	if from == to {
		return
	}

	//from.markForRemoval(w)
	from.removeWindow(w)
	to.AddWindow(w)
	to.adjustWindowPathsWhenMovedBetweenCols(w, from, to)
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
	GoTo               seek
	SelectBehaviour    selectBehaviour
	InCol              *Col
	GrowBodyBehaviour  growBodyBehaviour
	MoveLayerBehaviour moveLayerBehaviour
	Tail               bool
}

func (e *Editor) LoadFile(displayPath, loadPath *GlobalPath) *Window {
	return e.LoadFileOpts(displayPath, loadPath, LoadFileOpts{GrowBodyBehaviour: growBodyIfTooSmall})
}

func (e *Editor) LoadFileOpts(displayPath, loadPath *GlobalPath, opts LoadFileOpts) *Window {
	w := e.FindWindowForFileAndDisplay(loadPath.String())
	if w != nil {
		w.showIfHidden()
		if opts.MoveLayerBehaviour == moveToCurrentLayer {
			editor.EnsureWindowIsInCurrentLayer(w)
		}

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
	for _, l := range e.Layers {
		for _, c := range l.Cols {
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
	for _, l := range e.Layers {
		for _, c := range l.Cols {
			for _, w := range c.Windows {
				r = append(r, w)
			}
		}
	}
	return r
}

func (e *Editor) FindWindowForId(id int) *Window {
	for _, l := range e.Layers {
		for _, c := range l.Cols {
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

	l := e.activeLayer()
	if l == nil {
		return
	}

	e.layout.layout(gtx)
	//l.Layout()

	//	e.positionCols()

	//	e.removeColsMarkedForRemoval()
	e.opsForNextLayout.Perform(gtx)

	if e.redrawRequired {
		gtx.Execute(op.InvalidateCmd{})
	}
}

func (e *Editor) SignalRedrawRequired() {
	e.redrawRequired = true
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
	//l.layoutCols()
	layer := l.ed.activeLayer()
	if layer == nil {
		return
	}

	layer.Layout(gtx)

	st2.Pop()
	st.Pop()

	l.gtx = layout.Context{}
}

func (l *editorLayouter) fillBackground(gtx layout.Context) {
	paint.ColorOp{Color: color.NRGBA(l.style.BodyBgColor)}.Add(gtx.Ops)
	st := drawFilledBox(gtx, float32(gtx.Constraints.Max.X), float32(gtx.Constraints.Max.Y))
	paint.PaintOp{}.Add(gtx.Ops)
	st.Pop()

}

func (l *editorLayouter) offset(x, y int) op.TransformStack {
	return op.Offset(image.Point{x, y}).Push(l.gtx.Ops)
}

func (e *editorLayouter) drawBottomBorder(gtx layout.Context) {
	paint.ColorOp{Color: color.NRGBA(e.style.WinBorderColor)}.Add(gtx.Ops)
	w := float32(gtx.Metric.Dp(e.style.WinBorderWidth))
	st := drawFilledBox(gtx, float32(gtx.Constraints.Max.X), w)
	paint.PaintOp{}.Add(gtx.Ops)
	st.Pop()
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
	for _, l := range e.Layers {
		for _, c := range l.Cols {
			for _, w := range c.Windows {
				if w.fileType == typeFile && !w.IsErrorsWindow() {
					w.Put()
				}
			}
		}
	}
}

func (e *Editor) Getall() {
	for _, l := range e.Layers {
		for _, c := range l.Cols {
			for _, w := range c.Windows {
				if w.fileType == typeFile && !w.IsErrorsWindow() {
					w.Get()
				}
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

	for _, l := range e.Layers {
		l.layout.style = style
		for _, c := range l.Cols {
			c.SetStyle(style)
			for _, w := range c.Windows {
				w.SetStyle(style)
			}
		}
	}
}

func (e *Editor) Execute(cmd string, args []string) {
	e.Tag.AddOpForNextLayout(func(gtx layout.Context) {
		e.Tag.adapter.execute(&e.Tag.blockEditable.editable, gtx, cmd, args)
	})
}

func (e *Editor) SetOnlyFlashedWindow(win *Window) {
	for _, l := range e.Layers {
		for _, c := range l.Cols {
			for _, w := range c.Windows {
				w.setFlash(w == win)
			}
			for _, w := range c.unpositioned {
				w.setFlash(w == win)
			}
		}
	}
}

func (e *Editor) clearAllRecentlyTypedText() {
	for _, l := range e.Layers {
		for _, c := range l.Cols {
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
}

func (e *Editor) NumVisibleCols() int {
	l := e.activeLayer()
	if l == nil {
		return 0
	}

	return l.NumVisibleCols()
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
	for _, l := range e.Layers {
		for _, c := range l.Cols {
			if c.Name() == colName {
				c.SetVisible(true)
			}
		}
	}
}

func (e *Editor) SetFirstHiddenColVisible() {
	l := e.activeLayer()
	if l == nil {
		return
	}

	l.SetFirstHiddenColVisible()
}

func (e *Editor) ListCols(includeFiles, includeShowCommand bool) string {
	var buf bytes.Buffer
	al := e.activeLayer()

	for i, l := range e.Layers {
		fmt.Fprintf(&buf, "Layer %d", i)

		if l.Name != "" {
		    fmt.Fprintf(&buf, " '%s'", l.Name)
		}

		if al == l {
			fmt.Fprintf(&buf, " (active)")
		}

		if includeShowCommand {
			fmt.Fprintf(&buf, " ◊Setlyr %d◊", i)
		}
		fmt.Fprintf(&buf, "\n")

		for _, c := range l.Cols {
			buf.WriteString("  ")
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
					fmt.Fprintf(&buf, "    %s\n", file)
				}
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

func (e *Editor) markForRemoval(c *Col) {
	l := e.activeLayer()
	if l == nil {
		return
	}

	l.markForRemoval(c)
}
