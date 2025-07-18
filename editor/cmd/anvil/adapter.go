package main

import (
	"fmt"

	"gioui.org/layout"
)

// adapter is the interface between the editable and the environment it is
// embedded in (the editor)
type adapter interface {
	completeFilename(word string, callback CompletionsCallback)
	completeCommand(word string, callback CompletionsCallback)
	appendError(dir, msg string)
	clearErrors()
	copyAllSelectionsFromLastSelectedEditable(gtx layout.Context)
	cutAllSelectionsFromLastSelectedEditable(gtx layout.Context)
	textOfAllSelectionsInLastSelectedEditable() []string
	pasteToFocusedEditable(gtx layout.Context)
	execute(e *editable, gtx layout.Context, cmd string, args []string)
	plumb(e *editable, gtx layout.Context, obj string) (plumbed bool)
	loadFile(gtx layout.Context, displayPath, loadPath *GlobalPath, opts LoadFileOpts)
	loadFileInPlace(gtx layout.Context, displayPath, loadPath *GlobalPath, opts LoadFileOpts)
	textOfLastSelectionInEditor() string
	shiftEditorItemsDueToTextModification(startOfChange, lengthOfChange int)
	setFocusedEditable(e *editable)
	focusedEditable() *editable
	completer() *PathCompleter
	dir() string
	put()
	get()
	displayPath() *GlobalPath
	loadPath() *GlobalPath
	mark(markName string, displayPath, loadPath *GlobalPath, cursorIndex int)
	gotoMark(markName string)
	doWork(w Work)
	addJob(j Job)
	replaceCrWithTofu() bool
	setShellString(s string)
	addOpForNextLayout(op LayoutOp)
	setEditableWhereTertiaryButtonHoldStarted(ed *editable)
	getEditableWhereTertiaryButtonHoldStarted() *editable
	clearEditableWhereTertiaryButtonHoldStarted()
	style() Style
	setStyle(s Style)
	insertWhenTabPressed() string
}

// editableAdapter connects an editable with the rest of the editor (it's owning window, etc)
// so that it has less dependencies
type editableAdapter struct {
	executor *CommandExecutor
	// owner is the owner of the editable: a Window, Col or Editor.
	owner                           interface{}
	shellString                     string
	omitWindowPathWhenResolvingPath bool
}

func (a editableAdapter) completeFilename(word string, callback CompletionsCallback) {
	dir, base, err := computeDirAndBaseForFilenameCompletion(word, a.completer())
	log(LogCatgCompletion, "adapter: Complete filename on dir='%s' base='%s'\n", dir, base)

	// This will call editable.applyFilenameCompletions when complete
	err = FilenameCompletionsAsync(word, dir, base, callback)
	if err != nil {
		editor.AppendError(dir, err.Error())
		return
	}
}

func (a editableAdapter) completeCommand(word string, callback CompletionsCallback) {
	dir, err := computeDirForCommandCompletion(a.completer())
	log(LogCatgCompletion, "adapter: Complete command on dir='%s'\n", dir)

	err = CommandCompletionsAsync(word, dir, a.executor, callback)
	if err != nil {
		editor.AppendError(dir, err.Error())
		return
	}
}

func (a editableAdapter) appendError(dir, msg string) {
	editor.AppendError(dir, msg)
}

func (a editableAdapter) clearErrors() {
	dir := a.completer().Dir().String()
	editor.ClearErrors(dir)
}

func (a editableAdapter) copyAllSelectionsFromLastSelectedEditable(gtx layout.Context) {
	editor.copyAllSelectionsFromLastSelectedEditable(gtx)
}

func (a editableAdapter) cutAllSelectionsFromLastSelectedEditable(gtx layout.Context) {
	editor.cutAllSelectionsFromLastSelectedEditable(gtx)
}

func (a editableAdapter) pasteToFocusedEditable(gtx layout.Context) {
	editor.pasteToFocusedEditable(gtx)
}

func (a editableAdapter) execute(e *editable, gtx layout.Context, cmd string, args []string) {
	if args == nil {
		args = []string{}
	}

	log(LogCatgCmd, "adapter: Execute '%s' %v\n", cmd, args)
	if a.executor != nil {
		ctx := a.buildCmdContext(e, gtx, args)
		ctx.RawCommand = cmd
		a.executor.Do(cmd, ctx)
	}
}

func (a editableAdapter) dir() string {
	return a.completer().Dir().String()
}

