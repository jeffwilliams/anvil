package main

import (
	"bytes"
	"slices"
	"sort"
	"unicode"
	"unicode/utf8"

	"github.com/jeffwilliams/anvil/editor/internal/intvl"
	"github.com/jeffwilliams/anvil/editor/internal/pctbl"
	"github.com/jeffwilliams/anvil/editor/internal/runes"
	"github.com/jeffwilliams/anvil/editor/internal/words"
)

// EditableModel contains the state of an editable that is
// related to the text and text metainformation (cursor positions, selections, etc.)
// It excludes layout and event related state.
type editableModel struct {
	text                       pctbl.Table
	adapter                    adapter
	CursorIndices              []int
	TopLeftIndex               int
	selections                 []*selection
	typingInSelectedTextAction typingInSelectedTextAction
	primarySel                 *selection
	primarySelPurpose          selectionPurpose
	selectionBeingBuilt        *selection
	immutableRange             textRange
	syntaxTokens               []intvl.Interval
	completer                  *words.Completer
	// overridingCursorIndices specifies a list of cursor indices
	// that override where cursors are displayed.
	overridingCursorIndices  []int
	wordCompletion           completion
	fileCompletion           completion
	commandCompletion        completion
	manualHighlighting       []*SyntaxInterval
	runeOffsetCache          runes.OffsetCache
	matchingBracketInsertion matchingBracketInsertion
	writeLock                editableWriteLock
	recentlyTypedText        textRange
	// label is a name for this editable model used for debugging
	label string
}

func (e *editableModel) SetTextString(s string) {
	if e.writeLock.isLocked() {
		return
	}
	e.resetWhenAllTextReplaced()
	e.text.SetStringWithUndo(s)
}

func (e *editableModel) SetTextStringNoUndo(s string) {
	if e.writeLock.isLocked() {
		return
	}
	e.resetWhenAllTextReplaced()
	e.text.SetString(s)
}

func (e *editableModel) SetText(b []byte) {
	if e.writeLock.isLocked() {
		return
	}
	e.resetWhenAllTextReplaced()
	e.text.SetWithUndo(b)
}

func (e *editableModel) resetWhenAllTextReplaced() {
	if e.writeLock.isLocked() {
		return
	}
	e.clearSelections()
	e.CursorIndices = []int{0}
	e.TopLeftIndex = 0
}

func (e *editableModel) SetTextStringNoReset(s string) {
	if e.writeLock.isLocked() {
		return
	}
	e.text.SetString(s)
}

func (e *editableModel) Append(b []byte) {
	if e.writeLock.isLocked() {
		return
	}
	if e.text.Len() == 0 {
		text := e.Bytes()
		text = append(text, b...)
		e.text.Set(text)
	} else {
		e.text.Append(string(b))
	}
}

func (e editableModel) String() string {
	return e.text.String()
}

func (e editableModel) Bytes() []byte {
	return e.text.Bytes()
}

func (e *editableModel) removeFirstNRunes(doc []byte, runeOffset int) (data []byte, runeCount int) {
	byteOffset, err, runeCount := e.runeOffsetCache.Get(doc, runeOffset)
	if err != nil {
		log(LogCatgEd, "RuneOffsetCache.Get returned error: %v\n", err)
	}
	if byteOffset >= len(doc) {
		return
	}
	data = doc[byteOffset:]
	return
}

func (e *editableModel) firstNRunes(doc []byte, n int) (data []byte, runeCount int) {
	byteOffset, err, runeCount := e.runeOffsetCache.Get(doc, n)
	if err != nil {
		log(LogCatgEd, "RuneOffsetCache.Get returned error: %v\n", err)
	}
	data = doc[:byteOffset]
	return
}

func (e *editableModel) textObjectForAcquireAt(runeIndex int) (string, textRange) {
	return e.textObjectAt(runeIndex, false)
}

func (e *editableModel) textObjectForExecutionAt(runeIndex int) (string, textRange) {
	return e.textObjectAt(runeIndex, true)
}

func (e *editableModel) textObjectForSearchAt(runeIndex int) (string, textRange) {
	return e.textObjectAt(runeIndex, true)
}

