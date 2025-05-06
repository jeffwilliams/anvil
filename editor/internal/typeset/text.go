package typeset

import (
	"math"
	"unicode/utf8"

	"gioui.org/f32"
	"gioui.org/text"
	"golang.org/x/image/math/fixed"
)

type Text struct {
	lines           []Line
	lineHeight      fixed.Int26_6
	ascent          fixed.Int26_6
	descent         fixed.Int26_6
	sourceLineCount int // Count of (unwrapped) lines in the input text that this Text represents.
	byteCount       int
}

func (t Text) Lines() []Line {
	return t.lines
}

// SourceLineCount is the number of input lines (strings of runes terminated by newline) in the input
// passed to Layout.
func (t Text) SourceLineCount() int {
	return t.sourceLineCount
}

func (t Text) LineHeight() int {
	return t.lineHeight.Round()
}

func (t Text) LineAscent() fixed.Int26_6 {
	return t.ascent
}

func (t Text) LineDescent() fixed.Int26_6 {
	return t.descent
}

func (t Text) ByteCount() int {
	return t.byteCount
}

func (t Text) LineCount() int {
	return len(t.lines)
}

func (t Text) RuneCount() int {
	c := 0
	for _, l := range t.lines {
		c += l.RuneCount()
	}
	return c
}

func (t Text) Empty() bool {
	return t.LineCount() == 0
}

func (t Text) EndsWith(r rune) bool {
	if len(t.lines) == 0 {
		return false
	}

	return t.lines[len(t.lines)-1].EndsWith(r)
}

func (t Text) lineHeightAsFloat() float32 {
	return float32(t.lineHeight.Round())
}

func (t Text) IndexOfPixelCoord(pos f32.Point) int {

	if t.LineCount() == 0 {
		return 0
	}

	lineIndex := int(math.Floor(float64(pos.Y / t.lineHeightAsFloat())))
	if lineIndex < 0 {
		lineIndex = 0
	} else if lineIndex >= t.LineCount() {
		return t.RuneCount()
	}

	line := &t.lines[lineIndex]

	runeIndex := 0
	for i := 0; i < lineIndex; i++ {
		runeIndex += t.lines[i].RuneCount()
	}

	var xoffset fixed.Int26_6
	posX := fixed.I(int(math.Round(float64(pos.X))))

	reachedEnd := true
	for _, glyph := range line.glyphs {
		adv := glyph.Advance
		if xoffset+adv > posX {
			reachedEnd = false
			break
		}
		xoffset += adv
		runeIndex++
	}

	if reachedEnd && line.EndsWith('\n') {
		// This is to treat a click past the end of a complete (newline terminated) line as a click at the end of the line
		runeIndex--
	}

	return runeIndex
}

type Line struct {
	runes     []rune // Should this be []byte to get the length in bytes easier?
	glyphs    []text.Glyph
	byteCount int
	width     fixed.Int26_6
	ascent    fixed.Int26_6
}

func (l Line) Runes() []rune {
	return l.runes
}

func (l Line) RuneCount() int {
	return len(l.runes)
}

func (l Line) Glyphs() []text.Glyph {
	return l.glyphs
}

func (l Line) EndsWith(r rune) bool {
	if l.RuneCount() == 0 {
		return false
	}
	return l.runes[l.RuneCount()-1] == r
}

func (l Line) Width() fixed.Int26_6 {
	//return l.width.Round()
	return l.width
}

func (l Line) Ascent() fixed.Int26_6 {
	if l.ascent == 0 {
		for _, g := range l.glyphs {
			if g.Ascent > l.ascent {
				l.ascent = g.Ascent
			}
		}
	}
	return l.ascent
}

func (l *Line) Split(index int) (first, rest *Line) {
	if index < 0 || index > l.RuneCount() {
		first = l
		rest = nil
		return
	}

	firstRunes := l.runes[0:index]
	lastRunes := l.runes[index:]

	firstGlyphs := l.glyphs[0:index]
	lastGlyphs := l.glyphs[index:]

	firstByteCount := 0
	for _, r := range firstRunes {
		firstByteCount = utf8.RuneLen(r)
	}
	lastByteCount := l.byteCount - firstByteCount

	firstWidth := fixed.Int26_6(0)
	for _, g := range firstGlyphs {
		firstWidth += g.Advance
	}
	lastWidth := l.width - firstWidth

	first = &Line{
		runes:     firstRunes,
		glyphs:    firstGlyphs,
		byteCount: firstByteCount,
		width:     firstWidth,
	}

	rest = &Line{
		runes:     lastRunes,
		glyphs:    lastGlyphs,
		byteCount: lastByteCount,
		width:     lastWidth,
	}

	return
}
