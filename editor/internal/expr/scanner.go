package expr

import (
	"bytes"
	"fmt"
	"io"
	"unicode"
)

type Scanner struct {
	pos          int
	input        []rune
	tokens       []token
	errs         []error
	closingToken bool
	state        scannerState
	prevRune     rune

	delim               rune
	numberDelimsToMatch int
}

type scannerState int

const (
	stateNormal scannerState = iota
	stateInDelimitedText
	stateAtFinalDelimiter
)

type token struct {
	typ   tokenType
	value string
	// pos is the index of the rune in the input
	// where the token started
	pos int
}

func (t token) tokenType() tokenType {
	return t.typ
}

func (t token) len() int {
	if t.typ == stringTok {
		return len(t.value)
	} else {
		return 1
	}
}

func (t token) String() string {
	if t.value != "" {
		return fmt.Sprintf("(%s)", t.typ)
	} else {
		return fmt.Sprintf("(%s, %s)", t.typ, t.value)
	}
}

var nilToken = token{}

func (s *Scanner) Scan(expr string) (tokens []token, ok bool) {
	s.input = []rune(expr)
	// TODO: to generate less garbage, re-use the existing arrays.
	s.tokens = make([]token, 0, 10)
	s.errs = make([]error, 0, 10)

	for {
		t, err := s.next()
		if err != nil {
			if err == io.EOF {
				break
			}

			s.errs = append(s.errs, err)
		}
		s.addToken(t)
	}
	return s.tokens, len(s.errs) == 0
}

func (s *Scanner) next() (tok token, err error) {
	if s.atEnd() {
		return nilToken, io.EOF
	}

	if s.state == stateNormal {
		tok, err = s.nextInStateNormal()
	} else if s.state == stateInDelimitedText {
		tok, err = s.nextInStateInDelimitedText()
	} else if s.state == stateAtFinalDelimiter {
		tok, err = s.nextInStateAtFinalDelimiter()
	} else {
		return nilToken, io.EOF
	}

	return tok, nil
}

func (s *Scanner) nextInStateNormal() (tok token, err error) {
	r := s.nextNonSpaceRune()

	if s.atEnd() {
		return nilToken, io.EOF
	}

	tok.pos = s.pos

	switch r {
	case '#':
		s.pos++
		tok.typ = poundTok
	case '+':
		s.pos++
		tok.typ = plusTok
	case '-':
		s.pos++
		tok.typ = minusTok
	case ',':
		s.pos++
		tok.typ = commaTok
	case '.':
		s.pos++
		tok.typ = dotTok
	case ';':
		s.pos++
		tok.typ = semicolonTok
	case '/':
		s.pos++
		tok.typ = slashTok
		s.state = stateInDelimitedText
		if s.numberDelimsToMatch <= 0 {
			s.delim = '/'
			s.numberDelimsToMatch = s.delimitedStringsFollowingToken(s.lastToken())
		}
	case '?':
		s.pos++
		tok.typ = questionTok
		s.state = stateInDelimitedText
		if s.numberDelimsToMatch <= 0 {
			s.delim = '?'
			s.numberDelimsToMatch = s.delimitedStringsFollowingToken(s.lastToken())
		}
	case '$':
		s.pos++
		tok.typ = dollarTok
	case '{':
		s.pos++
		tok.typ = openGroupTok
	case '}':
		s.pos++
		tok.typ = closeGroupTok
	case '\'':
		s.pos++
		tok.typ = tickTok
	case 'x', 'y', 'z', 'g', 'v', 'n', 'l':
		s.pos++
		tok.typ = opTok
		tok.value = string(r)
	case 'p', 'P', 'd', 'a', 'c', 'i', 's', '=', 'C':
		s.pos++
		tok.typ = cmdTok
		tok.value = string(r)
	default:
		p := s.pos
		tok, err = s.num()
		if err != nil {
			return
		}
		tok.pos = p
	}

	s.prevRune = r

	return tok, nil

}

func (s *Scanner) nextInStateInDelimitedText() (tok token, err error) {
	p := s.pos
	tok = s.str(s.delim)
	tok.pos = p
	s.numberDelimsToMatch--
	if s.numberDelimsToMatch == 0 {
		s.state = stateAtFinalDelimiter
	} else {
		s.state = stateNormal
	}

	return tok, nil
}