func (e *editableModel) textObjectAt(runeIndex int, considerLozenges bool) (string, textRange) {
	w := runes.NewWalker(e.Bytes())
	sel := e.selectionContaining(runeIndex)
	if sel != nil {
		return string(w.TextBetweenRuneIndicesCache(sel.start, sel.end, &e.runeOffsetCache)), textRange{sel.start, sel.end}
	}

	w.SetRunePosCache(runeIndex, &e.runeOffsetCache)

	getWord := true
	var s string
	var tr textRange

	if considerLozenges {
		var wasDelimited bool
		s, wasDelimited = w.CurrentLozengeDelimitedStringInLine()
		tr.start, tr.end = w.CurrentLozengeDelimitedStringInLineBounds()
		if wasDelimited {
			getWord = false
		}
	}

	if getWord {
		s = w.CurrentWord()
		tr.start, tr.end = w.CurrentWordBounds()
	}

	return s, tr
}

type completionContext struct {

	// prefix is the initial prefix of the word to complete. It is
	// the substring from the start of the word to the cursor. If there is
	// instead a selection it is the entire selection.
	prefix string
	// word is the complete word under the cursor, or the selection
	word             string
	prefixStartIndex int
	prefixEndIndex   int
	wordStartIndex   int
	wordEndIndex     int
}

func (e *editableModel) wordObjectToComplete(runeIndex int) completionContext {
	w := runes.NewWalker(e.Bytes())
	w.SetRunePosCache(runeIndex, &e.runeOffsetCache)
	return e.itemToComplete(w, runeIndex, w.CurrentIdentifierBounds)
}

func (e *editableModel) filenameObjectToComplete(runeIndex int) completionContext {
	w := runes.NewWalker(e.Bytes())
	w.SetRunePosCache(runeIndex, &e.runeOffsetCache)
	return e.itemToComplete(w, runeIndex, w.CurrentWordBounds)
}

func (e *editableModel) itemToComplete(w runes.Walker, runeIndex int, getBounds func() (leftRuneIndex, rightRuneIndex int)) completionContext {
	sel := e.selectionContaining(runeIndex)
	if sel != nil {
		return completionContext{
			prefix:           string(w.TextBetweenRuneIndices(sel.start, sel.end)),
			word:             string(w.TextBetweenRuneIndices(sel.start, sel.end)),
			prefixStartIndex: sel.start,
			prefixEndIndex:   sel.end,
			wordStartIndex:   sel.start,
			wordEndIndex:     sel.end,
		}
	}

	// Take from the start of the identifier or word to the current cursor position
	leftRuneIndex, rightRuneIndex := getBounds()

	return completionContext{
		prefix: string(w.TextBetweenRuneIndices(leftRuneIndex, runeIndex)),
		word:   string(w.TextBetweenRuneIndices(leftRuneIndex, rightRuneIndex)),
		// TODO:
		prefixStartIndex: leftRuneIndex,
		prefixEndIndex:   runeIndex,
		wordStartIndex:   leftRuneIndex,
		wordEndIndex:     rightRuneIndex,
	}

}

func (e *editableModel) shiftItemsDueToTextModification(startOfChange, lengthOfChange int) {
	if e.writeLock.isLocked() {
		return
	}
	e.shiftSelectionsDueToTextModification(startOfChange, lengthOfChange)
	e.shiftSyntaxTokensDueToTextModification(startOfChange, lengthOfChange)
	e.shiftManualHighlightsDueToTextModification(startOfChange, lengthOfChange)
	e.adapter.shiftEditorItemsDueToTextModification(startOfChange, lengthOfChange)
	e.shiftCursorsDueToTextModification(startOfChange, lengthOfChange)
	e.shiftCompletersDueToTextModification(startOfChange, lengthOfChange)
}

func (e *editableModel) shiftCursorsDueToTextModification(startOfChange, lengthOfChange int) {
	for i, ndx := range e.CursorIndices {

		cursor := NewTextRange(ndx, ndx)
		newIndex, _ := computeShiftNeededDueToTextModificationBounds(&cursor, startOfChange, lengthOfChange, changeAtBoundsIsNotWithinSelection)
		e.CursorIndices[i] = newIndex
	}
}

