package runes

import (
	"fmt"
	"unicode"
	"unicode/utf8"
)

type Walker struct {
	bytes   []byte
	bytePos int
	runePos int
}

func NewWalker(b []byte) Walker {
	return Walker{
		bytes: b,
	}
}

func (r *Walker) Forward(n int) {
	for ; n > 0 && r.bytePos < len(r.bytes); n-- {
		r.forward1()
	}
}

func (r *Walker) forward1() (rn rune, eof bool) {
	if r.bytePos >= len(r.bytes) {
		eof = true
		return
	}

	size := 0
	rn, size = utf8.DecodeRune(r.bytes[r.bytePos:])
	if rn == utf8.RuneError {
		// In order to be more robust when reading files with invalid encodings,
		// try and make _some_ progress.
		size = 1
	}
	r.bytePos += size
	r.runePos++
	return
}

func (r *Walker) ForwardBytes(n int) {
	p := r.bytePos
	for r.bytePos-p < n {
		r.forward1()
	}
}

func (r *Walker) Backward(n int) {

	if r.bytePos > len(r.bytes) {
		r.bytePos = len(r.bytes)
	}
	for ; n > 0 && r.bytePos > 0; n-- {
		r.backward1()
	}
}

func (r *Walker) backward1() (rn rune, eof bool) {
	if r.bytePos <= 0 {
		eof = true
		return
	}

	size := 0
	rn, size = utf8.DecodeLastRune(r.bytes[:r.bytePos])
	if rn == utf8.RuneError {
		// In order to be more robust when reading files with invalid encodings,
		// try and make _some_ progress.
		size = 1
	}
	r.bytePos -= size
	r.runePos--
	return
}

func (r *Walker) SetBytePos(p int) {
	if p < 0 {
		p = 0
	}
	if p > len(r.bytes) {
		p = len(r.bytes)
	}

	r.bytePos = 0
	r.runePos = 0
	for r.bytePos < p {
		r.Forward(1)
	}
}

func (r *Walker) SetRunePos(p int) {
	r.bytePos = 0
	r.runePos = 0
	r.Forward(p)
}

// If an invalid UTF-8 sequence is encountered, error will be set, but this function will continue
// processing the text regardless so the result should still be usable.
func (r *Walker) SetRunePosCache(p int, cache *OffsetCache) error {
	byteOffset, err, runeCount := cache.Get(r.bytes, p)

	r.bytePos = byteOffset
	r.runePos = runeCount
	return err
}

func (r *Walker) AtEnd() bool {
	return r.bytePos >= len(r.bytes)
}

func (r *Walker) AtStart() bool {
	return r.bytePos <= 0
}

func (r *Walker) BytePos() int {
	return r.bytePos
}

func (r *Walker) RunePos() int {
	return r.runePos
}

func (r *Walker) Rune() rune {
	if r.AtEnd() {
		return 0
	}
	rn, _ := utf8.DecodeRune(r.bytes[r.bytePos:])
	return rn
}

// CurrentWord returns the word surrounding the current position. A Word is a space-separated string of characters.
func (r *Walker) CurrentWord() string {
	right, _ := r.rightWordBoundary()
	left, _ := r.leftWordBoundary()
	return string(r.bytes[left:right])
}

func (r *Walker) rightWordBoundary() (byteIndex, runeIndex int) {
	return r.rightBoundary(unicode.IsSpace)
}

func (r *Walker) leftWordBoundary() (byteIndex, runeIndex int) {
	return r.leftBoundary(unicode.IsSpace)
}

func (r *Walker) leftBoundary(stop func(rn rune) bool) (byteIndex, runeIndex int) {
	byteIndex = r.bytePos
	runeIndex = r.runePos
	for byteIndex > 0 {
		rn, size := utf8.DecodeLastRune(r.bytes[:byteIndex])
		if stop(rn) {
			break
		}
		byteIndex -= size
		runeIndex--
	}
	return
}

func (r *Walker) rightBoundary(stop func(rn rune) bool) (byteIndex, runeIndex int) {
	byteIndex = r.bytePos
	runeIndex = r.runePos
	for byteIndex < len(r.bytes) {
		rn, size := utf8.DecodeRune(r.bytes[byteIndex:])
		if stop(rn) {
			break
		}
		byteIndex += size
		runeIndex++
	}
	return
}

func (r *Walker) CurrentWordBounds() (startRuneIndex, endRuneIndex int) {
	_, startRuneIndex = r.leftWordBoundary()
	_, endRuneIndex = r.rightWordBoundary()
	return
}

func (r *Walker) CurrentIdentifier() string {
	right, _ := r.rightIdentifierBoundary()
	left, _ := r.leftIdentifierBoundary()
	return string(r.bytes[left:right])
}

