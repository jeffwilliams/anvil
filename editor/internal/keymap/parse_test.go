package keymap

import (
	"bytes"
	"fmt"
	"testing"

	"gioui.org/io/key"
	"github.com/stretchr/testify/assert"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name string
		doc  string
		defs []Definition
		err  error
	}{
		{
			name: "basic",
			doc: `
# Comment

keymap base

map a newline		
`,
			defs: []Definition{
				{
					Name: "base",
					Op:   Replace,
					Keys: map[Key][]ActionDefinition{
						Key{"a", 0}: []ActionDefinition{{"newline", nil}},
					},
				},
			},
		},
		{
			name: "two mappings",
			doc: `
# Comment

keymap base

   	map		a      newline		
   map b     exec   
`,
			defs: []Definition{
				{
					Name: "base",
					Op:   Replace,
					Keys: map[Key][]ActionDefinition{
						Key{"a", 0}: []ActionDefinition{{"newline", nil}},
						Key{"b", 0}: []ActionDefinition{{"exec", nil}},
					},
				},
			},
		},
		{
			name: "two entries in mapping",
			doc: `
# Comment

keymap base

map a newline no-indent ; exec 1 2 3
`,
			defs: []Definition{
				{
					Name: "base",
					Op:   Replace,
					Keys: map[Key][]ActionDefinition{
						Key{"a", 0}: []ActionDefinition{
							{"newline", []string{"no-indent"}},
							{"exec", []string{"1", "2", "3"}},
						},
					},
				},
			},
		},
		{
			name: "missing keymap directive",
			doc: `
# Comment

map a newline no-indent ; exec 1 2 3
`,
			defs: nil,
			err:  fmt.Errorf("map directives must follow keymap directives"),
		},
		{
			name: "modifiers",
			doc: `
keymap base

map C-a newline		
map CS-b newline		
`,
			defs: []Definition{
				{
					Name: "base",
					Op:   Replace,
					Keys: map[Key][]ActionDefinition{
						Key{"a", key.ModCtrl}:                []ActionDefinition{{"newline", nil}},
						Key{"b", key.ModCtrl | key.ModShift}: []ActionDefinition{{"newline", nil}},
					},
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {

			buf := bytes.NewBufferString(tc.doc)
			defs, err := LoadDefinitions(buf)
			assert.Equal(t, tc.err, err)
			assert.Equal(t, tc.defs, defs)
		})
	}
}
