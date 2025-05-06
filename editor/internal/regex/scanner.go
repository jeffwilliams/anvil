package regex

import (
	"fmt"
)

type scanner struct {
	pos    int
	input  []rune
	tokens []token
	errs   []error
	tok    token
}

type token struct {
	typ   tokenType
	value []rune
	// pos is the index of the rune in the input
	// where the token started
	pos int
}

func (t token) tokenType() tokenType {
	return t.typ
}

func (t token) Len() int {
	if t.value == nil {
		return 0
	}
	return len(t.value)
}

func (t token) String() string {
	if t.value == nil || len(t.value) == 0 {
		return fmt.Sprintf("(%s)", t.typ)
	} else {
		return fmt.Sprintf("(%s, %s)", t.typ, string(t.value))
	}
}

func (t *token) appendRunes(runes ...rune) {
	if runes == nil {
		return
	}
	t.value = append(t.value, runes...)
}

func (t token) clone() token {
	r := t
	r.value = make([]rune, len(t.value))
	copy(r.value, t.value)
	return r
}

var nilToken = token{}

func (s *scanner) Scan(expr string) (tokens []token, ok bool) {
	s.input = []rune(expr)
	// TODO: to generate less garbage, re-use the existing arrays.
	s.tokens = make([]token, 0, 10)
	s.errs = make([]error, 0, 10)

	s.scan()

	return s.tokens, len(s.errs) == 0
}

func (s *scanner) scan() {
	for !s.atEnd() {
		s.next()
	}
}

func (s *scanner) next() {
	s.tok.pos = s.pos
	r := s.read()

	switch r {
	case '\\':
		s.tok.appendRunes(r)
		s.scanBackslash()
	case '[':
		s.tok.appendRunes(r)
		s.scanBracket()
	case '(':
		s.tok.appendRunes(r)
		s.scanGroup()
	case '*', '+', '?', '{':
		s.tok.appendRunes(r)
		s.scanRepetition(r)
	case '|':
		s.tok.appendRunes(r)
		s.tok.typ = alternativeTok
		s.flushTok()
	case '^', '$':
		s.tok.appendRunes(r)
		s.tok.typ = basicAnchorTok
		s.flushTok()
	default:
		// sungle-rune match
		s.tok.appendRunes(r)
		s.tok.typ = literalTok
		if r == ')' {
			s.tok.typ = closeGroupTok
		}
		s.flushTok()
	}
}

func (s *scanner) scanBackslash() {
	if s.atEnd() {
		s.addError(fmt.Errorf("at index %d: backslash with nothing following", s.pos))
		return
	}

	r := s.read()

	s.tok.typ = classOrEscapeTok
	switch r {
	case 'p', 'P':
		s.tok.appendRunes(r)
		s.scanUnicodeClass()
		s.flushTok()
	case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
		s.tok.appendRunes(r)
		s.scanOctalCharCode()
		s.flushTok()
	case 'x':
		s.tok.appendRunes(r)
		s.scanHexCharCode()
		s.flushTok()
	case 'A', 'z':
		s.tok.typ = directedAnchorTok
		fallthrough
	default:
		s.tok.appendRunes(r)
		s.flushTok()
	}
}

func (s *scanner) scanUnicodeClass() {
	/*
		\pN            Unicode character class (one-letter name)
		\p{Greek}      Unicode character class
		\PN            negated Unicode character class (one-letter name)
		\P{Greek}      negated Unicode character class
	*/

	if s.atEnd() {
		s.addError(fmt.Errorf("at index %d: unicode class (\\p) with nothing following", s.pos))
		return
	}

	r := s.read()

	s.tok.appendRunes(r)

	if r == '{' {
		s.tok.appendRunes(r)
		runes := s.readDelimited('{', '}')
		s.tok.appendRunes(runes...)
	}
}

func (s *scanner) scanOctalCharCode() {
	// The code can only be three digits. So technically
	// \1234 means the octal code 123 followed by a literal 4.

	for !s.atEnd() {
		r := s.read()

		if r < '0' || r > '7' {
			// Not an octal digit
			s.unread()
			break
		}

		s.tok.appendRunes(r)

		if s.tok.Len() == 3 {
			break
		}
	}
}

func (s *scanner) scanHexCharCode() {
	/*
		\x7F           hex character code (exactly two digits)
		\x{10FFFF}     hex character code
	*/

	if s.atEnd() {
		s.addError(fmt.Errorf("at index %d: hex character code (\\x) with nothing following", s.pos))
		return
	}

	r := s.read()

	if r == '{' {
		s.tok.appendRunes(r)
		runes := s.readDelimited('{', '}')
		s.tok.appendRunes(runes...)
		return
	}

	if s.atEnd() {
		s.addError(fmt.Errorf("at index %d: hex character code (\\xNN) is missing last digit", s.pos))
		return
	}

	s.tok.appendRunes(r)
	r = s.read()
	s.tok.appendRunes(r)
}

func (s *scanner) scanBracket() {
	if s.atEnd() {
		s.addError(fmt.Errorf("at index %d: nothing follows [", s.pos))
		return
	}

	runes := s.readDelimited('[', ']')
	s.tok.appendRunes(runes...)
	s.tok.typ = classOrEscapeTok
	s.flushTok()
}

