package ansi

import (
	"image/color"
	"unicode/utf8"

	"github.com/jeffwilliams/anvil/editor/internal/intvl"
	ansip "github.com/leaanthony/go-ansi-parser"
)

func InitColors(colors [16]color.NRGBA) {
	for i, c := range colors {
		ansip.Cols[i].Rgb = NRGBAToAnsiRgb(&c)
	}
}

func HasEscapeCodes(text []byte) bool {
	return ansip.HasEscapeCodes(string(text))
}

func HighlightColorEscapeSequences(text []byte, runeOffset int, makeInterval func(start, end int, color color.NRGBA) intvl.Interval) (seq []intvl.Interval, err error) {
	parsed, err := ansip.Parse(string(text), ansip.WithIgnoreInvalidCodes())
	if err != nil {
		return
	}

	lenInRunes := func(start, length int) int {
		return utf8.RuneCount(text[start : start+length])
	}

	off := 0
	for _, t := range parsed {
		l := lenInRunes(t.Offset, t.Len)
		start := off + runeOffset
		end := start + l
		if t.FgCol != nil {
			seq = append(seq, makeInterval(start, end, ansiColorToNRGBA(t.FgCol)))
		}
		off += l
	}

	return
}

func ansiColorToNRGBA(c *ansip.Col) color.NRGBA {
	if c == nil {
		return color.NRGBA{}
	}
	return color.NRGBA{R: c.Rgb.R, G: c.Rgb.G, B: c.Rgb.B, A: 0xff}
}

func NRGBAToAnsiRgb(c *color.NRGBA) ansip.Rgb {
	if c == nil {
		return ansip.Rgb{}
	}
	return ansip.Rgb{R: c.R, G: c.G, B: c.B}
}
