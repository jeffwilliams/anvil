package pctbl

type Table interface {
	Bytes() []byte
	DebugString() string
	Delete(index, length int)
	DeleteWithUndoData(index, length int, undoData interface{})
	Insert(index int, text string)
	InsertWithUndoData(index int, text string, undoData interface{})
	Append(text string)
	IsMarked() bool
	Len() int
	Mark()
	Redo() (undoData []interface{})
	Set(text []byte)
	SetString(text string)
	SetStringWithUndo(text string)
	SetWithUndo(text []byte)
	String() string
	StartTransaction()
	EndTransaction()
	TruncateLastInsert(countToRemove int)
	Undo() (undoData []interface{})
}
