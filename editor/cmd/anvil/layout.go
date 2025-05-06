package main

import (
	"unicode/utf8"

	"gioui.org/layout"
	"gioui.org/text"
	"gioui.org/unit"
	"github.com/jeffwilliams/anvil/editor/internal/runes"
	"github.com/jeffwilliams/anvil/editor/internal/typeset"
)

type OpsForNextLayout []LayoutOp

type LayoutOp func(gtx layout.Context)

func (o *OpsForNextLayout) Add(op LayoutOp) {
	*o = append(*o, op)
}

func (o *OpsForNextLayout) Perform(gtx layout.Context) {
	for _, op := range *o {
		op(gtx)
	}

	if *o != nil && len(*o) > 0 {
		*o = (*o)[:0]
	}
}

// BackwardsLayouter lays out backwards from the end of doc.
type BackwardsLayouter struct {
	gtx             layout.Context
	doc             []byte
	walker          runes.Walker
	constraints     typeset.Constraints
	runeOffsetCache *runes.OffsetCache
	line            []byte
}

func NewBackwardsLayouter(doc []byte, runeCount int, runeOffsetCache *runes.OffsetCache, constraints typeset.Constraints) BackwardsLayouter {
	bl := BackwardsLayouter{
		doc:         doc,
		constraints: constraints,
		walker:      runes.NewWalker(doc),
		line:        make([]byte, 0, 400),
	}
	if runeCount >= 1 && runeOffsetCache != nil {
		bl.walker.SetRunePosCache(runeCount-1, runeOffsetCache)
	} else {
		// runeCount not set
		bl.walker.GoToEnd()
	}
	return bl
}

// wrappedCount is the number of lines in the window the unwrapped line consumes when wrapped
func (bl *BackwardsLayouter) Next() (eof bool, wrappedCount int, lineLenInRunes int) {
	if bl.walker.RunePos() == 0 {
		eof = true
		return
	}

	line, lineLenInRunes := bl.curLineBackwards()
	stripped, hadNl := stripTrailingNl(line)

	lo, errs := typeset.Layout(stripped, bl.constraints)
	if errs != nil && len(errs) > 0 {
		log(LogCatgEd, "layoutPreviousPageBackwardsFrom: errors laying out: %v\n", errs)
	}

	if len(stripped) == 0 && hadNl {
		wrappedCount++
	}

	wrappedCount += lo.LineCount()

	return
}

// Get the next line in the reverse direction
func (l *BackwardsLayouter) curLineBackwards() (line []byte, runeLen int) {

	l.walker.BackwardToStartOfLine()
	pos := l.walker.Position()
	l.line = l.line[:0]
	for {
		if l.walker.AtEnd() {
			break
		}
		r := l.walker.Rune()
		l.line = utf8.AppendRune(l.line, r)
		if r == '\n' {
			runeLen++
			break
		}
		runeLen++
		l.walker.Forward(1)
	}
	l.walker.SetPosition(pos)
	l.walker.Backward(1)
	return l.line, runeLen
}

func stripTrailingNl(s []byte) (stripped []byte, hadNl bool) {
	if len(s) == 0 {
		stripped = s
		return
	}
	if s[len(s)-1] == '\n' {
		return s[:len(s)-1], true
	}
	return s, false
}

type layouter struct {
	curFontIndex     int
	fontStyles       []FontStyle
	lineSpacing      unit.Dp
	cachedFontSize   int
	cachedLineHeight int
	cachedMetric     unit.Metric
}

func (l *layouter) setFontStyles(fontStyles []FontStyle) {
	l.fontStyles = fontStyles
	l.invalidateCache()
}

func (l *layouter) curFont() text.FontFace {
	return l.fontStyles[l.curFontIndex].FontFace
}

func (l *layouter) curFontSize() int {
	m := application.Metric()
	if m == nil {
		return int(l.fontStyles[l.curFontIndex].FontSize)
	}

	if l.cachedMetric != *m {
		l.invalidateCache()
	}
	l.cachedMetric = *m

	if l.cachedFontSize != 0 {
		return l.cachedFontSize
	}

	if *optPixelSizeFonts {
		return int(l.fontStyles[l.curFontIndex].FontSize)
	}

	sz := m.Sp(l.fontStyles[l.curFontIndex].FontSize)
	l.cachedFontSize = sz
	return sz
}

func (l *layouter) curFontName() string {
	return l.fontStyles[l.curFontIndex].FontName
}

func (l *layouter) nextFont() {
	if len(l.fontStyles) < 2 {
		return
	}
	l.curFontIndex = (l.curFontIndex + 1) % len(l.fontStyles)
	l.invalidateCache()
}

func (l *layouter) lineHeight() int {
	m := application.Metric()
	if m == nil {
		return int(l.fontStyles[l.curFontIndex].FontSize)
	}

	if l.cachedMetric != *m {
		l.invalidateCache()
	}
	l.cachedMetric = *m

	if l.cachedLineHeight != 0 {
		return l.cachedLineHeight
	}

	h, err := typeset.CalculateLineHeight(l.curFont(), l.curFontSize(), m.Dp(l.lineSpacing))
	if err != nil {
		log(LogCatgUI, "lineHeight: error calculating height: %v\n", err)
		l.cachedLineHeight = 0
		return 16
	}
	lh := h.Round()
	l.cachedLineHeight = lh
	return lh
}

func (l *layouter) invalidateCache() {
	l.cachedFontSize = 0
	l.cachedLineHeight = 0
}

func (l *layouter) lineSpacingScaled() int {
	m := application.Metric()
	if m == nil {
		return int(l.lineSpacing)
	}
	return m.Dp(l.lineSpacing)
}
