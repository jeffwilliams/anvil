package main

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"image"
	"image/color"

	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"github.com/jeffwilliams/anvil/editor/internal/events"
)

// Window is a single window in the editor, with it's own tag and body.
type Window struct {
	Tag  Tag
	Body Body
	TopY int // Y position of the top of the window within the column
	Id   int

	layoutBox layoutBox
	scrollbar scrollbar

	layout          windowLayouter
	overlayWithGrey bool
	col             *Col
	// displayPath is the filesystem path that is displayed in the window tag
	displayPath GlobalPath
	// displayPath is the filesystem path that is used when loading the file
	loadPath                      GlobalPath
	fileType                      fileType
	filler                        *FillEditableWithItemList
	initialTagUserArea            string
	setFocusOnNextLayout          bool
	tagShowsBodyAsChangedFromDisk bool
	bodyDims                      layout.Dimensions
	clones                        map[*Window]struct{}
	allowDirtyDelete              bool
	packingCoordChangedListeners  []func(oldVal, newVal int)
	customEdCommands              string
	fuzzySearch                   *FuzzySearcher
	onlyShowBasenamesInTag        bool
	insertWhenTabPressed          string
}

type fileType int

const (
	typeUnknown fileType = iota
	typeDir
	typeFile
)

func (t fileType) String() string {
	switch t {
	case typeUnknown:
		return "unknown"
	case typeDir:
		return "directory"
	case typeFile:
		return "file"
	}
	return "?"
}

type windowLayouter struct {
	layouter
	gtx    layout.Context
	window *Window
	style  Style

	// Temporary variable used to control later text drawing operations
	fgColor *color.NRGBA
}

func NewWindow(col *Col, style Style) *Window {
	w := &Window{
		layout: windowLayouter{
			style: style,
			layouter: layouter{
				fontStyles:  style.Fonts,
				lineSpacing: style.LineSpacing,
			},
		},
		col: col,
	}

	w.Id = application.WinIdGenerator().Get()
	w.layoutBox.window = w
	w.layout.window = w
	executor := NewCommandExecutor(w)
	w.initialTagUserArea = settings.Layout.WindowTagUserArea
	w.Tag.Init(&w.Body, style.tagBlockStyle(), style.tagEditableStyle(), executor, w, col.Scheduler)
	w.Body.Init(style.bodyBlockStyle(), style.bodyEditableStyle(), style.Syntax, executor, w, col.workChan)
	w.layoutBox.Init(style.layoutBoxStyle())
	w.scrollbar.Init(style.scrollbarStyle(), &w.Body)
	w.Body.AddTextChangeListener(w.redrawClonesOnTextChange)
	w.Body.AddTextChangeListener(w.disallowDirtyDelete)
	w.Body.AddTextChangeListener(w.notifyApiBodyChanged)
	w.setupInterception()
	w.AddPackingCoordChangeListener(w.layoutBox.WindowPackingCoordChanged)
	w.Body.completer = editor.Completer()
	w.fuzzySearch = NewFuzzySearcher(w, &w.Tag, &w.Body)

	return w
}

func (w *Window) setupInterception() {
	interceptor := &events.EventInterceptor{}
	w.scrollbar.eventInterceptor = interceptor
	interceptor.RegisterInterceptor(&w.layoutBox)
	interceptor.RegisterInterceptor(w)

	interceptor = &events.EventInterceptor{}
	w.layoutBox.eventInterceptor = interceptor
	interceptor.RegisterInterceptor(w)
}

func (c *Window) SetFocus(gtx layout.Context) {
	c.Body.AddOpForNextLayout(func(gtx layout.Context) {
		c.Body.SetFocus(gtx)
	})
}

func (c *Window) headerHeight() int {
	return c.layout.lineHeight()
}

func (c *Window) PackingCoord() float32 {
	return float32(c.TopY)
}

func (c *Window) SetPackingCoord(v float32) {
	old := c.TopY
	c.TopY = int(v)

	for _, l := range c.packingCoordChangedListeners {
		l(old, c.TopY)
	}
}

