package expr

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestParser(t *testing.T) {

	tests := []struct {
		name     string
		input    string
		expected interface{}
		ok       bool
		error    string
	}{
		{
			name:     "empty",
			input:    "",
			expected: nil,
			ok:       true,
			error:    "",
		},
		{
			name:     "#51",
			input:    "#51",
			expected: expr{terms: []interface{}{simpleAddr{typ: charAddrType, val: 51}}},
			ok:       true,
			error:    "",
		},
		{
			name:  "#51 d",
			input: "#51 d",
			expected: expr{
				terms:    []interface{}{simpleAddr{typ: charAddrType, val: 51}},
				commands: []command{{op: 'd'}},
			},
			ok:    true,
			error: "",
		},
		{
			name:  "20 dp",
			input: "20 dp",
			expected: expr{
				terms:    []interface{}{simpleAddr{typ: lineAddrType, val: 20}},
				commands: []command{{op: 'd'}, {op: 'p'}},
			},
			ok:    true,
			error: "",
		},
		{
			name:  "20+/ab/",
			input: "20+/ab/",
			expected: expr{
				terms: []interface{}{
					complexAddr{
						op: '+',
						l:  simpleAddr{typ: lineAddrType, val: 20, regex: ""},
						r:  simpleAddr{typ: forwardRegexAddrType, val: 0, regex: "ab"}}},
			},
			ok:    true,
			error: "",
		},
		{
			name:  "20+#30-/ab/",
			input: "20+#30-/ab/",
			expected: expr{
				terms: []interface{}{
					complexAddr{
						op: '+',
						l:  simpleAddr{typ: lineAddrType, val: 20, regex: ""},
						r: complexAddr{
							op: '-',
							l:  simpleAddr{typ: charAddrType, val: 30, regex: ""},
							r:  simpleAddr{typ: forwardRegexAddrType, val: 0, regex: "ab"}}}},
			},
			ok:    true,
			error: "",
		},
		{
			name:  "20+#30,/ab/",
			input: "20+#30,/ab/",
			expected: expr{
				terms: []interface{}{
					complexAddr{
						op: ',',
						l: complexAddr{
							op: '+',
							l:  simpleAddr{typ: lineAddrType, val: 20},
							r:  simpleAddr{typ: charAddrType, val: 30}},
						r: simpleAddr{typ: forwardRegexAddrType, regex: "ab"}}},
			},
			ok:    true,
			error: "",
		},
		{
			name:  "20+#30,/ab/ x/ar/ #4",
			input: "20+#30,/ab/ x/ar/ #4",
			expected: expr{
				terms: []interface{}{
					complexAddr{
						op: ',',
						l: complexAddr{
							op: '+',
							l:  simpleAddr{typ: lineAddrType, val: 20},
							r:  simpleAddr{typ: charAddrType, val: 30}},
						r: simpleAddr{typ: forwardRegexAddrType, regex: "ab"}},
					operation{op: 'x', regex: "ar"},
					simpleAddr{typ: charAddrType, val: 4},
				},
			},
			ok:    true,
			error: "",
		},
		{
			name:  "n/{/}/",
			input: "n/{/}/",
			expected: expr{
				terms: []interface{}{
					operation{op: 'n', regex: "", args: [2]string{"{", "}"}},
				},
			},
			ok:    true,
			error: "",
		},
		{
			name:  "{ x/abc/ x/def/ }",
			input: "{ x/abc/ x/def/ }",
			expected: expr{
				terms: []interface{}{
					group{
						terms: []interface{}{
							operation{op: 'x', regex: "abc"},
							operation{op: 'x', regex: "def"},
						},
					},
				},
			},
			ok:    true,
			error: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {

			var s Scanner
			toks, ok := s.Scan(tc.input)
			if !ok {
				t.Fatalf("Scan failed")
			}

			var p Parser
			p.matchLimit = 100
			tree, err := p.Parse(toks)
			// Uncomment below to print the parse tree
			/*
				fmt.Printf("test '%s': Parse tree returned:\n", tc.name)
				printTree(tree)
			*/

			if err != nil {
				if tc.ok {
					t.Fatalf("Parse failed when it should succeed. Error: %s", err)
				}

				if err.Error() != tc.error {
					t.Fatalf("Parse failed as expected, but with wrong error. Expected '%s' but got '%s'", tc.error, err.Error())
				}
				return
			}

			if err == nil && !tc.ok {
				t.Fatalf("Parse succeeded when it should have failed")
			}

			assert.Equal(t, tc.expected, tree)
		})
	}
}
