package runes

import "testing"

func TestWalkerGotoLineAndCol(t *testing.T) {

	tests := []struct {
		name            string
		input           string
		line            int
		col             int
		expectedRunePos int
	}{
		{
			name:            "simple",
			input:           "simple",
			line:            1,
			col:             1,
			expectedRunePos: 0,
		},
		{
			name:            "col only",
			input:           "simple",
			line:            1,
			col:             4,
			expectedRunePos: 3,
		},
		{
			name:            "2 line",
			input:           "simple\ntest",
			line:            2,
			col:             0,
			expectedRunePos: 7,
		},
		{
			name:            "2 line part 2",
			input:           "simple\ntest",
			line:            2,
			col:             1,
			expectedRunePos: 7,
		},
		{
			name:            "2 line part 3",
			input:           "simple\ntest",
			line:            2,
			col:             2,
			expectedRunePos: 8,
		},
		{
			name:            "empty lines",
			input:           "simple\n\n\ntest",
			line:            4,
			col:             2,
			expectedRunePos: 10,
		},
		{
			name:            "line too early",
			input:           "simple\n\n\ntest",
			line:            0,
			col:             0,
			expectedRunePos: 0,
		},
		{
			name:            "line too late",
			input:           "simple\n\n\ntest",
			line:            20,
			col:             0,
			expectedRunePos: 13,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := NewWalker([]byte(tc.input))

			w.GoToLineAndCol(tc.line, tc.col)
			pos := w.RunePos()
			if pos != tc.expectedRunePos {
				t.Fatalf("Expected rune position to be %d but was %d", tc.expectedRunePos, pos)
			}

		})
	}
}

func TestWalkerTextBetweenRuneIndices(t *testing.T) {

	tests := []struct {
		name       string
		input      string
		start, end int
		expected   string
	}{
		{
			name:     "simple",
			input:    "simple",
			start:    1,
			end:      4,
			expected: "imp",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := NewWalker([]byte(tc.input))

			s := string(w.TextBetweenRuneIndices(tc.start, tc.end))

			if s != tc.expected {
				t.Fatalf("Expected '%s' but got '%s'", tc.expected, s)
			}

		})
	}
}

func TestWalkerCurrentLineBounds(t *testing.T) {

	tests := []struct {
		name      string
		input     string
		runeIndex int
		expected  string
	}{
		{
			name:      "simple",
			input:     "simple",
			runeIndex: 1,
			expected:  "simple",
		},
		{
			name:      "simple",
			input:     "simple",
			runeIndex: 0,
			expected:  "simple",
		},
		{
			name:      "simple",
			input:     "simple",
			runeIndex: 5,
			expected:  "simple",
		},
		{
			name:      "line1\nline2",
			input:     "line1\nline2",
			runeIndex: 1,
			expected:  "line1",
		},
		{
			name:      "line1\nline2 part2",
			input:     "line1\nline2",
			runeIndex: 5,
			expected:  "line1",
		},
		{
			name:      "line1\nline2 part3",
			input:     "line1\nline2",
			runeIndex: 6,
			expected:  "line2",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := NewWalker([]byte(tc.input))

			w.SetRunePos(tc.runeIndex)
			a, b := w.CurrentLineBounds()
			s := string(w.TextBetweenRuneIndices(a, b))

			if s != tc.expected {
				t.Fatalf("Expected '%s' but got '%s'", tc.expected, s)
			}

		})
	}
}

func TestWalkerCurrentLineBoundsWithTrailingNl(t *testing.T) {

	tests := []struct {
		name      string
		input     string
		runeIndex int
		expected  string
	}{
		{
			name:      "simple",
			input:     "simple",
			runeIndex: 1,
			expected:  "simple",
		},
		{
			name:      "simple",
			input:     "simple",
			runeIndex: 0,
			expected:  "simple",
		},
		{
			name:      "simple",
			input:     "simple",
			runeIndex: 5,
			expected:  "simple",
		},
		{
			name:      "line1\nline2",
			input:     "line1\nline2",
			runeIndex: 1,
			expected:  "line1\n",
		},
		{
			name:      "line1\nline2 part2",
			input:     "line1\nline2",
			runeIndex: 5,
			expected:  "line1\n",
		},
		{
			name:      "line1\nline2 part3",
			input:     "line1\nline2",
			runeIndex: 6,
			expected:  "line2",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := NewWalker([]byte(tc.input))

			w.SetRunePos(tc.runeIndex)
			a, b := w.CurrentLineBoundsIncludingNl()
			s := string(w.TextBetweenRuneIndices(a, b))

			if s != tc.expected {
				t.Fatalf("Expected '%s' but got '%s'", tc.expected, s)
			}

		})
	}
}
