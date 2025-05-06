package pctbl

import (
	"bytes"
	"fmt"
	"unicode/utf8"
)

/*
Piece Chains http://www.catch22.net/tuts/neatpad/piece-chains#
https://en.wikipedia.org/wiki/Piece_table
https://darrenburns.net/posts/piece-table/
*/

/*
Merging Undos
-------------

In some cases it's convenient for users of the piecetable to be able to treat a series of operations as
if it was one operation for the purposes of undo and redo. For example, when the user types a series of letters
in a row, likely we would want to undo that as one series, rather than having to undo each typed letter in turn.
For lack of a better word, we can call this series of changes we want to treat as one single change a "transaction".

The PieceTable implements this by marking the most of the piece ranges that comprise a transaction in the undo table with
a flag (mergeUndo). When an undo operation is performed normally the peice range at the top of the undo stack would be
re-inserted into the piecetable (and the piece it's replacing swapped out). But if the flag is set then the undo continues
and does an undo of the _next_ piece range as well. If that is also marked with mergeUndo, then again we repeat.

To mark these pieces properly the user must start the transaction with TrackUndos(true) and end it after the last change
that belongs to the transaction with TrackUndos(false). Note that the first change in the transaction must _not_ be flagged;
if it was then the first change in the transaction would be undone and then the one before it as well. To keep track of that
state the mergeUndo flag in the PieceTable is used. A piecerange added to the undo stack is only marged with mergeUndo if
PieceTable.mergeUndo is true. But mergeUndo lags behind the TrackUndos setting by one change; it is only set to
true after the first piece in the transaction is pushed to the undo stack, meaning the first change in the transaction is
unmarked.

*/

type buffer int

const (
	invalid = iota
	original
	add
)

type PieceTable struct {
	buf                  [3][]byte
	bufLen               [3]int // Length of the buffer in runes
	length               int    // Length of the document
	pieces               piecelist
	marked               bool
	undoStack, redoStack pieceRangeStack
	lastInsertedPiece    *piece
	lastInsertEndIndex   int
	trackUndos           bool
	mergeUndo            bool
	undoData             []interface{}
	skipNextAppend       bool
}

func NewPieceTable(text []byte) *PieceTable {
	p := &PieceTable{}
	p.Set(text)
	return p
}

func (pt *PieceTable) Set(text []byte) {
	pt.trackUndos = true
	pt.buf = [3][]byte{}
	pt.bufLen = [3]int{}
	pt.undoStack = pieceRangeStack{}
	pt.redoStack = pieceRangeStack{}
	pt.lastInsertedPiece = nil
	pt.marked = false

	initPiecelist(&pt.pieces)
	pt.createFirstPiece(text)
	pt.length = pt.pieces.first().length
}

// StartTransaction is used to begin a transaction: a series of changes that are all undone and redone
// at once, as if they were all the same change. Logically operations on the piecetable are merged into one single
// undo until EndTransaction is called.
//
// This is useful when you must perform multiple small operations on the table that are really one large
// operation, such as substituting all strings with another string.
func (pt *PieceTable) StartTransaction() {
	pt.trackUndos = false
	if !pt.trackUndos {
		pt.mergeUndo = false
	}
}

// EndTransaction ends a transaction started with StartTransaction.
func (pt *PieceTable) EndTransaction() {
	pt.trackUndos = true
	// If the user just performed a transaction, they probably don't want the next inserted
	// text to be undone along with that transaction as if it is part of it. So we prevent
	// the optimization that tries to append the next inserted text to the previous piece
	// in this case.
	pt.skipNextAppend = true
}

func (pt *PieceTable) SetString(text string) {
	pt.Set([]byte(text))
}

func (pt *PieceTable) SetStringWithUndo(text string) {
	pt.SetWithUndo([]byte(text))
}

func (pt *PieceTable) createFirstPiece(text []byte) {
	pt.appendToBuf(original, text)
	piece := &piece{
		source:    original,
		start:     0,
		byteStart: 0,
		length:    utf8.RuneCount(text),
		byteLen:   len(text),
	}
	pt.pieces.insertBefore(pt.pieces.tail, piece)
}

func (pt *PieceTable) appendToBuf(buffer int, text []byte) {
	if pt.buf[buffer] == nil {
		pt.buf[buffer] = text
	} else {
		pt.buf[buffer] = append(pt.buf[buffer], text...)
	}
	pt.bufLen[buffer] += utf8.RuneCount(text)
}

