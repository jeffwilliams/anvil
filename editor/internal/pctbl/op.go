package pctbl

type opType int

const (
	opNone = iota
	opInsert
	opAppend
	opDelete
	opTruncate
	opUndo
	opRedo
	opSet
)

type op struct {
	opType opType
	index  int
	data   []byte
	length int
}

func (o opType) String() string {
	switch o {
	case opNone:
		return "opNone"
	case opInsert:
		return "opInsert"
	case opDelete:
		return "opDelete"
	case opTruncate:
		return "opTruncate"
	case opUndo:
		return "opUndo"
	case opRedo:
		return "opRedo"
	case opSet:
		return "opSet"

	}
	return "unknown"
}
