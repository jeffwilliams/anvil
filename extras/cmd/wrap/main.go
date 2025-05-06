/*
Wrap: wrap lines of text to some width. Read all of stdin, strip out newlines, and then relayout the text.
*/
package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"unicode"
	_ "embed"

	"github.com/speedata/hyphenation"
)


// Hyphenation file, taken from https://ctan.math.utah.edu/ctan/tex-archive/language/hyph-utf8/tex/generic/hyph-utf8/patterns/txt/
//go:embed hyph-en-gb.pat.txt
var hyphenPatternFile []byte

func main() {
	width := 80
	if len(os.Args) > 1 {
		var err error
		width, err = strconv.Atoi(os.Args[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: that width is not a valid number: %v\n", err)
			os.Exit(1)
		}
	}

	data, err := ioutil.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	runes := []rune(string(data))

	wrap, err := NewWrapper(width)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating wrapper: %v\n", err)
		os.Exit(1)
	}

	wrap.wrap(runes)
}

type wrapper struct {
	lineLen int
	word    Buffer
	width   int
	h       *hyphenation.Lang
}

func NewWrapper(width int) (*wrapper, error) {
	r := bytes.NewBuffer(hyphenPatternFile)

	h, err := hyphenation.New(r)
	if err != nil {
		return nil, err
	}

	w := &wrapper{
		width: width,
		h:     h,
	}

	return w, nil
}

func (w wrapper) wrap(runes []rune) {
	for _, r := range runes {
		if unicode.IsSpace(r) {
			w.spaceEncountered(r)
			continue
		}

		w.word.WriteRune(r)
	}
	
     w.outputWord()
	fmt.Printf("\n")
}

func (w *wrapper) spaceEncountered(r rune) {
	if w.word.RuneLen() > 0 {
		w.outputWord()
		if r == '\n' {
			fmt.Printf(" ")
			w.lineLen++
		}
	}
	
	if w.anotherRuneTooLong() {
		w.newline()
		return
	}

	if r != '\n' {
		fmt.Printf("%c", r)
		w.lineLen++
	}
}

func (w *wrapper) newline() {
	fmt.Printf("\n")
	w.lineLen = 0
}

func (w *wrapper) outputWord() {
	for {
		if w.word.RuneLen() == 0 {
			return
		}

		if !w.wordTooLong() {
			os.Stdout.Write(w.word.Bytes())
			w.lineLen += w.word.RuneLen()
			w.word.Reset()
			return
		}

		w.outputLongWord()
	}
}

func (w *wrapper) outputLongWord() {
	s := w.word.String()
	//fmt.Printf("gotta output %s\n", s)
	breaks := w.hyphenate(s)
	for i := len(breaks) - 1; i >= 0; i-- {
		if w.lineLen+breaks[i] < w.width {
			// break here
			w.breakAndOutputWord(s, breaks[i])
			return
		}
	}
	
	fmt.Printf("\n%s", s)
	w.lineLen = w.word.RuneLen()
	w.word.Reset()
}

func (w *wrapper) breakAndOutputWord(s string, index int) {
	r := []rune(s)
	a, b := split(r, index)
	fmt.Printf("%s-", a)
	w.newline()
	w.word.Reset()
	w.word.WriteRunes(b)
}

func (w wrapper) hyphenate(s string) []int {
	breaks := w.h.Hyphenate(s)
	// Don't break at positions <= 1
	for len(breaks) > 0 && breaks[0] <= 1 {
		breaks = breaks[1:]	
	}
	return breaks
}

func split(runes []rune, index int) (string, []rune) {
	return string(runes[:index]), runes[index:]
}

func (w wrapper) wordTooLong() bool {
	return w.lineLen+w.word.RuneLen() > w.width
}

func (w wrapper) anotherRuneTooLong() bool {
	return w.lineLen == w.width
}

type Buffer struct {
	bytes.Buffer
	runeLen int
}

func (b *Buffer) Reset() {
	b.Buffer.Reset()
	b.runeLen = 0	
}

func (b *Buffer) WriteRune(r rune) {
	b.Buffer.WriteRune(r)
	b.runeLen++		
}

func (b *Buffer) WriteRunes(r []rune) {
	for _, rn := range r {
		b.WriteRune(rn)
	}
}

func (b *Buffer) RuneLen() int {
	return b.runeLen
}

