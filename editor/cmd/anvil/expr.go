package main

import (
	"bytes"
	"fmt"
	"unicode/utf8"

	"gioui.org/layout"
	"github.com/jeffwilliams/anvil/editor/internal/escape"
	"github.com/jeffwilliams/anvil/editor/internal/expr"
	"github.com/jeffwilliams/anvil/editor/internal/pctbl"
	"github.com/jeffwilliams/anvil/editor/internal/runes"
)

type ExprHandler struct {
	pieceTable pctbl.Table
	// Call this after one of the changes below occurs
	afterChanged    func()
	file            string
	dir             string
	data            []byte
	editable        *editable
	toDisplay       bytes.Buffer
	cursorIndex     int
	toCopy          bytes.Buffer
	runeOffsetCache *runes.OffsetCache
}

func (handler *ExprHandler) Delete(r expr.Range) {
	l := r.End() - r.Start()
	log(LogCatgExpr, "editable expr handler: performing delete of length %d at %d", l, r.Start())
	handler.clearRuneOffsetCache()
	handler.sendWork(func() {
		handler.editable.deleteFromPieceTableUndoIndex(r.Start(), l, handler.cursorIndex)
	})
}

func (handler *ExprHandler) Copy(r expr.Range) {
	w := runes.NewWalker(handler.data)
	b := w.TextBetweenRuneIndices(r.Start(), r.End())
	handler.toCopy.Write(b)
	log(LogCatgExpr, "editable expr handler: performing copy of %d to %d", r.Start(), r.End())

	handler.sendWork(func() {
		handler.selectRange(r)
	})
}

func (handler *ExprHandler) Insert(index int, value []byte) {
	log(LogCatgExpr, "editable expr handler: performing insert of '%s' at %d", string(value), index)
	l := utf8.RuneCount(value)
	handler.clearRuneOffsetCache()
	handler.sendWork(func() {
		handler.editable.insertToPieceTableUndoIndex(index, string(value), handler.cursorIndex)
		s := NewSelection(index, index+l, Right)
		handler.selectRange(s)
	})
}

func (handler *ExprHandler) Display(r expr.Range) {
	sline, scol, fline, fcol := handler.rangeLinesAndCols(r)

	if sline == fline {
		fmt.Fprintf(&handler.toDisplay, "%s:%d ", handler.file, sline)
		if scol != fcol {
			fmt.Fprintf(&handler.toDisplay, "( %s:%d:%d )", handler.file, sline, scol)
		}
	} else {
		fmt.Fprintf(&handler.toDisplay, "%s:%d – %s:%d ", handler.file, sline, handler.file, fline)
		fmt.Fprintf(&handler.toDisplay, "( %s:%d:%d – %s:%d:%d )", handler.file, sline, scol, handler.file, fline, fcol)
	}
	handler.toDisplay.WriteRune('\n')
}

func (handler *ExprHandler) rangeLinesAndCols(r expr.Range) (startLine, startCol, endLine, endCol int) {
	// TODO: use cache for this. We would need to store the number of newlines in the cache along with the
	// rune to byte mappings
	w := runes.NewWalker(handler.data)

	line := 1
	col := 0
	i := 0

	lastr := ' '
	for ; i <= r.Start(); i++ {
		if lastr == '\n' {
			line++
			col = 0
		}

		lastr = w.Rune()
		w.Forward(1)

		col++
	}

	startLine = line
	startCol = col
	startResolved := true
	if lastr == '\n' {
		startResolved = false
	}

	for ; i < r.End(); i++ {
		if lastr == '\n' {
			line++
			col = 0
		}

		lastr = w.Rune()
		w.Forward(1)

		col++

		if lastr != '\n' && !startResolved {
			// The range started on a newline, and we learned that there
			// are more runes after the newline, so we can shift the start to
			// be at the beginning of the next line
			startLine = line
			startCol = col
			startResolved = true
		}
	}

	if !startResolved {
		// The range started on a newline, and there was no
		// later non-newline character. Treat the start as
		// one character before the newline
		if startCol > 0 {
			startCol--
		}
	}

	endLine = line
	endCol = col
	if lastr == '\n' {
		endCol--
	}

	return
}

func (handler *ExprHandler) DisplayContents(r expr.Range, prefix string, displayPosition bool) {
	w := runes.NewWalker(handler.data)
	b := w.TextBetweenRuneIndicesCache(r.Start(), r.End(), handler.getRuneOffsetCache())
	handler.toDisplay.WriteString(escape.ExpandEscapes(prefix))
	if displayPosition {
		sline, scol, _, _ := handler.rangeLinesAndCols(r)
		fmt.Fprintf(&handler.toDisplay, "%s:%d:%d ", handler.file, sline, scol)
	}
	handler.toDisplay.Write(b)

	handler.sendWork(func() {
		handler.selectRange(r)
	})
}

func (handler ExprHandler) Noop(r expr.Range) {
	handler.sendWork(func() {
		handler.selectRange(r)
	})
}