func (r *Walker) rightIdentifierBoundary() (byteIndex, runeIndex int) {
	return r.rightBoundary(func(rn rune) bool {
		return !isValidIdentifierRune(rn)
	})
}

func isValidIdentifierRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

func (r *Walker) leftIdentifierBoundary() (byteIndex, runeIndex int) {
	return r.leftBoundary(func(rn rune) bool {
		return !isValidIdentifierRune(rn)
	})
}

func (r *Walker) CurrentIdentifierBounds() (startRuneIndex, endRuneIndex int) {
	_, startRuneIndex = r.leftIdentifierBoundary()
	_, endRuneIndex = r.rightIdentifierBoundary()
	return
}

func (r *Walker) CurrentRunOfSpaces() string {
	right, _ := r.rightRunOfSpacesBoundary()
	left, _ := r.leftRunOfSpacesBoundary()
	return string(r.bytes[left:right])
}

func (r *Walker) rightRunOfSpacesBoundary() (byteIndex, runeIndex int) {
	return r.rightBoundary(func(rn rune) bool {
		return !unicode.IsSpace(rn) || rn == '\n'
	})
}

func (r *Walker) leftRunOfSpacesBoundary() (byteIndex, runeIndex int) {
	return r.leftBoundary(func(rn rune) bool {
		return !unicode.IsSpace(rn) || rn == '\n'
	})
}

func (r *Walker) CurrentRunOfSpacesBounds() (startRuneIndex, endRuneIndex int) {
	_, startRuneIndex = r.leftRunOfSpacesBoundary()
	_, endRuneIndex = r.rightRunOfSpacesBoundary()
	return
}

// CurrentLozengeDelimitedStringInLine returns the string in the line at the current position
// that is delimited by the lozenge character (◊) on the right and left. If the current position
// is not delimited by lozenges, isDelimited is false on return.
func (r *Walker) CurrentLozengeDelimitedStringInLine() (str string, isDelimited bool) {
	right, _ := r.rightRunUntilLozengeBoundary()
	left, _ := r.leftRunUntilLozengeBoundary()

	if left == 0 || r.runeAt(left) == '\n' || right == len(r.bytes) || r.runeAt(right) == '\n' {
		return
	}

	isDelimited = true
	str = string(r.bytes[left:right])

	return
}

func (r *Walker) runeAt(byteIndex int) rune {
	if byteIndex < 0 || byteIndex >= len(r.bytes) {
		return 0
	}
	rn, _ := utf8.DecodeRune(r.bytes[byteIndex:])
	return rn
}

func (r *Walker) rightRunUntilLozengeBoundary() (byteIndex, runeIndex int) {
	return r.rightBoundary(func(rn rune) bool {
		return rn == '◊' || rn == '\n'
	})
}

func (r *Walker) leftRunUntilLozengeBoundary() (byteIndex, runeIndex int) {
	return r.leftBoundary(func(rn rune) bool {
		return rn == '◊' || rn == '\n'
	})
}

func (r *Walker) ForwardToEndOfLine() {
	r.bytePos, r.runePos = r.rightEolBoundary()
}

func (r *Walker) rightEolBoundary() (byteIndex, runeIndex int) {
	return r.rightEolBoundaryExt(WithoutTrailingNewline)
}

func (r *Walker) atEnd() bool {
	return r.bytePos >= len(r.bytes)
}

type NewlineBehaviour int

const (
	WithoutTrailingNewline NewlineBehaviour = iota
	WithTrailingNewline
)

func (r *Walker) rightEolBoundaryExt(nl NewlineBehaviour) (byteIndex, runeIndex int) {
	byteIndex = r.bytePos
	runeIndex = r.runePos
	for byteIndex < len(r.bytes) {
		rn, size := utf8.DecodeRune(r.bytes[byteIndex:])
		if rn == '\n' && nl == WithoutTrailingNewline {
			break
		}
		byteIndex += size
		runeIndex++
		if rn == '\n' && nl == WithTrailingNewline {
			break
		}
	}
	return
}

func (r *Walker) BackwardToStartOfLine() {
	r.bytePos, r.runePos = r.leftEolBoundary()
}

func (r *Walker) leftEolBoundary() (byteIndex, runeIndex int) {
	byteIndex = r.bytePos
	runeIndex = r.runePos
	for byteIndex > 0 {
		rn, size := utf8.DecodeLastRune(r.bytes[:byteIndex])
		if rn == '\n' {
			break
		}
		byteIndex -= size
		runeIndex--
	}
	return
}

func (r *Walker) ForwardToStartOfNextWord() {
	start := r.runePos
	for {
		rn, eof := r.forward1()
		if eof {
			return
		}

		if unicode.IsSpace(rn) {
			break
		}
	}

	for {
		rn, eof := r.forward1()
		if eof {
			return
		}

		if !unicode.IsSpace(rn) {
			if r.runePos > start {
				r.backward1()
			}
			break
		}
	}
	return
}

