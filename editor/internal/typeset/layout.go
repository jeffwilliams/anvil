// Package typeset is for layout out text. It doesn't render.
package typeset

import (
	"fmt"
	"unicode/utf8"

	"gioui.org/text"
	"github.com/timtadh/data-structures/hashtable"
	"golang.org/x/image/math/fixed"
)

var cachingEnabled = true

func Layout(text []byte, constraints Constraints) (Text, []error) {
	runes := []rune(string(text))
	l := newLayouter(runes, constraints)
	t := l.layout()
	return t, l.errors
}

type layouter struct {
	input       []rune
	constraints Constraints

	currentLine     []rune
	nextRune        int
	wrapWidth       fixed.Int26_6
	tabStopInterval fixed.Int26_6
	maxHeight       fixed.Int26_6
	height          fixed.Int26_6
	extraLineGap    fixed.Int26_6
	text            Text
	lineBuilder     lineBuilder

	spaceGlyph     text.Glyph
	tofuGlyph      text.Glyph
	newlineGlyph   text.Glyph
	errors         []error
	shaper         *text.Shaper
	cache          *hashtable.Hash
	elastic        *elastic
	cachingEnabled bool
}

func newLayouter(input []rune, constraints Constraints) layouter {
	c := layoutCacheForConstraints(constraints)
	l := layouter{input: input, constraints: constraints, cache: c}
	if constraints.ElasticTabStops {
		l.elastic = newElastic(input, constraints.FontFace, constraints.FontSize, constraints.ElasticMinWidth, constraints.ElasticPadding)
	}

	l.init()
	return l
}

func (l *layouter) layout() Text {

	for {
		offset, r, eof := l.nextInputRune()
		if eof {
			break
		}

		if l.isAnotherLineTooMuch() {
			break
		}

		// Checks our cache of previously output lines.
		// The cache is keyed by the unwrapped input line. We read all the way to the newline
		// then check the cache for which lines that turns into.
		if l.currentLineEmpty() && cachingEnabled && l.cachingEnabled {
			e, err := l.cache.Get(runeSliceKey(l.currentLine))
			if err == nil {
				lines := e.([]Line)
				// We already processed a line like this before.
				// Advance the line length, but -1 because we just processed the first char of the next line.
				l.forwardBy(len(l.currentLine) - 1)
				l.outputLines(lines)
				l.currentLine = l.lineStartingAt(l.nextRune)
				l.incrementSourceLineCount()
				continue
			}
		}

		if r == '\n' {
			l.incrementSourceLineCount()
			l.appendNewlineToLine()
			l.cacheAndOutputLine()
			l.currentLine = l.lineStartingAt(l.nextRune)
			if l.elastic != nil {
				l.elastic.incrementLine()
			}
			continue
		}

		output := l.layoutRune(r, offset)
		if l.wrapWidth > 0 && l.lineWidthPlus(&output) > l.wrapWidth {
			l.cacheAndOutputLine()
		}

		l.appendRuneToLine(r, &output)
	}

	if !l.currentLineEmpty() {
		l.incrementSourceLineCount()
		// We don't want to cache an input line that we didn't fully
		// process into output lines.
		l.deleteCurrentLineFromCache()
		l.outputLine()
	}

	//l.beautifyText(&l.text)
	return l.text
}

func (l *layouter) nextInputRune() (offset int, rn rune, eof bool) {
	if l.nextRune >= len(l.input) {
		eof = true
		return
	}

	if l.currentLine == nil /*|| rn == '\n' */ {
		l.currentLine = l.lineStartingAt(l.nextRune)
	}

	offset = l.nextRune
	rn = l.input[l.nextRune]
	l.nextRune++

	return
}

func (l *layouter) forwardBy(n int) {
	l.nextRune += n
}

func (l *layouter) lineStartingAt(offset int) []rune {
	if offset >= len(l.input) {
		return nil
	}

	sl := l.input[offset:]
	for i, r := range sl {
		if r == '\n' {
			return sl[:i+1] // include newline
		}
	}
	return sl
}

func (l *layouter) init() {
	l.wrapWidth = fixed.I(l.constraints.WrapWidth)
	l.tabStopInterval = fixed.I(l.constraints.TabStopInterval)
	l.maxHeight = fixed.I(l.constraints.MaxHeight)
	l.extraLineGap = fixed.I(l.constraints.ExtraLineGap)
	l.cachingEnabled = !l.constraints.ElasticTabStops
	l.initShaper()
	l.initSpaceGlyph()
	l.initTofuGlyph()
	l.initLineHeight()
	l.initNewlineGlyph()
}

func (l *layouter) initShaper() {
	l.shaper = GetTextShaper(l.constraints.FontFace)
}

func (l *layouter) initSpaceGlyph() {
	var err error
	l.spaceGlyph, err = l.shapeOneRune(' ')
	if err != nil {
		l.errors = append(l.errors, fmt.Errorf("Got an error making space Glyph: %v. Perhaps font face contains no glyph for the space rune?", err))
	}
}