func (e *editableModel) shiftSyntaxTokensDueToTextModification(startOfChange, lengthOfChange int) {
	if e.syntaxTokens != nil {
		for _, t := range e.syntaxTokens {
			if i, ok := t.(*SyntaxInterval); ok {
				i.start, i.end = computeShiftNeededDueToTextModification(i, startOfChange, lengthOfChange)
			}
		}
	}
}

func (e *editableModel) shiftManualHighlightsDueToTextModification(startOfChange, lengthOfChange int) {
	for _, i := range e.manualHighlighting {
		i.start, i.end = computeShiftNeededDueToTextModification(i, startOfChange, lengthOfChange)
	}
}

func (e *editableModel) shiftCompletersDueToTextModification(startOfChange, lengthOfChange int) {
	e.wordCompletion.shiftDueToTextModification(startOfChange, lengthOfChange)
	e.fileCompletion.shiftDueToTextModification(startOfChange, lengthOfChange)
	e.commandCompletion.shiftDueToTextModification(startOfChange, lengthOfChange)
}

type changeAtSelectionBoundsBehaviour int

const (
	changeAtBoundsIsWithinSelection changeAtSelectionBoundsBehaviour = iota
	changeAtBoundsIsNotWithinSelection
)

type deleteBehavour int

const (
	deleteBehaviourShiftSelection deleteBehavour = iota
	deleteBehaviourErodeSelection
)

func computeShiftNeededDueToTextModification(i intvl.Interval, startOfChange, lengthOfChange int) (deltaStart, deltaEnd int) {
	return computeShiftNeededDueToTextModificationBounds(i, startOfChange, lengthOfChange, changeAtBoundsIsWithinSelection)
}

func computeShiftNeededDueToTextModificationBounds(i intvl.Interval, startOfChange, lengthOfChange int, b changeAtSelectionBoundsBehaviour) (deltaStart, deltaEnd int) {
	return computeShiftNeededDueToTextModificationBoundsExt(i, startOfChange, lengthOfChange, b, deleteBehaviourErodeSelection)
}

func computeShiftNeededDueToTextModificationBoundsExt(i intvl.Interval, startOfChange, lengthOfChange int, b changeAtSelectionBoundsBehaviour, db deleteBehavour) (deltaStart, deltaEnd int) {
	if i.End() <= startOfChange {
		return i.Start(), i.End()
	}

	if db == deleteBehaviourErodeSelection {
		endOfChange := startOfChange - lengthOfChange
		if lengthOfChange < 0 {
			if i.Start() >= startOfChange && i.Start() < endOfChange {
				// This is a delete and the interval overlaps the end of the delete.
				// Truncate the interval so that it is a smaller interval only consisting of the remainder
				// of the interval after the change. Then shift that interval left by the amount of the
				// change.
				if i.End() > endOfChange {
					shift := -lengthOfChange
					return endOfChange - shift, i.End() - shift
				} else {
					// Empty interval
					return endOfChange, endOfChange
				}
			} else if i.End() >= startOfChange && i.End() < endOfChange {
				return i.Start(), startOfChange
			}
		}
	}

	if b == changeAtBoundsIsWithinSelection {
		if i.Start() > startOfChange {
			return i.Start() + lengthOfChange, i.End() + lengthOfChange
		}
	} else {
		if i.Start() >= startOfChange {
			return i.Start() + lengthOfChange, i.End() + lengthOfChange
		}
	}

	// Change is inside this interval.
	deltaStart = i.Start()
	deltaEnd = i.End() + lengthOfChange
	if deltaEnd < deltaStart {
		deltaEnd = deltaStart
	}
	return
}

func (e *editableModel) insertToPieceTable(index int, text string) {
	if e.writeLock.isLocked() {
		return
	}
	e.insertToPieceTableUndoIndex(index, text, index)
}

func (e *editableModel) insertToPieceTableUndoIndex(index int, text string, undoIndex int) {
	if e.writeLock.isLocked() {
		return
	}
	l := utf8.RuneCountInString(text)
	// We actually only care if the user is trying to _start_ typing or pasting somewhere inside
	// the immutable range. If the paste would overlap the range but starts before it that is fine
	// because we will shift the immutable range after it.
	if e.affectsImmutableRange(index, index) {
		return
	}

	ud := newUndoDataForInsert(index, l, undoIndex)
	e.text.InsertWithUndoData(index, text, ud)
	e.shiftItemsDueToTextModification(index, l)
}

