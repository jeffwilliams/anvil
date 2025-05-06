package pctbl

import (
	"bytes"
	"fmt"
)

type pieceRange struct {
	first, last *piece
	userData    []interface{}
	marked      bool
	mergeUndo   bool
	//pieceList
	next *pieceRange
}

func (p *pieceRange) Len() int {
	if p.first == nil {
		return 0
	}

	l := 0
	for n := p.first; n != p.last.next; n = n.next {
		l += n.length
	}
	return l
}

func (p *pieceRange) debugString(pt *PieceTable) string {

	var buf bytes.Buffer

	i := 0
	for n := p.first; n != nil && n != p.last.next; n = n.next {
		if n != p.first {
			//buf.WriteRune(',')
			buf.WriteString("\n   ")
		}

		buf.WriteString(fmt.Sprintf("(%d) %p %#v ‘%s’", i, n, n, pt.textOf(n)))
		buf.WriteString("\n     ")
		buf.WriteString(fmt.Sprintf("prev=(%s) next=(%s)",
			pt.indexPointedTo(n.prev), pt.indexPointedTo(n.next)))
		i++
	}
	buf.WriteRune('\n')
	return buf.String()
}