func (s *Scanner) nextInStateAtFinalDelimiter() (tok token, err error) {
	r := s.nextNonSpaceRune()

	if s.atEnd() {
		return nilToken, io.EOF
	}

	tok.pos = s.pos

	switch r {
	case '/':
		s.pos++
		tok.typ = slashTok
		s.state = stateNormal
	case '?':
		s.pos++
		tok.typ = questionTok
		s.state = stateNormal
	}

	return tok, nil
}

func (s *Scanner) nextNonSpaceRune() rune {
	var r rune
	for {
		if s.atEnd() {
			return 0
		}

		r = s.input[s.pos]
		if !unicode.IsSpace(r) {
			break
		}
		s.pos++
	}
	return r
}

func (s *Scanner) delimitedStringsFollowingToken(tok token) int {
	if s.lastToken().tokenType() == opTok {
		switch s.lastToken().value {
		case "x", "y", "g", "v", "z", "l":
			return 1
		case "n":
			return 2
		}
	} else if s.lastToken().tokenType() == cmdTok {
		switch s.lastToken().value {
		case "a", "c", "i":
			return 1
		case "s":
			return 2
		}
	} else {
		// Must be a regexp address
		return 1
	}

	return 0
}

/*
func (s *Scanner) numOrRegex() (token, error) {
	last := s.lastToken().tokenType()
	if last == slashTok || last == questionTok {
		return s.str()
	} else {
		return s.num()
	}
}
*/

func (s *Scanner) str(delim rune) token {
	var buf bytes.Buffer

	// Read until we find a matching unescaped ? or /

	lookFor := delim

	nextRuneEscaped := false
	for {
		if s.atEnd() {
			return token{typ: stringTok, value: buf.String()}
		}

		r := s.input[s.pos]

		if !nextRuneEscaped && r == lookFor {
			return token{typ: stringTok, value: buf.String()}
		}

		if nextRuneEscaped && r != lookFor {
			buf.WriteRune('\\')
		}

		nextRuneEscaped = !nextRuneEscaped && r == '\\'

		if !nextRuneEscaped {
			buf.WriteRune(r)
		}
		s.pos++
	}
}

func (s *Scanner) num() (token, error) {
	var buf bytes.Buffer
	r := s.input[s.pos]

	if !s.isValidNumRune(r) {
		s.pos++ // Consume this bad character
		return nilToken, fmt.Errorf("Invalid character '%c' encountered", r)
	}

	for s.isValidNumRune(r) {
		buf.WriteRune(r)
		s.pos++

		if s.atEnd() {
			return token{typ: numTok, value: buf.String()}, nil
		}
		r = s.input[s.pos]
	}

	return token{typ: numTok, value: buf.String()}, nil
}

func (s *Scanner) isValidNumRune(r rune) bool {
	return r >= '0' && r <= '9'
}

func (s *Scanner) addToken(t token) {
	s.tokens = append(s.tokens, t)
}

func (s *Scanner) lastToken() token {
	if len(s.tokens) == 0 {
		return nilToken
	}
	return s.tokens[len(s.tokens)-1]
}

func (s *Scanner) atEnd() bool {
	return s.pos >= len(s.input)
}

type tokenType int

const (
	nilTok tokenType = iota
	poundTok
	plusTok
	minusTok
	commaTok
	dotTok
	semicolonTok
	slashTok
	questionTok
	dollarTok
	tickTok
	opTok
	cmdTok
	numTok
	stringTok
	openGroupTok
	closeGroupTok
)

func (t tokenType) String() string {
	switch t {
	case nilTok:
		return "nilTok"
	case poundTok:
		return "poundTok"
	case plusTok:
		return "plusTok"
	case minusTok:
		return "minusTok"
	case commaTok:
		return "commaTok"
	case dotTok:
		return "dotTok"
	case semicolonTok:
		return "semicolonTok"
	case slashTok:
		return "slashTok"
	case questionTok:
		return "questionTok"
	case dollarTok:
		return "dollarTok"
	case tickTok:
		return "tickTok"
	case opTok:
		return "opTok"
	case cmdTok:
		return "cmdTok"
	case numTok:
		return "numTok"
	case stringTok:
		return "stringTok"
	case openGroupTok:
		return "openGroupTok"
	case closeGroupTok:
		return "closeGroupTok"
	}
	return "<unknown token>"
}
