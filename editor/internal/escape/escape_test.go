package escape

import "testing"

func TestExpandEscapes(t *testing.T) {

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty",
			input:    "abc",
			expected: "abc",
		},
		{
			name:     "newline",
			input:    `abc\ndef`,
			expected: "abc\ndef",
		},
		{
			name:     "tab",
			input:    `abc\t\ndef`,
			expected: "abc\t\ndef",
		},
		{
			name:     "quote",
			input:    `\"abc\"`,
			expected: `"abc"`,
		},
		{
			name:     "backslash",
			input:    `\\`,
			expected: `\`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {

			actual := ExpandEscapes(tc.input)
			if actual != tc.expected {
				t.Fatalf("Expected '%s' but got '%s'", tc.expected, actual)
			}

		})
	}
}

func TestExpandEscapesAndUnquote(t *testing.T) {

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty",
			input:    "'abc'",
			expected: "abc",
		},
		{
			name:     "newline",
			input:    `"abc\ndef"`,
			expected: "abc\ndef",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {

			actual, err := ExpandEscapesAndUnquote(tc.input)
			if err != nil {
				t.Fatalf("Error when expanding")
			}

			if actual != tc.expected {
				t.Fatalf("Expected '%s' but got '%s'", tc.expected, actual)
			}

		})
	}
}
