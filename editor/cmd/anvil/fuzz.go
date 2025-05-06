package main

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/jeffwilliams/anvil/editor/internal/fuzzy"
)

type FuzzySearcher struct {
	win      *Window
	tag      *Tag
	body     *Body
	keyword  string
	lastTerm string
}

func NewFuzzySearcher(win *Window, tag *Tag, body *Body) *FuzzySearcher {
	s := &FuzzySearcher{
		tag:     tag,
		win:     win,
		body:    body,
		keyword: "◊Fuzz ",
	}

	tag.AddTextChangeListener(s.tagTextChanged)

	return s
}

func (f *FuzzySearcher) tagTextChanged(ch *TextChange) {
	_, _, userArea, err := f.tag.Parts()
	if err != nil {
		return
	}

	// Search backwards for ◊Fuzz
	l := len(f.keyword)
	i := strings.LastIndex(userArea, f.keyword)

	if i < 0 {
		return
	}

	end := len(userArea)
	j := strings.Index(userArea[i+1:], "◊")
	if j > 0 {
		end = j + i + 1
	}

	term := userArea[i+l : end]

	if term == f.lastTerm {
		return
	}
	f.lastTerm = term

	terms := strings.Fields(term)
	f.search(terms)

}

/*
search performs a fuzzy search in the lines of the window body. The window's text is split into lines,
then the lines are ranked using the fuzzy search library. For each line, each term is ranked against the
line and the ranks are summed, Lower ranks mean the line matches better. If there is no match for a term
then that term's rank is 1000. If no terms match then that line is not shown.

The ranked lines are then sorted and shown in a +Live window.
*/
func (f *FuzzySearcher) search(terms []string) {
	log(LogCatgFuzzy, "Fuzzy search for %d terms: %v\n", len(terms), terms)

	win := f.findLiveWindow()
	if len(terms) == 0 {
		if win != nil {
			win.Body.SetText([]byte{})
		}
		return
	}

	win = f.findOrCreateLiveWindow()
	if win == nil {
		return
	}

	// TODO: might be more efficient to just store indexes into the doc here instead of new strings.
	//lines := []string{}
	lines := f.getLines()

	f.rankLines(terms, lines)

	c := f.buildLiveWindowContents(lines)
	win.Body.SetText(c)

	editor.SetOnlyFlashedWindow(win)
	win.GrowIfBodyTooSmall()
}

func (f *FuzzySearcher) getLines() (lines []rankedline) {
	if f.win.fileType == typeDir {
		lines = f.getLinesDelim([]byte{'\n', '\t'})
		for i := range lines {
			lines[i].lineno = 0
		}
		return
	} else {
		return f.getLinesDelim([]byte{'\n'})
	}
}

func (f *FuzzySearcher) getLinesDelim(delims []byte) (lines []rankedline) {
	text := f.body.Bytes()
	lstart := 0
	lend := 0
	lineno := 1

	isDelim := func(r byte) bool {
		for _, d := range delims {
			if r == d {
				return true
			}
		}
		return false
	}

	for lend < len(text) {
		if isDelim(text[lend]) {
			line := string(text[lstart:lend])
			lines = append(lines, rankedline{line: line, lineno: lineno})

			lend++
			lineno++
			for lend < len(text) && isDelim(text[lend]) {
				lend++
				lineno++
			}
			lstart = lend
			continue
		}
		lend++
	}

	if lstart < len(text)-1 && len(text) > 0 {
		line := string(text[lstart:len(text)])
		lines = append(lines, rankedline{line: line, lineno: lineno})
	}
	return
}

func (f *FuzzySearcher) findLiveWindow() *Window {
	dir := f.tag.adapter.dir()
	name := fmt.Sprintf("%s+Live", dir)
	return editor.FindWindowForFileAndDisplay(name)
}

func (f *FuzzySearcher) findOrCreateLiveWindow() *Window {
	dir := f.tag.adapter.dir()

	name := fmt.Sprintf("%s+Live", dir)
	w := editor.FindOrCreateWindow(name)
	return w
}

func (f *FuzzySearcher) rankLines(terms []string, lines []rankedline) {
	f.rankLinesUsingSellers(terms, lines)
}

func (f *FuzzySearcher) rankLinesUsingSellers(terms []string, lines []rankedline) {
	for i, l := range lines {
		score := fuzzy.CalcScore(terms, l.line, fuzzy.CaseInsensitive)
		lines[i].rank = int(score.Score * 1000)
	}

	sort.Slice(lines, func(i, j int) bool {
		if lines[i].rank > lines[j].rank {
			return true
		} else if lines[i].rank < lines[j].rank {
			return false
		} else {
			return lines[i].line < lines[j].line
		}
	})
}

type rankedline struct {
	rank   int
	line   string
	lineno int
}

func (f *FuzzySearcher) runesInOneAreInTwo(one, two string) bool {
	for _, r := range one {
		if !strings.ContainsRune(two, r) {
			return false
		}
	}
	return true
}

func (f *FuzzySearcher) buildLiveWindowContents(lines []rankedline) []byte {
	base := f.win.displayPath.Base()

	var buf bytes.Buffer
	for _, l := range lines {
		if l.rank <= 0 {
			continue
		}
		buf.WriteString(l.line)
		if l.lineno != 0 {
			fmt.Fprintf(&buf, "\t%s:%d", base, l.lineno)
		}
		buf.WriteRune('\n')
	}

	return buf.Bytes()
}