func (c *Window) AddPackingCoordChangeListener(f func(oldVal, newVal int)) {
	c.packingCoordChangedListeners = append(c.packingCoordChangedListeners, f)
}

// Layout handles events and draws the window.
// The window is drawn as large as gtx.Constraints.Max allows.
// TODO: the row layout should pass the right constraints
func (c *Window) Layout(gtx layout.Context) layout.Dimensions {
	//log(LogCatgWin,"Window.Layout: window %s: body marked at start: %v\n",
	//	c.file,
	//	c.Body.text.IsMarked())

	c.layout.layout(gtx)

	// In case the Tag's file has changed, update our file from it.
	oldPath := c.DisplayPath().String()
	c.UpdateFilenameFromTag()

	changedToOrFromUnnamedWindow := oldPath == "" && c.DisplayPath().String() != "" ||
		c.DisplayPath().String() == "" && oldPath != ""

	if c.tagShowsBodyAsChangedFromDisk != c.bodyChangedFromDisk() || changedToOrFromUnnamedWindow {
		c.SetTagFromDisplayPath()
	}
	c.tagShowsBodyAsChangedFromDisk = c.bodyChangedFromDisk()

	// Window takes up all available space.
	return layout.Dimensions{Size: gtx.Constraints.Max}
}

func (w *Window) bodyChangedFromDisk() bool {
	return !w.Body.text.IsMarked()
}

func (l *windowLayouter) layout(gtx layout.Context) {

	l.gtx = gtx

	wholeStack := op.Offset(image.Point{0, l.window.TopY}).Push(gtx.Ops)
	originalConstraints := gtx.Constraints

	// Draw the lefthand scrollbar and little movement box
	gutterDims := l.layoutGutter(gtx)

	// Translate all later draw operations so they are to the right of the gutter
	gtx.Constraints.Max.X = gtx.Constraints.Max.X - gutterDims.Size.X
	windowStack := op.Offset(image.Point{gutterDims.Size.X, 0}).Push(gtx.Ops)

	tagDims := l.window.Tag.layout(gtx)

	// Translate all later draw operations so they are below the tag
	gtx.Constraints.Max.Y = gtx.Constraints.Max.Y - tagDims.Size.Y
	op.Offset(image.Point{0, tagDims.Size.Y}).Add(gtx.Ops)
	l.window.bodyDims = l.window.Body.layout(gtx)

	// Draw a line (border) at the bottom of the window
	borderw := gtx.Metric.Dp(l.style.WinBorderWidth)
	op.Offset(image.Point{0, gtx.Constraints.Max.Y - borderw}).Add(gtx.Ops)
	gtx.Constraints.Max.Y = gtx.Constraints.Max.Y - borderw

	// Undo the translation pushing things to the right of the gutter
	gtx.Constraints.Max.X = gtx.Constraints.Max.X + gutterDims.Size.X
	op.Offset(image.Point{-gutterDims.Size.X, 0}).Add(gtx.Ops)

	// Already saves clip/transfor state
	l.drawBottomBorder(gtx)

	windowStack.Pop()

	l.overlayWithGrey(gtx, originalConstraints)

	wholeStack.Pop()

	l.gtx = layout.Context{}
}

func (l *windowLayouter) overlayWithGrey(gtx layout.Context, originalConstraints layout.Constraints) {
	if !l.window.overlayWithGrey {
		return
	}

	st := clip.Rect{
		Min: image.Pt(0, 0),
		Max: image.Pt(originalConstraints.Max.X, originalConstraints.Max.Y),
	}.Push(gtx.Ops)
	paint.ColorOp{Color: color.NRGBA{R: 0xc0, G: 0xc0, B: 0xc0, A: 0x80}}.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)
	st.Pop()

}

func (l *windowLayouter) layoutGutter(gtx layout.Context) layout.Dimensions {
	l.window.layoutBox.layout(gtx)

	// Translate a bit vertically to draw the scrollbar below the layoutBox
	st := op.Offset(image.Point{0, l.lineHeight()}).Push(gtx.Ops)
	l.window.scrollbar.layout(gtx)

	st.Pop()

	return layout.Dimensions{Size: image.Point{X: gtx.Metric.Dp(l.style.GutterWidth), Y: gtx.Constraints.Max.Y}}
}

