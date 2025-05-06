package intvl

import (
	"bytes"
	"fmt"
	"testing"
)

type intvl struct {
	start, end int
	name       string
}

func (t intvl) Start() int {
	return t.start
}

func (t intvl) End() int {
	return t.end
}

type expected struct {
	position        int
	activeNames     []string
	nextPosition    int
	nextPositionNil bool
}

func intervalsMatch(a []Interval, intvlNames []string) bool {
	if len(a) != len(intvlNames) {
		return false
	}

	for _, e := range a {
		intv := e.(*intvl)
		found := false
		for _, name := range intvlNames {
			if intv.name == name {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func intvlsString(a []Interval) string {
	var buf bytes.Buffer

	buf.WriteRune('[')
	for _, e := range a {
		intv := e.(*intvl)
		fmt.Fprintf(&buf, "%s ", intv.name)
	}
	buf.WriteRune(']')
	return buf.String()
}

func TestIntervals(t *testing.T) {
	tests := []struct {
		name     string
		input    []intvl
		expected []expected
	}{
		{
			name:  "simple",
			input: []intvl{{1, 4, "a"}},
			expected: []expected{
				{0, []string{}, 1, false},
				{1, []string{"a"}, 4, false},
				{3, []string{"a"}, 4, false},
				{4, []string{}, 0, true},
				{5, []string{}, 0, true},
			},
		},
		{
			name:  "initial interval",
			input: []intvl{{0, 4, "a"}},
			expected: []expected{
				{0, []string{"a"}, 4, false},
				{1, []string{"a"}, 4, false},
				{4, []string{}, 0, true},
				{4, []string{}, 0, true},
				{5, []string{}, 0, true},
			},
		},
		{
			name:  "small interval",
			input: []intvl{{3, 4, "a"}},
			expected: []expected{
				{0, []string{}, 3, false},
				{1, []string{}, 3, false},
				{3, []string{"a"}, 4, false},
				{4, []string{}, 0, true},
				{5, []string{}, 0, true},
			},
		},
		{
			name: "two intervals",
			input: []intvl{
				{3, 4, "a"},
				{7, 10, "b"},
			},
			expected: []expected{
				{0, []string{}, 3, false},
				{1, []string{}, 3, false},
				{3, []string{"a"}, 4, false},
				{4, []string{}, 7, false},
				{6, []string{}, 7, false},
				{7, []string{"b"}, 10, false},
				{9, []string{"b"}, 10, false},
				{10, []string{}, 0, true},
				{11, []string{}, 0, true},
			},
		},
		{
			name: "overlap1",
			input: []intvl{
				{3, 6, "a"},
				{5, 10, "b"},
			},
			expected: []expected{
				{0, []string{}, 3, false},
				{3, []string{"a"}, 5, false},
				{4, []string{"a"}, 5, false},
				{5, []string{"a", "b"}, 6, false},
				{6, []string{"b"}, 10, false},
				{8, []string{"b"}, 10, false},
				{11, []string{}, 0, true},
			},
		},
		{
			name: "overlap2",
			input: []intvl{
				{3, 6, "a"},
				{5, 10, "b"},
			},
			expected: []expected{
				{4, []string{"a"}, 5, false},
				{8, []string{"b"}, 10, false},
				{11, []string{}, 0, true},
			},
		},
		{
			name: "overlap3",
			input: []intvl{
				{5, 10, "b"},
				{3, 6, "a"},
			},
			expected: []expected{
				{4, []string{"a"}, 5, false},
				{8, []string{"b"}, 10, false},
				{11, []string{}, 0, true},
			},
		},
		{
			name: "empty interval",
			input: []intvl{
				{3, 3, "a"},
			},
			expected: []expected{
				{1, []string{}, 0, true},
				{3, []string{}, 0, true},
				{5, []string{}, 0, true},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var seq IntervalSequence
			for i := range tc.input {
				seq.Add(&tc.input[i])
			}

			iter := seq.Iter()

			pos := 0
			for _, exp := range tc.expected {
				if pos < exp.position {
					iter.ForwardTo(exp.position)
					pos = exp.position
				}

				nxt := iter.Next()

				if iter.AtEnd() && nxt != nil {
					t.Fatalf("On position %d: At end of iteration but Next returns a change", exp.position)
				}

				b := nxt == nil
				if b && !exp.nextPositionNil {
					t.Fatalf("On position %d: expected Next to return non-nil but it returned nil", exp.position)
				} else if !b && exp.nextPositionNil {
					t.Fatalf("On position %d: expected Next to return nil but it returned non-nil %v", exp.position, nxt)
				}

				if nxt != nil && nxt.AbsolutePosition != exp.nextPosition {
					t.Fatalf("On position %d: Expected next change to be at %d but was at %d", exp.position, exp.nextPosition, nxt.AbsolutePosition)
				}

				if !intervalsMatch(iter.Active(), exp.activeNames) {
					t.Fatalf("On position %d: Expected the intervals %v but got %v", exp.position, exp.activeNames, intvlsString(iter.Active()))
				}

			}
		})
	}
}

func TestIntervalsWalk(t *testing.T) {
	var seq IntervalSequence
	seq.Add(intvl{2, 10, "a"})
	seq.Add(intvl{1, 3, "b"})
	seq.Add(intvl{3, 6, "c"})

	iter := seq.Iter()

	i := 0
	for chg := iter.Next(); chg != nil; chg = iter.Next() {
		nxt := chg.AbsolutePosition

		iter.ForwardTo(nxt)

		intervals := iter.Active()

		switch i {
		case 0:
			// b
			if len(intervals) != 1 {
				t.Fatalf("At change %d at index %d: expected %d intervals but got %d", chg.AbsolutePosition, i, 1, len(intervals))
			}
		case 1:
			// b and a
			if len(intervals) != 2 {
				t.Fatalf("At change %d at index %d: expected %d intervals but got %d", chg.AbsolutePosition, i, 2, len(intervals))
			}
		case 2:
			// c and a
			if len(intervals) != 2 {
				t.Fatalf("At change %d at index %d: expected %d intervals but got %d", chg.AbsolutePosition, i, 2, len(intervals))
			}
		}
		i++
	}

}