func (r *Walker) BackwardToWordStart() {
	start := r.runePos
	rn, eof := r.backward1()
	if eof {
		return
	}

	if unicode.IsSpace(rn) {
		for {
			rn, eof := r.backward1()
			if eof {
				return
			}

			if !unicode.IsSpace(rn) {
				break
			}
		}
	}

	for {
		rn, eof := r.backward1()
		if eof {
			return
		}

		if unicode.IsSpace(rn) {
			if r.runePos < start {
				r.forward1()
			}
			break
		}
	}

	return
}

// CurrentLineBounds returns the start and end of the current line, not including any trailing newline
func (r *Walker) CurrentLineBounds() (startRuneIndex, endRuneIndex int) {
	_, startRuneIndex = r.leftEolBoundary()
	_, endRuneIndex = r.rightEolBoundary()
	return
}

// CurrentLineBounds returns the start and end of the current line, with any trailing newline
func (r *Walker) CurrentLineBoundsIncludingNl() (startRuneIndex, endRuneIndex int) {
	_, startRuneIndex = r.leftEolBoundary()
	_, endRuneIndex = r.rightEolBoundaryExt(WithTrailingNewline)
	return
}

func (r *Walker) TextBetweenRuneIndices(start, end int) []byte {

	bytePos := 0
	size := 0
	for bytePos < len(r.bytes) && start > 0 {
		_, size = utf8.DecodeRune(r.bytes[bytePos:])
		bytePos += size
		start--
		end--
	}

	endBytePos := bytePos
	for endBytePos < len(r.bytes) && end > 0 {
		_, size = utf8.DecodeRune(r.bytes[endBytePos:])
		endBytePos += size
		end--
	}

	return r.bytes[bytePos:endBytePos]

}

func (r *Walker) TextBetweenRuneIndicesCache(start, end int, cache *OffsetCache) []byte {
	bytePos := 0
	size := 0
	bytePos, err, runeCount := cache.Get(r.bytes, start)
	if err == nil {
		end -= runeCount
	}
	if err != nil {
		// Find it the hard way
		bytePos = 0
		for bytePos < len(r.bytes) && start > 0 {
			_, size = utf8.DecodeRune(r.bytes[bytePos:])
			bytePos += size
			start--
			end--
		}
	}

	endBytePos := bytePos
	for endBytePos < len(r.bytes) && end > 0 {
		_, size = utf8.DecodeRune(r.bytes[endBytePos:])
		endBytePos += size
		end--
	}

	return r.bytes[bytePos:endBytePos]

}

// IndexInLine returns the index in runes of the position in the current line. Zero is the first index.
func (r *Walker) IndexInLine() int {
	i := r.runePos
	_, j := r.leftEolBoundary()
	return i - j
}

func (r *Walker) LineLen() int {
	_, i := r.leftEolBoundary()
	_, j := r.rightEolBoundary()
	return j - i
}

func (r *Walker) GoToStart() {
	r.bytePos = 0
	r.runePos = 0
}

func (r *Walker) GoToEnd() {
	p := len(r.bytes)
	r.SetBytePos(p)
}

// The line and column are 1 based.
func (r *Walker) GoToLineAndCol(line, col int) {
	r.GoToStart()

	for line = line - 1; line > 0; line-- {
		r.ForwardToEndOfLine()
		r.Forward(1)
	}

	// Make sure line is long enough to move to that column
	start, end := r.CurrentLineBounds()
	length := end - start
	if col > length {
		return
	}
	//func (r *RuneWalker) CurrentLineBounds() (startRuneIndex, endRuneIndex int) {

	r.Forward(col - 1)
}

func (r *Walker) ForwardLines(n int) (eof bool) {
	for ; n > 0 && r.bytePos < len(r.bytes); n-- {
		eof = r.ForwardLine()
		if eof {
			return
		}
	}
	return
}

func (r *Walker) ForwardLine() (eof bool) {
	if r.bytePos >= len(r.bytes) {
		eof = true
		return
	}

	var rn rune
	for {
		rn, eof = r.forward1()
		if eof {
			return
		}

		if rn == '\n' {
			return
		}
	}
}

func (w *Walker) prevRune() (r rune) {
	if w.bytePos > 0 {
		r, _ = utf8.DecodeLastRune(w.bytes[:w.bytePos])
	}
	return
}

func (r *Walker) IsInRunOfSpaces() bool {
	// Is the current rune a space (or we are at the end); and the one before is a space (or at the beginning)
	prevRn := ' '
	if r.bytePos > 0 {
		prevRn = r.prevRune()
	}

	if !unicode.IsSpace(prevRn) {
		return false
	}

	curRn := ' '
	if !r.AtEnd() {
		curRn = r.Rune()
	}

	if !unicode.IsSpace(curRn) {
		return false
	}

	return true
}

