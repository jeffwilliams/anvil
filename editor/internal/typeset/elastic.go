package typeset

import (
	"fmt"

	"gioui.org/text"
	"golang.org/x/image/math/fixed"
)

// elastic implements elastic tabstops for our layouter.
// In our implementation, when a tab is encountered we adjust the tab width so as to
// align the text in the column block. This tab width varies from line to line.
//
// [1] https://nick-gravgaard.com/elastic-tabstops/
// [2] https://observablehq.com/@shaunlebron/elastic-tabstops
//
// Usage:
//
//	for each line {
//	  for char in line {
//	    if char is tab:
//	      padding = elastic.tabWidth()
//	      elastic.incrementTab()
//	  }
//	  elastic.incrementLine()
//	}
type elastic struct {
	input []rune
	// lines contains the lines of the input. Each entry (each line)
	// is represented as a slice of cells. Each cell is the text
	// within that cell.
	lines         [][]cell
	minWidth      fixed.Int26_6
	padding       fixed.Int26_6
	callerLineNum int
	callerTabNum  int
	shaper        *text.Shaper
	shaperParams  text.Parameters
}

func newElastic(input []rune, fontFace text.FontFace, fontSize, minWidth, padding int) *elastic {
	if padding > minWidth {
		minWidth = padding
	}

	e := &elastic{
		input:  input,
		shaper: GetTextShaper(fontFace),
		shaperParams: text.Parameters{
			Font:    fontFace.Font,
			PxPerEm: fixed.I(fontSize),
			DisableSpaceTrim: true,
		},
		minWidth: fixed.I(minWidth),
		padding:  fixed.I(padding),
	}

	// Set the lines field
	e.calculateLines()

	return e
}

func (e *elastic) calculateLines() {
	cellStart := 0
	lineStart := 0
	cells := []cell{}

	appendCell := func(endIndex int) {
		cel := cell{text: e.input[cellStart:endIndex], width: fixed.I(-1)}
		cells = append(cells, cel)
		cellStart = endIndex + 1
	}

	appendLine := func(endIndex int) {
		e.lines = append(e.lines, cells)
		lineStart = endIndex + 1
	}

	for i, r := range e.input {
		switch r {
		case '\t':
			appendCell(i)
		case '\n':
			appendCell(i)
			appendLine(i)
			cells = []cell{}
		}
	}

	if lineStart < len(e.input) {
		appendCell(len(e.input))
		appendLine(len(e.input))
	}

}

func (e *elastic) incrementLine() {
	e.callerLineNum++
	e.callerTabNum = 0
}

func (e *elastic) incrementTab() {
	e.callerTabNum++
}

// tabWidth returns the width of the tab that terminates a cell in units of pixels.
func (e *elastic) tabWidth() (width fixed.Int26_6, err error) {
	block := e.currentColumnBlock()
	if block == nil {
		err = fmt.Errorf("line or cell out of range")
		return
	}

	totalWidth := block.width
	textWidth := e.currentCell().width
	width = totalWidth - textWidth
	return
}

func (e *elastic) layedOutTextWidth(s []rune) (width fixed.Int26_6) {
	e.shaper.LayoutString(e.shaperParams, string(s))

	for {
		g, ok := e.shaper.NextGlyph()
		if !ok {
			return
		}
		width += roundFixed(g.Advance)
	}

	return
}

func (e *elastic) currentColumnBlock() *columnBlock {

	// if we already created and cached the current columnBlock, then
	// return that. Otherwise compute it.
	//
	// To compute it, find out what line and cell we are in. Then get the same cell
	// in the previous lines and same cell in following lines. The text in that list
	// of cells comprises the column block. With that text, lay it all out and find
	// the longest text out of all the cells. Add some padding and consider that the
	// width. if the width it is below a threshold, make it the threshold.
	//
	// Store a pointer in each of the cells in the lines to this block.

	c := e.currentCell()
	if c == nil {
		return nil
	}
	if c.block != nil {
		return c.block
	}

	bldr := blockBuilder{
		lines:     e.lines,
		lineIndex: e.callerLineNum,
		cellIndex: e.callerTabNum,
		getWidth:  e.getCellWidth,
		padding:   e.padding,
		minWidth:  e.minWidth,
	}

	block := bldr.buildBlock()
	return block
}

type blockBuilder struct {
	lines     [][]cell
	lineIndex int
	cellIndex int
	getWidth  func(cell *cell) fixed.Int26_6
	padding   fixed.Int26_6
	minWidth  fixed.Int26_6
}

func (bld blockBuilder) buildBlock() *columnBlock {
	if len(bld.lines) == 0 {
		return nil
	}

	lineHasCell := func(lineIndex, cellIndex int) bool {
		// We don't consider the last text as a true cell.
		return len(bld.lines[lineIndex])-1 > cellIndex
	}

	startCellLineIndex := bld.lineIndex
	endCellLineIndex := bld.lineIndex
	cellIndexWithinLine := bld.cellIndex

	if !lineHasCell(bld.lineIndex, bld.cellIndex) {
		return nil
	}

	for startCellLineIndex >= 0 {
		if !lineHasCell(startCellLineIndex, cellIndexWithinLine) {
			break
		}
		startCellLineIndex--
	}
	startCellLineIndex++

	for endCellLineIndex < len(bld.lines) {
		if !lineHasCell(endCellLineIndex, cellIndexWithinLine) {
			break
		}
		endCellLineIndex++
	}
	endCellLineIndex--

	block := &columnBlock{}

	var largestTextWidth fixed.Int26_6
	for i := startCellLineIndex; i <= endCellLineIndex; i++ {
		bld.lines[i][cellIndexWithinLine].block = block
		//w := e.lines[startCellLineIndex][endCellLineIndex].getWidth(e)
		w := bld.getWidth(&bld.lines[i][cellIndexWithinLine])
		if w > largestTextWidth {
			largestTextWidth = w
		}
	}

	w := largestTextWidth + bld.padding
	if w < bld.minWidth {
		w = bld.minWidth
	}
	block.width = w

	return block
}

func (e *elastic) currentCell() *cell {
	if e.callerLineNum < 0 || e.callerLineNum >= len(e.lines) {
		return nil
	}
	if e.callerTabNum < 0 || e.callerTabNum >= len(e.lines[e.callerLineNum]) {
		return nil
	}
	return &e.lines[e.callerLineNum][e.callerTabNum]
}

func (e *elastic) getCellWidth(cell *cell) fixed.Int26_6 {
	return cell.getWidth(e)
}

type cell struct {
	text []rune
	// width is the width of the text in the cell, if computed.
	// It is fixed.I(-1) otherwise.
	width fixed.Int26_6
	// The column block this cell belongs to, if computed. Set to nil if not
	// computed.
	block *columnBlock
}

func (c *cell) getWidth(e *elastic) fixed.Int26_6 {
	if c.width == fixed.I(-1) {
		c.width = e.layedOutTextWidth(c.text)
	}

	return c.width
}

type columnBlock struct {
	width fixed.Int26_6
}
