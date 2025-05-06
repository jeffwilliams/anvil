package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func texts(texts ...string) [][]byte {
	var r [][]byte
	for _, t := range texts {
		r = append(r, []byte(t))
	}
	return r
}

func TestEscapeSequences(t *testing.T) {

	type result struct {
		text      []byte
		sequences []EscapeSequence
	}

	type test struct {
		name    string
		input   [][]byte
		results []result
	}

	tests := []test{
		{
			"empty",
			texts(""),
			[]result{
				{text: []byte("")},
			},
		},
		{
			"one slice of only text",
			texts("just text"),
			[]result{
				{
					text:      []byte("just text"),
					sequences: nil,
				},
			},
		},
		{
			"one slice of only text",
			texts("just text"),
			[]result{
				{
					text:      []byte("just text"),
					sequences: nil,
				},
			},
		},
		{
			"two slices of only text",
			texts(
				"just text",
				"just text"),
			[]result{
				{text: []byte("just text")},
				{text: []byte("just text")},
			},
		},
		{
			"ends in escape",
			texts("Hello world\033"),
			[]result{
				{text: []byte("Hello world"),
					sequences: nil,
				},
			},
		},
		{
			"two slices of only text",
			texts(
				"just text",
				"just text"),
			[]result{
				{text: []byte("just text")},
				{text: []byte("just text")},
			},
		},
		{
			"text with one escape (0 params and intermediates) in middle",
			texts("Hello \033[@world"),
			[]result{
				{text: []byte("Hello world"),
					sequences: []EscapeSequence{
						{Index: 6,
							FinalByte: '@',
							Type:      SeqTypeControlSequence,
						},
					},
				},
			},
		},
		{
			"text with two escapes (0 params and intermediates) in middle",
			texts("Hello\033[@ \033[@world"),
			[]result{
				{text: []byte("Hello world"),
					sequences: []EscapeSequence{
						{Index: 5,
							FinalByte: '@',
							Type:      SeqTypeControlSequence,
						},
						{Index: 6,
							FinalByte: '@',
							Type:      SeqTypeControlSequence,
						},
					},
				},
			},
		},
		{
			"text with two consecutive escapes (0 params and intermediates) in middle",
			texts("Hello\033[@\033[@ world"),
			[]result{
				{text: []byte("Hello world"),
					sequences: []EscapeSequence{
						{Index: 5,
							FinalByte: '@',
							Type:      SeqTypeControlSequence,
						},
						{Index: 5,
							FinalByte: '@',
							Type:      SeqTypeControlSequence,
						},
					},
				},
			},
		},
		{
			"escape with intermediates",
			texts("Hello\033 F world"), // 7-bit controls
			[]result{
				{text: []byte("Hello world"),
					sequences: []EscapeSequence{
						{Index: 5,
							FinalByte:         'F',
							IntermediateBytes: []uint8{0x20},
						},
					},
				},
			},
		},
		{
			"escape with intermediates and invalid",
			texts("Hello\033\r F world"), // carriage return is not allowed here inside the sequence.
			[]result{
				{text: []byte("Hello world"),
					sequences: []EscapeSequence{
						{Index: 5,
							FinalByte:         'F',
							IntermediateBytes: []uint8{0x20},
						},
					},
				},
			},
		},
		{
			"escape with intermediates and invalid2",
			texts("Hello\033 \r\rF world"), // carriage return is not allowed here inside the sequence.
			[]result{
				{text: []byte("Hello world"),
					sequences: []EscapeSequence{
						{Index: 5,
							FinalByte:         'F',
							IntermediateBytes: []uint8{0x20},
						},
					},
				},
			},
		},
		{
			"control sequence with param",
			texts("Hello\033[20X world"),
			[]result{
				{text: []byte("Hello world"),
					sequences: []EscapeSequence{
						{Index: 5,
							FinalByte:      'X',
							Type:           SeqTypeControlSequence,
							ParameterBytes: []byte("20"),
						},
					},
				},
			},
		},
		{
			"control sequence with params",
			texts("Hello\033[10;20r world"),
			[]result{
				{text: []byte("Hello world"),
					sequences: []EscapeSequence{
						{Index: 5,
							FinalByte:      'r',
							Type:           SeqTypeControlSequence,
							ParameterBytes: []byte("10;20"),
						},
					},
				},
			},
		},
		{
			"control sequence with params and invalid",
			texts("Hello\033[\r10;\r20r world"),
			[]result{
				{text: []byte("Hello world"),
					sequences: []EscapeSequence{
						{Index: 5,
							FinalByte:      'r',
							Type:           SeqTypeControlSequence,
							ParameterBytes: []byte("10;"), // Not all is parsed.
						},
					},
				},
			},
		},
		{
			"control sequence with params and intermediate",
			texts("Hello\033[1;1;9;9$z world"),
			[]result{
				{text: []byte("Hello world"),
					sequences: []EscapeSequence{
						{Index: 5,
							FinalByte:         'z',
							Type:              SeqTypeControlSequence,
							ParameterBytes:    []byte("1;1;9;9"),
							IntermediateBytes: []byte{'$'},
						},
					},
				},
			},
		},
		{
			"control sequence split",
			texts("Hello\033[1;1", ";9;9$z world"),
			[]result{
				{text: []byte("Hello")},
				{text: []byte(" world"),
					sequences: []EscapeSequence{
						{Index: 0,
							FinalByte:         'z',
							Type:              SeqTypeControlSequence,
							ParameterBytes:    []byte("1;1;9;9"),
							IntermediateBytes: []byte{'$'},
						},
					},
				},
			},
		},
		{
			"control sequence split 2",
			texts("Hello\033", "[1;1;9;9$z world"),
			[]result{
				{text: []byte("Hello")},
				{text: []byte(" world"),
					sequences: []EscapeSequence{
						{Index: 0,
							FinalByte:         'z',
							Type:              SeqTypeControlSequence,
							ParameterBytes:    []byte("1;1;9;9"),
							IntermediateBytes: []byte{'$'},
						},
					},
				},
			},
		},
		{
			"control sequence split 3",
			texts("Hello\033[1;1;9;9$", "z world"),
			[]result{
				{text: []byte("Hello")},
				{text: []byte(" world"),
					sequences: []EscapeSequence{
						{Index: 0,
							FinalByte:         'z',
							Type:              SeqTypeControlSequence,
							ParameterBytes:    []byte("1;1;9;9"),
							IntermediateBytes: []byte{'$'},
						},
					},
				},
			},
		},
		{
			"control sequence split 4",
			texts("Hello\033[1;1;9;9$z", " world"),
			[]result{
				{text: []byte("Hello"),
					sequences: []EscapeSequence{
						{Index: 5,
							FinalByte:         'z',
							Type:              SeqTypeControlSequence,
							ParameterBytes:    []byte("1;1;9;9"),
							IntermediateBytes: []byte{'$'},
						},
					},
				},
				{text: []byte(" world")},
			},
		},
		{
			"osc with bel",
			texts("a\033]\ab"),
			[]result{
				{text: []byte("ab"),
					sequences: []EscapeSequence{
						{Index: 1,
							FinalByte: '\a',
							Type:      SeqTypeControlString,
						},
					},
				},
			},
		},
		{
			"osc with string terminator",
			texts("a\033]0;win-title\033\nb"),
			[]result{
				{text: []byte("ab"),
					sequences: []EscapeSequence{
						{Index: 1,
							FinalByte:         '\n',
							Type:              SeqTypeControlString,
							IntermediateBytes: []byte("0;win-title"),
						},
					},
				},
			},
		},
		{
			"control sequence pair multi string",
			texts("hello\033[0m", "\033[01;34m world"),
			[]result{
				{text: []byte("hello"),
					sequences: []EscapeSequence{
						{Index: 5,
							FinalByte:      'm',
							Type:           SeqTypeControlSequence,
							ParameterBytes: []byte{'0'},
						},
					},
				},
				{text: []byte(" world"),
					sequences: []EscapeSequence{
						{Index: 0,
							FinalByte:      'm',
							Type:           SeqTypeControlSequence,
							ParameterBytes: []byte("01;34"),
						},
					},
				},
			},
		},
		{
			"control sequence pair multi string 2",
			texts("hello\033[0m", "      \033[01;34m world"),
			[]result{
				{text: []byte("hello"),
					sequences: []EscapeSequence{
						{Index: 5,
							FinalByte:      'm',
							Type:           SeqTypeControlSequence,
							ParameterBytes: []byte{'0'},
						},
					},
				},
				{text: []byte("       world"),
					sequences: []EscapeSequence{
						{Index: 6,
							FinalByte:      'm',
							Type:           SeqTypeControlSequence,
							ParameterBytes: []byte("01;34"),
						},
					},
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := NewEscapeSequenceParser()

			if len(tc.input) > len(tc.results) {
				t.Fatalf("Internal test error: test does not have enough results to match the number of input sequences")
			}

			for i, input := range tc.input {
				text, seqs := p.Input(input)
				r := tc.results[i]
				assert.Equal(t, r.text, text, "When comparing text")
				assert.Equal(t, r.sequences, seqs, "When comparing sequences")
			}
		})
	}
}