func (pt *PieceTable) appendStringToBuf(buffer int, text string) {
	pt.appendToBuf(buffer, []byte(text))
}

// Len returns the length of the piece table in units of runes (not bytes)
func (pt *PieceTable) Len() int {
	return pt.length
}

func (pt *PieceTable) SetWithUndo(text []byte) {
	f := pt.pieces.first()
	l := pt.pieces.last()

	if f == nil || l == nil {
		pt.Set(text)
		return
	}

	undo := &pieceRange{
		first: f,
		last:  l,
	}
	undo.marked = pt.marked

	s := string(text)

	newPiece := &piece{
		source:    add,
		start:     pt.bufLen[add],
		byteStart: len(pt.buf[add]),
		length:    utf8.RuneCountInString(s),
		byteLen:   len(text),
	}
	pt.appendStringToBuf(add, s)
	pt.lastInsertedPiece = newPiece
	pt.lastInsertEndIndex = newPiece.length

	f.swapLeft(newPiece)
	l.swapRight(newPiece)

	if !pt.trackUndos && pt.mergeUndo {
		undo.mergeUndo = true
	}
	pt.mergeUndo = !pt.trackUndos

	pt.undoStack.push(undo)
	pt.length = newPiece.length
	pt.marked = false
	pt.redoStack = pieceRangeStack{}
}

func (pt *PieceTable) Insert(index int, text string) {
	pt.InsertWithUndoData(index, text, nil)
}

func (pt *PieceTable) InsertWithUndoData(index int, text string, undoData interface{}) {
	if index < 0 {
		return
	}

	if pt.tryAppendingToLastCreatedPiece(index, text, undoData) {
		return
	}

	oldPiece, offsetInPiece, byteOffsetInPiece := pt.findPiece(index)

	if oldPiece == nil {
		return
	}

	newPiece := &piece{
		source:    add,
		start:     pt.bufLen[add],
		byteStart: len(pt.buf[add]),
		length:    utf8.RuneCountInString(text),
		byteLen:   len(text),
	}

	pt.appendStringToBuf(add, text)
	pt.lastInsertedPiece = newPiece
	pt.lastInsertEndIndex = index + newPiece.length

	var undo *pieceRange
	var firstReplacementPiece, lastReplacementPiece *piece

	if offsetInPiece == oldPiece.length {
		// End of the piece: make a new piece after oldPiece
		firstReplacementPiece, lastReplacementPiece, undo = pt.insertByAppending(oldPiece, newPiece)
	} else if offsetInPiece == 0 {
		// Start of the piece: make a new piece before oldPiece
		firstReplacementPiece, lastReplacementPiece, undo = pt.insertByPrepending(oldPiece, newPiece)
	} else {
		// Split the existing piece into two
		firstReplacementPiece, lastReplacementPiece, undo = pt.insertBySplitting(oldPiece, offsetInPiece, byteOffsetInPiece, newPiece)
	}

	if undoData != nil {
		undo.userData = append(undo.userData, pt.undoData...)
		undo.userData = append(undo.userData, undoData)
		pt.undoData = pt.undoData[:0]
	}
	undo.marked = pt.marked

	// Swap oldPiece out of the list, replacing it with the list segment we computed
	oldPiece.swap(firstReplacementPiece, lastReplacementPiece)

	if !pt.trackUndos && pt.mergeUndo {
		undo.mergeUndo = true
	}
	pt.mergeUndo = !pt.trackUndos

	pt.undoStack.push(undo)
	pt.length += newPiece.length
	pt.marked = false
	pt.redoStack = pieceRangeStack{}

	//fmt.Printf("PT: After insert: %s\n", pt.DebugString())
}

func (pt *PieceTable) tryAppendingToLastCreatedPiece(index int, text string, undoData interface{}) (didAppend bool) {
	if pt.skipNextAppend {
		pt.skipNextAppend = false
		return
	}

	if index == pt.lastInsertEndIndex && pt.lastInsertedPiece != nil {
		pt.appendStringToBuf(add, text)
		c := utf8.RuneCountInString(text)
		pt.lastInsertedPiece.length += c
		pt.lastInsertedPiece.byteLen += len(text)
		pt.lastInsertEndIndex += c
		pt.length += c
		pt.marked = false

		// We are appending to a piece that has already been added, and so is at the top of the undo history.
		undo := pt.undoStack.top()
		if undo != nil {
			undo.userData = append(undo.userData, undoData)
		}

		didAppend = true
	}
	return
}