func (l *windowLayouter) drawBottomBorder(gtx layout.Context) {
	paint.ColorOp{Color: color.NRGBA(l.style.WinBorderColor)}.Add(gtx.Ops)
	st := drawFilledBox(gtx, float32(gtx.Constraints.Max.X), float32(gtx.Metric.Dp(l.style.WinBorderWidth)))
	paint.PaintOp{}.Add(gtx.Ops)
	st.Pop()
}

func (l *windowLayouter) drawLayoutBox(gtx layout.Context) {
	l.window.layoutBox.draw(gtx)
}

func (l *windowLayouter) drawScrollbar(gtx layout.Context) {
	lh := int(l.lineHeight())

	gw := gtx.Metric.Dp(l.style.GutterWidth)
	gwless1 := gtx.Metric.Dp(l.style.GutterWidth - 1)

	// Draw a thick bar, then a thin right column
	st := clip.Rect{
		Min: image.Pt(0, lh),
		Max: image.Pt(gw, gtx.Constraints.Max.Y)}.Push(gtx.Ops)
	paint.ColorOp{Color: color.NRGBA(l.style.ScrollBgColor)}.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)
	st.Pop()

	bdy := l.window.Body
	textLen := len(bdy.Bytes())
	r := bdy.TopLeftIndex

	dist := 0
	if textLen > 0 {
		dist = (gtx.Constraints.Max.Y - lh) * r / textLen
	}

	disp, err := bdy.LenOfDisplayedTextInBytes(gtx)
	if err != nil {
		disp = lh
	}

	end := 0
	if textLen > 0 {
		end = (gtx.Constraints.Max.Y - lh) * (r + disp) / textLen
	}

	// Draw the button
	st = clip.Rect{
		Min: image.Pt(0, lh+dist),
		Max: image.Pt(gwless1, lh+end)}.Push(gtx.Ops)
	paint.ColorOp{Color: color.NRGBA(l.style.ScrollFgColor)}.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)
	st.Pop()
}

func removeFirstNRunes(b []byte, n int) []byte {
	for ; n > 0; n-- {
		_, size := utf8.DecodeRune(b)
		b = b[size:]
	}
	return b
}

func firstNRunes(b []byte, n int) (first, rest []byte, runeCount int) {
	off := 0
	for ; n > 0 && off < len(b); n-- {
		_, size := utf8.DecodeRune(b[off:])
		off += size
		runeCount++
	}
	first = b[0:off]
	rest = b[off:]
	return
}

func firstNRunesStr(s string, n int) (first, rest string, runeCount int) {
	b := []byte(s)
	fb, rb, runeCount := firstNRunes(b, n)
	first = string(fb)
	rest = string(rb)
	return
}

func (w *Window) SetTagFromDisplayPath() {
	w.Tag.label = fmt.Sprintf("tag of %s", w.DisplayPath())
	w.Body.label = fmt.Sprintf("body of %s", w.DisplayPath())

	if w.onlyShowBasenamesInTag {
		w.setTagToBasename()
		return
	}

	var t string
	if w.customEdCommandsSet() {
		t = w.customEdCommands
	} else if w.IsErrorsWindow() {
		t = w.edCommandsForErrorsWindow()
	} else if w.fileType == typeFile {
		t = w.edCommandsForFile()
	} else if w.fileType == typeDir {
		t = w.edCommandsForDir()
	} else {
		// This is usually an unnamed window
		t = w.edCommandsForUnknown()
	}

	userArea, err := w.userArea(w.DisplayPath().String())

	if err != nil {
		w.Tag.Set(w.DisplayPath().String(), t, "")
	} else {
		w.Tag.Set(w.DisplayPath().String(), t, userArea)
	}

}