func (handler ExprHandler) selectRange(r expr.Range) {
	handler.editable.AddSelection(r.Start(), r.End())
}

func (handler ExprHandler) Done() {
	handler.sendWork(handler.done)
}

func (handler ExprHandler) done() {
	if handler.toDisplay.Len() > 0 {
		editor.AppendError(handler.dir, handler.toDisplay.String())
	}

	if handler.toCopy.Len() > 0 {
		handler.editable.AddOpForNextLayout(func(gtx layout.Context) {
			handler.editable.writeTextToClipboard(gtx, handler.toCopy.String())
		})
	}

	if handler.afterChanged != nil {
		handler.afterChanged()
	}
}

func (handler ExprHandler) sendWork(f func()) {
	editor.WorkChan() <- exprHandlerWork{handler.editable, f}
}

func (handler *ExprHandler) getRuneOffsetCache() *runes.OffsetCache {
	if handler.runeOffsetCache == nil {
		c := runes.NewOffsetCache(0)
		handler.runeOffsetCache = &c
	}
	return handler.runeOffsetCache
}

func (handler *ExprHandler) clearRuneOffsetCache() {
	if handler.runeOffsetCache == nil {
		return
	}
	handler.runeOffsetCache.Clear()
}

type EditableExprExecutor struct {
	editable *editable
	handler  *ExprHandler
	dir      string
	vm       expr.Interpreter
	win      *Window
}

func NewEditableExprExecutor(e *editable, win *Window, dir string, handler *ExprHandler) EditableExprExecutor {
	return EditableExprExecutor{editable: e,
		handler: handler,
		dir:     dir,
		win:     win,
	}
}

func (ex EditableExprExecutor) Do(cmd string) {
	ok := ex.createInterpreter(cmd)
	if !ok {
		return
	}

	ranges := ex.buildInitialRanges()
	ex.log(cmd, ranges)
	//ex.runInterpreter(ranges)
	ex.runInterpreterAsync(ranges)
}

func (ex *EditableExprExecutor) createInterpreter(cmd string) (ok bool) {
	var s expr.Scanner
	toks, ok := s.Scan(cmd)
	if !ok {
		editor.AppendError(ex.dir, "Scanning addressing expression failed")
		return false
	}

	var p expr.Parser
	p.SetMatchLimit(1000)
	tree, err := p.Parse(toks)
	if err != nil {
		editor.AppendError(ex.dir, err.Error())
		return false
	}

	ex.vm, err = expr.NewInterpreter(ex.handler.data, tree, ex.handler, ex.editable.firstCursorIndex())
	if err != nil {
		editor.AppendError(ex.dir, err.Error())
		return false
	}

	return true
}

func (ex *EditableExprExecutor) buildInitialRanges() []expr.Range {
	ranges := make([]expr.Range, len(ex.editable.selections))
	for i, sel := range ex.editable.selections {
		ranges[i] = sel
	}
	if len(ranges) == 0 {
		ranges = append(ranges, textRange{0, utf8.RuneCount(ex.handler.data)})
	}

	return ranges
}

func (ex *EditableExprExecutor) log(cmd string, ranges []expr.Range) {
	log(LogCatgCmd, "Executing addressing expression %s on ranges ", cmd)
	for _, r := range ranges {
		log(LogCatgCmd, "(%d,%d) ", r.Start(), r.End())
	}
}

func (ex *EditableExprExecutor) runInterpreter(initialRanges []expr.Range) {
	ex.editable.StartTransaction()

	err := ex.vm.Execute(initialRanges)
	ex.editable.EndTransaction()
	if err != nil {
		editor.AppendError(ex.dir, err.Error())
		return
	}
}

func (ex *EditableExprExecutor) runInterpreterAsync(initialRanges []expr.Range) {
	ex.editable.StartTransaction()
	ex.editable.writeLock.lock()
	// The code that saves deletes in OptimizedPieceTable is slow and we don't need
	// it when doing expressions.
	ex.editable.SetSaveDeletes(false)

	finished := make(chan struct{})
	go ex.win.greyoutIfOpIsTakingTooLong(finished)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				dumpPanic(r)
				dumpLogs()
				dumpGoroutines()
				panic(r)
			}
		}()

		err := ex.vm.Execute(initialRanges)
		editor.WorkChan() <- basicWork{func() {
			ex.editable.writeLock.unlock()
			ex.editable.SetSaveDeletes(true)
		}}
		ex.editable.EndTransaction()
		finished <- struct{}{}
		if err != nil {
			editor.AppendError(ex.dir, err.Error())
			return
		}
	}()
}

type exprHandlerWork struct {
	editable *editable
	f        func()
}

func (w exprHandlerWork) Service() (done bool) {
	w.editable.writeLock.unlock()
	w.f()
	w.editable.writeLock.lock()
	return true
}

func (w exprHandlerWork) Job() Job {
	return nil
}
