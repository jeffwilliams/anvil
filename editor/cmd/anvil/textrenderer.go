package main

import (
	"bytes"
	"image"

	"image/color"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"github.com/jeffwilliams/anvil/editor/internal/cache"
	"github.com/jeffwilliams/anvil/editor/internal/typeset"
	"golang.org/x/image/math/fixed"
)

/*
Some notes about GIOUI text layout that are not immediately obvious:

	* a text.Layout may have fewer Glyphs than there were runes in the input text. Some runes (such as newline) may have no glyph.
	* the text.Layout method may try and read from the RuneReader after the RuneReadera already returned EOF

*/

type TextRenderer struct {
	layouter
	fontSize                 int
	lineHeight               func() int
	lineSpacing              func() int
	fontFace                 text.FontFace
	fgColor                  Color
	bgColor                  Color
	drawBgColor              bool
	tabStopInterval          int
	shaper                   *text.Shaper
	cachedTextColumnLayouter cachedTextColumnLayouter
}

type TextShapers map[text.FontFace]*text.Shaper

func (t *TextShapers) Get(fontFace text.FontFace) *text.Shaper {
	shaper, ok := (*t)[fontFace]
	if ok {
		return shaper
	}

	shaper = text.NewShaper(text.WithCollection([]text.FontFace{fontFace}), text.NoSystemFonts())
	(*t)[fontFace] = shaper
	return shaper
}

var textShapers = make(TextShapers)

func NewTextRenderer(fontFace text.FontFace, fontSize int, lineSpacing func() int, fgColor Color, lineHeight func() int) *TextRenderer {
	tr := &TextRenderer{
		fontSize:    fontSize,
		fontFace:    fontFace,
		lineSpacing: lineSpacing,
		fgColor:     fgColor,
		lineHeight:  lineHeight,
	}

	//tr.spaceGlyphID, _ = tr.GlyphIDFor(' ')
	tr.tabStopInterval = 14
	tr.shaper = typeset.GetTextShaper(fontFace)
	return tr
}

func (tr *TextRenderer) SetFgColor(c Color) {
	tr.fgColor = c
}

func (tr *TextRenderer) SetBgColor(c Color) {
	tr.bgColor = c
	tr.drawBgColor = true
}

func (tr *TextRenderer) SetDrawBg(b bool) {
	tr.drawBgColor = b
}

func (tr *TextRenderer) SetTabStopInterval(i int) {
	tr.tabStopInterval = i
}

func (tr *TextRenderer) DrawTextline(gtx layout.Context, line *typeset.Line) {
	tr.drawTextBackground(gtx, line)
	tr.drawTextForeground(gtx, line)
}

func (tr *TextRenderer) drawTextBackground(gtx layout.Context, line *typeset.Line) {
	tr.DrawTextBgRect(gtx, line.Width().Round())
}

func (tr *TextRenderer) DrawTextBgRect(gtx layout.Context, width int) {
	if !tr.drawBgColor {
		return
	}
	stack := clip.Rect{Max: image.Pt(width, tr.lineHeight())}.Push(gtx.Ops)
	paint.ColorOp{Color: color.NRGBA(tr.bgColor)}.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)
	stack.Pop()
}

func (tr *TextRenderer) drawTextForeground(gtx layout.Context, line *typeset.Line) {
	ascent := line.Ascent().Round()
	paint.ColorOp{Color: color.NRGBA(tr.fgColor)}.Add(gtx.Ops)

	// The layed-out text is clipped relative to the baseline. This means the Ascent is
	// drawn above the current offset; i.e. if the y offset is 0, the ascent is clipped
	// off the top of the screen (negative). So we need to move it down as needed.
	op.Offset(image.Point{0, ascent}).Add(gtx.Ops)
	path := tr.shape(line)

	stack := clip.Outline{Path: path}.Op().Push(gtx.Ops)
	op.Offset(image.Point{0, -ascent}).Add(gtx.Ops)

	paint.PaintOp{}.Add(gtx.Ops)
	stack.Pop()
}

func (tr *TextRenderer) shape(line *typeset.Line) clip.PathSpec {
	return tr.shaper.Shape(line.Glyphs())
}

func (tr *TextRenderer) LayoutItemsInColumns(gtx layout.Context, items []string) []byte {
	l := tr.cachedTextColumnLayouter.l
	if tr.cachedTextColumnLayouter.l == nil || !tr.cachedTextColumnLayouter.matchesConstraints(tr) {
		l = NewTextColumnLayouter(gtx, tr.fontFace, tr.fontSize, tr.lineSpacing(), tr.tabStopInterval)
		// We cache the column layouter because the text layout for calculating the item widths is expensive.
		// We basically cache the item widths. Since they depend on the MaxX constraint from gtx which might
		// have changed since we last cached the value we need to re-set the gtx every time we use
		// the latouter. It doesn't affect the item widths, only how many columns are present in the layout.
		tr.cachedTextColumnLayouter.l = l
		tr.cachedTextColumnLayouter.setConstraints(tr)
	}
	l.gtx = gtx

	return l.LayoutItemsInColumns(items)
}

