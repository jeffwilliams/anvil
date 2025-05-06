package regex

import (
	"bytes"
	"fmt"

	"github.com/jeffwilliams/anvil/editor/internal/errs"
)

/*

  Parser parses a regex according to a simplified regex grammar specialized for reversing
	a regular expression.

	re -> alts

	alts -> ( terms '|' alts ) | terms

	terms -> term*

	term -> repeat | atom | flagsgroup | flags | group | directed_anchor
	atom -> dot | class_or_escape | basic_anchor | literal
	group -> namedgroup | numberedgroup | flagsgroup

	namedgroup -> "(?P<" name '>' re ')'
	numberedgroup -> '(' re ')'
	flagsgroup -> "(?" chars ':' re  ")"

	flags -> "(?" chars  ")"
	basic_anchor -> ^ | $ | \b | \B
	directed_anchor -> \A | \z
	class_or_escape -> '[' string ']' | \d | \D | \pN | \p{Name} | '\a' | '\*' ...
	dot -> '.'
	repeat -> ( group | dot | class_or_escape | basic_anchor | literal ) repetition
	repetition -> '*' | '+' | '?' | '{n,m}' | '*?' | '+?' | '??' | '{n,m}?'
	literal -> 'a' | 'b' | ... | 'z' | '0' | ...

	Notes:

	1. Some anchors need special treatment for reversing. I.e. ^abc would become cba$
  2. When tokenizing class_or_escape, note that there may be nested brackets ([)

	References:
	[1] https://users.cs.fiu.edu/~prabakar/resource/Linux/notes/regexp/regexp1.pdf
	[2] https://pkg.go.dev/regexp/syntax@go1.20.6
*/

type parser struct {
	tokens  []token
	errors  errs.Errors
	current int
}

func (p *parser) Parse(tokens []token) (tree *astnode, err error) {
	p.tokens = tokens
	p.errors = errs.New()
	p.current = 0
	return p.parse()
}

func (p *parser) parse() (tree *astnode, err error) {
	if p.atEnd() {
		return
	}

	tree = &astnode{typ: rootNode}
	tree.addChild(p.alts())

	if !p.atEnd() {
		p.addErrorAtPosition("extra tokens after end of command")
	}

	err = p.errors.NilIfEmpty()
	return
}

func (p *parser) re() (node *astnode) {
	return p.alts()
}

func (p *parser) alts() (node *astnode) {
	for !p.atEnd() {
		t := p.terms()
		if t == nil {
			return
		}

		if node == nil {
			node = new(astnode)
			node.typ = alternativesNode
		}

		node.children = append(node.children, t)

		if !p.matchValue([]rune{'|'}) {
			if len(node.children) == 1 {
				node = node.children[0]
			}
			return
		}
	}
	return
}

func (p *parser) terms() (node *astnode) {
	for {
		t := p.term()
		if t == nil {
			return
		}
		if node == nil {
			node = new(astnode)
			node.typ = termsNode
		}
		node.children = append(node.children, t)
	}
}

func (p *parser) term() (node *astnode) {
	if p.atEnd() {
		return
	}

	if p.matchType(classOrEscapeTok, flagsTok, directedAnchorTok, literalTok, basicAnchorTok) {
		node = new(astnode)
		node.tok = p.previous()
		switch p.previous().typ {
		case classOrEscapeTok:
			node.typ = classOrEscapeNode
		case flagsTok:
			node.typ = flagsNode
		case directedAnchorTok:
			node.typ = directedAnchorNode
		case basicAnchorTok:
			node.typ = basicAnchorNode
		case literalTok:
			node.typ = literalNode
		}
	} else if p.matchType(openNumberedGroupTok, openFlagsGroupTok, openNamedGroupTok) {
		node = p.group()
	} else {
		return
	}

	// Check for repitition. If found make it the parent
	if p.matchType(repetitionTok) {
		rep := new(astnode)
		rep.typ = repetitionNode
		rep.tok = p.previous()
		rep.children = []*astnode{node}
		node = rep
	}

	return
}

