package main

type undoData struct {
	cursorIndex    int
	startOfChange  int
	lengthOfChange int
}

func newUndoDataForInsert(index, length int, cursorIndex int) *undoData {
	return &undoData{
		cursorIndex:    cursorIndex,
		startOfChange:  index,
		lengthOfChange: length,
	}
}

func newUndoDataForDelete(index, length int, cursorIndex int) *undoData {
	return &undoData{
		cursorIndex:    cursorIndex,
		startOfChange:  index,
		lengthOfChange: -length,
	}
}