type cachedTextColumnLayouter struct {
	l               *TextColumnLayouter
	fontSize        int
	lineSpacing     int
	fontFace        text.FontFace
	tabStopInterval int
}

func (l cachedTextColumnLayouter) matchesConstraints(tr *TextRenderer) bool {
	return l.fontSize == tr.fontSize && l.lineSpacing == tr.lineSpacing() &&
		l.fontFace == tr.fontFace && l.tabStopInterval == tr.tabStopInterval
}

func (l *cachedTextColumnLayouter) setConstraints(tr *TextRenderer) {
	l.fontFace = tr.fontFace
	l.fontSize = tr.fontSize
	l.lineSpacing = tr.lineSpacing()
	l.tabStopInterval = tr.tabStopInterval
}

type direction int

const (
	Forward direction = iota
	Reverse
)

func (d direction) String() string {
	switch d {
	case Forward:
		return "forward"
	case Reverse:
		return "reverse"
	default:
		return "unknown"
	}
}

type textStyle struct {
	FgColor Color
	BgColor Color
}

type TextColumnLayouter struct {
	longestItemWidth fixed.Int26_6
	longestItemIndex int
	colWidth         fixed.Int26_6
	colCount         int
	itemWidths       []fixed.Int26_6

	suffix          string
	gtx             layout.Context
	fontSize        int
	fontFace        text.FontFace
	lineSpacing     int
	tabStopInterval int
	itemWidthCache  cache.Cache[string, fixed.Int26_6]
}

func NewTextColumnLayouter(gtx layout.Context, fontFace text.FontFace, fontSize, lineSpacing, tabStopInterval int) *TextColumnLayouter {
	return &TextColumnLayouter{
		suffix:          "  \t",
		gtx:             gtx,
		fontSize:        fontSize,
		fontFace:        fontFace,
		lineSpacing:     lineSpacing,
		tabStopInterval: tabStopInterval,
		itemWidthCache:  cache.New[string, fixed.Int26_6](6000),
	}
}

func (l *TextColumnLayouter) LayoutItemsInColumns(items []string) []byte {
	l.reset()
	l.calculateItemWidths(items)
	l.findLongestItem()
	l.calculateColumnProperties()
	return l.writeItemsInColumns(items)
}

func (l *TextColumnLayouter) reset() {
	l.longestItemWidth = 0
	l.longestItemIndex = 0
	l.colWidth = 0
	l.colCount = 0
	l.itemWidths = nil
}

func (l *TextColumnLayouter) calculateItemWidths(items []string) {
	l.itemWidths = make([]fixed.Int26_6, len(items))

	constraints := typeset.Constraints{
		FontSize:        l.fontSize,
		FontFace:        l.fontFace,
		TabStopInterval: l.tabStopInterval,
		ExtraLineGap:    l.lineSpacing,
	}

	for i, it := range items {
		text := it + l.suffix

		e := l.itemWidthCache.Get(text)
		if e != nil {
			l.itemWidths[i] = e.Val
			continue
		}

		lines, _ := typeset.Layout([]byte(text), constraints)
		if lines.LineCount() == 0 {
			continue
		}
		l.itemWidths[i] = lines.Lines()[0].Width()
		l.itemWidthCache.Set(text, l.itemWidths[i])
	}
}

func (l *TextColumnLayouter) findLongestItem() {
	for i, w := range l.itemWidths {
		if w > l.longestItemWidth {
			l.longestItemWidth = w
			l.longestItemIndex = i
		}
	}
}

func (l *TextColumnLayouter) calculateColumnProperties() {
	if l.longestItemWidth == 0 {
		l.colCount = 1
		return
	}

	l.colWidth = l.longestItemWidth
	l.colCount = l.gtx.Constraints.Max.X / l.colWidth.Ceil()
	if l.colCount == 0 {
		l.colCount = 1
	}
}

func (l *TextColumnLayouter) writeItemsInColumns(items []string) []byte {

	shouldStartNewColumn := func(i int) bool {
		return i > 0 && i%l.colCount == 0
	}

	isLastItemInColumn := func(i int) bool {
		return i > 0 && i%l.colCount == l.colCount-1
	}

	var buf bytes.Buffer
	for i, it := range items {
		if shouldStartNewColumn(i) {
			buf.WriteRune('\n')
		}

		xtra := (l.colWidth - l.itemWidths[i]).Ceil()
		tabs := xtra / l.tabStopInterval
		if xtra%l.tabStopInterval != 0 {
			xtra++
		}
		buf.WriteString(it)

		if !isLastItemInColumn(i) {
			buf.WriteString("  \t")
			for tabs > 0 {
				buf.WriteRune('\t')
				tabs--
			}
		}
	}
	buf.WriteRune('\n')

	return buf.Bytes()
}