func (pt *PieceTable) insertByAppending(oldPiece, newPiece *piece) (firstReplacementPiece, lastReplacementPiece *piece, undo *pieceRange) {
	undo = &pieceRange{
		first: oldPiece,
		last:  oldPiece,
	}

	pre := &piece{
		source:    oldPiece.source,
		start:     oldPiece.start,
		byteStart: oldPiece.byteStart,
		length:    oldPiece.length,
		byteLen:   oldPiece.byteLen,
	}

	pre.append_(newPiece)
	firstReplacementPiece = pre
	lastReplacementPiece = newPiece
	return
}

func (pt *PieceTable) insertByPrepending(oldPiece, newPiece *piece) (firstReplacementPiece, lastReplacementPiece *piece, undo *pieceRange) {
	undo = &pieceRange{
		first: oldPiece,
		last:  oldPiece,
	}

	pre := &piece{
		source:    oldPiece.source,
		start:     oldPiece.prev.start,
		byteStart: oldPiece.prev.byteStart,
		length:    oldPiece.prev.length,
		byteLen:   oldPiece.prev.byteLen,
	}

	post := &piece{
		source:    oldPiece.source,
		start:     oldPiece.start,
		byteStart: oldPiece.byteStart,
		length:    oldPiece.length,
		byteLen:   oldPiece.byteLen,
	}

	pre.append_(newPiece)
	newPiece.append_(post)

	firstReplacementPiece = pre
	lastReplacementPiece = post
	return
}

func (pt *PieceTable) insertBySplitting(oldPiece *piece, offsetInOldPiece, byteOffsetInOldPiece int, newPiece *piece) (firstReplacementPiece, lastReplacementPiece *piece, undo *pieceRange) {
	// Save the piece that we are replacing in the undo list
	undo = &pieceRange{
		first: oldPiece,
		last:  oldPiece,
	}

	pre := &piece{
		source:    oldPiece.source,
		start:     oldPiece.start,
		byteStart: oldPiece.byteStart,
		length:    offsetInOldPiece,
		byteLen:   byteOffsetInOldPiece,
	}

	post := &piece{
		source:    oldPiece.source,
		start:     oldPiece.start + offsetInOldPiece,
		byteStart: oldPiece.byteStart + byteOffsetInOldPiece,
		length:    oldPiece.length - offsetInOldPiece,
		byteLen:   oldPiece.byteLen - byteOffsetInOldPiece,
	}

	pre.append_(newPiece)
	newPiece.append_(post)

	firstReplacementPiece = pre
	lastReplacementPiece = post
	return
}

func (pt *PieceTable) Append(text string) {
	pt.InsertWithUndoData(pt.length, text, nil)
}

func (pt *PieceTable) Delete(index, length int) {
	pt.DeleteWithUndoData(index, length, nil)
}

func (pt *PieceTable) DeleteWithUndoData(index, length int, undoData interface{}) {
	if length <= 0 || index < 0 {
		return
	}

	// Don't allow appending to the last inserted piece after a delete; it may have been deleted.
	pt.lastInsertedPiece = nil
	pt.lastInsertEndIndex = 0

	delStartPiece, delStartOffsetInPiece, delByteStartOffsetInPiece := pt.findPiece(index)
	delEndPiece, delEndOffsetInPiece, delByteEndOffsetInPiece := pt.findPiece(index + length)

	if delStartPiece == nil || delEndPiece == nil {
		return
	}

	pt.length -= length

	undo := &pieceRange{
		first:  delStartPiece,
		last:   delEndPiece,
		marked: pt.marked,
	}

	if undoData != nil {
		undo.userData = []interface{}{undoData}
	}

	var newEndPiece, newStartPiece *piece

	newStartPiece = &piece{
		source:    delStartPiece.source,
		start:     delStartPiece.start,
		byteStart: delStartPiece.byteStart,
		length:    delStartOffsetInPiece,
		byteLen:   delByteStartOffsetInPiece,
	}

	endLen := delEndPiece.length - delEndOffsetInPiece
	if endLen == 0 {
		newEndPiece = newStartPiece
	} else {
		newEndPiece = &piece{
			source:    delEndPiece.source,
			start:     delEndPiece.start + delEndOffsetInPiece,
			byteStart: delEndPiece.byteStart + delByteEndOffsetInPiece,
			length:    delEndPiece.length - delEndOffsetInPiece,
			byteLen:   delEndPiece.byteLen - delByteEndOffsetInPiece,
		}
		newStartPiece.append_(newEndPiece)
	}

	if newStartPiece.length == 0 {
		newStartPiece = newEndPiece
	}

	delStartPiece.swapLeft(newStartPiece)
	delEndPiece.swapRight(newEndPiece)

	pt.marked = false

	if !pt.trackUndos && pt.mergeUndo {
		undo.mergeUndo = true
	}
	pt.mergeUndo = !pt.trackUndos

	pt.undoStack.push(undo)
	pt.redoStack = pieceRangeStack{}

	//fmt.Printf("PT: After delete: %s\n", pt.DebugString())
}

