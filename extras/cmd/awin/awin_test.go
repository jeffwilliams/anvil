package main

import "testing"

func TestPromptOrLastFullLine(t *testing.T) {

	type test struct {
		input, output string
	}

	tests := []test{
		{"", ""},
		{"a", "a"},
		{"abc", "abc"},
		{"abc\n", "abc\n"},
		{"abc\nx", "x"},
		{"abc\nxyz", "xyz"},
		{"abc\nxyz\n", "xyz\n"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			output := promptOrLastFullLine(tc.input)
			if output != tc.output {
				t.Fatalf("For '%s', expected '%s' does not match actual '%s'", tc.input, tc.output, output)
			}
		})
	}
}

func TestAppendRespectingCrs(t *testing.T) {

	type test struct {
		name, body, suffix, output string
	}

	tests := []test{
		{"simple",
			"string ",
			"suffix",
			"string suffix"},
		{"case1",
			"string ",
			"\rsuffix",
			"suffix"},
		{"case2",
			"string ",
			"\rsuffix\rtoffee",
			"toffee"},
		{"case3",
			"string ",
			"suffix\rtoffee",
			"toffee"},
		{"case4",
			"string\nline2",
			"\rsuffix\rcheese",
			"string\ncheese"},
		{"case5",
			"line1\n\n",
			"\rsuffix",
			"line1\n\nsuffix"},
		{"case6",
			"line1\n\n",
			"\rsuffix\r",
			"line1\n\n"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			output := appendRespectingCRs([]byte(tc.body), []byte(tc.suffix))
			if string(output) != tc.output {
				t.Fatalf("Expected '%s' does not match actual '%s'", tc.output, string(output))
			}
		})
	}
}