func (e *editableModel) deleteFromPieceTable(index, length int) {
	if e.writeLock.isLocked() {
		return
	}
	e.deleteFromPieceTableUndoIndex(index, length, index)
}

func (e *editableModel) deleteFromPieceTableUndoIndex(index, length, undoIndex int) {
	if e.writeLock.isLocked() {
		return
	}
	if e.affectsImmutableRange(index, index+length) {
		return
	}

	ud := newUndoDataForDelete(index, length, undoIndex)
	opt, ok := e.text.(*pctbl.OptimizedPieceTable)
	if ok {
		opt.DeleteWithUndoDataCache(index, length, ud, &e.runeOffsetCache)
	} else {
		e.text.DeleteWithUndoData(index, length, ud)
	}
	e.shiftItemsDueToTextModification(index, -length)
}

func (e *editableModel) affectsImmutableRange(changeStartIndex, changeEndIndex int) bool {
	if e.immutableRange.Len() != 0 {
		r := NewTextRange(changeStartIndex, changeEndIndex)
		if r.Overlaps(&e.immutableRange) {
			return true
		}
	}
	return false
}

func (e *editableModel) firstCursorIndex() int {
	if len(e.CursorIndices) == 0 {
		log(LogCatgEd, "Error: there are no cursor indices")
		return 0
	}
	return e.CursorIndices[0]
}

func (e *editableModel) lastCursorIndex() int {
	if len(e.CursorIndices) == 0 {
		log(LogCatgEd, "Error: there are no cursor indices")
		return 0
	}
	return e.CursorIndices[len(e.CursorIndices)-1]
}

func (e *editableModel) setToOneCursorIndex(ndx int) {
	if e.writeLock.isLocked() {
		return
	}
	if len(e.CursorIndices) < 1 {
		log(LogCatgEd, "Error: there are no cursor indices")
		e.CursorIndices = []int{0}
		return
	}

	e.CursorIndices[0] = ndx
	e.CursorIndices = e.CursorIndices[0:1]
}

func (e *editableModel) SetCursorIndex(cursor, index int) {
	if e.writeLock.isLocked() {
		return
	}
	if cursor >= 0 && cursor < len(e.CursorIndices) {
		e.CursorIndices[cursor] = index
	}
}

func (e *editableModel) removeCursorAt(ndx int) (removed bool) {
	if e.writeLock.isLocked() {
		return
	}
	if len(e.CursorIndices) == 1 {
		return false
	}
	for i, v := range e.CursorIndices {
		if v == ndx {
			newIndices := make([]int, len(e.CursorIndices)-1)
			copy(newIndices, e.CursorIndices[:i])
			if i != len(e.CursorIndices)-1 {
				copy(newIndices[i:], e.CursorIndices[i+1:])
			}
			e.CursorIndices = newIndices
			return true
		}
	}
	return false
}

func (e *editableModel) replaceAllSelectionsWith(text string) {
	if e.writeLock.isLocked() {
		return
	}
	for _, sel := range e.selections {
		e.replaceSelectionWith(sel, text)
	}
}

func (e *editableModel) replaceSelectionWith(sel *selection, text string) {
	if e.writeLock.isLocked() {
		return
	}
	st := sel.start

	e.deleteFromPieceTableUndoIndex(sel.start, sel.Len(), e.firstCursorIndex())
	e.insertToPieceTable(st, text)

	sel.start = st
	sel.end = sel.start + utf8.RuneCountInString(text)
}

func (e *editableModel) replacePrimarySelectionWith(text string) {
	if e.writeLock.isLocked() {
		return
	}
	if e.primarySel != nil {
		e.replaceSelectionWith(e.primarySel, text)
	}
}

func (e *editableModel) appendToAllSelections(text string) {
	if e.writeLock.isLocked() {
		return
	}
	for _, sel := range e.selections {
		e.appendToSelection(sel, text)
	}
}

