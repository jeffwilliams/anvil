package expr

import (
	"fmt"
	"github.com/jeffwilliams/anvil/editor/internal/errs"
	"github.com/jeffwilliams/anvil/editor/internal/escape"
	"os"
	"runtime/debug"
	"strconv"
	"unicode/utf8"
)

// Recursive Descent parser
// https://craftinginterpreters.com/parsing-expressions.html

type Parser struct {
	tokens  []token
	errors  errs.Errors
	current int

	// For debugging
	matchLimit int
	matchCalls int
}

func (p *Parser) Parse(tokens []token) (tree interface{}, err error) {
	p.tokens = tokens
	p.errors = errs.New()
	p.current = 0
	return p.parse()
}

func (p *Parser) SetMatchLimit(i int) {
	p.matchLimit = i
}

func (p *Parser) parse() (tree interface{}, err error) {
	if p.atEnd() {
		return
	}

	tree = p.expr()

	if !p.atEnd() {
		p.addErrorAtPosition("extra tokens after end of command")
	}

	err = p.errors.NilIfEmpty()
	return
}

func (p *Parser) expr() interface{} {

	var expr expr

	for !p.atEnd() {
		t := p.term()
		if t == nil {
			break
		}
		expr.terms = append(expr.terms, t)
	}

	for !p.atEnd() {
		c := p.command()
		if c == nil {
			break
		}
		expr.commands = append(expr.commands, c.(command))
	}

	return expr
}

func (p *Parser) term() interface{} {
	grp := p.group()
	if grp != nil {
		return grp
	}

	addr := p.addr()
	if addr != nil {
		return addr
	}

	operation := p.operation()
	if operation != nil {
		return operation
	}
	return nil
}

func (p *Parser) command() interface{} {
	if p.match(cmdTok) {
		op, _ := utf8.DecodeRuneInString(p.previous().value)

		cmd := command{op: op}
		switch op {
		case 'd', '=', 'C':
			return cmd
		case 'p', 'P':
			// p may take 0 or 1 arguments
			if !p.match(slashTok) {
				return cmd
			}
			if !p.match(stringTok) {
				p.addErrorAtPositionf("expected string after '%c/'", op)
				return nil
			}
			cmd.args[0] = p.previous().value
			if !p.match(slashTok) {
				p.addErrorAtPositionf("expected slash after '%c/...'", op)
				return nil
			}
		case 'a', 'c', 'i':
			// Consume 1 argument
			if !p.match(slashTok) {
				p.addErrorAtPositionf("expected slash after '%c'", op)
				return nil
			}
			if !p.match(stringTok) {
				p.addErrorAtPositionf("expected string after '%c/'", op)
				return nil
			}
			cmd.args[0] = escape.ExpandEscapes(p.previous().value)
			if !p.match(slashTok) {
				p.addErrorAtPositionf("expected slash after '%c/...'", op)
				return nil
			}
		case 's':
			// Consume 2 arguments
			if !p.match(slashTok) {
				p.addErrorAtPositionf("expected slash after '%c'", op)
				return nil
			}
			if !p.match(stringTok) {
				p.addErrorAtPositionf("expected string after '%c/'", op)
				return nil
			}
			cmd.args[0] = p.previous().value
			if !p.match(slashTok) {
				p.addErrorAtPositionf("expected slash after '%c/...'", op)
				return nil
			}
			if !p.match(stringTok) {
				p.addErrorAtPositionf("expected string after '%c/.../'", op)
				return nil
			}
			cmd.args[1] = escape.ExpandEscapes(p.previous().value)
			if !p.match(slashTok) {
				p.addErrorAtPositionf("expected slash after '%c/.../...'", op)
				return nil
			}
		default:
			return nil
		}
		return cmd
	}
	return nil
}

func (p *Parser) group() interface{} {

	var group group

	if !p.match(openGroupTok) {
		return nil
	}

	for !p.atEnd() {
		t := p.term()
		if t == nil {
			break
		}
		group.terms = append(group.terms, t)
	}

	if !p.match(closeGroupTok) {
		p.addErrorAtPositionf("Missing closing '%s' for Opening '%s'", closeGroupTok, openGroupTok)
		return nil
	}

	return group
}

