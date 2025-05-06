package main

import "testing"

type testRange struct {
	start, end int
}

func (r testRange) Start() int {
	return r.start
}

func (r testRange) End() int {
	return r.end
}

func TestRangeLinesAndCols(t *testing.T) {

	tests := []struct {
		name              string
		inputDoc          string
		inputRange        testRange
		expectedStartLine int
		expectedStartCol  int
		expectedEndLine   int
		expectedEndCol    int
	}{
		{
			name:              "empty",
			inputDoc:          "abc",
			inputRange:        testRange{0, 0},
			expectedStartLine: 1,
			expectedStartCol:  1,
			expectedEndLine:   1,
			expectedEndCol:    1,
		},
		{
			name:              "initial",
			inputDoc:          "fiddler in an old time string band",
			inputRange:        testRange{0, 4},
			expectedStartLine: 1,
			expectedStartCol:  1,
			expectedEndLine:   1,
			expectedEndCol:    4,
		},
		{
			name:              "mid",
			inputDoc:          "fiddler in an old time string band",
			inputRange:        testRange{4, 7},
			expectedStartLine: 1,
			expectedStartCol:  5,
			expectedEndLine:   1,
			expectedEndCol:    7,
		},
		{
			name:              "eol",
			inputDoc:          "line1\n",
			inputRange:        testRange{0, 5},
			expectedStartLine: 1,
			expectedStartCol:  1,
			expectedEndLine:   1,
			expectedEndCol:    5,
		},
		{
			name:              "eol2",
			inputDoc:          "line1\n",
			inputRange:        testRange{0, 6},
			expectedStartLine: 1,
			expectedStartCol:  1,
			expectedEndLine:   1,
			expectedEndCol:    5,
		},
		{
			name:              "after-eol",
			inputDoc:          "line1\n",
			inputRange:        testRange{5, 6},
			expectedStartLine: 1,
			expectedStartCol:  5,
			expectedEndLine:   1,
			expectedEndCol:    5,
		},
		{
			name:              "second-line",
			inputDoc:          "line1\nline1",
			inputRange:        testRange{6, 7},
			expectedStartLine: 2,
			expectedStartCol:  1,
			expectedEndLine:   2,
			expectedEndCol:    1,
		},
		{
			name:              "border",
			inputDoc:          "line1\nline1",
			inputRange:        testRange{5, 7},
			expectedStartLine: 2,
			expectedStartCol:  1,
			expectedEndLine:   2,
			expectedEndCol:    1,
		},
		{
			name:              "multiline",
			inputDoc:          "line1\nline1",
			inputRange:        testRange{2, 8},
			expectedStartLine: 1,
			expectedStartCol:  3,
			expectedEndLine:   2,
			expectedEndCol:    2,
		},
		{
			name:              "third-line",
			inputDoc:          "line1\nline2\nline3\n",
			inputRange:        testRange{12, 17},
			expectedStartLine: 3,
			expectedStartCol:  1,
			expectedEndLine:   3,
			expectedEndCol:    5,
		},
		{
			name:              "empty-lines",
			inputDoc:          "line1\n\nline3\n",
			inputRange:        testRange{1, 9},
			expectedStartLine: 1,
			expectedStartCol:  2,
			expectedEndLine:   3,
			expectedEndCol:    2,
		},
		{
			name:              "empty-lines",
			inputDoc:          "line1\n\nline3\n",
			inputRange:        testRange{5, 9},
			expectedStartLine: 3,
			expectedStartCol:  1,
			expectedEndLine:   3,
			expectedEndCol:    2,
		},
	}

	for _, tc := range tests {
		var h ExprHandler

		t.Run(tc.name, func(t *testing.T) {

			h.data = []byte(tc.inputDoc)
			sl, sc, el, ec := h.rangeLinesAndCols(tc.inputRange)

			if sl != tc.expectedStartLine ||
				sc != tc.expectedStartCol ||
				el != tc.expectedEndLine ||
				ec != tc.expectedEndCol {
				t.Fatalf("Expected %d:%d - %d:%d but got %d:%d - %d:%d", tc.expectedStartLine, tc.expectedStartCol, tc.expectedEndLine, tc.expectedEndCol, sl, sc, el, ec)
			}

		})
	}
}