func (c *Window) setTagToBasename() {
	path := c.DisplayPath().Base()
	editorArea := ""
	if c.Tag.layedoutText != nil && c.Tag.layedoutText.LineCount() > 1 {
		// Keep the tag the same height as when it displays the full path.
		// That way it is easy to visually track the window with the basename you found
		// when the full path is shown
		var buf bytes.Buffer
		for i := 0; i < c.Tag.layedoutText.LineCount()-1; i++ {
			buf.WriteRune('\n')
		}
		editorArea = buf.String()
	}

	c.Tag.Set(path, editorArea, "")
}

func (w *Window) edCommandsForFile() string {
	//log(LogCatgWin,"Window.fileTag: body marked: %v\n", c.Body.text.IsMarked())
	put := ""
	del := "Del"
	if w.bodyChangedFromDisk() {
		put = "Put"
		if w.allowDirtyDelete {
			del = "Del!"
		}
	}
	return fmt.Sprintf(" %s Snarf %s |", del, put)
}

func (c *Window) edCommandsForDir() string {
	return fmt.Sprintf(" Del Snarf Get |")
}

func (w *Window) edCommandsForUnknown() string {
	put := ""
	if w.DisplayPath().String() != "" {
		put = "Put "
	}
	del := "Del"
	if w.bodyChangedFromDisk() && w.allowDirtyDelete {
		del = "Del!"
	}
	return fmt.Sprintf(" %s Snarf %s|", del, put)
}

func (c *Window) edCommandsForErrorsWindow() string {
	return fmt.Sprintf(" Del Snarf |")
}

func (c *Window) customEdCommandsSet() bool {
	return c.customEdCommands != ""
}

func (c *Window) userArea(path string) (string, error) {
	var userArea string
	var err error

	if c.initialTagUserArea != "" {
		userArea = c.initialTagUserArea
		if IsErrorsWindow(path) && !strings.HasSuffix(userArea, " Clr") && !strings.Contains(userArea, " Clr ") {
			userArea = " Clr" + userArea
		}

		c.initialTagUserArea = ""
	}

	if userArea == "" {
		_, _, userArea, err = c.Tag.Parts()
	}

	return userArea, err
}

// markTextAsUnchanged marks the window body text to be the same as the
// contents on disk. This is used to decide whether to display the Put command.
func (w *Window) markTextAsUnchanged() {
	w.Body.text.Mark()
}

func (w *Window) LoadFile(displayPath, loadPath *GlobalPath) error {
	opts := LoadFileOpts{
		SelectBehaviour:   selectText,
		GrowBodyBehaviour: growBodyIfTooSmall,
	}
	return w.LoadFileOpts(displayPath, loadPath, opts)

}

func (w *Window) LoadFileOpts(displayPath, loadPath *GlobalPath, opts LoadFileOpts) error {
	var ldr FileLoader

	w.Body.SetTextString("")
	w.markTextAsUnchanged()

	loadData := true
	load, err := ldr.LoadAsync(loadPath.String())
	if err != nil {
		pe, ok := err.(*fs.PathError)
		// Don't consider the file not existing as fatal, just open an empty window
		if ok && errors.Is(pe, fs.ErrNotExist) {
			loadData = false
		} else {
			log(LogCatgWin, "Window.Load: error: %T %v\n", err, err)
			return err
		}
	}

	if loadData {
		wl := &WindowDataLoad{
			DataLoad: *load,
			Win:      NewWindowHolder(w),
			Jobname:  loadPath.Base(),
			Opts:     opts,
		}
		wl.Start(editor.WorkChan())
		editor.AddJob(wl)
	}

	w.SetPathsAndTag(displayPath, loadPath)

	w.RemoveUndoHistoryFromTag()

	return nil
}

func (w *Window) RemoveUndoHistoryFromTag() {
	w.Tag.SetTextStringNoUndo(w.Tag.String())
}

