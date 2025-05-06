package expr

type Handler interface {
	Delete(Range)
	Copy(Range)
	Insert(index int, value []byte)
	Display(Range)
	DisplayContents(r Range, prefix string, displayPosition bool)
	Noop(Range)
	Done()
}
