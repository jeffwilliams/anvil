package main

import (
	"encoding/json"
	"fmt"
	"image/color"
	"os"
	"strings"

	"gioui.org/text"
	"gioui.org/unit"
	"github.com/jeffwilliams/anvil/editor/internal/draw"
	"golang.org/x/image/colornames"
)

type Style struct {
	Fonts                           []FontStyle
	TagFgColor                      Color
	TagBgColor                      Color
	TagPathBasenameColor            Color
	BodyFgColor                     Color
	BodyBgColor                     Color
	LayoutBoxFgColor                Color
	LayoutBoxUnsavedBgColor         Color
	LayoutBoxBgColor                Color
	ScrollFgColor                   Color
	ScrollBgColor                   Color
	ScrollBorderColor               Color
	GutterWidth                     unit.Dp
	WinBorderColor                  Color
	WinBorderWidth                  unit.Dp
	PrimarySelectionFgColor         Color
	PrimarySelectionBgColor         Color
	ExecutionSelectionFgColor       Color
	ExecutionSelectionBgColor       Color
	SecondarySelectionFgColor       Color
	SecondarySelectionBgColor       Color
	ErrorsTagFgColor                Color
	ErrorsTagBgColor                Color
	ErrorsTagPathBasenameColor      Color
	ErrorsTagFlashFgColor           Color
	ErrorsTagFlashBgColor           Color
	ErrorsTagFlashPathBasenameColor Color
	TabStopInterval                 unit.Dp
	TabStopPadding                  unit.Dp
	Syntax                          SyntaxStyle
	Ansi                            AnsiStyle
	LineSpacing                     unit.Dp
	TextLeftPadding                 unit.Dp
	TextRightPadding                unit.Dp
	TextBottomPadding               unit.Dp
	TrayFgColor                     Color
	TrayBgColor                     Color
	TrayInnerBorderColor            Color
	TrayInnerBorderWidth            unit.Dp
	TrayOuterBorderColor            Color
	TrayOuterBorderWidth            unit.Dp
	CursorProg                      string
	compiledCursorProg              []draw.Op
}

type FontStyle struct {
	FontName string
	FontSize unit.Sp
	FontFace text.FontFace `json:"-"` // Don't write the Font property to file.
}

type SyntaxStyle struct {
	KeywordColor      Color
	NameColor         Color
	StringColor       Color
	NumberColor       Color
	OperatorColor     Color
	CommentColor      Color
	PreprocessorColor Color
	HeadingColor      Color
	SubheadingColor   Color
	InsertedColor     Color
	DeletedColor      Color
}

type AnsiStyle struct {
	Colors [16]Color
}

func (as AnsiStyle) AsColors() [16]color.NRGBA {
	var c [16]color.NRGBA
	for i, v := range as.Colors {
		c[i] = color.NRGBA(v)
	}
	return c
}

func (s Style) tagEditableStyle() editableStyle {
	return editableStyle{
		Fonts:              s.Fonts,
		FgColor:            s.TagFgColor,
		LineSpacing:        s.LineSpacing,
		compiledCursorProg: s.compiledCursorProg,
		PrimarySelection: textStyle{
			FgColor: s.PrimarySelectionFgColor,
			BgColor: s.PrimarySelectionBgColor,
		},
		SecondarySelection: textStyle{
			FgColor: s.SecondarySelectionFgColor,
			BgColor: s.SecondarySelectionBgColor,
		},
		ExecutionSelection: textStyle{
			FgColor: s.ExecutionSelectionFgColor,
			BgColor: s.ExecutionSelectionBgColor,
		},
		TabStopInterval:   s.TabStopInterval,
		TextLeftPadding:   s.TextLeftPadding,
		TextRightPadding:  s.TextRightPadding,
		TextBottomPadding: s.TextBottomPadding,
	}
}

