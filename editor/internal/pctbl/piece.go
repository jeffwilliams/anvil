package pctbl

type piece struct {
	source buffer
	// start and length are in units of runes
	start, length int
	// byteStart and byteLen are in units of bytes
	byteStart, byteLen int
	prev, next         *piece
}

// append_ appends the piece n to p, setting up the pointers
// as if they were in a double-linked list
func (p *piece) append_(n *piece) {
	p.next = n
	n.prev = p
}

// swap replaces `p` in the list it is in with the
// sequence of nodes (a list fragment) starting with `start` and ending with `end`.
// `p`'s linkage still points to the previous place so that it can be added to an undo stack.
func (p *piece) swap(start, end *piece) {
	p.swapLeft(start)
	p.swapRight(end)
}

// swapLeft replaces `p` in the list with `n` such that
// the previous node to `p` is now linked with `n` properly instead of `p`.
func (p *piece) swapLeft(n *piece) {
	p.prev.next = n
	n.prev = p.prev
}

func (p *piece) swapRight(n *piece) {
	p.next.prev = n
	n.next = p.next
}