func (l *layouter) initTofuGlyph() {
	var err error
	l.tofuGlyph, err = l.shapeOneRune('□')
	if err != nil {
		l.errors = append(l.errors, fmt.Errorf("Got an error making tofu Glyph: %v. Perhaps font face contains no glyph for the space rune?", err))
	}

}

func (l *layouter) initLineHeight() {
	l.text.lineHeight, l.text.ascent, l.text.descent = l.calculateLineMetricsBasedOn('X')
}

func (l *layouter) calculateLineMetricsBasedOn(r rune) (height, ascent, descent fixed.Int26_6) {
	g, err := l.shapeOneRune(r)
	if err != nil {
		l.errors = append(l.errors, fmt.Errorf("Shaping rune %c failed: %w", r, err))
		return
	}

	ascent = g.Ascent
	descent = g.Descent
	height = g.Ascent + g.Descent
	return
}

func calculateLineMetricsBasedOn(r rune, fontFace text.FontFace, fontSize int, extraLineGap fixed.Int26_6) (height, ascent, descent fixed.Int26_6, err error) {
	var g text.Glyph
	g, err = shapeOneRune(r, fontFace, fontSize)

	ascent = g.Ascent
	descent = g.Descent
	height = g.Ascent + g.Descent
	return
}

func CalculateLineHeight(face text.FontFace, fontSize, extraLineGap int) (height fixed.Int26_6, err error) {
	height, _, _, err = calculateLineMetricsBasedOn('X', face, fontSize, fixed.I(extraLineGap))
	return
}

func (l *layouter) shapeOneRune(r rune) (glyph text.Glyph, err error) {
	params := text.Parameters{
		Font:             l.constraints.FontFace.Font,
		PxPerEm:          fixed.I(l.constraints.FontSize),
		DisableSpaceTrim: true,
	}

	l.shaper.LayoutString(params, string(r))
	glyph, ok := l.shaper.NextGlyph()
	if !ok {
		err = fmt.Errorf("text.Shape.LayoutString returned with ok=false for a single rune")
	}
	return
}

func shapeOneRune(r rune, fontFace text.FontFace, fontSize int) (glyph text.Glyph, err error) {
	params := text.Parameters{
		Font:    fontFace.Font,
		PxPerEm: fixed.I(fontSize),
	}

	collection := []text.FontFace{fontFace}
	// TODO: This is slow to figure out the info for one rune. Can we remove it?
	shaper := text.NewShaper(text.WithCollection(collection))

	shaper.LayoutString(params, string(r))
	glyph, ok := shaper.NextGlyph()
	if !ok {
		err = fmt.Errorf("text.Shape.LayoutString returned with ok=false for a single rune")
	}
	return
}

func (l *layouter) initNewlineGlyph() {
	l.newlineGlyph = text.Glyph{
		ID: l.spaceGlyph.ID,
	}
}

func (l *layouter) isAnotherLineTooMuch() bool {
	return l.maxHeight > 0 && l.height+l.text.lineHeight > l.maxHeight
}

func (l *layouter) incrementSourceLineCount() {
	l.text.sourceLineCount++
}

func (l *layouter) appendNewlineToLine() {
	l.lineBuilder.append_('\n', l.newlineGlyph)
}

func (l *layouter) cacheAndOutputLine() {
	line := l.lineBuilder.getAndReset()
	l.cacheLine(line)
	l.text.lines = append(l.text.lines, line)
	l.text.byteCount += line.byteCount
	l.height += l.text.lineHeight
}

func (l *layouter) cacheLine(line Line) {
	if cachingEnabled && l.cachingEnabled {
		k := runeSliceKey(l.currentLine)
		e, err := l.cache.Get(k)
		if err == nil {
			lines := e.([]Line)
			lines = append(lines, line)
			l.cache.Put(k, lines)
		} else {
			l.cache.Put(k, []Line{line})
		}
	}
}

func (l *layouter) outputLine() {
	line := l.lineBuilder.getAndReset()
	l.text.lines = append(l.text.lines, line)
	l.text.byteCount += line.byteCount
	l.height += l.text.lineHeight
}

func (l *layouter) deleteCurrentLineFromCache() {
	l.cache.Remove(runeSliceKey(l.currentLine))
}

func (l *layouter) outputLines(lines []Line) {
	l.text.lines = append(l.text.lines, lines...)
	for _, ln := range lines {
		l.text.byteCount += ln.byteCount
		l.height += l.text.lineHeight
	}
}

func (l *layouter) layoutRune(r rune, offset int) text.Glyph {
	g, err := l.shapeOneRune(r)
	if err != nil {
		return l.tofuGlyph
	}

	l.expandTabsInGlyph(r, &g)
	l.replaceCarriageReturnsInGlyph(r, &g)

	return g
}

