package words

import (
	"reflect"
	"testing"
)

func TestWordsIn(t *testing.T) {

	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "word",
			input:    "word",
			expected: []string{"word"},
		},
		{
			name:     "a word",
			input:    "a word",
			expected: []string{"a", "word"},
		},
		{
			name:     "wandering",
			input:    "I wandered the world; for a time, at least. Oblivious to the pressures---and reality---of the order. ",
			expected: []string{"I", "wandered", "the", "world", "for", "a", "time", "at", "least", "Oblivious", "to", "the", "pressures", "and", "reality", "of", "the", "order"},
		},
		{
			name:     "identifiers",
			input:    "ident_1 ident_word",
			expected: []string{"ident_1", "ident_word"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {

			actual := wordsIn([]byte(tc.input))

			if !reflect.DeepEqual(actual, tc.expected) {
				t.Fatalf("expected %v but got %v", tc.expected, actual)
			}

		})
	}
}

func TestCommonPrefix(t *testing.T) {
	testCommonPrefixFn := func(s1, s2, result string) {
		r := commonPrefix(s1, s2)
		if r != result {
			t.Fatalf("for common prefix begween '%s' and '%s' expected '%s' but got '%s'", s1, s2, result, r)
		}
	}

	testCommonPrefixFn("cat", "rat", "")
	testCommonPrefixFn("", "rat", "")
	testCommonPrefixFn("", "", "")
	testCommonPrefixFn("hello", "heck", "he")
	testCommonPrefixFn("fellow", "fell", "fell")
	testCommonPrefixFn("fell", "fellow", "fell")
}
