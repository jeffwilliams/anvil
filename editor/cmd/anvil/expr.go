package main

import (
	"bytes"
	"context"
	"fmt"
	"unicode/utf8"

	"gioui.org/layout"
	"github.com/Takatochi/go-tee-lib/tee"
	"github.com/jeffwilliams/anvil/editor/internal/escape"
	"github.com/jeffwilliams/anvil/editor/internal/expr"
	"github.com/jeffwilliams/anvil/editor/internal/pctbl"
	"github.com/jeffwilliams/anvil/editor/internal/runes"
	"github.com/jeffwilliams/anvil/editor/internal/sync"
)

var ExprHandlerBatchSize = 200

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
	batch           []exprHandlerOp
}

func (handler *ExprHandler) Delete(r expr.Range) {
	l := r.End() - r.Start()
	log(LogCatgExpr, "editable expr handler: performing delete of length %d at %d", l, r.Start())
	handler.clearRuneOffsetCache()
	handler.sendDelete(r.Start(), l)
}

func (handler *ExprHandler) Copy(r expr.Range) {
	w := runes.NewWalker(handler.data)
	b := w.TextBetweenRuneIndices(r.Start(), r.End())
	handler.toCopy.Write(b)
	log(LogCatgExpr, "editable expr handler: performing copy of %d to %d", r.Start(), r.End())

	handler.sendSelectRange(r)
}

func (handler *ExprHandler) Insert(index int, value []byte) {
	log(LogCatgExpr, "editable expr handler: performing insert of '%s' at %d", string(value), index)
	handler.clearRuneOffsetCache()
	handler.sendInsert(index, value)
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
	handler.sendSelectRange(r)
}

func (handler *ExprHandler) Noop(r expr.Range) {
	handler.sendSelectRange(r)
}

func (handler *ExprHandler) Done() {
	handler.sendBatchWork(handler.batch)
	handler.sendFnWork(handler.done)
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

func (handler *ExprHandler) sendSelectRange(r expr.Range) {
	var op exprHandlerOp
	op.opcode = exprHandlerOpcodeSelectRange
	rangeStart, rangeEnd := op.PropsForSelect()
	*rangeStart, *rangeEnd = r.Start(), r.End()

	handler.addToBatchAndSendIfFull(op)
}

func (handler *ExprHandler) sendDelete(start, len int) {
	var op exprHandlerOp
	op.opcode = exprHandlerOpcodeDelete
	opStart, opLen := op.PropsForDelete()
	*opStart, *opLen = start, len

	handler.addToBatchAndSendIfFull(op)
}

func (handler *ExprHandler) sendInsert(index int, value []byte) {
	var op exprHandlerOp
	op.opcode = exprHandlerOpcodeInsert
	opIndex, opValue := op.PropsForInsert()
	*opIndex, *opValue = index, value

	handler.addToBatchAndSendIfFull(op)
}

func (handler *ExprHandler) addToBatchAndSendIfFull(op exprHandlerOp) {
	handler.batch = append(handler.batch, op)

	if len(handler.batch) >= ExprHandlerBatchSize {
		b := make([]exprHandlerOp, len(handler.batch))
		copy(b, handler.batch)
		handler.sendBatchWork(b)
		handler.batch = handler.batch[:0]
	}
}

func (handler *ExprHandler) sendBatchWork(batch []exprHandlerOp) {
	if len(batch) == 0 {
		return
	}
	editor.WorkChan() <- exprHandlerBatchWork{handler, batch}
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

func (ex EditableExprExecutor) Do(cmd string) sync.Future {
	ok := ex.createInterpreter(cmd)
	if !ok {
		return sync.CompletedFuture
	}

	ranges := ex.buildInitialRanges()
	ex.log(cmd, ranges)
	//ex.runInterpreter(ranges)
	return ex.runInterpreterAsync(ranges)
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

func (ex *EditableExprExecutor) runInterpreterAsync(initialRanges []expr.Range) sync.Future {
	ex.editable.StartTransaction()
	ex.editable.writeLock.lock()
	// The code that saves deletes in OptimizedPieceTable is slow and we don't need
	// it when doing expressions.
	ex.editable.SetSaveDeletes(false)

	finished := make(chan struct{})
	finishedTee := tee.NewTee[struct{}](2, 0)
	c := finishedTee.GetOutputChannels()

	future := sync.NewFuture()

	go ex.win.greyoutIfOpIsTakingTooLong(c[0])
	go func() {
		<-c[1]
		future.Done()
	}()

	finishedTee.Run(context.Background(), finished)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				dumpPanicFiles(r)
				panic(r)
			}
		}()

		err := ex.vm.Execute(initialRanges)
		editor.WorkChan() <- basicWork{func() {
			ex.editable.writeLock.unlock()
			ex.editable.SetSaveDeletes(true)
			ex.editable.EndTransaction()
		}}
		finished <- struct{}{}
		if err != nil {
			editor.AppendError(ex.dir, err.Error())
			return
		}
	}()

	return future
}

type exprHandlerBatchWork struct {
	handler *ExprHandler
	batch   []exprHandlerOp
}

func (w exprHandlerBatchWork) Service() (done bool) {
	w.handler.editable.writeLock.unlock()
	for _, e := range w.batch {
		e.Do(w.handler)
	}
	w.handler.editable.writeLock.lock()
	return true
}

func (w exprHandlerBatchWork) Job() Job {
	return nil
}

type exprHandlerOp struct {
	opcode int
	ints   [2]int
	bytes  []byte
}

type exprHandlerOpcode int

const (
	exprHandlerOpcodeSelectRange = iota
	exprHandlerOpcodeDelete
	exprHandlerOpcodeInsert
)

func (op *exprHandlerOp) PropsForSelect() (rangeStart *int, rangeEnd *int) {
	return &op.ints[0], &op.ints[1]
}

func (op *exprHandlerOp) PropsForDelete() (start *int, len *int) {
	return &op.ints[0], &op.ints[1]
}

func (op *exprHandlerOp) PropsForInsert() (index *int, value *[]byte) {
	return &op.ints[0], &op.bytes
}

func (o *exprHandlerOp) Do(handler *ExprHandler) {
	switch o.opcode {
	case exprHandlerOpcodeSelectRange:
		rangeStart, rangeEnd := o.PropsForSelect()
		handler.editable.AddSelection(*rangeStart, *rangeEnd)
	case exprHandlerOpcodeDelete:
		start, len := o.PropsForDelete()
		handler.editable.deleteFromPieceTableUndoIndex(*start, *len, handler.cursorIndex)
	case exprHandlerOpcodeInsert:
		index, value := o.PropsForInsert()
		handler.editable.insertToPieceTableUndoIndex(*index, string(*value), handler.cursorIndex)
		l := utf8.RuneCount(*value)
		handler.editable.AddSelection(*index, *index+l)
	}
}

func (handler *ExprHandler) sendFnWork(f func()) {
	editor.WorkChan() <- exprHandlerFnWork{handler, f}
}

type exprHandlerFnWork struct {
	handler *ExprHandler
	f       func()
}

func (w exprHandlerFnWork) Service() (done bool) {
	w.handler.editable.writeLock.unlock()
	w.f()
	w.handler.editable.writeLock.lock()
	return true
}

func (w exprHandlerFnWork) Job() Job {
	return nil
}