func (s *scanner) scanGroup() {
	if s.atEnd() {
		s.addError(fmt.Errorf("at index %d: nothing follows (", s.pos))
		return
	}

	r := s.read()

	switch r {
	case '?':
		s.tok.appendRunes(r)
		s.scanQuestionGroup()
	default:
		s.unread()
		s.tok.typ = openNumberedGroupTok
		s.flushTok()
	}
}

func (s *scanner) scanQuestionGroup() {
	if s.atEnd() {
		s.addError(fmt.Errorf("at index %d: nothing follows (?", s.pos))
		return
	}

	r := s.read()

	if r == 'P' {
		s.tok.appendRunes(r)
		s.scanNamedGroup()
		s.flushTok()
		return
	}

	s.tok.appendRunes(r)

	if r == ':' {
		s.tok.typ = openNumberedGroupTok
		s.flushTok()
		return
	}

	for {
		if s.atEnd() {
			s.addError(fmt.Errorf("at index %d: unclosed (?", s.pos))
			return
		}

		r := s.read()
		if r == ':' {
			s.tok.appendRunes(r)
			s.tok.typ = openFlagsGroupTok
			s.flushTok()
			return
		}

		if r == ')' {
			s.tok.appendRunes(r)
			s.tok.typ = flagsTok
			s.flushTok()
			return
		}

		s.tok.appendRunes(r)
	}

}

func (s *scanner) scanNamedGroup() {
	if s.atEnd() {
		s.addError(fmt.Errorf("at index %d: nothing follows (?P", s.pos))
		return
	}

	r := s.read()
	if r != '<' {
		s.addError(fmt.Errorf("at index %d: After (?P the next rune must be <", s.pos))
		return
	}

	if s.atEnd() {
		s.addError(fmt.Errorf("at index %d: nothing follows (?P<", s.pos))
		return
	}

	s.tok.appendRunes(r)
	runes := s.readDelimited('<', '>')
	s.tok.typ = openNamedGroupTok
	s.tok.appendRunes(runes...)
	return

}

func (s *scanner) scanRepetition(r rune) {

	s.tok.typ = repetitionTok
	switch r {
	case '*', '+', '?':
		s.scanOptionalQuestion()
	case '{':
		s.scanRepetitionRange()
	}
}

func (s *scanner) scanOptionalQuestion() {
	if s.atEnd() {
		s.flushTok()
		return
	}

	r := s.read()
	if r == '?' {
		s.tok.appendRunes(r)
	} else {
		s.unread()
	}

	s.flushTok()
}

func (s *scanner) scanRepetitionRange() {
	if s.atEnd() {
		s.addError(fmt.Errorf("at index %d: nothing follows {", s.pos))
		return
	}

	runes := s.readDelimited('{', '}')
	s.tok.appendRunes(runes...)

	s.scanOptionalQuestion()
}

func (s *scanner) addError(e error) {
	s.errs = append(s.errs, e)
}

// readDelimited reads delimited text. It reads until the delimiter 'end' is encountered,
// assuming that nested 'start' and 'end' pairs are allowed in the delimited text. This function
// assumes the delimiter 'start' has just been read.
func (s *scanner) readDelimited(start, end rune) []rune {
	// TODO: make this a static variable and re-use it to reduce garbage
	result := make([]rune, 0, 10)

	nesting := 1
	for nesting > 0 {
		if s.atEnd() {
			s.addError(fmt.Errorf("at index %d: expected %c to close opening %c", s.pos, start, end))
			return nil
		}

		r := s.read()

		switch r {
		case start:
			nesting++
		case end:
			nesting--
		}

		result = append(result, r)
	}

	return result
}

func (s *scanner) flushTok() {
	s.tokens = append(s.tokens, s.tok)

	s.tok.typ = nilTok
	s.tok.value = []rune{}
	s.tok.pos = -1
}
func (s *scanner) read() rune {
	var r rune
	if s.atEnd() {
		return 0
	}

	r = s.input[s.pos]
	s.pos++
	return r
}

func (s *scanner) unread() {
	if s.pos > 0 {
		s.pos--
	}
}
func (s *scanner) atEnd() bool {
	return s.pos >= len(s.input)
}

type tokenType int

const (
	nilTok tokenType = iota
	literalTok
	classOrEscapeTok
	directedAnchorTok
	openNumberedGroupTok
	openFlagsGroupTok
	openNamedGroupTok
	closeGroupTok
	flagsTok
	repetitionTok
	alternativeTok
	basicAnchorTok
)

func (t tokenType) String() string {
	switch t {
	case nilTok:
		return "nilTok"
	case literalTok:
		return "literalTok"
	case classOrEscapeTok:
		return "classOrEscapeTok"
	case directedAnchorTok:
		return "directedAnchorTok"
	case openFlagsGroupTok:
		return "openFlagsGroupTok"
	case openNumberedGroupTok:
		return "openNumberedGroupTok"
	case openNamedGroupTok:
		return "openNamedGroupTok"
	case closeGroupTok:
		return "closeGroupTok"
	case flagsTok:
		return "flagsTok"
	case repetitionTok:
		return "repetitionTok"
	case alternativeTok:
		return "alternativeTok"
	}
	return "<unknown token>"
}