func (pt *PieceTable) Undo() (undoData []interface{}) {
	b := pt.stepAlongUndoRedoSequence(&pt.undoStack, &pt.redoStack)
	//fmt.Printf("PT: After Undo: %s\n", pt.DebugString())
	return b
}

func (pt *PieceTable) Redo() (undoData []interface{}) {
	b := pt.stepAlongUndoRedoSequence(&pt.redoStack, &pt.undoStack)
	return b
}

func (pt *PieceTable) TruncateLastInsert(countToRemove int) {
	if countToRemove <= 0 {
		return
	}

	if countToRemove > pt.lastInsertedPiece.length {
		countToRemove = pt.lastInsertedPiece.length
	}

	buf := pt.buf[pt.lastInsertedPiece.source]

	pt.lastInsertedPiece.length -= countToRemove
	pt.length -= countToRemove

	count := 0
	blen := len(pt.buf[pt.lastInsertedPiece.source])
	for countToRemove > 0 {
		_, l := utf8.DecodeLastRune(buf[0 : blen-count])
		count += l
		countToRemove--
	}
	pt.lastInsertedPiece.byteLen -= count

	pt.buf[pt.lastInsertedPiece.source] = pt.buf[pt.lastInsertedPiece.source][0 : blen-count]
}

func (pt *PieceTable) stepAlongUndoRedoSequence(from, to *pieceRangeStack) (undoData []interface{}) {
	if from.top() == nil {
		return
	}

	//fmt.Printf("PT: PieceTable.stepAlongUndoRedoSequence: pt from (undo or redo) stack is: %s\n",
	//	pt.stackDebugString(from))

	newPieceRange := from.pop()

	oldPieceRange := &pieceRange{
		first:     newPieceRange.first.prev.next,
		last:      newPieceRange.last.next.prev,
		userData:  newPieceRange.userData,
		marked:    pt.marked,
		mergeUndo: pt.mergeUndo,
	}

	oldPieceRange.first.swapLeft(newPieceRange.first)
	oldPieceRange.last.swapRight(newPieceRange.last)

	to.push(oldPieceRange)

	pt.length -= oldPieceRange.Len()
	pt.length += newPieceRange.Len()

	pt.marked = newPieceRange.marked
	pt.mergeUndo = newPieceRange.mergeUndo

	if newPieceRange.mergeUndo && from.top() != nil {
		//fmt.Printf("PieceTable: mergeUndo: stepping again\n")
		d := pt.stepAlongUndoRedoSequence(from, to)
		undoData = append(undoData, newPieceRange.userData...)
		undoData = append(undoData, d...)
		return
	}

	cpy := make([]interface{}, len(newPieceRange.userData))
	copy(cpy, newPieceRange.userData)
	return cpy
}

func (pt *PieceTable) findPiece(index int) (piece *piece, offset, byteOffset int) {
	for n := pt.pieces.first(); n != pt.pieces.tail; n = n.next {
		if index-n.length <= 0 {
			piece = n
			offset = index

			bytes := pt.textOf(piece)

			if piece.length == piece.byteLen {
				// optimization: all runes are 1 byte long
				byteOffset = index
				return
			}

			byteOffset = runeIndexToByteIndex(index, bytes, piece.length)
			return
		}
		index -= n.length
	}
	return
}

func (pt *PieceTable) textOf(p *piece) []byte {
	b := pt.buf[p.source]
	return b[p.byteStart : p.byteLen+p.byteStart]
}

func (pt *PieceTable) String() string {
	var buf bytes.Buffer
	//fmt.Printf("PieceTable.String: list: %s\n", pt.pieces)
	pt.intoBuf(&buf)
	return buf.String()
}

func (pt *PieceTable) Bytes() []byte {
	var buf bytes.Buffer
	//fmt.Printf("PieceTable.String: list: %s\n", pt.pieces)
	pt.intoBuf(&buf)
	return buf.Bytes()
}

