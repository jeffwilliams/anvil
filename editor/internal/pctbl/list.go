package pctbl

import (
	"bytes"
	"fmt"
)

type piecelist struct {
	// We use sentinel values to represent the head and tail (as done in catch 22's James Brown's imlementation)
	head, tail *piece
}

func initPiecelist(l *piecelist) {
	l.head = &piece{}
	l.tail = &piece{}
	l.head.next = l.tail
	l.tail.prev = l.head
}

func (l piecelist) first() *piece {
	return l.head.next
}

func (l piecelist) last() *piece {
	return l.tail.prev
}

func (l *piecelist) insertBefore(existing, toadd *piece) {
	toadd.next = existing
	toadd.prev = existing.prev

	existing.prev.next = toadd
	existing.prev = toadd
}

func (l *piecelist) insertAfter(existing, toadd *piece) {
	toadd.prev = existing
	toadd.next = existing.next

	existing.next.prev = toadd
	existing.next = toadd
}

func (l *piecelist) Len() int {
	c := 0
	for n := l.head.next; n != l.tail; n = n.next {
		c++
	}
	return c
}

func (l *piecelist) remove(existing *piece) {
	if existing == nil || existing == l.head || existing == l.tail {
		return
	}

	existing.prev.next = existing.next
	existing.next.prev = existing.prev
}

func (l piecelist) String() string {
	var buf bytes.Buffer
	for n := l.head; n != nil; n = n.next {
		if n != l.head {
			//buf.WriteRune(',')
			buf.WriteString("\n  ")
		}

		if n == l.head || n == l.tail {
			buf.WriteString("SENT ") // sentinel
		}
		buf.WriteString(fmt.Sprintf("%p %#v\n", n, n))
	}
	return buf.String()
}
