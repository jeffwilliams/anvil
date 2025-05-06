package regex

import (
	"bytes"
	"fmt"
)

func ReverseRegex(re string) (string, error) {
	if re == "" {
		return "", nil
	}

	r := reverser{input: re}
	err := r.parse()
	if err != nil {
		return "", err
	}

	r.adjustFlagNodesForReversal()
	r.reverse()

	sb := stringBuilder{tree: r.tree}

	return sb.String(), nil
}

type reverser struct {
	input string
	tree  *astnode
}

func (r *reverser) parse() error {
	var s scanner
	toks, ok := s.Scan(r.input)

	if !ok {
		return fmt.Errorf("scanning failed: %v", s.errs)
	}

	var p parser
	var err error
	r.tree, err = p.Parse(toks)
	if err != nil {
		return fmt.Errorf("parsing failed: %v", err)
	}

	return nil
}

func (r *reverser) reverse() {
	n := r.tree

	n = n.firstChild()
	if n == nil {
		return
	}

	if n.typ == alternativesNode {
		for _, ch := range n.children {
			r.terms(ch)
		}
		return
	}

	r.terms(n)
}

func (r reverser) terms(n *astnode) {
	if n == nil {
		return
	}

	r.reverseNodes(n.children)

	for _, ch := range n.children {
		switch ch.typ {

		case namedGroupNode, numberedGroupNode, flagsGroupNode:
			r.terms(ch.firstChild())

		case repetitionNode:
			r.terms(ch)

		case basicAnchorNode:
			r.adjustBasicAnchor(ch)
		}
	}
}

func (r reverser) reverseNodes(s []*astnode) {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
}

func (r reverser) adjustFlagNodesForReversal() {
	n := r.tree

	n = n.firstChild()
	if n == nil {
		return
	}

	if n.typ == alternativesNode {
		for _, ch := range n.children {
			r.adjustFlagEndingNodesInTerms(ch)
		}
		return
	}

	r.adjustFlagEndingNodesInTerms(n)

}

// adjustFlagEndingNodesInTerms scans through the parse tree in
// left-to-right order and "reverses" the flag groups. Each
// flag group that actually changes the current set of flags for the regex
// can be thought of as marking the boundary of a region where a set of flags
// is on.
//
// For example, in:
//
// abc(?i)def(?-i)ghi
//
// the (?i) group begins a region where the 'i' flag is set, amd (?-i)
// ends that region.
//
// This function changes the opening boundary marker that sets the
// flags for the region to instead unset the flags, and at the end of the
// boundary changes the marker to set the flags instead of unset them.
// For the example this would result in:
//
// abc(?-i)def(?i)ghi
//
// Thus when all the nodes in the regex are reversed (or the regular expression)
// is read from right to left) these flag groups properly delimit the new
// flag boundaries.
func (r reverser) adjustFlagEndingNodesInTerms(n *astnode) {
	var tok *token
	newChildren := make([]*astnode, 0, len(n.children))
	var globalFlags reFlags

	makeFlagsNode := func(pos int, flagsStr []rune) *astnode {
		val := []rune{'(', '?'}
		val = append(val, flagsStr...)
		val = append(val, ')')

		// Add a node that inverts the change
		return &astnode{
			typ: flagsNode,
			tok: token{
				typ:   flagsTok,
				pos:   pos,
				value: val,
			},
		}

	}

	for i, ch := range n.children {
		switch ch.typ {
		case flagsNode:
			modified, values := parseTokenFlags(&ch.tok)
			modified, values = changedFlagsWhenTokenApplied(globalFlags, modified, values)
			globalFlags = values

			// If this set of flags doesn't actually change the global flags state (i.e. it
			// sets a flag that is already set) then it can be ignored.
			// Also, if this would be the last child after reversing, there is no need to
			// change any flags; the re is complete anyway
			if modified > 0 && i > 0 {
				inverted := invertModifiedFlags(modified, values)
				unapply := makeFlagsNode(ch.tok.pos, flagsString(inverted, modified))
				newChildren = append(newChildren, unapply)
			}

			tok = &ch.tok
		case namedGroupNode, numberedGroupNode, flagsGroupNode:
			r.adjustFlagEndingNodesInTerms(ch.firstChild())
			newChildren = append(newChildren, ch)
		default:
			newChildren = append(newChildren, ch)
		}
	}

	if tok != nil && globalFlags != 0 {
		// In the flags string, only enable the flags that are set in the global flags
		apply := makeFlagsNode(tok.pos, flagsString(globalFlags, globalFlags))
		newChildren = append(newChildren, apply)
	}

	n.children = newChildren
}

type regionFlags uint32

const (
	regionFlagStartOfRegionAfterReverse = 1 << iota
	regionFlagEndOfRegionAfterReverse
)

func (r reverser) adjustBasicAnchor(n *astnode) {
	if len(n.tok.value) == 0 {
		return
	}

	switch n.tok.value[0] {
	case '^':
		n.tok.value[0] = '$'
	case '$':
		n.tok.value[0] = '^'
	}
}

type stringBuilder struct {
	tree *astnode
	buf  bytes.Buffer
}

func (sb *stringBuilder) String() string {
	sb.buf.Reset()
	n := sb.tree

	if len(n.children) == 0 {
		return ""
	}
	n = n.children[0]

	switch n.typ {
	case alternativesNode:
		sb.alternatives(n)
	case termsNode:
		sb.terms(n)
	}

	return sb.buf.String()
}

func (sb *stringBuilder) alternatives(n *astnode) {
	for i, ch := range n.children {
		if i > 0 {
			sb.buf.WriteRune('|')
		}
		if ch.typ != termsNode {
			continue
		}
		sb.terms(ch)
	}
}

func (sb *stringBuilder) terms(n *astnode) {
	if n == nil {
		return
	}

	for _, ch := range n.children {
		switch ch.typ {
		case literalNode, classOrEscapeNode, flagsNode, directedAnchorNode, basicAnchorNode:
			sb.writeRunes(ch.tok.value)

		case namedGroupNode, numberedGroupNode, flagsGroupNode:
			if ch.typ == numberedGroupNode {
				sb.writeRunes(ch.tok.value)
			} else {
				sb.writeRunes(ch.tok.value)
			}
			sb.terms(ch.firstChild())
			sb.buf.WriteRune(')')

		case repetitionNode:
			sb.terms(ch)
			sb.writeRunes(ch.tok.value)
		}
	}
}

func (sb *stringBuilder) writeRunes(runes []rune) {
	for _, r := range runes {
		sb.buf.WriteRune(r)
	}
}
