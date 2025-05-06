package main

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/jeffwilliams/anvil/editor/internal/typeset"
)

func TestBackwardsLayouter(t *testing.T) {

	tests := []struct {
		name       string
		inputLines []string
	}{
		{
			name:       "one line short",
			inputLines: []string{"Hello."},
		},
		{
			name:       "one line",
			inputLines: []string{"This is a line."},
		},
		{
			name:       "one line with newline",
			inputLines: []string{"This is a line.\n", ""},
		},
		{
			name: "four lines",
			inputLines: []string{
				"This is a line.\n",
				"This too.\n",
				"Another one...\n",
				"And the last line is much longer than the other lines, because why not?",
			},
		},
	}

	constraints := typeset.Constraints{
		FontFace:   VariableFont,
		FontFaceId: "blah",
		FontSize:   14,
		WrapWidth:  100,
		MaxHeight:  -1,
	}

	maxNumberOfWrappedLinesSeen := 0

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {

			combinedInput := strings.Join(tc.inputLines, "")
			bl := NewBackwardsLayouter([]byte(combinedInput), utf8.RuneCountInString(combinedInput), nil, constraints)

			for i := 0; i < len(tc.inputLines); i++ {
				j := len(tc.inputLines) - i - 1
				eof, wrappedCount, lineLenInRunes := bl.Next()

				if wrappedCount > maxNumberOfWrappedLinesSeen {
					maxNumberOfWrappedLinesSeen = wrappedCount
				}

				t.Logf("Next() for line '%s' resulted in %d wrapped lines\n", tc.inputLines[j], wrappedCount)

				// Get "expected" value for comparison
				text, errs := typeset.Layout([]byte(tc.inputLines[j]), constraints)
				if errs != nil {
					t.Fatalf("Test aborted: typeset.Layout failed: %v", errs)
				}
				t.Logf("laying out line: '%s' resulted in %d wrapped lines\n", tc.inputLines[j], text.LineCount())
				for _, l := range text.Lines() {
					t.Logf("  '%s'\n", string(l.Runes()))
				}

				if eof && i < len(tc.inputLines)-1 {
					t.Fatalf("Expected Next() to return values %d times before EOF, but only got %d \n",
						len(tc.inputLines), i+1)
					break
				}

				if wrappedCount != text.LineCount() {
					t.Fatalf("For input line %d BackwardsLayouter returned a wrapped count of %d lines, but typeset.Layout returned %d\n",
						j, wrappedCount, text.LineCount())
				}

				c := utf8.RuneCountInString(tc.inputLines[j])
				if lineLenInRunes != c {
					t.Fatalf("For input line %d BackwardsLayouter returned a rune count of %d, but it is actually %d\n",
						lineLenInRunes, wrappedCount, c)
				}

			}

		})
	}

	t.Logf("In all the testcases the max number of wrapped lines seen was %d\n", maxNumberOfWrappedLinesSeen)
}