func (p *Parser) addr() interface{} {
	la := p.innerAddr()

	if p.match(commaTok, semicolonTok) {
		typ := p.previous().tokenType()
		r := '0'
		switch typ {
		case commaTok:
			r = ','
		case semicolonTok:
			r = ';'
		}

		ra := p.addr()
		if ra == nil {
			p.addErrorAtPositionf("expected addr after '%c'", r)
			return nil
		}

		return complexAddr{op: r, l: la, r: ra}
	}

	return la
}

func (p *Parser) innerAddr() interface{} {
	la := p.simpleAddr()

	if la != nil && p.match(plusTok, minusTok) {
		typ := p.previous().tokenType()
		r := '0'
		switch typ {
		case plusTok:
			r = '+'
		case minusTok:
			r = '-'
		}

		ra := p.innerAddr()
		if ra == nil {
			p.addErrorAtPositionf("expected addr after '%c'", r)
			return nil
		}

		return complexAddr{op: r, l: la, r: ra}
	}

	return la
}

func (p *Parser) simpleAddr() interface{} {
	var a simpleAddr
	var err error

	if p.match(poundTok) {
		a.typ = charAddrType

		if !p.match(numTok) {
			p.addErrorAtPositionf("expected number after '#'")
			return nil
		}

		a.val, err = strconv.Atoi(p.previous().value)
		if err != nil {
			p.addErrorAtPositionf("parsing number %s failed", p.previous().value)
			return nil
		}
	} else if p.match(numTok) {
		a.typ = lineAddrType
		a.val, err = strconv.Atoi(p.previous().value)
		if err != nil {
			p.addErrorAtPositionf("parsing number %s failed", p.previous().value)
			return nil
		}
	} else if p.match(slashTok, questionTok) {

		r := '0'
		switch p.previous().tokenType() {
		case slashTok:
			a.typ = forwardRegexAddrType
			r = '/'
		case questionTok:
			a.typ = backwardRegexAddrType
			r = '?'
		}

		if !p.match(stringTok) {
			p.addErrorAtPositionf("expected string after '%c'", r)
			return nil
		}

		a.regex = p.previous().value

		if !p.match(slashTok, questionTok) {
			p.addErrorAtPositionf("expected slash after '%c...'", r)
			return nil
		}
	} else if p.match(dollarTok) {
		a.typ = endAddrType
	} else if p.match(dotTok) {
		a.typ = dotAddrType
	} else {
		return nil
	}

	return a
}

func (p *Parser) operation() interface{} {

	if p.match(opTok) {
		op, _ := utf8.DecodeRuneInString(p.previous().value)

		if op == 'n' {
			return p.nOperation()
		}

		operation := operation{op: op}

		// Consume 1 argument
		if !p.match(slashTok) {
			p.addErrorAtPositionf("expected slash after '%c'", op)
			return nil
		}
		if !p.match(stringTok) {
			p.addErrorAtPositionf("expected string after '%c/'", op)
			return nil
		}

		operation.regex = p.previous().value
		if op == 'l' {
			operation.regex = fmt.Sprintf(`^.*%s.*\n`, operation.regex)
		}

		if !p.match(slashTok) {
			p.addErrorAtPositionf("expected slash after '%c/...'", op)
			return nil
		}

		return operation
	}
	return nil
}

func (p *Parser) nOperation() interface{} {
	op := 'n'
	operation := operation{op: op}

	// Consume 2 arguments
	if !p.match(slashTok) {
		p.addErrorAtPositionf("expected slash after '%c'", op)
		return nil
	}
	if !p.match(stringTok) {
		p.addErrorAtPositionf("expected string after '%c/'", op)
		return nil
	}
	operation.args[0] = p.previous().value

	if !p.match(slashTok) {
		p.addErrorAtPositionf("expected slash after '%c/...'", op)
		return nil
	}
	if !p.match(stringTok) {
		p.addErrorAtPositionf("expected string after '%c/.../'", op)
		return nil
	}
	operation.args[1] = p.previous().value
	if !p.match(slashTok) {
		p.addErrorAtPositionf("expected slash after '%c/.../...'", op)
		return nil
	}
	return operation
}

