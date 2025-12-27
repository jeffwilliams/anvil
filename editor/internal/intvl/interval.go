package intvl

import (
	"github.com/jeffwilliams/anvil/editor/internal/slice"
	"sort"
)

// An interval represents a half-open range including the first element but not the last.
type Interval interface {
	Start() int
	End() int
}

type IntervalSequence struct {
	pts    []intervalEndpt
	sorted bool
}

type intervalEndpt struct {
	coord    int
	interval Interval
	typ      intervalEndptType
}

type intervalEndptType int

const (
	start intervalEndptType = iota
	end
)

func newIntervalEndpts(i Interval) (a, b intervalEndpt) {
	return intervalEndpt{
			coord:    i.Start(),
			interval: i,
			typ:      start,
		},
		intervalEndpt{
			coord:    i.End(),
			interval: i,
			typ:      end,
		}
}

func (s *IntervalSequence) Add(i Interval) {
	s.AddWithoutSort(i)
	s.sort()
}

func (s *IntervalSequence) isEmpty(i Interval) bool {
	return i.Start() == i.End()
}

// AddWithoutSort adds the interval but doesn't sort it into the right place.
// For the data to be usable you MUST call Sort before getting an iterator.
func (s *IntervalSequence) AddWithoutSort(i Interval) {
	if s.isEmpty(i) {
		return
	}

	s.init()
	a, b := newIntervalEndpts(i)
	s.pts = append(s.pts, a, b)
	s.sorted = false
}

func (s *IntervalSequence) Sort() {
	s.sort()
}

func (s *IntervalSequence) init() {
	if s.pts == nil {
		s.pts = make([]intervalEndpt, 0, 20)
	}
}

func (s *IntervalSequence) Reset() {
	if s.pts != nil {
		s.pts = s.pts[0:0]
	}
}

func (s *IntervalSequence) sort() {
	sort.Slice(s.pts, func(i, j int) bool {
		return s.pts[i].coord < s.pts[j].coord
	})
	s.sorted = true
}

func (s *IntervalSequence) Del(i Interval) {
	match := func(ndx int) bool {
		return intervalsEqual(s.pts[ndx].interval, i)
	}

	s.pts = slice.RemoveFirstMatchFromSlicePreserveOrder(s.pts, match).([]intervalEndpt)
}

func intervalsEqual(i, j Interval) bool {
	return i.Start() == j.Start() && i.End() == j.End()
}

func (s *IntervalSequence) Iter() IntervalIter {
	if !s.sorted && len(s.pts) > 0 {
		panic("IntervalSequence is not sorted; can't get an iterator")
	}

	pts := make([]intervalEndpt, len(s.pts))
	for i, x := range s.pts {
		pts[i] = x
	}

	return IntervalIter{
		pts:    pts,
		active: make([]Interval, 0, 10),
	}
}

type IntervalIter struct {
	pts    []intervalEndpt
	pos    int
	active []Interval
	// nonthreadsafe
	upcoming IntervalChange
	sendInit bool
}

func (it *IntervalIter) ForwardTo(position int) {
	if position < it.pos {
		return
	}

	for !it.AtEnd() && it.nextChangeIsBeforeOrAt(position) {
		if it.pts[0].typ == start {
			it.activateInterval(it.pts[0].interval)
		} else {
			it.deactivateInterval(it.pts[0].interval)
		}
		it.removeFirstPt()
	}

	it.pos = position
}

func (it IntervalIter) AtEnd() bool {
	return len(it.pts) == 0
}

func (it IntervalIter) nextChangeIsBeforeOrAt(position int) bool {
	return it.pts[0].coord <= position
}

func (it *IntervalIter) deactivateInterval(i Interval) {
	match := func(ndx int) bool {
		return it.active[ndx] == i
	}

	it.active = slice.RemoveFirstMatchFromSlicePreserveOrder(it.active, match).([]Interval)
}

func (it *IntervalIter) activateInterval(i Interval) {
	it.active = append(it.active, i)
}

func (it *IntervalIter) removeFirstPt() {
	it.pts = it.pts[1:]
}

func (it IntervalIter) ForwardBy(distance int) {
	position := it.pos + distance
	it.ForwardTo(position)
}

func (it IntervalIter) Active() []Interval {
	return it.active
}

// Next returns the index of the first character that begins the next section of consistent intervals.
// For example if the current position is not inside an interval and the next interval begins at
// 3, then 3 would be returned. If the current position _is_ inside an interval and the last character
// of that interval is 5 (i.e. the interval end property is 6) then 6 would be returned because that is
// the first character of the next set of consistent intervals.
// Note: this method is NOT threadsafe.
func (it *IntervalIter) Next() *IntervalChange {
	it.initUpcoming()

	if it.pos == 0 {
		it.ForwardTo(0)
	}

	pt := it.ptAtIndex(0)
	if pt == nil {
		return nil
	}

	// If this is the end of a segment then it is not the next change anymore
	if pt.typ == end && pt.coord == it.pos {
		pt = it.ptAtIndex(1)
	}

	if pt == nil {
		return nil
	}

	it.upcoming.AbsolutePosition = pt.coord
	it.upcoming.OffsetFromCurrentPosition = pt.coord - it.pos

	return &it.upcoming
}

func (it *IntervalIter) ptAtIndex(i int) *intervalEndpt {
	if i < len(it.pts) {
		return &it.pts[i]
	}
	return nil
}

/*
func (it *IntervalIter) InitialOrNext() *IntervalChange {
	if !it.sendInit {
		it.initUpcoming()
		it.sendInit = true
		return &it.upcoming
	} else {
		return it.Next()
	}
}
*/

func (it *IntervalIter) initUpcoming() {
	it.upcoming.AbsolutePosition = 0
	it.upcoming.OffsetFromCurrentPosition = 0
}

type IntervalChange struct {
	AbsolutePosition          int
	OffsetFromCurrentPosition int
}

func Overlaps(a, b Interval) bool {
	exl := a.End() <= b.Start() || b.End() <= a.Start()
	return !exl
}

// Contains returns true if child is contained within or equals parent.
func Contains(parent, child Interval) bool {
	return child.Start() >= parent.Start() && child.End() <= parent.End()
}