func (s Style) tagBlockStyle() blockStyle {
	return blockStyle{
		StandardBgColor:              color.NRGBA(s.TagBgColor),
		ErrorBgColor:                 color.NRGBA(s.ErrorsTagBgColor),
		ErrorFlashBgColor:            color.NRGBA(s.ErrorsTagFlashBgColor),
		StandardFgColor:              color.NRGBA(s.TagFgColor),
		ErrorFgColor:                 color.NRGBA(s.ErrorsTagFgColor),
		ErrorFlashFgColor:            color.NRGBA(s.ErrorsTagFlashFgColor),
		PathBasenameColor:            color.NRGBA(s.TagPathBasenameColor),
		ErrorsPathBasenameColor:      color.NRGBA(s.ErrorsTagPathBasenameColor),
		ErrorsFlashPathBasenameColor: color.NRGBA(s.ErrorsTagFlashPathBasenameColor),
	}
}

func (s Style) bodyBlockStyle() blockStyle {
	return blockStyle{
		StandardBgColor: color.NRGBA(s.BodyBgColor),
	}
}

func (s Style) bodyEditableStyle() editableStyle {
	return editableStyle{
		Fonts:              s.Fonts,
		FgColor:            s.BodyFgColor,
		LineSpacing:        s.LineSpacing,
		compiledCursorProg: s.compiledCursorProg,
		PrimarySelection: textStyle{
			FgColor: s.PrimarySelectionFgColor,
			BgColor: s.PrimarySelectionBgColor,
		},
		SecondarySelection: textStyle{
			FgColor: s.SecondarySelectionFgColor,
			BgColor: s.SecondarySelectionBgColor,
		},
		ExecutionSelection: textStyle{
			FgColor: s.ExecutionSelectionFgColor,
			BgColor: s.ExecutionSelectionBgColor,
		},
		TabStopInterval:   s.TabStopInterval,
		TabStopPadding:    s.TabStopPadding,
		TextLeftPadding:   s.TextLeftPadding,
		TextRightPadding:  s.TextRightPadding,
		TextBottomPadding: s.TextBottomPadding,
	}
}

func (s Style) layoutBoxStyle() layoutBoxStyle {
	return layoutBoxStyle{
		FgColor:        color.NRGBA(s.LayoutBoxFgColor),
		UnsavedBgColor: color.NRGBA(s.LayoutBoxUnsavedBgColor),
		BgColor:        color.NRGBA(s.LayoutBoxBgColor),
		GutterWidth:    s.GutterWidth,
		LineSpacing:    s.LineSpacing,
		Fonts:          s.Fonts,
	}
}

func (s Style) scrollbarStyle() scrollbarStyle {
	return scrollbarStyle{
		FgColor:     color.NRGBA(s.ScrollFgColor),
		BgColor:     color.NRGBA(s.ScrollBgColor),
		BorderColor: color.NRGBA(s.ScrollBorderColor),
		GutterWidth: s.GutterWidth,
		Fonts:       s.Fonts,
	}
}

func (s Style) trayStyle() floatStyle {
	fs := floatStyle{
		InnerBorderColor: s.TrayInnerBorderColor,
		InnerBorderWidth: s.TrayInnerBorderWidth,
		OuterBorderColor: s.TrayOuterBorderColor,
		OuterBorderWidth: s.TrayOuterBorderWidth,
		blockStyle:       s.tagBlockStyle(),
		editableStyle:    s.tagEditableStyle(),
	}

	fs.blockStyle.StandardBgColor = color.NRGBA(s.TrayBgColor)
	fs.blockStyle.StandardFgColor = color.NRGBA(s.TrayFgColor)
	fs.editableStyle.BgColor = s.TrayBgColor
	fs.editableStyle.FgColor = s.TrayFgColor

	return fs
}

func MustParseHexColor(s string) (c Color) {
	c, err := ParseHexColor(s)
	if err != nil {
		panic(err)
	}
	return c
}

