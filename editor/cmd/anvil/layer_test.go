package main

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInjectDisplayColsIntoAllCols(t *testing.T) {

	ca := &Col{}
	cb := &Col{}
	cc := &Col{}
	cd := &Col{}

	colName := func(c *Col) string {
		switch c {
		case ca:
			return "ca"
		case cb:
			return "cb"
		case cc:
			return "cc"
		case cd:
			return "cd"
		default:
			return "?"
		}
	}

	colsString := func(cols []*Col) string {
		var b bytes.Buffer
		for _, c := range cols {
			fmt.Fprintf(&b, "%s ", colName(c))
		}
		return b.String()
	}

	tests := []struct {
		name                string
		inputCols           []*Col
		inputUnpositioned   []*Col
		inputVisibleCols    []colPosition
		inputLeftVisibleCol int
		inputPackables      []Packable
		expected            []*Col
	}{
		{
			// ca was shown, then we added cb. Both ca and cb are shown.
			name:                "add cb",
			inputCols:           []*Col{ca},
			inputUnpositioned:   []*Col{cb},
			inputVisibleCols:    []colPosition{{}},
			inputLeftVisibleCol: 0,
			inputPackables: []Packable{
				&colPosition{col: ca},
				&colPosition{col: cb},
			},
			expected: []*Col{ca, cb},
		},
		{
			// ca and cb was shown, then we added cc.
			name:                "add cc",
			inputCols:           []*Col{ca, cb},
			inputUnpositioned:   []*Col{cc},
			inputVisibleCols:    []colPosition{{}, {}},
			inputLeftVisibleCol: 0,
			inputPackables: []Packable{
				&colPosition{col: ca},
				&colPosition{col: cb},
				&colPosition{col: cc},
			},
			expected: []*Col{ca, cb, cc},
		},
		{
			// ca not shown, cb was shown, then we added cc.
			name:                "ca hidden, add cc",
			inputCols:           []*Col{ca, cb},
			inputUnpositioned:   []*Col{cc},
			inputVisibleCols:    []colPosition{{}},
			inputLeftVisibleCol: 1,
			inputPackables: []Packable{
				&colPosition{col: cb},
				&colPosition{col: cc},
			},
			expected: []*Col{ca, cb, cc},
		},
		{
			name:                "add into middle",
			inputCols:           []*Col{ca, cb, cc},
			inputUnpositioned:   []*Col{cd},
			inputVisibleCols:    []colPosition{{}, {}},
			inputLeftVisibleCol: 0,
			inputPackables: []Packable{
				&colPosition{col: ca},
				&colPosition{col: cb},
				&colPosition{col: cd},
			},
			//expected: []*Col{ca,cb,cc,cd}, // wrong
			expected: []*Col{ca, cb, cd, cc},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var layer Layer
			layer.leftVisibleCol = tc.inputLeftVisibleCol
			layer.unpositioned = tc.inputUnpositioned
			layer.Cols = tc.inputCols
			layer.visibleCols = tc.inputVisibleCols

			cols := layer.injectDisplayColsIntoAllCols(tc.inputPackables)

			//fmt.Printf("Compare %#v to %#v\n", tc.expected, cols)
			assert.Equal(t, len(cols), len(tc.expected))
			for i := range cols {
				if tc.expected[i] != cols[i] {
					t.Errorf("expected is: %s", colsString(tc.expected))
					t.Errorf("actual is: %s", colsString(cols))
					t.Fatalf("at index %d expected %s but got %s", i, colName(tc.expected[i]), colName(cols[i]))
				}
			}
		})
	}
}