func (a editableAdapter) completer() *PathCompleter {
	var completer *PathCompleter

	switch v := a.executor.source.(type) {
	case *Window:
		if a.omitWindowPathWhenResolvingPath {
			completer = NewPathCompleterForWindowOmitWinPath(v)
		} else {
			completer = NewPathCompleterForWindow(v)
		}
	case *Col:
		completer = NewPathCompleterForColumn(v)
	default:
		completer = NewPathCompleter()
	}

	return completer
}

func (a editableAdapter) buildCmdContext(e *editable, gtx layout.Context, args []string) *CmdContext {
	completer := a.completer()

	dir := a.completer().Dir().String()
	// TODO: the PathCompleter.Dir() function doesn't do a full check to see if the file is a directory or not.
	// The user might have modified the file path and then tried executing a command like `pwd`. SHould we do a
	// proper check here on the filetype if the path is unknown?

	return &CmdContext{Gtx: gtx,
		Completer:   completer,
		Dir:         dir,
		Editable:    e.executeOn,
		Args:        args,
		Selections:  e.selections,
		ShellString: a.shellString,
	}
}
func (a *editableAdapter) setShellString(s string) {
	a.shellString = s
}

func (a editableAdapter) plumb(e *editable, gtx layout.Context, obj string) (plumbed bool) {
	if plumber != nil && a.executor != nil {
		ctx := a.buildCmdContext(e, gtx, nil)
		var err error
		plumbed, err = plumber.Plumb(obj, a.executor, ctx)
		if err != nil {
			log(LogCatgPlumb, "adapter: Error plumbing: %v\n", err)
		}
	}
	return
}

func (a editableAdapter) column() *Col {
	var col *Col

	switch v := a.owner.(type) {
	case Window:
	case *Window:
		col = v.col
	case Col:
		col = &v
	case *Col:
		col = v
	}

	return col
}

func (a editableAdapter) loadFile(gtx layout.Context, displayPath, loadPath *GlobalPath, opts LoadFileOpts) {
	opts.InCol = a.column()
	w := editor.LoadFileOpts(displayPath, loadPath, opts)
	if w != nil {
		w.SetFocus(gtx)
	}
}

func (a editableAdapter) loadFileInPlace(gtx layout.Context, displayPath, loadPath *GlobalPath, opts LoadFileOpts) {
	win, ok := a.owner.(*Window)
	if !ok {
		return
	}

	err := win.LoadFileOpts(displayPath, loadPath, opts)
	if err != nil {
		log(LogCatgWin, "adapter: Loading file into window failed: %v\n", err)
	}
}

func (a editableAdapter) textOfLastSelectionInEditor() string {
	sel := editor.lastSelection
	if sel.isSet && sel.editable != nil {
		return sel.editable.textOfSelection(sel.sel)
	}
	return ""
}

func (a editableAdapter) textOfAllSelectionsInLastSelectedEditable() []string {
	sel := editor.lastSelection
	ed := sel.editable
	if !sel.isSet || ed == nil {
		return nil
	}

	res := []string{}
	for _, s := range ed.selections {
		res = append(res, ed.textOfSelection(s))
	}
	return res
}

func (a editableAdapter) shiftEditorItemsDueToTextModification(startOfChange, lengthOfChange int) {
	editor.Marks.ShiftDueToTextModification(a.loadPath(), startOfChange, lengthOfChange)
}

func (a editableAdapter) setFocusedEditable(e *editable) {
	w := (*Window)(nil)
	if win, ok := a.owner.(*Window); ok {
		w = win
	}

	editor.setFocusedEditable(e, w)
}

func (a editableAdapter) focusedEditable() *editable {
	return editor.getFocusedEditable()
}

func (a editableAdapter) put() {
	w, ok := a.owner.(*Window)
	if ok {
		w.Put()
	}
}

func (a editableAdapter) get() {
	w, ok := a.owner.(*Window)
	if ok {
		w.Get()
	}
}

func (a editableAdapter) displayPath() *GlobalPath {
	w, ok := a.owner.(*Window)
	if !ok {
		return nil
	}
	return &w.displayPath
}

func (a editableAdapter) loadPath() *GlobalPath {
	w, ok := a.owner.(*Window)
	if !ok {
		return nil
	}
	return &w.loadPath
}

func (a editableAdapter) mark(markName string, displayPath, loadPath *GlobalPath, cursorIndex int) {
	editor.Marks.Set(markName, displayPath, loadPath, cursorIndex)
}

func (a editableAdapter) gotoMark(markName string) {
	displayPath, globalPath, seek, ok := editor.Marks.Seek(markName)
	if ok {
		editor.LoadFileOpts(displayPath, globalPath, LoadFileOpts{GoTo: seek, SelectBehaviour: dontSelectText})
	}
}

