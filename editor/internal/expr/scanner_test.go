package expr

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestScanner(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []token
		ok       bool
		errors   []error
	}{
		{
			name:     "empty",
			input:    "",
			expected: []token{},
			ok:       true,
			errors:   []error{},
		},
		{
			name:     "+xy",
			input:    "+xy",
			expected: []token{{typ: plusTok, pos: 0}, {typ: opTok, pos: 1, value: "x"}, {typ: opTok, pos: 2, value: "y"}},
			ok:       true,
			errors:   []error{},
		},
		{
			name:     "x/xbc/",
			input:    "x/xbc/",
			expected: []token{{typ: opTok, pos: 0, value: "x"}, {typ: slashTok, pos: 1}, {typ: stringTok, pos: 2, value: "xbc"}, {typ: slashTok, pos: 5}},
			ok:       true,
			errors:   []error{},
		},
		{
			name:     `/a\/b/`,
			input:    `/a\/b/`,
			expected: []token{{typ: slashTok, pos: 0}, {typ: stringTok, pos: 1, value: "a/b"}, {typ: slashTok, pos: 5}},
			ok:       true,
			errors:   []error{},
		},
		{
			name:     `/ab\\/`,
			input:    `/ab\\/`,
			expected: []token{{typ: slashTok, pos: 0}, {typ: stringTok, pos: 1, value: `ab\\`}, {typ: slashTok, pos: 5}},
			ok:       true,
			errors:   []error{},
		},
		{
			name:     `/item one/`,
			input:    `/item one/`,
			expected: []token{{typ: slashTok, pos: 0}, {typ: stringTok, pos: 1, value: "item one"}, {typ: slashTok, pos: 9}},
			ok:       true,
			errors:   []error{},
		},
		{
			name:     "52 /abc/",
			input:    "52 /abc/",
			expected: []token{{typ: numTok, pos: 0, value: "52"}, {typ: slashTok, pos: 3}, {typ: stringTok, pos: 4, value: "abc"}, {typ: slashTok, pos: 7}},
			ok:       true,
			errors:   []error{},
		},
		// This test ensures that a string parsed inside slashes reverts to the non-string state after the end slash
		{
			name:  "/a/ d",
			input: "/a/ d",
			expected: []token{
				{typ: slashTok, pos: 0}, {typ: stringTok, pos: 1, value: "a"}, {typ: slashTok, pos: 2},
				{typ: cmdTok, pos: 4, value: "d"},
			},
			ok:     true,
			errors: []error{},
		},
		{
			name:     "x/x c/",
			input:    "x/x c/",
			expected: []token{{typ: opTok, pos: 0, value: "x"}, {typ: slashTok, pos: 1}, {typ: stringTok, pos: 2, value: "x c"}, {typ: slashTok, pos: 5}},
			ok:       true,
			errors:   []error{},
		},
		{
			name:     "#51 d",
			input:    "#51 d",
			expected: []token{{typ: poundTok, pos: 0}, {typ: numTok, pos: 1, value: "51"}, {typ: cmdTok, pos: 4, value: "d"}},
			ok:       true,
			errors:   []error{},
		},
		{
			name:     `x/abc\/def/`,
			input:    `x/abc\/def/`,
			expected: []token{{typ: opTok, pos: 0, value: "x"}, {typ: slashTok, pos: 1}, {typ: stringTok, pos: 2, value: `abc/def`}, {typ: slashTok, pos: 10}},
			ok:       true,
			errors:   []error{},
		},
		{
			name:     `x/abc\ndef/`,
			input:    `x/abc\ndef/`,
			expected: []token{{typ: opTok, pos: 0, value: "x"}, {typ: slashTok, pos: 1}, {typ: stringTok, pos: 2, value: `abc\ndef`}, {typ: slashTok, pos: 10}},
			ok:       true,
			errors:   []error{},
		},
		{
			name:     `n/{/}/ d`,
			input:    `n/{/}/ d`,
			expected: []token{{typ: opTok, pos: 0, value: "n"}, {typ: slashTok, pos: 1}, {typ: stringTok, pos: 2, value: `{`}, {typ: slashTok, pos: 3}, {typ: stringTok, pos: 4, value: `}`}, {typ: slashTok, pos: 5}, {typ: cmdTok, pos: 7, value: "d"}},
			ok:       true,
			errors:   []error{},
		},
		{
			name:     `x/abc  def/`,
			input:    `x/abc  def/`,
			expected: []token{{typ: opTok, pos: 0, value: "x"}, {typ: slashTok, pos: 1}, {typ: stringTok, pos: 2, value: `abc  def`}, {typ: slashTok, pos: 10}},
			ok:       true,
			errors:   []error{},
		},
		{
			name:     `s/abc  def/abc def/ d`,
			input:    `s/abc  def/abc def/ d`,
			expected: []token{{typ: cmdTok, pos: 0, value: "s"}, {typ: slashTok, pos: 1}, {typ: stringTok, pos: 2, value: `abc  def`}, {typ: slashTok, pos: 10}, {typ: stringTok, pos: 11, value: `abc def`}, {typ: slashTok, pos: 18}, {typ: cmdTok, pos: 20, value: "d"}},
			ok:       true,
			errors:   []error{},
		},
		{
			name:  "{/a/}",
			input: "{/a/}",
			expected: []token{
				{typ: openGroupTok, pos: 0},
				{typ: slashTok, pos: 1}, {typ: stringTok, pos: 2, value: "a"}, {typ: slashTok, pos: 3},
				{typ: closeGroupTok, pos: 4},
			},
			ok:     true,
			errors: []error{},
		},
	}

	for _, tc := range tests {

		t.Run(tc.name, func(t *testing.T) {

			var s Scanner
			toks, ok := s.Scan(tc.input)
			if ok != tc.ok {
				t.Fatalf("Scan returned ok=%v but expected %v. errors: %v", ok, tc.ok, s.errs)
			}

			if ok {
				assert.Equal(t, tc.expected, toks)
			}

			assert.Equal(t, tc.errors, s.errs)
		})
	}
}