func (r *Walker) IsAtBracket() bool {
	if !r.AtEnd() {
		if IsABracket(r.Rune()) {
			return true
		}
	}
	return false
}

func (r *Walker) TextWithinBracketsBounds() (startRuneIndex, endRuneIndex int, err error) {
	const (
		forward = iota
		backwards
	)

	rn := r.Rune()
	opener, closer := MatchingBracket(rn)
	var dir int

	switch rn {
	case '{':
		dir = forward
	case '[':
		dir = forward
	case '(':
		dir = forward
	case '<':
		dir = forward
	case '}':
		dir = backwards
	case ']':
		dir = backwards
	case ')':
		dir = backwards
	case '>':
		dir = backwards
	}

	w := *r

	nesting := 1

	startRuneIndex = w.RunePos()

	for {
		if dir == forward {
			w.Forward(1)
			if w.AtEnd() {
				err = fmt.Errorf("Reached end of runes without finding matching bracket")
				return
			}
		} else {
			w.Backward(1)
			if w.AtStart() {
				err = fmt.Errorf("Reached start of runes without finding matching bracket")
				return
			}
		}

		if w.Rune() == opener {
			nesting++
		} else if w.Rune() == closer {
			nesting--
		}
		if nesting == 0 {
			// Done!
			endRuneIndex = w.RunePos()
			break
		}
	}

	if endRuneIndex < startRuneIndex {
		startRuneIndex, endRuneIndex = endRuneIndex, startRuneIndex
	}
	startRuneIndex++

	return
}

func (r *Walker) IsInRunOfSymbols() bool {
	prevRn := '*'
	if r.bytePos > 0 {
		prevRn = r.prevRune()
	}

	if unicode.IsSpace(prevRn) || isValidIdentifierRune(prevRn) {
		return false
	}

	curRn := '*'
	if !r.AtEnd() {
		curRn = r.Rune()
	}

	if unicode.IsSpace(curRn) || isValidIdentifierRune(curRn) {
		return false
	}

	return true
}

func (r *Walker) CurrentRunOfSymbols() string {
	right, _ := r.rightRunOfSymbolsBoundary()
	left, _ := r.leftRunOfSymbolsBoundary()
	return string(r.bytes[left:right])
}

func (r *Walker) rightRunOfSymbolsBoundary() (byteIndex, runeIndex int) {
	return r.rightBoundary(func(rn rune) bool {
		return unicode.IsSpace(rn) || rn == '\n' || isValidIdentifierRune(rn)
	})
}

func (r *Walker) leftRunOfSymbolsBoundary() (byteIndex, runeIndex int) {
	return r.leftBoundary(func(rn rune) bool {
		return unicode.IsSpace(rn) || rn == '\n' || isValidIdentifierRune(rn)
	})
}

func (r *Walker) CurrentRunOfSymbolsBounds() (startRuneIndex, endRuneIndex int) {
	_, startRuneIndex = r.leftRunOfSymbolsBoundary()
	_, endRuneIndex = r.rightRunOfSymbolsBoundary()
	return
}

func (r *Walker) IsAtQuote() bool {
	rn := r.Rune()
	return rn == '"' || rn == '\'' || rn == '`' || rn == '◊'
}

func (r *Walker) TextWithinQuotesInCurrentLine() (startRuneIndex, endRuneIndex int, err error) {

	// If the current character is not a quote character (" or ') then return an error.
	// Otherwise, search left and right in the line for a matching quote rune. If there are matches
	// in both directions, fail. Otherwise return the matching one.

	if !r.IsAtQuote() {
		err = fmt.Errorf("Not starting on a quote")
		return
	}

	qt := r.Rune()

	w := *r
	backIndex := -1
	for {
		w.Backward(1)
		if w.AtStart() || w.Rune() == '\n' {
			break
		}
		if w.Rune() == qt {
			backIndex = w.RunePos()
			break
		}
	}

	w = *r
	forwardIndex := -1
	for {
		w.Forward(1)
		if w.AtEnd() || w.Rune() == '\n' {
			break
		}
		if w.Rune() == qt {
			forwardIndex = w.RunePos()
			break
		}
	}

	if forwardIndex != -1 {
		return r.RunePos() + 1, forwardIndex, nil
	}

	return backIndex + 1, r.RunePos(), nil
}

func (r *Walker) IsAtStartOfLine() bool {
	if r.AtEnd() {
		return false
	}

	if r.AtStart() {
		return true
	}

	return r.prevRune() == '\n'
}

type Pos struct {
	bytePos int
	runePos int
}

func (r Walker) Position() Pos {
	return Pos{r.bytePos, r.runePos}
}

func (r *Walker) SetPosition(p Pos) {
	r.bytePos = p.bytePos
	r.runePos = p.runePos
}