func (p *parser) group() (node *astnode) {
	if p.atEnd() {
		p.addErrorAtPosition("Expected group contents after opening (")
		return
	}
	openPos := p.runePosition() + 1
	tok := p.previous()

	inner := p.re()

	if !p.matchValue([]rune{')'}) {
		p.addErrorAtPositionf("Expected ) to close group opened at %d", openPos)
		return
	}

	node = new(astnode)
	switch tok.typ {
	case openNumberedGroupTok:
		node.typ = numberedGroupNode
	case openNamedGroupTok:
		node.typ = namedGroupNode
	case openFlagsGroupTok:
		node.typ = flagsGroupNode
	}
	node.tok = tok
	node.children = []*astnode{inner}
	return
}

func (p *parser) matchType(types ...tokenType) bool {
	for _, t := range types {
		if p.checkType(t) {
			p.advance()
			return true
		}
	}
	return false
}

func (p *parser) checkType(typ tokenType) bool {
	if p.atEnd() {
		return false
	}
	return p.peek().tokenType() == typ
}

func (p *parser) matchValue(vals ...[]rune) bool {
	for _, t := range vals {
		if p.checkValue(t) {
			p.advance()
			return true
		}
	}
	return false
}

func (p *parser) checkValue(val []rune) bool {
	if p.atEnd() {
		return false
	}
	return same(p.peek().value, val)
}

func same(a, b []rune) bool {
	if len(a) != len(b) {
		return false
	}
	for i, x := range a {
		if b[i] != x {
			return false
		}
	}
	return true
}

func (p *parser) advance() token {
	if !p.atEnd() {
		p.current++
	}
	return p.previous()
}

func (p *parser) peek() token {
	return p.tokens[p.current]
}

func (p *parser) previous() token {
	return p.tokens[p.current-1]
}

func (p *parser) atEnd() bool {
	return p.current >= len(p.tokens)
}

func (p *parser) position() int {
	return p.current
}

func (p *parser) runePosition() int {
	if p.current == 0 {
		return 1
	}

	return p.previous().pos + p.previous().Len()
}

func (p *parser) addError(e error) {
	p.errors.Add(e)
}

func (p *parser) addErrorAtPosition(msg string) {
	p.addError(fmt.Errorf("At character %d: %s", p.runePosition()+1, msg))
}

func (p *parser) addErrorAtPositionf(msg string, args ...interface{}) {
	msg2 := fmt.Sprintf(msg, args...)
	p.addErrorAtPosition(msg2)
}

type astnode struct {
	tok      token
	typ      astNodeType
	children []*astnode
	userData interface{}
}

func (a *astnode) addChild(n *astnode) {
	a.children = append(a.children, n)
}

func (a *astnode) String() string {
	return a.str(0)
}

func (a *astnode) str(indent int) string {
	var buf bytes.Buffer

	for i := 0; i < indent; i++ {
		buf.WriteRune(' ')
	}

	fmt.Fprintf(&buf, "typ: %s tok: %s\n", a.typ, a.tok)
	for _, ch := range a.children {
		buf.WriteString(ch.str(indent + 2))
	}
	return buf.String()
}

func (a *astnode) firstChild() *astnode {
	if len(a.children) == 0 {
		return nil
	}

	return a.children[0]
}

type astNodeType int

const (
	rootNode = iota
	alternativesNode
	literalNode
	termsNode
	classOrEscapeNode
	flagsGroupNode
	namedGroupNode
	numberedGroupNode
	flagsNode
	directedAnchorNode
	repetitionNode
	basicAnchorNode
)

func (t astNodeType) String() string {
	switch t {
	case rootNode:
		return "rootNode"
	case alternativesNode:
		return "alternativesNode"
	case literalNode:
		return "literalNode"
	case termsNode:
		return "termsNode"
	case classOrEscapeNode:
		return "classOrEscapeNode"
	case flagsGroupNode:
		return "flagsGroupNode"
	case namedGroupNode:
		return "namedGroupNode"
	case numberedGroupNode:
		return "numberedGroupNode"
	case flagsNode:
		return "flagsNode"
	case directedAnchorNode:
		return "directedAnchorNode"
	case repetitionNode:
		return "repetitionNode"
	default:
		return "unknown"
	}
}
