package regex

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestScanRegex(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []token
		ok       bool
		errors   []error
	}{
		{
			name:  "(empty)",
			input: "",
			//expected: []token{{typ: nilTok, pos: 0, value: []rune("")}},
			expected: []token{},
			ok:       true,
			errors:   []error{},
		},
		{
			name:  "abc",
			input: "abc",
			expected: []token{
				{typ: literalTok, pos: 0, value: []rune{'a'}},
				{typ: literalTok, pos: 1, value: []rune{'b'}},
				{typ: literalTok, pos: 2, value: []rune{'c'}},
			},
			ok:     true,
			errors: []error{},
		},
		{
			name:  "^abc$",
			input: "^abc$",
			expected: []token{
				{typ: basicAnchorTok, pos: 0, value: []rune{'^'}},
				{typ: literalTok, pos: 1, value: []rune{'a'}},
				{typ: literalTok, pos: 2, value: []rune{'b'}},
				{typ: literalTok, pos: 3, value: []rune{'c'}},
				{typ: basicAnchorTok, pos: 4, value: []rune{'$'}},
			},
			ok:     true,
			errors: []error{},
		},
		{
			name:  `a\ac`,
			input: `a\ac`,
			expected: []token{
				{typ: literalTok, pos: 0, value: []rune{'a'}},
				{typ: classOrEscapeTok, pos: 1, value: []rune(`\a`)},
				{typ: literalTok, pos: 3, value: []rune{'c'}},
			},
			ok:     true,
			errors: []error{},
		},
		{
			name:  `\x{00}A\x{11}`,
			input: `\x{00}A\x{11}`,
			expected: []token{
				{typ: classOrEscapeTok, pos: 0, value: []rune(`\x{00}`)},
				{typ: literalTok, pos: 6, value: []rune(`A`)},
				{typ: classOrEscapeTok, pos: 7, value: []rune(`\x{11}`)},
			},
			ok:     true,
			errors: []error{},
		},
		{
			name:  `\11b\x1A\xa3`,
			input: `\11b\x1A\xa3`,
			expected: []token{
				{typ: classOrEscapeTok, pos: 0, value: []rune(`\11`)},
				{typ: literalTok, pos: 3, value: []rune(`b`)},
				{typ: classOrEscapeTok, pos: 4, value: []rune(`\x1A`)},
				{typ: classOrEscapeTok, pos: 8, value: []rune(`\xa3`)},
			},
			ok:     true,
			errors: []error{},
		},
		{
			name:  `\A11\z`,
			input: `\A11\z`,
			expected: []token{
				{typ: directedAnchorTok, pos: 0, value: []rune(`\A`)},
				{typ: literalTok, pos: 2, value: []rune(`1`)},
				{typ: literalTok, pos: 3, value: []rune(`1`)},
				{typ: directedAnchorTok, pos: 4, value: []rune(`\z`)},
			},
			ok:     true,
			errors: []error{},
		},
		{
			name:  `[a-z][\p{UnicodeThing}][[:digit:]]`,
			input: `[a-z][\p{UnicodeThing}][[:digit:]]`,
			expected: []token{
				{typ: classOrEscapeTok, pos: 0, value: []rune("[a-z]")},
				{typ: classOrEscapeTok, pos: 5, value: []rune(`[\p{UnicodeThing}]`)},
				{typ: classOrEscapeTok, pos: 23, value: []rune(`[[:digit:]]`)},
			},
			ok:     true,
			errors: []error{},
		},
		{
			name:  `(ab)`,
			input: `(ab)`,
			expected: []token{
				{typ: openNumberedGroupTok, pos: 0, value: []rune(`(`)},
				{typ: literalTok, pos: 1, value: []rune(`a`)},
				{typ: literalTok, pos: 2, value: []rune(`b`)},
				{typ: closeGroupTok, pos: 3, value: []rune(`)`)},
			},
			ok:     true,
			errors: []error{},
		},
		{
			name:  `(?:ab)`,
			input: `(?:ab)`,
			expected: []token{
				{typ: openNumberedGroupTok, pos: 0, value: []rune(`(?:`)},
				{typ: literalTok, pos: 3, value: []rune(`a`)},
				{typ: literalTok, pos: 4, value: []rune(`b`)},
				{typ: closeGroupTok, pos: 5, value: []rune(`)`)},
			},
			ok:     true,
			errors: []error{},
		},
		{
			name:  `(?i)(?P<blah>.)(?i:.)`,
			input: `(?i)(?P<blah>.)(?i:.)`,
			expected: []token{
				{typ: flagsTok, pos: 0, value: []rune(`(?i)`)},
				{typ: openNamedGroupTok, pos: 4, value: []rune(`(?P<blah>`)},
				{typ: literalTok, pos: 13, value: []rune(`.`)},
				{typ: closeGroupTok, pos: 14, value: []rune(`)`)},
				{typ: openFlagsGroupTok, pos: 15, value: []rune(`(?i:`)},
				{typ: literalTok, pos: 19, value: []rune(`.`)},
				{typ: closeGroupTok, pos: 20, value: []rune(`)`)},
			},
			ok:     true,
			errors: []error{},
		},
		{
			name:  `(?i)a(?-i)b`,
			input: `(?i)a(?-i)b`,
			expected: []token{
				{typ: flagsTok, pos: 0, value: []rune(`(?i)`)},
				{typ: literalTok, pos: 4, value: []rune(`a`)},
				{typ: flagsTok, pos: 5, value: []rune(`(?-i)`)},
				{typ: literalTok, pos: 10, value: []rune(`b`)},
			},
			ok:     true,
			errors: []error{},
		},
		{
			name:  `a*b*?c?d{4,5}?`,
			input: `a*b*?c?d{4,5}?e??f{4}`,
			expected: []token{
				{typ: literalTok, pos: 0, value: []rune(`a`)},
				{typ: repetitionTok, pos: 1, value: []rune(`*`)},
				{typ: literalTok, pos: 2, value: []rune(`b`)},
				{typ: repetitionTok, pos: 3, value: []rune(`*?`)},
				{typ: literalTok, pos: 5, value: []rune(`c`)},
				{typ: repetitionTok, pos: 6, value: []rune(`?`)},
				{typ: literalTok, pos: 7, value: []rune(`d`)},
				{typ: repetitionTok, pos: 8, value: []rune(`{4,5}?`)},
				{typ: literalTok, pos: 14, value: []rune(`e`)},
				{typ: repetitionTok, pos: 15, value: []rune(`??`)},
				{typ: literalTok, pos: 17, value: []rune(`f`)},
				{typ: repetitionTok, pos: 18, value: []rune(`{4}`)},
			},
			ok:     true,
			errors: []error{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {

			var s scanner
			toks, ok := s.Scan(tc.input)

			if ok != tc.ok {
				t.Fatalf("Scan returned ok=%v but expected %v. errors: %v", ok, tc.ok, s.errs)
			}

			if ok {
				b := assert.Equal(t, tc.expected, toks)
				if !b {
					t.Logf("tokens are: \n")
					for i, tok := range toks {
						t.Logf(" %d) %s\n", i, tok)
					}
				}
			}

			assert.Equal(t, tc.errors, s.errs)
		})
	}
}