func (a editableAdapter) doWork(w Work) {
	editor.WorkChan() <- w
}

func (a editableAdapter) addJob(j Job) {
	editor.AddJob(j)
}

func (a editableAdapter) replaceCrWithTofu() bool {
	return settings.Typesetting.ReplaceCRWithTofu
}

func (a editableAdapter) addOpForNextLayout(op LayoutOp) {
	editor.AddOpForNextLayout(op)
}

func (a editableAdapter) setEditableWhereTertiaryButtonHoldStarted(ed *editable) {
	editor.setEditableWhereTertiaryButtonHoldStarted(ed)
}

func (a editableAdapter) getEditableWhereTertiaryButtonHoldStarted() *editable {
	return editor.getEditableWhereTertiaryButtonHoldStarted()
}

func (a editableAdapter) clearEditableWhereTertiaryButtonHoldStarted() {
	editor.clearEditableWhereTertiaryButtonHoldStarted()
}

func (a editableAdapter) style() Style {
	return WindowStyle
}

func (a editableAdapter) setStyle(s Style) {
	WindowStyle = s
	editor.SetStyle(s)
}

func (a editableAdapter) insertWhenTabPressed() string {
	win, ok := a.owner.(*Window)
	if !ok {
		return editor.getInsertWhenTabPressed()
	}

	s := win.getInsertWhenTabPressed()
	if s == "" {
		s = editor.getInsertWhenTabPressed()
	}
	return s
}

type nilAdapter struct{}

func (a nilAdapter) completeFilename(word string, callback CompletionsCallback)         {}
func (a nilAdapter) completeCommand(word string, callback CompletionsCallback)          {}
func (a nilAdapter) appendError(dir, msg string)                                        {}
func (a nilAdapter) copyAllSelectionsFromLastSelectedEditable(gtx layout.Context)       {}
func (a nilAdapter) cutAllSelectionsFromLastSelectedEditable(gtx layout.Context)        {}
func (a nilAdapter) textOfAllSelectionsInLastSelectedEditable() []string                { return nil }
func (a nilAdapter) pasteToFocusedEditable(gtx layout.Context)                          {}
func (a nilAdapter) execute(e *editable, gtx layout.Context, cmd string, args []string) {}
func (a nilAdapter) plumb(e *editable, gtx layout.Context, obj string) (plumbed bool)   { return false }
func (a nilAdapter) loadFile(gtx layout.Context, displayPath, loadPath *GlobalPath, opts LoadFileOpts) {
}
func (a nilAdapter) loadFileInPlace(gtx layout.Context, displayPath, loadPath *GlobalPath, opts LoadFileOpts) {
}
func (a nilAdapter) textOfLastSelectionInEditor() string                                     { return "" }
func (a nilAdapter) shiftEditorItemsDueToTextModification(startOfChange, lengthOfChange int) {}
func (a nilAdapter) setFocusedEditable(e *editable)                                          {}
func (a nilAdapter) focusedEditable() *editable                                              { return nil }
func (a nilAdapter) completer() *PathCompleter                                               { return nil }
func (a nilAdapter) completeN(file string, n int) (path *GlobalPath, err error) {
	return nil, fmt.Errorf("not implemented")
}
func (a nilAdapter) dir() string                                                              { return "" }
func (a nilAdapter) put()                                                                     {}
func (a nilAdapter) get()                                                                     {}
func (a nilAdapter) displayPath() *GlobalPath                                                 { return nil }
func (a nilAdapter) loadPath() *GlobalPath                                                    { return nil }
func (a nilAdapter) mark(markName string, displayPath, loadPath *GlobalPath, cursorIndex int) {}
func (a nilAdapter) gotoMark(markName string)                                                 {}
func (a nilAdapter) doWork(w Work)                                                            {}
func (a nilAdapter) replaceCrWithTofu() bool                                                  { return false }
func (a nilAdapter) setShellString(s string)                                                  {}
func (a nilAdapter) addOpForNextLayout(op LayoutOp)                                           {}
func (a nilAdapter) addJob(j Job)                                                             {}
func (a nilAdapter) setEditableWhereTertiaryButtonHoldStarted(ed *editable)                   {}
func (a nilAdapter) getEditableWhereTertiaryButtonHoldStarted() *editable                     { return nil }
func (a nilAdapter) clearEditableWhereTertiaryButtonHoldStarted()                             {}
func (a nilAdapter) style() Style                                                             { return Style{} }
func (a nilAdapter) setStyle(s Style)                                                         {}
func (a nilAdapter) insertWhenTabPressed() string                                             { return "\t" }
func (a nilAdapter) clearErrors()                                                             {}
