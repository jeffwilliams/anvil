package main

import (
	"bytes"
	"fmt"

	"github.com/jeffwilliams/anvil/api/go/anvil"
	"github.com/jeffwilliams/terminal"
)

type Converter struct {
	termState *terminal.State
	bodyText  bytes.Buffer
	body      []byte
	cursorPos int
	tints     []anvil.Tint
}

func NewConverter(termState *terminal.State) Converter {
	return Converter{termState: termState}
}

func (c *Converter) Convert(winWidthInRunes, winHeightInRunes int) {
	c.buildBody(winWidthInRunes, winHeightInRunes)
	c.determineCursorPos()
	c.calculateTints(winWidthInRunes, winHeightInRunes)
}

func (c *Converter) buildBody(winWidthInRunes, winHeightInRunes int) {
	c.bodyText.Reset()

	var line bytes.Buffer

	for y := range winHeightInRunes {
		line.Reset()
		for x := range winWidthInRunes {
			rn, _, _ := c.termState.Cell(x, y)
			line.WriteRune(rn)
		}
		//l := strings.TrimRight(line.String(), " ")
		l := line.String()
		c.bodyText.WriteString(l)
		c.bodyText.WriteRune('\n')
	}

	c.body = make([]byte, c.bodyText.Len())
	copy(c.body, c.bodyText.Bytes())
}

func (c *Converter) determineCursorPos() {
	x, y := c.termState.Cursor()
	str := string(c.body)
	c.cursorPos = 0
	for _, r := range str {
		if y <= 0 {
			if x <= 0 {
				break
			}
			x--
			c.cursorPos++
			continue
		}

		if r == '\n' {
			y--
		}
		c.cursorPos++
	}
}

type color struct {
	fg, bg terminal.Color
}

func (c *Converter) calculateTints(winWidthInRunes, winHeightInRunes int) {
	colors := c.buildColorList(winWidthInRunes, winHeightInRunes)
	c.tints = c.convertColorsToTints(colors)
}

func (c *Converter) buildColorList(winWidthInRunes, winHeightInRunes int) []color {
	var colors []color

	for y := range winHeightInRunes {
		for x := range winWidthInRunes {
			_, fg, bg := c.termState.Cell(x, y)
			colors = append(colors, color{fg, bg})
		}
		// Append a placeholder rune for the newline
		colors = append(colors, color{terminal.DefaultFG, terminal.DefaultBG})
	}

	return colors
}

func (c *Converter) convertColorsToTints(colors []color) []anvil.Tint {
	var tints []anvil.Tint

	tints = []anvil.Tint{}

	defaultColor := color{terminal.DefaultFG, terminal.DefaultBG}
	lastColor := defaultColor

	const (
		notInInterval = iota
		inInterval
	)

	state := notInInterval
	var tint anvil.Tint
	// Find intervals
	for i, color := range colors {
		switch state {
		case notInInterval:
			if color != lastColor {
				// new interval start
				tint.Start = i
				tint.Tint = c.tint(color.fg, color.bg)
				state = inInterval
			}
		case inInterval:
			if color != lastColor {
				// last interval end and new interval start
				tint.End = i
				tints = append(tints, tint)
				if color == defaultColor {
					state = notInInterval
					break
				}

				tint.Start = i
				tint.Tint = c.tint(color.fg, color.bg)
			}
		}
		lastColor = color
	}

	if state == inInterval {
		tint.End = len(colors)
		tints = append(tints, tint)
	}

	return tints
}

func (c *Converter) tint(fg, bg terminal.Color) string {
	f := c.colorText(fg)
	b := c.colorText(bg)
	if b != "" {
		return fmt.Sprintf("%s/%s", f, b)
	}
	return fmt.Sprintf("%s", f)
}

func (c *Converter) colorText(color terminal.Color) string {
	if color == terminal.DefaultFG {
		return ""
	}
	if color == terminal.DefaultBG {
		return ""
	}
	return xtermColorToHex(uint8(color & 0xff))
}

func xtermColorToHex(c uint8) string {
	ansi := [16][3]uint8{
		{0x00, 0x00, 0x00}, // 0 black
		{0x80, 0x00, 0x00}, // 1 red
		{0x00, 0x80, 0x00}, // 2 green
		{0x80, 0x80, 0x00}, // 3 yellow
		{0x00, 0x00, 0x80}, // 4 blue
		{0x80, 0x00, 0x80}, // 5 magenta
		{0x00, 0x80, 0x80}, // 6 cyan
		{0xc0, 0xc0, 0xc0}, // 7 white (light gray)
		{0x80, 0x80, 0x80}, // 8 bright black (dark gray)
		{0xff, 0x00, 0x00}, // 9 bright red
		{0x00, 0xff, 0x00}, // 10 bright green
		{0xff, 0xff, 0x00}, // 11 bright yellow
		{0x00, 0x00, 0xff}, // 12 bright blue
		{0xff, 0x00, 0xff}, // 13 bright magenta
		{0x00, 0xff, 0xff}, // 14 bright cyan
		{0xff, 0xff, 0xff}, // 15 bright white
	}

	var r, g, b uint8

	switch {
	case c < 16:
		r, g, b = ansi[c][0], ansi[c][1], ansi[c][2]

	case c >= 16 && c <= 231:
		// 6x6x6 color cube
		c -= 16
		r = (c / 36) % 6
		g = (c / 6) % 6
		b = c % 6

		conv := func(v uint8) uint8 {
			if v == 0 {
				return 0
			}
			return 55 + v*40
		}

		r, g, b = conv(r), conv(g), conv(b)

	default:
		// Grayscale 232–255
		gray := uint8(8 + (c-232)*10)
		r, g, b = gray, gray, gray
	}

	return fmt.Sprintf("#%02X%02X%02X", r, g, b)
}
