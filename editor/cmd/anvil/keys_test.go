package main

import (
	"bytes"
	"testing"

	"github.com/jeffwilliams/anvil/editor/internal/keymap"
)

func TestDefaultKeymapLoads(t *testing.T) {
	buf := bytes.NewBuffer(defaultKeymapDefinitions)
	_, err := keymap.LoadDefinitions(buf)
	if err != nil {
		t.Fatalf("Loading default keymap failed: %v", err)
	}
}
