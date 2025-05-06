package pctbl

type pieceRangeStack struct {
	top_  *pieceRange
	count int
}

func (s *pieceRangeStack) push(r *pieceRange) {
	r.next = s.top_
	s.top_ = r
	s.count++
}

func (s *pieceRangeStack) top() *pieceRange {
	return s.top_
}

func (s *pieceRangeStack) pop() *pieceRange {
	if s.top_ == nil {
		return nil
	}

	r := s.top_
	s.top_ = s.top_.next
	r.next = nil
	s.count--
	return r
}

func (s *pieceRangeStack) each(fn func(r *pieceRange)) {
	for i := s.top_; i != nil; i = i.next {
		fn(i)
	}
}