func (w *Window) Put() error {
	if w.DisplayPath().String() == "" {
		editor.AppendError("", "Can't Put: filename is empty")
		return fmt.Errorf("Can't Put with an empty filename")
	}

	completer := NewPathCompleterForColumn(w.col)
	p, _ := completer.Complete(w.DisplayPath().String())
	w.SetLoadPath(p)

	var ldr FileLoader
	b := w.Body.Bytes()

	save, err := ldr.SaveAsync(w.LoadPath().String(), b)
	if err != nil {
		log(LogCatgWin, "Window.Save: error: %v\n", err)
		editor.AppendError("", err.Error())
		return err
	}

	ws := &WindowDataSave{
		Jobname: w.DisplayPath().Base(),
		Win:     w,
		errs:    save.Errs,
		kill:    save.Kill,
	}
	ws.Start(editor.WorkChan())
	editor.AddJob(ws)

	return nil
}

func (w *Window) Get() error {
	return w.GetWithSelect(dontSelectText, growBodyIfTooSmall)
}

func (w *Window) GetWithSelect(selectBehaviour selectBehaviour, growBodyBehaviour growBodyBehaviour) error {
	ci := w.Body.blockEditable.firstCursorIndex()

	opts := LoadFileOpts{
		GoTo:              seek{seekType: seekToRunePos, runePos: ci},
		SelectBehaviour:   selectBehaviour,
		GrowBodyBehaviour: growBodyBehaviour,
	}

	// In case the user changed the display string, update the file we'll load
	if s := w.LoadPath().String(); s != "+Errors" && s != "+Live" && s != "" {
		completer := NewPathCompleterForColumn(w.col)
		p := completer.CompleteNoCheck(w.DisplayPath().String())
		if w.LoadPath().String() != p.String() {
			w.SetLoadPath(p)
		}
	}
	savedCursors := w.Tag.saveCursorsAndSelections()

	err := w.LoadFileOpts(w.DisplayPath(), w.LoadPath(), opts)
	if err != nil {
		return err
	}
	w.Tag.restoreCursorsAndSelections(savedCursors)

	return nil
}

type FillEditableWithItemList struct {
	items     []string
	render    *TextRenderer
	lastWidth int
}

func NewFillEditableWithItemList(l *layouter, style *Style, items []string) *FillEditableWithItemList {
	r := NewTextRenderer(l.curFont(), l.curFontSize(), l.lineSpacingScaled, Color{}, l.lineHeight)

	m := application.Metric()
	i := int(style.TabStopInterval)
	if m != nil {
		i = m.Dp(style.TabStopInterval)
	}
	r.SetTabStopInterval(i)

	return &FillEditableWithItemList{
		items:  items,
		render: r,
	}
}

func (f *FillEditableWithItemList) AppendItems(items []string) {
	f.items = append(f.items, items...)
	f.lastWidth = 0 // Force a redraw
}

func (f *FillEditableWithItemList) preDrawHook(e *editable, gtx layout.Context) {
	w := gtx.Constraints.Max.X
	if w == f.lastWidth {
		return
	}

	b := f.render.LayoutItemsInColumns(gtx, f.items)
	// Add a few extra blank lines to make it easy to append commands to the end of the directory output.
	b = append(b, '\n')
	b = append(b, '\n')
	e.SetText(b)
	f.lastWidth = w
}

func (c *Window) SetPathsAndTag(displayPath, loadPath *GlobalPath) {
	c.SetDisplayPath(displayPath)
	c.SetLoadPath(loadPath)
	//c.displayPath.EnsureDirEndsInSlash()
	c.makeDisplayPathRelativeToColPath()
	c.ensureDirEndsInSlash()
	c.fileType = fileType(loadPath.DirState())
	c.setBodyCompletionSource()
	c.SetTagFromDisplayPath()
}

