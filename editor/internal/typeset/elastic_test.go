package typeset

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/image/math/fixed"
)

var z = fixed.I(-1)

func TestCalculateLines(t *testing.T) {

	tests := []struct {
		name          string
		input         string
		expectedLines [][]cell
	}{
		{
			name:          "empty",
			input:         "",
			expectedLines: nil,
		},
		{
			name:  "single-line-no-tabs",
			input: "abc",
			expectedLines: [][]cell{
				[]cell{
					cell{text: []rune("abc"), width: z},
				},
			},
		},
		{
			name:  "two-lines-no-tabs",
			input: "abc\ndef",
			expectedLines: [][]cell{
				[]cell{
					cell{text: []rune("abc"), width: z},
				},
				[]cell{
					cell{text: []rune("def"), width: z},
				},
			},
		},
		{
			name:  "one-line-one-tab",
			input: "abc\tdef",
			expectedLines: [][]cell{
				[]cell{
					cell{text: []rune("abc"), width: z},
					cell{text: []rune("def"), width: z},
				},
			},
		},
		{
			name:  "empty-line",
			input: "abc\n",
			expectedLines: [][]cell{
				[]cell{
					cell{text: []rune("abc"), width: z},
				},
			},
		},
		{
			name:  "empty-cell",
			input: "abc\t",
			expectedLines: [][]cell{
				[]cell{
					cell{text: []rune("abc"), width: z},
					cell{text: []rune(""), width: z},
				},
			},
		},
		{
			name:  "multi",
			input: "abc\tdef\nline2\tcell2",
			expectedLines: [][]cell{
				[]cell{
					cell{text: []rune("abc"), width: z},
					cell{text: []rune("def"), width: z},
				},
				[]cell{
					cell{text: []rune("line2"), width: z},
					cell{text: []rune("cell2"), width: z},
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {

			e := elastic{input: []rune(tc.input)}
			e.calculateLines()

			assert.Equal(t, tc.expectedLines, e.lines)
		})
	}
}

var charWidth = fixed.I(10)
var padding = fixed.I(5)
var minWidth = fixed.I(20)

func testingGetWidth(cell *cell) fixed.Int26_6 {
	// For testing, make each rune the same length
	//fmt.Printf("testingGetWidth: '%s' is %d chars at %d/char is %d\n", string(cell.text), len(cell.text), charWidth, fixed.Int26_6(len(cell.text))*charWidth)
	return fixed.Int26_6(len(cell.text)) * charWidth
}

func TestBuildBlock(t *testing.T) {

	tests := []struct {
		name      string
		inputText string
		lineIndex int
		cellIndex int
		expected  columnBlock
	}{
		{
			name:      "empty",
			inputText: "",
			lineIndex: 0,
			cellIndex: 0,
			expected:  columnBlock{width: 0},
		},
		{
			name:      "single-line-no-tabs",
			inputText: "abc",
			lineIndex: 0,
			cellIndex: 0,
			expected:  columnBlock{width: charWidth*3 + padding},
		},
		{
			name:      "short-single-line-no-tabs",
			inputText: "a",
			lineIndex: 0,
			cellIndex: 0,
			expected:  columnBlock{width: minWidth},
		},
		{
			name:      "single-line-one-tab",
			inputText: "abc\t",
			lineIndex: 0,
			cellIndex: 0,
			expected:  columnBlock{width: charWidth*3 + padding},
		},
		{
			name:      "single-line-one-tab-second-cell",
			inputText: "abc\tdefg",
			lineIndex: 0,
			cellIndex: 1,
			expected:  columnBlock{width: charWidth*4 + padding},
		},
		{
			name: "multi-line-one-tab",
			// longest cell in block 0 has 3 runes
			inputText: "abc\tde\nde\t",
			lineIndex: 0,
			cellIndex: 0,
			expected:  columnBlock{width: charWidth*3 + padding},
		},
		{
			name: "multi-line-one-tab-second-cell",
			// longest cell in block 0 has 3 runes
			inputText: "abc\tdefg\nde\t",
			lineIndex: 0,
			cellIndex: 1,
			expected:  columnBlock{width: charWidth*4 + padding},
		},
		{
			name: "multi-line-one-tab-second-cell-empty",
			// longest cell in block 0 has 3 runes
			inputText: "abc\t\nde\t",
			lineIndex: 0,
			cellIndex: 1,
			expected:  columnBlock{width: minWidth},
		},
		{
			name: "blocks-separated-by-lines",
			// Here we have a column block with two lines having abc then de, then a blank line,
			// then a block having abcdef. So the first ones should be elasticed separately
			inputText: "abc\t\nde\t\n\nabcdef\t",
			lineIndex: 0,
			cellIndex: 0,
			expected:  columnBlock{width: charWidth*3 + padding},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {

			e := elastic{input: []rune(tc.inputText)}
			e.calculateLines()

			bldr := blockBuilder{
				lines:     e.lines,
				lineIndex: tc.lineIndex,
				cellIndex: tc.cellIndex,
				getWidth:  testingGetWidth,
				padding:   padding,
				minWidth:  minWidth,
			}

			block := bldr.buildBlock()
			if block == nil {
				return
			}

			assert.Equal(t, tc.expected, *block)
		})
	}
}