func (pt *PieceTable) intoBuf(buf *bytes.Buffer) {
	for n := pt.pieces.first(); n != pt.pieces.tail; n = n.next {
		buf.Write(pt.textOf(n))
	}
}

func (pt *PieceTable) DebugString() string {
	var buf bytes.Buffer

	fmt.Fprintf(&buf, "marked: %v track: %v merge: %v\n", pt.marked, pt.trackUndos, pt.mergeUndo)
	fmt.Fprintf(&buf, "Pieces (%d):\n  ", pt.pieces.Len())
	fmt.Fprintf(&buf, "%s\n\n  ", pt.pieceListDebugString())
	fmt.Fprintf(&buf, "Undo stack (%d) \n%s\n", pt.undoStack.count, pt.stackDebugString(&pt.undoStack))
	fmt.Fprintf(&buf, "Redo stack (%d) \n%s\n", pt.redoStack.count, pt.stackDebugString(&pt.redoStack))

	return buf.String()
}

func (pt *PieceTable) pieceListDebugString() string {
	var buf bytes.Buffer
	l := pt.pieces
	i := 0
	for n := l.head; n != nil; n = n.next {
		if n != l.head {
			//buf.WriteRune(',')
			buf.WriteString("\n  ")
		}

		buf.WriteString(fmt.Sprintf("(%d) ", i)) // sentinel
		if n == l.head || n == l.tail {
			buf.WriteString("SENT ") // sentinel
		}
		buf.WriteString(fmt.Sprintf("%p %#v ‘%s’", n, n, pt.textOf(n)))
		i++
	}
	return buf.String()
}

func (pt *PieceTable) stackDebugString(stk *pieceRangeStack) string {
	var buf bytes.Buffer

	i := 0
	for n := stk.top_; n != nil; n = n.next {

		buf.WriteString(fmt.Sprintf("  (%d) first: (%p) last: (%p) marked: %v merge: %v pieces:\n   %s",
			i, n.first, n.last, n.marked, n.mergeUndo, n.debugString(pt)))
		i++
	}
	return buf.String()

}

// For debugging
func (pt *PieceTable) indexPointedTo(p *piece) string {
	i := pt.listIndexOfPiece(p)
	if i >= 0 {
		return fmt.Sprintf("list %d", i)
	}

	indexInStack := func(stk *pieceRangeStack, p *piece) (ok bool, sindex string) {
		i = 0
		for r := stk.top_; r != nil; r = r.next {
			j := 0
			for n := r.first; n != r.last.next && n != nil; n = n.next {
				if n == p {
					sindex = fmt.Sprintf("stack %d.%d", i, j) // Refers to (index in stack).(index in range)
					ok = true
					return
				}
				j++
			}
			i++
		}
		return
	}

	ok, s := indexInStack(&pt.undoStack, p)
	if ok {
		return fmt.Sprintf("u%s", s)
	}

	ok, s = indexInStack(&pt.redoStack, p)
	if ok {
		return fmt.Sprintf("r%s", s)
	}

	return "-1"
}

// listIndexOfPiece returns the index of the piece, INCLUDING the sentinel values at the beginning and end
// of the list
func (pt *PieceTable) listIndexOfPiece(p *piece) int {
	i := 0
	for n := pt.pieces.head; n != nil; n = n.next {
		if n == p {
			return i
		}
		i++
	}
	return -1
}

func (pt *PieceTable) Mark() {
	pt.marked = true
	pt.redoStack.each(func(r *pieceRange) {
		r.marked = false
	})
	pt.undoStack.each(func(r *pieceRange) {
		r.marked = false
	})
}

func (pt *PieceTable) IsMarked() bool {
	return pt.marked
}

// runeIndexToByteIndex converts the index `rindex` into the slice `b` to a byte index.
// Parameter runeLen must be the length of b in runes.
func runeIndexToByteIndex(rindex int, b []byte, runeLen int) int {

	// LastRuneLen is a little slower than RuneLen, but probably faster
	// than scanning forwards to near the end of a long byte slice.
	threeFifths := runeLen * 3 / 5
	byteOffset := 0

	if rindex <= threeFifths {
		for i := 0; i < rindex; i++ {
			sz := RuneLen(b)
			b = b[sz:]
			byteOffset += sz
		}
		return byteOffset
	}

	byteOffset = len(b)
	for i := runeLen; i > rindex; i-- {
		sz := LastRuneLen(b)
		b = b[:len(b)-sz]
		runeLen--
		byteOffset -= sz
	}
	return byteOffset
}
