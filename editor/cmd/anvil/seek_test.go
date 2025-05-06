package main

import (
	"regexp"
	"testing"
)

func TestSeekParse(t *testing.T) {

	tests := []struct {
		name                 string
		input                string
		expectedSeeklessName string
		expectedSeek         seek
	}{
		{
			name:                 "file.c",
			input:                "file.c",
			expectedSeeklessName: "file.c",
			expectedSeek: seek{
				line: 0,
				col:  0,
			},
		},
		{
			name:                 "file.c#200",
			input:                "file.c#200",
			expectedSeeklessName: "file.c",
			expectedSeek: seek{
				seekType: seekToRunePos,
				runePos:  200,
			},
		},
		{
			name:                 "file.c!test",
			input:                "file.c!test",
			expectedSeeklessName: "file.c",
			expectedSeek: seek{
				seekType: seekToRegex,
				regex:    regexp.MustCompile(`test`),
			},
		},
		{
			name:                 "file.c:12",
			input:                "file.c:12",
			expectedSeeklessName: "file.c",
			expectedSeek: seek{
				line: 12,
				col:  0,
			},
		},
		{
			name:                 "file.c:1",
			input:                "file.c:1",
			expectedSeeklessName: "file.c",
			expectedSeek: seek{
				line: 1,
				col:  0,
			},
		},
		{
			name:                 "file.c:",
			input:                "file.c:",
			expectedSeeklessName: "file.c",
			expectedSeek: seek{
				line: 0,
				col:  0,
			},
		},
		{
			name:                 "host:file.c",
			input:                "host:file.c",
			expectedSeeklessName: "host:file.c",
			expectedSeek:         seek{},
		},
		{
			name:                 "host:file.c#201",
			input:                "host:file.c#201",
			expectedSeeklessName: "host:file.c",
			expectedSeek: seek{
				seekType: seekToRunePos,
				runePos:  201,
			},
		},
		{
			name:                 "host:file.c!test",
			input:                "host:file.c!test",
			expectedSeeklessName: "host:file.c",
			expectedSeek: seek{
				seekType: seekToRegex,
				regex:    regexp.MustCompile(`test`),
			},
		},
		{
			name:                 "file.c!test",
			input:                "file.c!test",
			expectedSeeklessName: "file.c",
			expectedSeek: seek{
				seekType: seekToRegex,
				regex:    regexp.MustCompile(`test`),
			},
		},
		{
			name:                 "file.c:",
			input:                "file.c:",
			expectedSeeklessName: "file.c",
			expectedSeek: seek{
				line: 0,
				col:  0,
			},
		},
		{
			name:                 "file.c::",
			input:                "file.c::",
			expectedSeeklessName: "file.c",
			expectedSeek: seek{
				line: 0,
				col:  0,
			},
		},
		{
			name:                 "file.c:67:",
			input:                "file.c:67:",
			expectedSeeklessName: "file.c",
			expectedSeek: seek{
				line: 67,
				col:  0,
			},
		},
		{
			name:                 "file.c:20:8",
			input:                "file.c:20:8",
			expectedSeeklessName: "file.c",
			expectedSeek: seek{
				line: 20,
				col:  8,
			},
		},
		{
			name:                 "file.c:20:80",
			input:                "file.c:20:80",
			expectedSeeklessName: "file.c",
			expectedSeek: seek{
				line: 20,
				col:  80,
			},
		},
		{
			name:                 "google.com:file.c:20",
			input:                "google.com:file.c:20",
			expectedSeeklessName: "google.com:file.c",
			expectedSeek: seek{
				line: 20,
			},
		},
		{
			name:                 "192.168.1.2:file.c:20",
			input:                "192.168.1.2:file.c:20",
			expectedSeeklessName: "192.168.1.2:file.c",
			expectedSeek: seek{
				line: 20,
			},
		},
		{
			name:                 "192.168.1.2:5001:file.c",
			input:                "192.168.1.2:5001:file.c",
			expectedSeeklessName: "192.168.1.2:5001:file.c",
			expectedSeek:         seek{},
		},
		{
			name:                 "192.168.1.2:5001:file.c#55",
			input:                "192.168.1.2:5001:file.c#55",
			expectedSeeklessName: "192.168.1.2:5001:file.c",
			expectedSeek: seek{
				seekType: seekToRunePos,
				runePos:  55,
			},
		},
		{
			name:                 "192.168.1.2:file.c:10:5",
			input:                "192.168.1.2:file.c:10:5",
			expectedSeeklessName: "192.168.1.2:file.c",
			expectedSeek: seek{
				line: 10,
				col:  5,
			},
		},
		{
			name:                 "192.168.1.2:5001:file.c:10",
			input:                "192.168.1.2:5001:file.c:10",
			expectedSeeklessName: "192.168.1.2:5001:file.c",
			expectedSeek: seek{
				line: 10,
			},
		},
		{
			name:                 "192.168.1.2:5001:file.c:10:20",
			input:                "192.168.1.2:5001:file.c:10:20",
			expectedSeeklessName: "192.168.1.2:5001:file.c",
			expectedSeek: seek{
				line: 10,
				col:  20,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {

			actualSeeklessName, seek, err := parseSeekFromFilename(tc.input)

			if err != nil {
				t.Fatalf("parsing seek resulted in an error: %v", err)
			}

			if actualSeeklessName != tc.expectedSeeklessName {
				t.Fatalf("expected seekless name to be %s but it is %s", tc.expectedSeeklessName, actualSeeklessName)
			}
			//if seek != tc.expectedSeek {
			if !seeksEqual(seek, tc.expectedSeek) {
				t.Fatalf("expected seek be %#v but it is %#v", tc.expectedSeek, seek)
			}

		})
	}
}

func seeksEqual(a, b seek) bool {
	if a != b {
		// There is no easy way to compare compiled regex pointers, so we'll just make sure
		// they are both not nil or both nil if it's the regex that's causing the mismatch.
		if a.regex == nil && b.regex != nil || b.regex == nil && a.regex != nil {
			return false
		}

		// Compare without regex
		x := a
		y := b
		x.regex = nil
		y.regex = nil

		return x == y
	}

	return true

}