func (p *Parser) match(types ...tokenType) bool {

	if p.matchLimit > 0 {
		p.matchCalls++
		if p.matchCalls > p.matchLimit {
			p.abortAndPrintState()
		}
	}

	for _, t := range types {
		if p.check(t) {
			p.advance()
			return true
		}
	}
	return false
}

func (p *Parser) check(typ tokenType) bool {
	if p.atEnd() {
		return false
	}
	return p.peek().tokenType() == typ
}

func (p *Parser) advance() token {
	if !p.atEnd() {
		p.current++
	}
	return p.previous()
}

func (p *Parser) peek() token {
	return p.tokens[p.current]
}

func (p *Parser) previous() token {
	return p.tokens[p.current-1]
}

func (p *Parser) atEnd() bool {
	return p.current >= len(p.tokens)
}

func (p *Parser) position() int {
	return p.current
}

func (p *Parser) runePosition() int {
	if p.current == 0 {
		return 1
	}

	return p.previous().pos + p.previous().len()
}

func (p *Parser) addError(e error) {
	p.errors.Add(e)
}

func (p *Parser) addErrorAtPosition(msg string) {
	p.addError(fmt.Errorf("At character %d: %s", p.runePosition()+1, msg))
}

func (p *Parser) addErrorAtPositionf(msg string, args ...interface{}) {
	msg2 := fmt.Sprintf(msg, args...)
	p.addErrorAtPosition(msg2)
}

func (p *Parser) abortAndPrintState() {
	fmt.Fprintf(os.Stderr, "Aborting due to possible loop\n")
	fmt.Fprintf(os.Stderr, "Tokens: %v\n", p.tokens)
	tok := "<at end>"
	if !p.atEnd() {
		tok = fmt.Sprintf("%s", p.tokens[p.current])
	}
	fmt.Fprintf(os.Stderr, "trying to match: %d (%s)\n", p.current, tok)
	debug.PrintStack()
	panic("Abort")
}

type expr struct {
	terms    []interface{}
	commands []command
}

type simpleAddr struct {
	typ   simpleAddrType
	val   int
	regex string
	rev   bool
}

type simpleAddrType int

const (
	lineAddrType simpleAddrType = iota
	charAddrType
	forwardRegexAddrType
	backwardRegexAddrType
	endAddrType
	dotAddrType
)

func (s simpleAddrType) String() string {
	switch s {
	case lineAddrType:
		return "lineAddrType"
	case charAddrType:
		return "charAddrType"
	case forwardRegexAddrType:
		return "forwardRegexAddrType"
	case backwardRegexAddrType:
		return "backwardRegexAddrType"
	case endAddrType:
		return "endAddrType"
	case dotAddrType:
		return "dotAddrType"
	default:
		return "?"
	}
}

type complexAddr struct {
	l, r interface{}
	op   rune
	rev  bool
}

func (c *complexAddr) reverseRight(r bool) {
	switch v := c.r.(type) {
	case *complexAddr:
		v.reverse(r)
	case complexAddr:
		v.reverse(r)
	case simpleAddr:
		v.rev = true
		c.r = v
	}
}

func (c *complexAddr) reverseLeft(r bool) {
	switch v := c.l.(type) {
	case *complexAddr:
		v.reverse(r)
	case complexAddr:
		v.reverse(r)
	case simpleAddr:
		v.rev = true
		c.l = v
	}
}

func (c *complexAddr) reverse(r bool) {
	c.reverseLeft(r)
	c.reverseRight(r)
}

type command struct {
	op   rune
	args [2]string
}

type operation struct {
	op    rune
	regex string
	args  [2]string
}

type group struct {
	terms []interface{}
}