func (l *layouter) expandTabsInGlyph(r rune, g *text.Glyph) {
	if r != '\t' {
		return
	}

	if l.elastic != nil {
		w, err := l.elastic.tabWidth()
		advance := w
		if err != nil {
			advance = l.tabStopInterval
		}
		//advance := nextTabStop - l.lineWidth()
		g.Advance = advance
		g.Offset = fixed.Point26_6{0, 0}
		g.ID = l.spaceGlyph.ID
		g.Ascent = l.text.LineAscent()

		l.elastic.incrementTab()
		return
	}

	nextTabStop := (l.lineBuilder.lineWidth()/l.tabStopInterval + 1) * l.tabStopInterval
	advance := nextTabStop - l.lineWidth()
	g.Advance = advance
	g.Offset = fixed.Point26_6{0, 0}
	g.ID = l.spaceGlyph.ID
	g.Ascent = l.text.LineAscent()
}

func (l *layouter) replaceCarriageReturnsInGlyph(r rune, g *text.Glyph) {
	if r != '\r' || l.tofuGlyph.ID == 0 || !l.constraints.ReplaceCRWithTofu {
		return
	}

	*g = l.tofuGlyph
}

func (l *layouter) lineWidthPlus(g *text.Glyph) fixed.Int26_6 {
	return g.Advance + l.lineBuilder.lineWidth()
}

func (l *layouter) lineWidth() fixed.Int26_6 {
	return l.lineBuilder.line.width
}

func (l *layouter) appendRuneToLine(r rune, g *text.Glyph) {

	g.X = l.lineBuilder.get().Width()
	g.Advance = roundFixed(g.Advance)

	l.lineBuilder.append_(r, *g)
}

func (l *layouter) beautifyText(text *Text) {
	for i := range text.lines {
		l.beautifyLine(&text.lines[i])
	}
}

// beautifyLine is meant to adjust the spacing between the glyphs within the line
// so that it looks better. The idea is that we re-layout all the runes of the entire
// line together so that the text layout engine can figure out the internal spacing.
// However, visually I didn't see any improvement so it's not used for now.
func (l *layouter) beautifyLine(line *Line) {
	params := text.Parameters{
		Font:    l.constraints.FontFace.Font,
		PxPerEm: fixed.I(l.constraints.FontSize),
	}

	l.shaper.LayoutString(params, string(line.runes))

	line.glyphs = line.glyphs[:0]
	for i := 0; ; i++ {
		g, ok := l.shaper.NextGlyph()
		if !ok {
			break
		}
		if i < len(line.runes) {
			r := line.runes[i]
			// This is not working right yet:
			l.expandTabsInGlyph(r, &g)
			l.replaceCarriageReturnsInGlyph(r, &g)
		}
		line.glyphs = append(line.glyphs, g)
	}
}

func roundFixed(i fixed.Int26_6) fixed.Int26_6 {
	// Take the whole part and then if the highest bit of the
	// fractional part is 1 (>= 0.5) add 1 to the whole part,
	// but if the highest bit of the fractional part is 0 then add 0
	// to the whole part.
	return (i & 0xfffc0) + ((i & 0x20) << 1)
}

func fixedIsNotWhole(i fixed.Int26_6) bool {
	v := (i >> 6) << 6
	return v != i
}

func fixedAsString(i fixed.Int26_6) string {
	return fmt.Sprintf("%d %d/%d", i>>6, i&0x3F, 0x40)
}

func (l *layouter) currentLineEmpty() bool {
	return l.lineBuilder.empty()
}

type lineBuilder struct {
	line Line
}

func (b *lineBuilder) append_(r rune, g text.Glyph) {
	b.line.runes = append(b.line.runes, r)
	b.line.glyphs = append(b.line.glyphs, g)
	b.line.byteCount += utf8.RuneLen(r)
	b.line.width += g.Advance
}

func (b *lineBuilder) getAndReset() (line Line) {
	line = b.get()
	b.line.runes = nil
	b.line.glyphs = nil
	b.line.width = 0
	b.line.byteCount = 0
	return
}

func (b *lineBuilder) get() (line Line) {
	if b.line.runes == nil {
		line.runes = emptyRuneSlice
		line.glyphs = emptyGlyphSlice
	}
	line = b.line
	return
}

func (b *lineBuilder) empty() bool {
	return len(b.line.runes) == 0
}

func (b *lineBuilder) lineWidth() fixed.Int26_6 {
	return b.line.width
}

var emptyRuneSlice = []rune{}
var emptyGlyphSlice = []text.Glyph{}

// Constraints constrain how the text is layed out
type Constraints struct {
	FontSize int
	// FontFaceId is the name for the Face in the Face field. It must be set uniquely
	// for different Face values
	FontFaceId string
	FontFace   text.FontFace
	// a WrapWidth of 0 means do not wrap.
	WrapWidth       int // in pixels
	TabStopInterval int // in pixels
	// a MaxHeight of 0 means process all the input text, no matter how many lines it creates.
	MaxHeight         int // stop laying out when this height is reached. Use -1 to layout all text.
	ExtraLineGap      int
	ReplaceCRWithTofu bool
	ElasticTabStops   bool
	ElasticMinWidth   int // in pixels
	ElasticPadding    int // in pixels
}