func ParseHexColor(s string) (c Color, err error) {
	c.A = 0xff

	if s[0] != '#' {
		err = fmt.Errorf("Invalid hex color format when parsing '%s': does not begin with #", s)
		return
	}

	hexToByte := func(b byte) byte {
		switch {
		case b >= '0' && b <= '9':
			return b - '0'
		case b >= 'a' && b <= 'f':
			return b - 'a' + 10
		case b >= 'A' && b <= 'F':
			return b - 'A' + 10
		}
		err = fmt.Errorf("Invalid hex color format when parsing '%s': contains a character that is not 0-9, a-f or A-F", s)
		return 0
	}

	switch len(s) {
	case 7:
		c.R = hexToByte(s[1])<<4 + hexToByte(s[2])
		c.G = hexToByte(s[3])<<4 + hexToByte(s[4])
		c.B = hexToByte(s[5])<<4 + hexToByte(s[6])
	case 4:
		c.R = hexToByte(s[1]) * 17
		c.G = hexToByte(s[2]) * 17
		c.B = hexToByte(s[3]) * 17
	default:
		err = fmt.Errorf("Invalid hex color format when parsing '%s': length is not 4 or 7 bytes", s)
		return
	}
	return
}

func ReadStyle(path string, defaults *Style) (s Style, err error) {
	if defaults != nil {
		s = *defaults
	}

	file, e := os.Open(path)
	if e != nil {
		err = e
		return
	}
	defer file.Close()

	enc := json.NewDecoder(file)
	err = enc.Decode(&s)
	return
}

// WriteStyle writes the style to a file.
// Note that we omit marshalling the Font property because it is pretty big. However it would be interesting
// to be able to export the font to the file, modify it by hand and import it again.
func WriteStyle(path string, s Style) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	enc := json.NewEncoder(file)
	enc.SetIndent("", "  ")
	return enc.Encode(s)
}

type Color color.NRGBA

func (c Color) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("\"#%02x%02x%02x\"", c.R, c.G, c.B)), nil
}

func (c *Color) UnmarshalJSON(b []byte) error {
	s := string(b)
	if b[0] != '"' || b[len(s)-1] != '"' {
		return fmt.Errorf("Invalid hex color format when unmarshalling JSON color '%s': color should be a string value (in double-quotes)", s)
	}
	col, err := ParseHexColor(string(b[1 : len(b)-1]))
	if err != nil {
		return err
	}
	*c = col
	return nil
}

func ColorFromName(name string) (c Color, ok bool) {
	name = strings.ToLower(name)

	col, ok := colornames.Map[name]
	if !ok {
		return
	}
	return Color{R: col.R, G: col.G, B: col.B, A: col.A}, true
}

func (c Color) String() string {
	return fmt.Sprintf("\"#%02x%02x%02x\"", c.R, c.G, c.B)
}

// ParseTint parses a special formatted foreground and background color descriptor.
// Tints have the form 'FG/BG' where one of FG or BG or both must be present. FG or
// BG can be either a color name or an RGB color code of the form #RRGGBB
func ParseTint(tint string) (fg, bg Color, err error) {
	parseColor := func(s string) (Color, error) {
		color, ok := ColorFromName(s)
		if ok {
			return color, nil
		}

		color, err := ParseHexColor(s)
		return color, err
	}

	var err1, err2 error
	i := strings.Index(tint, "/")
	if i < 0 {
		fg, err1 = parseColor(tint)
	} else {
		if tint[0] == '/' {
			bg, err1 = parseColor(tint[1:])
		} else {
			parts := strings.Split(tint, "/")
			fg, err1 = parseColor(parts[0])
			bg, err2 = parseColor(parts[1])
		}
	}

	if err1 != nil {
		err = err1
	}

	if err2 != nil {
		err = err2
	}

	return
}

func ToHexColor(c Color) string {
	return fmt.Sprintf("#%x%x%x", c.R, c.G, c.B)
}