func (e *editableModel) appendToSelection(sel *selection, text string) {
	if e.writeLock.isLocked() {
		return
	}
	e.insertToPieceTable(sel.end, text)
	delta := utf8.RuneCountInString(text)
	sel.end += delta
}

func (e *editableModel) appendToPrimarySelection(text string) {
	if e.writeLock.isLocked() {
		return
	}
	if e.primarySel != nil {
		e.appendToSelection(e.primarySel, text)
	}
}

func (e *editableModel) InsertBeforeAndAfterAllSelections(before, after string) {
	if e.writeLock.isLocked() {
		return
	}
	e.text.StartTransaction()
	for _, sel := range e.selections {
		e.insertToPieceTable(sel.start, before)
		e.insertToPieceTable(sel.end, after)
	}
	e.text.EndTransaction()
}

func (e *editableModel) SetTopLeft(topLeft int) {
	if e.writeLock.isLocked() {
		return
	}
	w := runes.NewWalker(e.Bytes())
	w.SetRunePosCache(topLeft, &e.runeOffsetCache)
	w.BackwardToStartOfLine()
	e.TopLeftIndex = w.RunePos()
}

func (e *editableModel) DeleteTextAt(index, length int) {
	if e.writeLock.isLocked() {
		return
	}
	e.deleteFromPieceTable(index, length)
}

func (e *editableModel) AddSelection(start, end int) {
	if e.writeLock.isLocked() {
		return
	}
	e.addSecondarySelection(start, end, Left)
}

func (e *editableModel) moveCursorToStartOfPrimarySelection() {
	if e.writeLock.isLocked() {
		return
	}
	if e.primarySel != nil {
		e.setToOneCursorIndex(e.primarySel.Start())
	}
}

func (e *editableModel) StartTransaction() {
	if e.writeLock.isLocked() {
		return
	}
	e.text.StartTransaction()
}

func (e *editableModel) EndTransaction() {
	if e.writeLock.isLocked() {
		return
	}
	e.text.EndTransaction()
}

func (e *editableModel) RotateSelections() {
	if e.writeLock.isLocked() {
		return
	}
	savedSelections := make([]*selection, len(e.selections))
	copy(savedSelections, e.selections)

	sort.Slice(savedSelections, func(i, j int) bool {
		return savedSelections[i].Start() < savedSelections[j].Start()
	})

	shiftSavedSelections := func(startOfChange, lenOfChange int, b changeAtSelectionBoundsBehaviour) {
		for k := 0; k < len(savedSelections); k++ {
			sel := savedSelections[k]
			sel.start, sel.end = computeShiftNeededDueToTextModificationBoundsExt(sel, startOfChange, lenOfChange, b, deleteBehaviourShiftSelection)
		}
	}

	newSelections := make([]*selection, 0, len(e.selections))

	shiftNewSelections := func(startOfChange, lenOfChange int, b changeAtSelectionBoundsBehaviour) {
		for k := 0; k < len(newSelections); k++ {
			sel := newSelections[k]
			sel.start, sel.end = computeShiftNeededDueToTextModificationBoundsExt(sel, startOfChange, lenOfChange, b, deleteBehaviourShiftSelection)
		}
	}

	e.clearSelections()
	e.StartTransaction()
	for fromi := 0; fromi < len(savedSelections); fromi++ {
		//for fromi := 0; fromi < 1; fromi++ {
		toi := (fromi + 1) % len(savedSelections)
		from := savedSelections[fromi]
		to := savedSelections[toi]

		// Cut the text from the `from` selection. Shift all the selections left, except for the
		// `from` selection which we shrink to len 0.
		text := e.textOfSelection(from)
		e.deleteFromPieceTable(from.Start(), from.Len())
		shiftSavedSelections(from.Start(), -from.Len(), changeAtBoundsIsWithinSelection)
		shiftNewSelections(from.Start(), -from.Len(), changeAtBoundsIsNotWithinSelection)

		// Paste the text to just before the `to` selection. Shift all the selections right, including the `to`
		// selection, since the text it should highlight has been shifted right due to the insert
		e.insertToPieceTable(to.Start(), text)
		start := to.Start()
		shiftSavedSelections(to.Start(), utf8.RuneCountInString(text), changeAtBoundsIsNotWithinSelection)
		shiftNewSelections(to.Start(), utf8.RuneCountInString(text), changeAtBoundsIsNotWithinSelection)

		newSelections = append(newSelections, NewSelectionPtr(start, start+utf8.RuneCountInString(text), Right))
	}
	e.EndTransaction()

	// Apply new selections
	for _, s := range newSelections {
		e.addSelection(s)
	}

}

