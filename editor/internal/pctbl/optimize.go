package pctbl

import (
	"bytes"

	"github.com/jeffwilliams/anvil/editor/internal/runes"
)

// OptimizedPieceTable optimizes the undo and redo behaviour of a piecetable
// for a few special cases
type OptimizedPieceTable struct {
	ptbl        *PieceTable
	lastOp      op
	cachedBytes []byte
	saveDeletes bool
}

func Optimize(ptbl *PieceTable) *OptimizedPieceTable {
	return &OptimizedPieceTable{
		ptbl:        ptbl,
		saveDeletes: true,
	}
}

func (c *OptimizedPieceTable) Bytes() []byte {
	if c.cachedBytes != nil {
		//fmt.Printf("OptimizedPieceTable: cache hit!\n")
		return c.cachedBytes
	}

	b := c.ptbl.Bytes()
	c.cachedBytes = b
	return b
}

func (c *OptimizedPieceTable) DebugString() string {
	return c.ptbl.DebugString()
}

func (c *OptimizedPieceTable) Delete(index, length int) {
	if index < 0 || length < 0 {
		return
	}
	c.saveDelete(index, length)
	c.invalidateCache()
	c.ptbl.Delete(index, length)
}

func (c *OptimizedPieceTable) DeleteWithUndoData(index, length int, undoData interface{}) {
	c.DeleteWithUndoDataCache(index, length, undoData, nil)
}

func (c *OptimizedPieceTable) DeleteWithUndoDataCache(index, length int, undoData interface{}, runeOffsetCache *runes.OffsetCache) {
	if index < 0 || length < 0 {
		return
	}
	c.saveDeleteCache(index, length, runeOffsetCache)
	c.invalidateCache()
	c.ptbl.DeleteWithUndoData(index, length, undoData)
}

func (c *OptimizedPieceTable) saveDelete(index, length int) {
	c.saveDeleteCache(index, length, nil)
}

func (c *OptimizedPieceTable) saveDeleteCache(index, length int, runeOffsetCache *runes.OffsetCache) {
	if !c.saveDeletes {
		return
	}

	b := c.Bytes()
	w := runes.NewWalker(b)
	if runeOffsetCache != nil {
		w.SetRunePosCache(index, runeOffsetCache)
	} else {
		w.SetRunePos(index)
	}
	left := w.BytePos()
	w.Forward(length)
	right := w.BytePos()

	data := make([]byte, right-left)
	copy(data, b[left:right])
	c.lastOp.opType = opDelete
	c.lastOp.index = index
	c.lastOp.data = data
}

func (c *OptimizedPieceTable) SetSaveDeletes(b bool) {
	c.saveDeletes = b
}

func (c *OptimizedPieceTable) Insert(index int, text string) {
	if index < 0 {
		return
	}

	c.invalidateCache()

	// This is to optimize the common case of cutting then pasting a selection at
	// the same place to perform a copy.
	if c.isInsertUndoOfLastOperation(index, text) {
		c.ptbl.Undo()
		c.lastOp.opType = opInsert
		c.lastOp.index = index
		c.lastOp.data = nil
		return
	}

	c.ptbl.Insert(index, text)
	c.lastOp.opType = opInsert
	c.lastOp.index = index
	c.lastOp.data = nil
}

func (c *OptimizedPieceTable) Append(text string) {
	c.invalidateCache()
	c.ptbl.Append(text)
	c.lastOp.opType = opAppend
}

func (c *OptimizedPieceTable) isInsertUndoOfLastOperation(index int, text string) bool {
	if c.lastOp.opType == opDelete && c.lastOp.index == index && len(c.lastOp.data) == len(text) {
		b := []byte(text)

		//result := bytes.Equal(b, c.lastOp.data)
		//fmt.Printf("OptimizedPieceTable.isInsertUndoOfLastOperation: lastOp=%d index=%d text='%s'. Returning %v\n", c.lastOp.opType, c.lastOp.index, c.lastOp.data, result)
		return bytes.Equal(b, c.lastOp.data)
	}
	//fmt.Printf("OptimizedPieceTable.isInsertUndoOfLastOperation: lastOp=%d index=%d text='%s'. Returning false\n", c.lastOp.opType, c.lastOp.index, c.lastOp.data)
	return false
}

func (c *OptimizedPieceTable) InsertWithUndoData(index int, text string, undoData interface{}) {
	if index < 0 || len(text) == 0 {
		return
	}

	c.invalidateCache()

	// This is to optimize the common case of cutting then pasting a selection at
	// the same place to perform a copy.
	if c.isInsertUndoOfLastOperation(index, text) {
		c.ptbl.Undo()
		c.lastOp.opType = opInsert
		return
	}

	c.ptbl.InsertWithUndoData(index, text, undoData)
	c.lastOp.opType = opInsert
}

func (c *OptimizedPieceTable) IsMarked() bool {
	return c.ptbl.IsMarked()
}

func (c *OptimizedPieceTable) Len() int {
	return c.ptbl.Len()
}

func (c *OptimizedPieceTable) Mark() {
	c.ptbl.Mark()
}

func (c *OptimizedPieceTable) Redo() (undoData []interface{}) {
	c.invalidateCache()
	c.lastOp.opType = opRedo
	return c.ptbl.Redo()
}

func (c *OptimizedPieceTable) Set(text []byte) {
	c.invalidateCache()
	c.ptbl.Set(text)
	c.lastOp.opType = opSet
}

func (c *OptimizedPieceTable) SetString(text string) {
	c.invalidateCache()
	c.ptbl.SetString(text)
	c.lastOp.opType = opSet
}

func (c *OptimizedPieceTable) SetStringWithUndo(text string) {
	c.invalidateCache()
	c.ptbl.SetStringWithUndo(text)
	c.lastOp.opType = opSet
}

func (c *OptimizedPieceTable) SetWithUndo(text []byte) {
	c.invalidateCache()
	c.ptbl.SetWithUndo(text)
	c.lastOp.opType = opSet
}

func (c *OptimizedPieceTable) String() string {
	return c.ptbl.String()
}

func (c *OptimizedPieceTable) StartTransaction() {
	c.ptbl.StartTransaction()
}

func (c *OptimizedPieceTable) EndTransaction() {
	c.ptbl.EndTransaction()
}

func (c *OptimizedPieceTable) TruncateLastInsert(countToRemove int) {
	if countToRemove <= 0 {
		return
	}
	c.invalidateCache()
	c.ptbl.TruncateLastInsert(countToRemove)
	c.lastOp.opType = opTruncate
}

func (c *OptimizedPieceTable) Undo() (undoData []interface{}) {
	c.invalidateCache()
	c.lastOp.opType = opUndo
	return c.ptbl.Undo()
}

func (c *OptimizedPieceTable) invalidateCache() {
	c.cachedBytes = nil
}
