package main

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

type Tag struct {
	blockEditable
	flash bool
}

func (t *Tag) Init(body *Body, style blockStyle, editableStyle editableStyle, executor *CommandExecutor, owner interface{}, scheduler *Scheduler) {
	t.blockEditable.Init(style, editableStyle, scheduler)
	t.executeOn = &t.editable
	if body != nil {
		t.executeOn = &body.editable
	}
	t.PreventScrolling = true
	t.SetAdapter(&editableAdapter{
		executor: executor,
		owner:    owner,
	})
	t.AddTextChangeListener(t.highlightBasenameOnTextChange)
	t.blockEditable.editable.shouldOmitWinPathWhenAcquiring = t.shouldOmitWinPathWhenAcquiring
}

func (t Tag) Parts() (path, editorArea, userArea string, err error) {
	parts, _, err := t.calcParts()
	s := t.String()
	path = s[parts.path[0]:parts.path[1]]
	editorArea = s[parts.editorArea[0]:parts.editorArea[1]]
	userArea = s[parts.userArea[0]:parts.userArea[1]]

	return
}

func (t *Tag) Set(path, editorArea, userArea string) {

	savedCursors := t.saveCursorsAndSelections()

	pathLen := utf8.RuneCountInString(path)
	editorAreaLen := utf8.RuneCountInString(editorArea)

	t.SetTextString(fmt.Sprintf("%s%s%s", path, editorArea, userArea))

	t.immutableRange.start = pathLen
	t.immutableRange.end = editorAreaLen + pathLen

	t.setFgAndBgColors(path)
	t.ClearManualHighlights()
	t.highlightBasename(path)

	// Setting the text using SetTextString resets the cursor positions,
	// so we save and restore them
	t.restoreCursorsAndSelections(savedCursors)
}

func (t *Tag) setFgAndBgColors(path string) {
	defer t.blockEditable.editable.invalidateLayedoutText()

	useErrorWinColors := IsErrorsWindow(path) || IsLiveWindow(path)
	permittedToUseFlashColors := useErrorWinColors || BasenameLikelyStartsWith(path, '+')

	if permittedToUseFlashColors && t.flash {
		t.blockEditable.bgcolor = t.blockEditable.style.ErrorFlashBgColor
		t.blockEditable.editable.style.FgColor = Color(t.blockEditable.style.ErrorFlashFgColor)
		return	
	}

	if useErrorWinColors {
		t.blockEditable.bgcolor = t.blockEditable.style.ErrorBgColor
		t.blockEditable.editable.style.FgColor = Color(t.blockEditable.style.ErrorFgColor)
	} else {
		t.blockEditable.bgcolor = t.blockEditable.style.StandardBgColor
		t.blockEditable.editable.style.FgColor = Color(t.blockEditable.style.StandardFgColor)
	}
}

func BasenameLikelyStartsWith(path string, r rune) bool {
	// This function doesn't handle the case of filenames on Unix that contain a backslash properly. Hence the "Likely"

	if len(path) == 0 {
		return false
	}

	i := strings.LastIndexAny(path, "\\/")
	if i < 0 {
		return rune(path[0]) == r
	}

	if i == len(path)-1 {
		return false
	}

	return rune(path[i+1]) == r
}

func (t *Tag) pathBasenameColor(path string) Color {
	if IsErrorsWindow(path) || IsLiveWindow(path) {
		if t.flash {
			return Color(t.blockEditable.style.ErrorsFlashPathBasenameColor)
		} else {
			return Color(t.blockEditable.style.ErrorsPathBasenameColor)
		}
	} else {
		return Color(t.blockEditable.style.PathBasenameColor)
	}
}

type tagParts struct {
	path       section
	editorArea section
	userArea   section
}

type section [2]int

func (sec section) Section(s string) string {
	return s[sec[0]:sec[1]]
}

func (t Tag) calcParts() (inBytes tagParts, inRunes tagParts, err error) {
	s := t.String()

	return calculateTagParts(s)
}

func calculateTagParts(tag string) (inBytes tagParts, inRunes tagParts, err error) {
	if tag == "" {
		return
	}

	i := strings.IndexRune(tag, '|')
	if i < 0 {
		err = fmt.Errorf("Tag does not contain |")
		return
	}
	inBytes.userArea[0] = i + 1
	inBytes.userArea[1] = len(tag)

	j := strings.LastIndex(tag[:i], " Del")
	if j < 0 {
		err = fmt.Errorf("Tag does not contain ' Del'")
		return
	}

	inBytes.editorArea[0] = j
	inBytes.editorArea[1] = i + 1

	inBytes.path[0] = 0
	inBytes.path[1] = j

	part := inBytes.path.Section(tag)
	inRunes.path[1] = utf8.RuneCountInString(part)

	part = inBytes.editorArea.Section(tag)
	inRunes.editorArea[0] = inRunes.path[1]
	inRunes.editorArea[1] = utf8.RuneCountInString(part) + inRunes.editorArea[0]

	part = inBytes.userArea.Section(tag)
	inRunes.userArea[0] = inRunes.editorArea[1]
	inRunes.userArea[1] = utf8.RuneCountInString(part) + inRunes.userArea[0]

	return
}

func (t *Tag) SetStyle(style blockStyle, editableStyle editableStyle) {
	t.blockEditable.SetStyle(style, editableStyle)
	path, _, _, _ := t.Parts()
	t.setFgAndBgColors(path)
}

func (t *Tag) SetFlash(b bool) {
	t.flash = b
	path, _, _, _ := t.Parts()
	t.setFgAndBgColors(path)
	t.highlightBasename(path)
}

func (t *Tag) highlightBasename(path string) {
	t.ClearManualHighlights()
	pathLen := utf8.RuneCountInString(path)

	g := NewGlobalPath(path, GlobalPathUnknown)
	b := g.Base()
	baseLen := utf8.RuneCountInString(b)

	start := pathLen - baseLen
	if strings.HasSuffix(path, "/") || strings.HasSuffix(path, "\\") {
		start--
	}
	end := t.immutableRange.start + 1
	log(LogCatgEd, "Tag.highlightBasename: highlighting basename of path %s, between %d and %d\n", path, start, end)

	color := t.pathBasenameColor(path)
	t.AddManualHighlight(start, end, color)
}

func (t *Tag) highlightBasenameOnTextChange(ch *TextChange) {
	path, _, _, err := t.Parts()
	if err != nil {
		return
	}

	t.highlightBasename(path)
}

func (t *Tag) shouldOmitWinPathWhenAcquiring(r *textRange) bool {
	// Specifically if we are acquiring part of the path of the window then behave specially
	_, parts, err := t.calcParts()
	if err != nil {
		return false
	}

	p := NewTextRange(parts.path[0], parts.path[1])

	if p.Contains(r) {
		log(LogCatgEd, "Tag.shouldOmitWinPathWhenAcquiring: The text range for the path of the window contains the text being acquired")
		return true
	}

	return false
}