func (c *Window) makeDisplayPathRelativeToColPath() {
	colPath, ok := c.col.Path()
	if !ok {
		return
	}

	cp := colPath.String()
	if strings.HasSuffix(cp, "/") || strings.HasSuffix(cp, "\\") {
		cp = cp[:len(cp)-1]
	}
	dp := c.LoadPath().String()

	if !strings.HasPrefix(dp, cp) {
		return
	}

	log(LogCatgWin, "Window.makeDisplayPathRelativeToColPath: colpath: %s loadpath: %s\n", cp, dp)

	if dp == cp {
		c.SetDisplayPath(NewGlobalPath(".", c.LoadPath().DirState()))
		log(LogCatgWin, "Window.makeDisplayPathRelativeToColPath(1): changing displaypath to '%s'\n", c.displayPath)
		return
	}

	// Only consider the column path valid if it consists of full components separated by the path separator.
	// That is, if the window path is '/usr/share/file.txt', don't consider a column path '/usr/sha' as a valid
	// prefix.
	if strings.HasPrefix(dp, cp) && (strings.HasSuffix(cp, "/") || strings.HasSuffix(cp, "\\")) {
		dp = dp[len(cp):]
		c.SetDisplayPath(NewGlobalPath(dp, c.LoadPath().DirState()))
		log(LogCatgWin, "Window.makeDisplayPathRelativeToColPath(2): changing displaypath to '%s'\n", c.DisplayPath())
		return
	}

	if len(dp) > len(cp) && (dp[len(cp)] == '/' || dp[len(cp)] == '\\') {
		dp = dp[len(cp)+1:]
		if dp == "" {
			dp = "."
		}
		c.displayPath = *NewGlobalPath(dp, c.LoadPath().DirState())
		log(LogCatgWin, "Window.makeDisplayPathRelativeToColPath(3): changing displaypath to '%s'\n", c.DisplayPath())
	}
}

func (c *Window) ensureDirEndsInSlash() {
	if c.DisplayPath().DirState() != GlobalPathIsDir {
		return
	}

	colPath, ok := c.col.Path()
	if !ok {
		return
	}

	remote := c.DisplayPath().IsRemote()
	if !remote && !isWindowsPath(c.DisplayPath().Path()) && colPath.IsRemote() {
		remote = true
	}

	sep := string(filepath.Separator)
	if remote {
		sep = "/"
	}

	if !strings.HasSuffix(c.DisplayPath().Path(), sep) {
		c.displayPath.SetPath(c.DisplayPath().Path() + sep)
	}

}

func (w *Window) UpdateFilenameFromTag() {
	// We support filenames with spaces using the same heuristic as Russ Cox used for acme in plan9port:
	// See https://github.com/rsc/plan9port/commit/6267213474dd5449c161ca2e68ee16d9c0ffba07
	/*  " |" ends left half of tag
	 * If we find " Del Snarf" in the left half of the tag
	 * (before the pipe), that ends the file name.
	 */
	tag := string(w.Tag.Bytes())
	n := strings.Index(tag, " |")
	if n < 0 {
		return
	}

	n = strings.Index(tag[:n], " Del")
	if n < 0 {
		return
	}

	fname := tag[:n]

	if fname == "" {
		w.SetLoadPath(&GlobalPath{})
		w.SetDisplayPath(&GlobalPath{})
		return
	}

	if fname == "+Errors" || fname == "+Live" {
		return
	}

	// TODO: We are doing this on every layout; even for other windows! We should only do it if the tag actually changed. But
	// if we do, we have the issue where the user changes the column and we never learn about it and update our path relative to the colomn.
	// Maybe on column tag change, we should update this instead.
	if w.col != nil {
		completer := NewPathCompleterForColumn(w.col)
		w.SetLoadPath(completer.CompleteNoCheck(fname))
		w.SetDisplayPath(NewGlobalPath(fname, w.LoadPath().DirState()))
		// This code handles a special case. Say the window path is /home/user/src/anvil-suite/anvil/src/anvil, and that it is a directory. Then the user
		// cuts the beginning of the path so we are left with src/anvil. We don't know now whether the prefix represents a directory or not. We could
		// check, but if the path is remote that is costly. Instead we assume the filetype didn't change; if it was a directory, it is still a directory.
		// This is not perfect, but should work in most cases. The alternative is that other code that needs to tell if the path is a directory or not
		// (such as adapter.execute) can resolve it at that time.
		if w.LoadPath().DirState() == GlobalPathUnknown {
			w.LoadPath().SetDirState(GlobalPathDirState(w.fileType))
			w.DisplayPath().SetDirState(GlobalPathDirState(w.fileType))
		}
	}

	w.setBodyCompletionSource()
}

