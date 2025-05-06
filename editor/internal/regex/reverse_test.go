package regex

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestStringBuilder(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "abc",
			input: "abc",
		},
		{
			name:  "a*c",
			input: "a*c",
		},
		{
			name:  "ab|cd",
			input: "ab|cd",
		},
		{
			name:  "ab|cd|.",
			input: "ab|cd|.",
		},
		{
			name:  "(ab)*",
			input: "(ab)*",
		},
		{
			name:  "(?i)a",
			input: "(?i)a",
		},
		{
			name:  "(?P<splort>(?m:(ab)))",
			input: "(?P<splort>(?m:(ab)))",
		},
		{
			name:  "[[:digit:]][a-z]",
			input: "[[:digit:]][a-z]",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {

			var s scanner
			toks, ok := s.Scan(tc.input)

			if !ok {
				t.Fatalf("Scan failed. errors: %v", s.errs)
			}

			var p parser
			tree, err := p.Parse(toks)
			if err != nil {
				t.Fatalf("Parse returned an error: %v", err)
			}

			sb := stringBuilder{tree: tree}
			assert.Equal(t, tc.input, sb.String())
		})
	}
}

func TestReverseRegex(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "abc",
			input:    "abc",
			expected: "cba",
		},
		{
			name:     "a*c",
			input:    "a*c",
			expected: "ca*",
		},
		{
			name:     "ab|cd",
			input:    "ab|cd",
			expected: "ba|dc",
		},
		{
			name:     "ab|cd|.",
			input:    "ab|cd|.",
			expected: "ba|dc|.",
		},
		{
			name:     "(ab)*",
			input:    "(ab)*",
			expected: "(ba)*",
		},
		{
			name:     "(?i)a",
			input:    "(?i)a",
			expected: "(?i)a",
		},
		{
			name:     "(?P<splort>(?m:(ab)))",
			input:    "(?P<splort>(?m:(ab)))",
			expected: "(?P<splort>(?m:(ba)))",
		},
		{
			name:     "[[:digit:]][a-z]",
			input:    "[[:digit:]][a-z]",
			expected: "[a-z][[:digit:]]",
		},
		{
			name:     "abc(?i)x",
			input:    "abc(?i)x",
			expected: "(?i)x(?-i)cba",
		},
		{
			name:     "(?i)a(?-i)bc",
			input:    "(?i)a(?-i)bc",
			expected: "cb(?i)a",
		},
		{
			name:     "(?i)str(?m)xyz",
			input:    "(?i)str(?m)xyz",
			expected: "(?im)zyx(?-m)rts",
		},
		{
			name:     "(?i)str(?i)xyz",
			input:    "(?i)str(?i)xyz",
			expected: "(?i)zyxrts",
		},
		{
			name:     "(?U)str",
			input:    "(?U)str",
			expected: "(?U)rts",
		},
		{
			name:     "(?-mi)str",
			input:    "(?-mi)str",
			expected: "rts",
		},
		{
			name:     "(?i)str(?s-i)xyz",
			input:    "(?i)str(?s-i)xyz",
			expected: "(?s)zyx(?i-s)rts",
		},
		{
			name:     "(?P<splort>(?m:(ab)))",
			input:    "(?P<splort>(?m:(ab)))",
			expected: "(?P<splort>(?m:(ba)))",
		},
		{
			name:     "[[:digit:]][a-z]",
			input:    "[[:digit:]][a-z]",
			expected: "[a-z][[:digit:]]",
		},
		{
			name:     "(x(ab)(cd)y)",
			input:    "(x(ab)(cd)y)",
			expected: "(y(dc)(ba)x)",
		},
		{
			name:     "(x?(ab){4}(cd){1,3}y*)",
			input:    "(x?(ab){4}(cd){1,3}y*)",
			expected: "(y*(dc){1,3}(ba){4}x?)",
		},
		{
			name:     "(ab)*",
			input:    "(ab)*",
			expected: "(ba)*",
		},
		{
			name:     "(empty)",
			input:    "",
			expected: "",
		},
		{
			name:     "abc",
			input:    "abc",
			expected: "cba",
		},
		{
			name:     `a\dbc`,
			input:    `a\dbc`,
			expected: `cb\da`,
		},
		{
			name:     `\D\s\D\w\W`,
			input:    `\D\s\S\w\W`,
			expected: `\W\w\S\s\D`,
		},
		{
			name:     `ab\x{10FFFF}cd`,
			input:    `ab\x{10FFFF}cd`,
			expected: `dc\x{10FFFF}ba`,
		},
		{
			name:     `\x7F\x{10FFFF}`,
			input:    `\x7F\x{10FFFF}`,
			expected: `\x{10FFFF}\x7F`,
		},
		{
			name:     "ab*c",
			input:    "ab*c",
			expected: "cb*a",
		},
		{
			name:     "c*",
			input:    "c*",
			expected: "c*",
		},
		{
			name:     "a*b*?c+?d??",
			input:    "a*b*?c+?d??",
			expected: "d??c+?b*?a*",
		},
		{
			name:     "a{5}b{1,4}c",
			input:    "a{5}b{1,4}c",
			expected: "cb{1,4}a{5}",
		},
		{
			name:     "[^a-z]",
			input:    "[^a-z]",
			expected: "[^a-z]",
		},
		{
			name:     "[^a-z][456]",
			input:    "[^a-z][456]",
			expected: "[456][^a-z]",
		},
		{
			name:     "[^a-z][[:alnum:]]a",
			input:    "[^a-z][[:alnum:]]a",
			expected: "a[[:alnum:]][^a-z]",
		},
		{
			name:     "[^a-z]*",
			input:    "[^a-z]*",
			expected: "[^a-z]*",
		},
		{
			name:     "a(def)*c",
			input:    "a(def)*c",
			expected: "c(fed)*a",
		},
		{
			name:     "a(?:def)*c",
			input:    "a(?:def)*c",
			expected: "c(?:fed)*a",
		},
		{
			name:     "a(?flags)*c",
			input:    "a(?flags)*c",
			expected: "c(?flags)*a",
		},
		{
			name:     "a(?flags:re)*c",
			input:    "a(?flags:re)*c",
			expected: "c(?flags:er)*a",
		},
		{
			name:     "(?i)html|regex",
			input:    "(?i)html|regex",
			expected: "(?i)lmth|xeger",
		},
		{
			name:     "a(?i)bc",
			input:    "a(?i)bc",
			expected: "(?i)cb(?-i)a",
		},
		{
			name:     "(?i:a*b)",
			input:    "(?i:a*b)",
			expected: "(?i:ba*)",
		},
		{
			name:     "^# a",
			input:    "^# a",
			expected: "a #$",
		},
		{
			name:     "abc$",
			input:    "abc$",
			expected: "^cba",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {

			output, err := ReverseRegex(tc.input)

			if err != nil {
				t.Fatalf("Reverse returned an error: %v", err)
			}

			assert.Equal(t, tc.expected, output)
		})
	}
}