func (e *editableModel) removeDuplicateCursors() {
	if e.writeLock.isLocked() {
		return
	}
	seen := make(map[int]struct{})
	newCursors := make([]int, 0, len(e.CursorIndices))

	i := 0
	for _, v := range e.CursorIndices {
		if _, ok := seen[v]; ok {
			continue
		}
		newCursors = append(newCursors, v)
		seen[v] = struct{}{}
		i++
	}

	e.CursorIndices = newCursors
}

func (e *editableModel) changeSelectionsToCursors(sideToMakeCursorOn horizontalDirection) {
	if e.writeLock.isLocked() {
		return
	}
	e.CursorIndices = e.CursorIndices[:0]
	for _, s := range e.selections {
		ndx := s.start
		if sideToMakeCursorOn == Right {
			ndx = s.end
		}
		e.CursorIndices = append(e.CursorIndices, ndx)
	}
	e.removeDuplicateCursors()
	e.clearSelections()
	return
}

func (e *editableModel) makeCursorAtEachLineInSelections() {
	if e.writeLock.isLocked() {
		return
	}
	if !e.SelectionsPresent() {
		return
	}

	e.CursorIndices = e.CursorIndices[:0]
	for _, s := range e.selections {

		e.CursorIndices = append(e.CursorIndices, s.start)
		w := runes.NewWalker(e.Bytes())
		w.SetRunePosCache(s.start, &e.runeOffsetCache)

		for {
			w.ForwardToEndOfLine()
			w.Forward(1)
			if w.RunePos() >= s.end {
				break
			}
			e.CursorIndices = append(e.CursorIndices, w.RunePos())
		}
	}
	e.removeDuplicateCursors()
	e.clearSelections()
	return
}

func (e *editableModel) reduceCursorsToOne() {
	if e.writeLock.isLocked() {
		return
	}
	if e.SelectionsPresent() {
		return
	}

	e.CursorIndices = e.CursorIndices[:1]
	return
}

func (e *editable) InsertLozenge() {
	if e.writeLock.isLocked() {
		return
	}
	if e.SelectionsPresent() {
		e.InsertBeforeAndAfterAllSelections("◊", "◊")
		e.contractSelectionsOnLeftBy(1)
	} else {
		e.InsertText("◊")
	}
}

func (e *editableModel) Len() int {
	return e.text.Len()
}

func (e *editableModel) AddManualHighlightForEachSelection(fgColor, bgColor Color) {
	if e.writeLock.isLocked() {
		return
	}
	for _, s := range e.selections {
		e.AddManualHighlight(s.start, s.end, fgColor, bgColor)
	}
}

func (e *editableModel) AddManualHighlight(start, end int, fgColor, bgColor Color) {
	if e.writeLock.isLocked() {
		return
	}
	e.addManualHighlightIgnoreLock(start, end, fgColor, bgColor)
}

func (e *editableModel) addManualHighlightIgnoreLock(start, end int, fgColor, bgColor Color) {
	if end <= start {
		return
	}

	s := NewSyntaxInterval(start, end, fgColor, bgColor)
	for i, m := range e.manualHighlighting {
		// TODO: If the interval is the same color as the existing one, replace the existing one with
		// the union of the two.
		if intvl.Overlaps(s, m) {
			v := intvl.Diff(m, s)
			e.manualHighlighting[i].start = v.Start()
			e.manualHighlighting[i].end = v.End()
		}
	}
	e.manualHighlighting = append(e.manualHighlighting, s)
	e.manualHighlighting = slices.DeleteFunc(e.manualHighlighting, func(n *SyntaxInterval) bool {
		return intvl.Len(n) == 0
	})
}