func (c *Window) Append(b []byte) {
	c.Body.Append(b)
}

func (c *Window) Zerox() (nw *Window, err error) {
	if c.fileType == typeDir {
		err = fmt.Errorf("not allowed on directories\n")
		return
	}

	nw = editor.NewWindow(nil)
	if nw == nil {
		err = fmt.Errorf("failed to create window\n")
		return
	}

	// The body of the new window and the current window will share the same piece table
	nw.Body.text = c.Body.text

	nw.SetPathsAndTag(c.DisplayPath().Clone(), c.LoadPath().Clone())

	c.addClone(nw)
	nw.addClone(c)

	nw.Body.blockEditable.CursorIndices = make([]int, len(c.Body.blockEditable.CursorIndices))
	copy(nw.Body.blockEditable.CursorIndices, c.Body.blockEditable.CursorIndices)
	nw.Body.blockEditable.TopLeftIndex = c.Body.blockEditable.TopLeftIndex

	nw.maybeEnableSyntax()
	return
}

func (c *Window) BodyHeight() int {
	return c.bodyDims.Size.Y
}

func (w *Window) GrowIfBodyTooSmall() {
	if w.BodyHeight() < w.layout.lineHeight()*9 && w.col != nil {
		w.col.Grow(w)
	}
}

func (w *Window) addClone(c *Window) {
	if w.clones == nil {
		w.clones = make(map[*Window]struct{})
	}

	w.clones[c] = struct{}{}
}

func (w *Window) removeClone(c *Window) {
	if w.clones == nil {
		return
	}

	delete(w.clones, c)
}

func (w *Window) hasClones() bool {
	return len(w.clones) > 0
}

func (w *Window) redrawClonesOnTextChange(ch *TextChange) {
	for c := range w.clones {
		if c == w {
			continue
		}

		// Don't notify us.
		c.Body.textChanged(dontFireListeners, *ch)

		c.Body.AddOpForNextLayout(func(gtx layout.Context) {
			if ch.Length != 0 {
				log(LogCatgWin, "redrawClonesOnTextChange: changing top left index of editable from %d to %d\n", c.Body.TopLeftIndex, c.Body.TopLeftIndex+ch.Length)
				w.shiftClonesTopLeftDueToTextModification(&c.Body, ch)
				c.Body.shiftItemsDueToTextModification(ch.Offset, ch.Length)
			}
			// This is to force a redraw
			c.Body.invalidateLayedoutText()
		})
	}
}

func (w *Window) shiftClonesTopLeftDueToTextModification(cloneBody *Body, ch *TextChange) {
	if cloneBody.TopLeftIndex >= ch.Offset {
		cloneBody.TopLeftIndex += ch.Length
	}
}

func (w *Window) removeFromAllClones() {
	for c := range w.clones {
		if c == w {
			continue
		}

		c.removeClone(w)
	}
}

func (w *Window) maybeEnableSyntax() {
	if w.fileType == typeFile {
		w.Body.EnableSyntax(w.DisplayPath().String())
		w.setBodyCompletionSource()
		w.Body.BuildCompletions()
		w.Body.HighlightSyntax()
	}
}

func (w *Window) IsErrorsWindow() bool {
	return IsErrorsWindow(w.DisplayPath().String())
}

func IsErrorsWindow(windowFilename string) bool {
	return strings.HasSuffix(windowFilename, "+Errors")
}

func (w *Window) IsLiveWindow() bool {
	return IsLiveWindow(w.DisplayPath().String())
}

func IsLiveWindow(windowFilename string) bool {
	return strings.HasSuffix(windowFilename, "+Live")
}

func (w *Window) CanDelete() bool {
	if w.IsErrorsWindow() || w.IsLiveWindow() || w.fileType == typeDir {
		return true
	}

	if w.hasClones() {
		// If there are clones, we're just closing a view of the window; there's still
		// one open.
		return true
	}

	if w.bodyChangedFromDisk() && !w.allowDirtyDelete {
		return false
	}
	return true
}

