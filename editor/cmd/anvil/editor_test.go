package main

import (
	"strings"
	"testing"
)

func TestRemoveTagFromString(t *testing.T) {

	tests := []struct {
		name   string
		tag    string
		job    string
		output string
	}{
		{
			name:   "not in tag",
			tag:    "Newcol",
			job:    "ls",
			output: "Newcol",
		},
		{
			name:   "ls first",
			tag:    "ls Newcol",
			job:    "ls",
			output: "Newcol",
		},
		{
			name:   "ls first part 2",
			tag:    "ls sleep Newcol",
			job:    "ls",
			output: "sleep Newcol",
		},
		{
			name:   "ls middle",
			tag:    "sleep ls Newcol",
			job:    "ls",
			output: "sleep Newcol",
		},
		{
			name:   "ls last",
			tag:    "sleep ls",
			job:    "ls",
			output: "sleep",
		},
		{
			name:   "ls last part 2",
			tag:    "sleep ls ",
			job:    "ls",
			output: "sleep",
		},
		{
			name:   "ls only",
			tag:    "ls",
			job:    "ls",
			output: "",
		},
		{
			name:   "ls only part 2",
			tag:    "ls ",
			job:    "ls",
			output: "",
		},
		{
			name:   "job is substring 1",
			tag:    "tmp+Errors tmp Newcol",
			job:    "tmp",
			output: "tmp+Errors Newcol",
		},
		{
			name:   "job is substring 2",
			tag:    "tmp tmp+Errors Newcol",
			job:    "tmp",
			output: "tmp+Errors Newcol",
		},
		{
			name:   "job is substring 3",
			tag:    "tmp+Errors tmp Newcol",
			job:    "tmp+Errors",
			output: "tmp Newcol",
		},
		{
			name:   "job is substring 4",
			tag:    "tmp+Errors tmp",
			job:    "tmp",
			output: "tmp+Errors",
		},
		{
			name:   "job is substring 5",
			tag:    "boot oo Newcol",
			job:    "oo",
			output: "boot Newcol",
		},
		{
			name:   "job is substring 6",
			tag:    "tmp tmp+Errors tmp",
			job:    "tmp",
			output: "tmp+Errors tmp",
		},
		{
			name:   "job is substring 6",
			tag:    "boo oo Newcol",
			job:    "oo",
			output: "boo Newcol",
		},
		{
			name:   "job is substring 7",
			tag:    "oo boo Newcol",
			job:    "oo",
			output: "boo Newcol",
		},
		{
			name:   "job is substring 8",
			tag:    "a oo boo Newcol",
			job:    "oo",
			output: "a boo Newcol",
		},
		{
			name:   "job is substring 9",
			tag:    "a oo boo",
			job:    "oo",
			output: "a boo",
		},
		{
			name:   "job is substring 9",
			tag:    "a boo oo",
			job:    "oo",
			output: "a boo",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {

			result, _, _ := removeJobFromTagString(tc.job, tc.tag)

			if result != tc.output {
				t.Fatalf("Expected '%s' but got '%s'", tc.output, result)
			}
		})
	}
}

func TestSubstitute(t *testing.T) {

	tests := []struct {
		name         string
		template     string
		replacements []string
		output       string
	}{
		{
			name:         "simple",
			template:     "echo $1",
			replacements: []string{"foo"},
			output:       "echo foo",
		},
		{
			name:         "simple2",
			template:     "echo $1 ",
			replacements: []string{"foo"},
			output:       "echo foo ",
		},
		{
			name:         "index too low",
			template:     "echo $0",
			replacements: []string{"foo"},
			output:       "echo ",
		},
		{
			name:         "index too high",
			template:     "echo $2",
			replacements: []string{"foo"},
			output:       "echo ",
		},
		{
			name:         "triple",
			template:     "echo $3 $2 $1",
			replacements: []string{"3", "2", "1"},
			output:       "echo 1 2 3",
		},
		{
			name:         "dollar",
			template:     "echo $$",
			replacements: []string{"3", "2", "1"},
			output:       "echo $",
		},
		{
			name:         "nil replacements",
			template:     "echo $1",
			replacements: nil,
			output:       "echo ",
		},
		{
			name:         "star",
			template:     "echo $*",
			replacements: []string{"foo", "bar"},
			output:       "echo foo bar",
		},
		{
			name:         "star nil replace",
			template:     "echo $*",
			replacements: nil,
			output:       "echo ",
		},
		{
			name:         "dollar replace",
			template:     "echo $$b",
			replacements: nil,
			output:       "echo $b",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {

			result := substitute(tc.template, tc.replacements)

			if result != tc.output {
				t.Fatalf("Expected '%s' but got '%s'", tc.output, result)
			}
		})
	}
}