func (e *editableModel) AddManualHighlights(a []SyntaxInterval) {
	if e.writeLock.isLocked() {
		return
	}
	for _, v := range a {
		e.addManualHighlightIgnoreLock(v.start, v.end, v.fgColor, v.bgColor)
	}
}

func (e *editableModel) ManualHighlights() []*SyntaxInterval {
	return e.manualHighlighting
}

func (e *editableModel) ClearManualHighlights() {
	if e.writeLock.isLocked() {
		return
	}
	e.manualHighlighting = e.manualHighlighting[:0]
}

func (e *editableModel) ClearSelectedManualHighlights() {
	if e.writeLock.isLocked() {
		return
	}
	var toRemove []*SyntaxInterval

	for _, s := range e.selections {
		for _, m := range e.manualHighlighting {
			if intvl.Overlaps(s, m) {
				toRemove = append(toRemove, m)
			}
		}
	}

	if len(toRemove) == 0 {
		return
	}

	var toKeep []*SyntaxInterval
manual:
	for _, m := range e.manualHighlighting {
		for _, r := range toRemove {
			if m == r {
				continue manual
			}
		}
		toKeep = append(toKeep, m)
	}
	e.manualHighlighting = toKeep
}

func (e *editableModel) SetSaveDeletes(b bool) {
	opt, ok := e.text.(*pctbl.OptimizedPieceTable)
	if !ok {
		return
	}
	opt.SetSaveDeletes(b)
}

func (e *editableModel) updateRecentlyTypedTextWithInsertion(index, length int) {
	if !e.recentlyTypedText.Valid() {
		e.recentlyTypedText.start = index
		e.recentlyTypedText.end = index + length
		log(LogCatgEd, "recently typed: insertion starts new recently typed text as %d,%d.\n", e.recentlyTypedText.start, e.recentlyTypedText.end)
		return
	}

	if len(e.CursorIndices) > 1 || e.SelectionsPresent() {
		log(LogCatgEd, "recently typed: more than one cursor or selections present. resetting recently typed text\n")
		e.recentlyTypedText.start = -1
		return
	}

	if index != e.recentlyTypedText.end {
		log(LogCatgEd, "recently typed: insertion at %d but last typing ended at %d. ignoring and resetting.\n", index, e.recentlyTypedText.end)
		e.recentlyTypedText.start = -1
		return
	}

	e.recentlyTypedText.end += length
	log(LogCatgEd, "recently typed: insertion updates recently typed text to %d,%d\n", e.recentlyTypedText.start, e.recentlyTypedText.end)
}

func (e *editable) updateRecentlyTypedTextWithDeletion(index, length int) {
	if !e.recentlyTypedText.Valid() {
		log(LogCatgEd, "recently typed: deletion when recently typed text is invalid\n")
		return
	}

	if len(e.CursorIndices) > 1 || e.SelectionsPresent() {
		log(LogCatgEd, "recently typed: more than one cursor or selections present. resetting recently typed text\n")
		e.recentlyTypedText.start = -1
		return
	}

	if index+length != e.recentlyTypedText.end {
		//if index != e.recentlyTypedText.end {
		log(LogCatgEd, "recently typed: deletion at %d of length %d, which ends at %d but last typing ended at %d. ignoring and resetting.\n", index, length, index+length, e.recentlyTypedText.end)
		e.recentlyTypedText.start = -1
		return
	}

	e.recentlyTypedText.end -= length
	log(LogCatgEd, "recently typed: deletion updates recently typed text to %d,%d\n", e.recentlyTypedText.start, e.recentlyTypedText.end)
}

func (e *editable) selectRecentlyTypedText() {
	if !e.recentlyTypedText.Valid() {
		return
	}

	w := runes.NewWalker(e.Bytes())
	w.SetRunePosCache(e.recentlyTypedText.start, &e.runeOffsetCache)
	for w.RunePos() < e.recentlyTypedText.end {
		if !unicode.IsSpace(w.Rune()) {
			break
		}
		w.Forward(1)
	}

	if w.RunePos() == e.recentlyTypedText.end {
		return
	}

	e.addPrimarySelection(w.RunePos(), e.recentlyTypedText.end)
}