func (w *Window) SetAllowDirtyDelete(b bool) {
	w.allowDirtyDelete = b
}

func (w *Window) disallowDirtyDelete(c *TextChange) {
	w.SetAllowDirtyDelete(false)
}

func (w *Window) notifyApiBodyChanged(c *TextChange) {
	n := ApiNotification{
		WinId:  w.Id,
		Offset: c.Offset,
		Len:    c.Length,
	}

	if c.Length >= 0 {
		n.Op = ApiNotificationOpInsert
	} else {
		n.Op = ApiNotificationOpDelete
		n.Len = -n.Len
	}

	addApiNotificationToAllSessions(n)
}

func (w *Window) notifyPut() {
	n := ApiNotification{
		WinId: w.Id,
		Op:    ApiNotificationOpPut,
	}

	addApiNotificationToAllSessions(n)
}

func (w *Window) SetStyle(style Style) {
	w.layout.style = style
	w.layout.setFontStyles(style.Fonts)
	w.layout.layouter.lineSpacing = style.LineSpacing
	w.Tag.SetStyle(style.tagBlockStyle(), style.tagEditableStyle())
	w.Body.SetStyle(style.bodyBlockStyle(), style.bodyEditableStyle(), style.Syntax)
	w.layoutBox.SetStyle(style.layoutBoxStyle())
	w.scrollbar.SetStyle(style.scrollbarStyle())
}

func (w *Window) showIfHidden() {
	max := w.col.MaximizedWindow()
	if max != nil && max != w {
		w.col.Optimize()
	}
}

func (w *Window) setFlash(b bool) {
	w.Tag.SetFlash(b)
}

func (w *Window) InterceptEvent(gtx layout.Context, ev event.Event) (processed bool) {
	// This is used to snoop events from the scrollbar and layoutbox in order to
	// mark windows as unflashed whenever a scrollbar or layoutbox is clicked.
	pe, ok := ev.(*pointer.Event)
	if !ok {
		return
	}

	if pe.Kind != pointer.Press {
		return
	}

	editor.SetOnlyFlashedWindow(nil)
	return false
}

func (w *Window) centerBodyOnFirstCursorOrPrimarySelection() {
	w.Body.AddOpForNextLayout(func(gtx layout.Context) {
		w.Body.centerOnFirstCursorOrPrimarySelection(gtx)
	})
}

func (w *Window) setBodyCompletionSource() {
	src := w.DisplayPath().String()
	if src == "" {
		src = fmt.Sprintf("unnamed-%p", w.Body.Tag())
	}
	w.Body.completionSource = src
}

func (w *Window) greyoutIfOpIsTakingTooLong(opFinished chan struct{}) {
	if w == nil {
		return
	}

	tmr := time.NewTimer(2 * time.Second)

	greyout := func() {
		w.overlayWithGrey = true
	}

	unGreyout := func() {
		w.overlayWithGrey = false
	}

loop:
	for {
		select {
		case <-tmr.C:
			editor.WorkChan() <- basicWork{greyout}
		case <-opFinished:
			editor.WorkChan() <- basicWork{unGreyout}
			break loop
		}
	}
}

func (w *Window) setOnlyShowBasenamesInTag(only bool) {
	w.onlyShowBasenamesInTag = only
	if only {
		// Here we save the old user area so that it is restored when
		// the set the tag back to the normal display.
		_, _, userArea, err := w.Tag.Parts()
		if err == nil {
			w.initialTagUserArea = userArea
		}
	}
}

func (w *Window) setInsertWhenTabPressed(s string) {
	w.insertWhenTabPressed = s
}

func (w *Window) getInsertWhenTabPressed() string {
	return w.insertWhenTabPressed
}

func (w *Window) DisplayPath() *GlobalPath {
	return &w.displayPath
}

func (w *Window) SetDisplayPath(p *GlobalPath) {
	w.displayPath = *p
}

func (w *Window) LoadPath() *GlobalPath {
	return &w.loadPath
}

func (w *Window) SetLoadPath(p *GlobalPath) {
	w.loadPath = *p
}