func TestBasenameLikelyStartsWith(t *testing.T) {

	tests := []struct {
		name     string
		path     string
		needle   rune
		expected bool
	}{
		{
			name:     "empty",
			path:     "",
			needle:   '+',
			expected: false,
		},
		{
			name:     "no-directory",
			path:     "+thing",
			needle:   '+',
			expected: true,
		},
		{
			name:     "no-directory negative",
			path:     "+thing",
			needle:   '0',
			expected: false,
		},
		{
			name:     "root",
			path:     "/+thing",
			needle:   '+',
			expected: true,
		},
		{
			name:     "root negative",
			path:     "/+thing",
			needle:   '0',
			expected: false,
		},
		{
			name:     "dir",
			path:     "/p/+thing",
			needle:   '+',
			expected: true,
		},
		{
			name:     "dir negative",
			path:     "/p/+thing",
			needle:   '0',
			expected: false,
		},
		{
			name:     "windows",
			path:     "C:\\+thing",
			needle:   '+',
			expected: true,
		},
		{
			name:     "windows negative",
			path:     "C:\\+thing",
			needle:   '0',
			expected: false,
		},
		{
			name:     "windows negative 2",
			path:     "C:\\+thing",
			needle:   'C',
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {

			result := BasenameLikelyStartsWith(tc.path, tc.needle)

			if result != tc.expected {
				t.Fatalf("Expected '%v' but got '%v'", tc.expected, result)
			}
		})
	}
}

func TestMoveActiveLayerTo(t *testing.T) {
	tests := []struct {
		name          string
		initialLayers []string
		indexToMove   int
		moveToIndex   int
		finalLayers   []string
	}{
		{
			name:          "no-layers",
			initialLayers: []string{},
			indexToMove:   1,
			moveToIndex:   2,
			finalLayers:   []string{},
		},
		{
			name:          "one-layer",
			initialLayers: []string{"base"},
			indexToMove:   1,
			moveToIndex:   2,
			finalLayers:   []string{"base"},
		},
		{
			name:          "two-layer-no-op",
			initialLayers: []string{"base", "tax"},
			indexToMove:   0,
			moveToIndex:   0,
			finalLayers:   []string{"base", "tax"},
		},
		{
			name:          "two-layer-no-op2",
			initialLayers: []string{"base", "tax"},
			indexToMove:   0,
			moveToIndex:   -1,
			finalLayers:   []string{"base", "tax"},
		},
		{
			name:          "two-layer-no-op3",
			initialLayers: []string{"base", "tax"},
			indexToMove:   1,
			moveToIndex:   9,
			finalLayers:   []string{"base", "tax"},
		},
		{
			name:          "two-layer-move-up",
			initialLayers: []string{"base", "tax"},
			indexToMove:   0,
			moveToIndex:   1,
			finalLayers:   []string{"tax", "base"},
		},
		{
			name:          "two-layer-move-up-high",
			initialLayers: []string{"base", "tax"},
			indexToMove:   0,
			moveToIndex:   9,
			finalLayers:   []string{"tax", "base"},
		},
		{
			name:          "two-layer-move-down",
			initialLayers: []string{"base", "tax"},
			indexToMove:   1,
			moveToIndex:   -1,
			finalLayers:   []string{"tax", "base"},
		},
		{
			name:          "two-layer-move-down",
			initialLayers: []string{"base", "tax"},
			indexToMove:   1,
			moveToIndex:   0,
			finalLayers:   []string{"tax", "base"},
		},
		{
			name:          "five-layer-move",
			initialLayers: []string{"a", "b", "c", "d", "e"},
			indexToMove:   1,
			moveToIndex:   3,
			finalLayers:   []string{"a", "c", "d", "b", "e"},
		},
		{
			name:          "five-layer-move-back",
			initialLayers: []string{"a", "b", "c", "d", "e"},
			indexToMove:   3,
			moveToIndex:   1,
			finalLayers:   []string{"a", "d", "b", "c", "e"},
		},
		{
			name:          "five-layer-move-to-first",
			initialLayers: []string{"a", "b", "c", "d", "e"},
			indexToMove:   2,
			moveToIndex:   0,
			finalLayers:   []string{"c", "a", "b", "d", "e"},
		},
		{
			name:          "five-layer-move-to-last",
			initialLayers: []string{"a", "b", "c", "d", "e"},
			indexToMove:   2,
			moveToIndex:   9,
			finalLayers:   []string{"a", "b", "d", "e", "c"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {

			var ed Editor
			for _, n := range tc.initialLayers {
				ed.Layers = append(ed.Layers, &Layer{Name: n})
			}

			ed.activeLayerIndex = tc.indexToMove
			ed.MoveActiveLayerTo(tc.moveToIndex)

			if len(ed.Layers) != len(tc.finalLayers) {
				t.Fatalf("After move, the number of layers changed")
			}

			for i, l := range ed.Layers {
				if l.Name != tc.finalLayers[i] {
					var names []string
					for _, l := range ed.Layers {
						names = append(names, l.Name)
					}
					t.Fatalf("Expected layer '%s' at index i but there is instead '%s'. All layers:\n %s", tc.finalLayers[i], l.Name, strings.Join(names, "\n"))
				}
			}
		})
	}
}
