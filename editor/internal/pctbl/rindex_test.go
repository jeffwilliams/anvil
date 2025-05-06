package pctbl

import (
	"testing"
	"unicode/utf8"
)

func TestRuneIndexToByteIndex(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		runeIndex int
		byteIndex int
	}{
		{
			name:      "basic",
			input:     "123412341234123",
			runeIndex: 0,
			byteIndex: 0,
		},
		{
			name:      "low1",
			input:     "123412341234123",
			runeIndex: 1,
			byteIndex: 1,
		},
		{
			name:      "low5",
			input:     "123412341234123",
			runeIndex: 5,
			byteIndex: 5,
		},
		{
			name:      "high12",
			input:     "123412341234123",
			runeIndex: 12,
			byteIndex: 12,
		},
		{
			name:      "high14",
			input:     "123412341234123",
			runeIndex: 14,
			byteIndex: 14,
		},
		// The section symbol is 2 UTF-8 bytes long
		{
			name:      "section low",
			input:     "§§§§§§§§§§", // 10 runes
			runeIndex: 3,
			byteIndex: 6,
		},
		{
			name:      "section high",
			input:     "§§§§§§§§§§", // 10 runes
			runeIndex: 8,
			byteIndex: 16,
		},
		{
			name:      "section high limit",
			input:     "§§§§§§§§§§", // 10 runes
			runeIndex: 9,
			byteIndex: 18,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {

			bytes := []byte(tc.input)
			runeCount := utf8.RuneCount(bytes)
			actual := runeIndexToByteIndex(tc.runeIndex, bytes, runeCount)
			if actual != tc.byteIndex {
				t.Fatalf("actual byte index %d does not match expected %d", actual, tc.byteIndex)
			}
		})
	}
}