func (e *editable) ClearRecentlyTypedText() {
	e.recentlyTypedText.start = -1
}

func (e *editable) sortCursorIndicesByDisplayOrder() {
	sort.Ints(e.CursorIndices)
}

func (e *editable) cursorIndicesInDisplayOrder() []int {
	a := make([]int, len(e.CursorIndices))
	copy(a, e.CursorIndices)
	sort.Ints(a)
	return a
}

type savedCursors struct {
	cursors []int
}

type savedSelections struct {
	selections []*selection
	primarySel *selection
}

func (e editable) saveCursors() savedCursors {
	var saved savedCursors

	saved.cursors = make([]int, len(e.CursorIndices))
	copy(saved.cursors, e.CursorIndices)

	return saved
}

func (e *editable) restoreCursors(saved savedCursors) {
	for _, c := range saved.cursors {
		e.CursorIndices = e.CursorIndices[:0]
		if c <= e.Len() {
			e.CursorIndices = append(e.CursorIndices, c)
		}
	}
}

func (e editable) saveSelections() savedSelections {
	var saved savedSelections

	saved.selections = e.copySelections()
	if e.primarySel != nil {
		for i, s := range e.selections {
			if s == e.primarySel {
				saved.primarySel = saved.selections[i]
			}
		}
	}

	return saved
}

func (e *editable) restoreSelections(saved savedSelections) {
	e.selections = e.selections[:0]
	for _, s := range saved.selections {
		if s.start > e.Len() {
			continue
		}

		if s.end > e.Len() {
			s.end = e.Len()
		}
		e.selections = append(e.selections, s)
		if s == saved.primarySel {
			e.primarySel = s
		}
	}
}

func (e editable) saveCursorsAndSelections() (savedCursors, savedSelections) {
	sc := e.saveCursors()
	ss := e.saveSelections()
	return sc, ss
}

func (e *editable) restoreCursorsAndSelections(sc savedCursors, ss savedSelections) {
	e.restoreCursors(sc)
	e.restoreSelections(ss)
}

func (e *editable) textAfterFirstBlankLine() []byte {
	s := e.Bytes()
	i := bytes.Index(s, []byte{'\n', '\n'})
	if i >= 0 {
		return s[i+1:]
	}
	return []byte{}
}

type readOnlyPieceTable struct {
	text    []byte
	marked  bool
	textlen int
}

func newReadOnlyPieceTable(tbl pctbl.Table) readOnlyPieceTable {
	return readOnlyPieceTable{
		text:    tbl.Bytes(),
		marked:  tbl.IsMarked(),
		textlen: tbl.Len(),
	}
}

func (t readOnlyPieceTable) Bytes() []byte {
	return t.text
}
func (t readOnlyPieceTable) DebugString() string {
	return ""
}
func (t readOnlyPieceTable) Delete(index, length int) {
}
func (t readOnlyPieceTable) DeleteWithUndoData(index, length int, undoData interface{}) {
}
func (t readOnlyPieceTable) Insert(index int, text string) {
}
func (t readOnlyPieceTable) InsertWithUndoData(index int, text string, undoData interface{}) {
}
func (t readOnlyPieceTable) Append(text string) {
}
func (t readOnlyPieceTable) IsMarked() bool {
	return t.marked
}
func (t readOnlyPieceTable) Len() int {
	return t.textlen
}
func (t readOnlyPieceTable) Mark() {
}
func (t readOnlyPieceTable) Redo() (undoData []interface{}) {
	return nil
}
func (t readOnlyPieceTable) Set(text []byte) {
}
func (t readOnlyPieceTable) SetString(text string) {
}
func (t readOnlyPieceTable) SetStringWithUndo(text string) {
}
func (t readOnlyPieceTable) SetWithUndo(text []byte) {
}
func (t readOnlyPieceTable) String() string {
	return string(t.text)
}
func (t readOnlyPieceTable) StartTransaction() {
}
func (t readOnlyPieceTable) EndTransaction() {
}
func (t readOnlyPieceTable) TruncateLastInsert(countToRemove int) {
}
func (t readOnlyPieceTable) Undo() (undoData []interface{}) {
	return nil
}
