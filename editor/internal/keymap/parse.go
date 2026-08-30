package keymap

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"gioui.org/io/key"
)

type Definition struct {
	Name        string
	Op          Operation
	Fallthrough bool
	Keys        map[Key][]ActionDefinition
}

type ActionDefinition struct {
	ActionName string
	Params     []string
}

type Operation int

const (
	Replace Operation = iota
	Update
)

func LoadDefinitionsFromFile(path string) (defs []Definition, err error) {
	var file *os.File
	file, err = os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()

	return LoadDefinitions(file)
}

func LoadDefinitions(r io.Reader) (defs []Definition, err error) {
	var p parser
	return p.parse(r)
}

type parser struct {
	in     io.Reader
	defs   []Definition
	lineno int
}

func (p parser) parse(in io.Reader) ([]Definition, error) {
	p.in = in

	scanner := bufio.NewScanner(in)
	for scanner.Scan() {
		line := scanner.Text()
		p.lineno++
		err := p.parseLine(line)
		if err != nil {
			return nil, err
		}
	}

	return p.defs, nil
}

func (p *parser) parseLine(line string) (err error) {
	if line == "" {
		return
	}

	if p.isComment(line) {
		return
	}

	fields := strings.Fields(line)
	if len(fields) == 0 {
		return
	}

	switch fields[0] {
	case "keymap":
		err = p.startNewKeymap(fields)
	case "map":
		err = p.addMappingToCurrentKeymap(fields)
	case "default":
		fields = prepend(fields, "map")
		err = p.addMappingToCurrentKeymap(fields)
	default:
		err = fmt.Errorf("unknown directive '%s' on line %d", fields[0], p.lineno)
	}

	return
}

func prepend(fields []string, val string) []string {
	fields2 := make([]string, len(fields)+1)
	copy(fields2[1:], fields)
	fields2[0] = val
	return fields2
}

func (p parser) isComment(line string) bool {
	return line[0] == '#'
}

func (p *parser) startNewKeymap(fields []string) error {
	if len(fields) < 2 {
		return fmt.Errorf("keymap directive on line %d has no name", p.lineno)
	}
	name := fields[1]

	def := Definition{Name: name, Keys: make(map[Key][]ActionDefinition)}

	for _, e := range fields[2:] {
		switch e {
		case "fallthrough":
			def.Fallthrough = true
		case "replace":
			def.Op = Replace
		case "update":
			def.Op = Update
		default:
			return fmt.Errorf("keymap directive on line %d has unknown flag '%s'", p.lineno, e)
		}
	}

	//	if len(fields) > 2 {
	//		var err error
	//		op, err = p.parseOperation(fields[2])
	//		if err != nil {
	//			return err
	//		}
	//	}

	p.defs = append(p.defs, def)
	return nil
}

func (p *parser) addMappingToCurrentKeymap(fields []string) error {
	if len(fields) < 2 {
		return fmt.Errorf("map directive on line %d has no key definition", p.lineno)
	}

	if len(fields) < 3 {
		return fmt.Errorf("map directive on line %d has no action", p.lineno)
	}

	if len(p.defs) < 1 {
		return fmt.Errorf("map directives must follow keymap directives")
	}

	keydef := fields[1]
	actiondef := strings.Join(fields[2:], " ")

	key, err := p.parseKeyDef(keydef)
	if err != nil {
		return fmt.Errorf("map directive is invalid: %s", err.Error())
	}

	defs, err := p.parseActionDefs(actiondef)
	if err != nil {
		return fmt.Errorf("map directive is invalid: %s", err.Error())
	}

	p.defs[len(p.defs)-1].Keys[key] = defs

	return nil
}

func (p parser) parseOperation(s string) (Operation, error) {
	switch s {
	case "replace":
		return Replace, nil
	case "update":
		return Update, nil
	default:
		return Replace, fmt.Errorf("invalid operation %s", s)
	}
}

func (p parser) parseKeyDef(s string) (Key, error) {
	// Format: [<modifier-letter>*-]<key>

	var k Key

	ndx := strings.Index(s, "-")
	if ndx < 0 {
		k.Name = s
		return k, nil
	}

	k.Name = s[ndx+1:]
	modRunes := []rune(s[:ndx])

	if len(modRunes) == 0 {
		fmt.Errorf("invalid key definition '%s' on line %d; it seems incomplete", s, p.lineno)
	}

	for _, r := range modRunes {
		switch r {
		case 'C':
			k.Modifiers |= key.ModCtrl
		case 'S':
			k.Modifiers |= key.ModShift
		case 'A':
			k.Modifiers |= key.ModAlt
		case 'K':
			k.Modifiers |= key.ModCommand
		}
	}

	return k, nil
}

func (p parser) parseActionDefs(s string) (defs []ActionDefinition, err error) {
	list := strings.Split(s, ";")
	defs = make([]ActionDefinition, len(list))
	for i, def := range list {
		def = strings.TrimSpace(def)
		if def == "" {
			continue
		}
		fields := strings.Fields(def)
		if len(fields) == 0 {
			err = fmt.Errorf("invalid action definition '%s' on line %d; it seems incomplete", s, p.lineno)
			return
		}

		defs[i].ActionName = fields[0]
		if len(fields) > 1 {
			defs[i].Params = fields[1:]
		}
	}
	return
}

func (k Definition) String() string {
	var buf bytes.Buffer

	fmt.Fprintf(&buf, "# Anvil keymap\n\n")
	fmt.Fprintf(&buf, "keymap %s %s", k.Name, k.Op)
	if k.Fallthrough {
		fmt.Fprintf(&buf, " fallthrough")
	}
	fmt.Fprintf(&buf, "\n\n")

	sortedKeys := make([]Key, 0, len(k.Keys))
	for k := range k.Keys {
		if k.Name == "default" {
			continue
		}

		sortedKeys = append(sortedKeys, k)
	}

	sort.Slice(sortedKeys, func(i, j int) bool {
		if sortedKeys[i].Name == sortedKeys[j].Name {
			return sortedKeys[i].Modifiers < sortedKeys[j].Modifiers
		}
		return sortedKeys[i].Name < sortedKeys[j].Name
	})

	printActionList := func(list []ActionDefinition) {
		for i, action := range list {
			if i != 0 {
				fmt.Fprintf(&buf, "; ")
			}

			fmt.Fprintf(&buf, "%s", action.ActionName)
			for _, p := range action.Params {
				fmt.Fprintf(&buf, " %s", p)
			}
		}
		fmt.Fprintf(&buf, "\n")
	}

	printKey := func(key Key) {
		list := k.Keys[key]
		fmt.Fprintf(&buf, "map %s ", key)
		printActionList(list)
	}

	for _, key := range sortedKeys {
		printKey(key)
	}

	if list, defaultFound := k.Keys[Key{"default", 0}]; defaultFound {
		fmt.Fprintf(&buf, "default ")
		printActionList(list)
	}

	return buf.String()
}

func (o Operation) String() string {
	switch o {
	case Replace:
		return "replace"
	case Update:
		return "update"
	default:
		return "unknown-operation"
	}
}

/*
# Use unifont to view this: Font
# keymap <name> [<flag>...]
# flags:
#	replace: replace the keymap with the bindings defined here.
#	update: keep the existing keymap, but add the new bindings defined here
#			or if a key binding exists, replace the existing binding with this one.
#			default is replace.
#   fallthrough: set fallthrough to true
#
# override-text-keys <all|some|none>
#   Should the keys that GIO treats as normal text (key.EditEvent) be instead overridden by this keymap. all means that all normal keys are overridden, even if there is no mapping for the key; if there is no mapping and the key is pressed that key is ignored. some declares that the keymap contains one or more mappings that override a normal key, but keys that have no mapping are still treated as normal text. none (the default) means this mapping doesn't override any of the normal keys.

keymap base replace

# Return. '⌤' is newline from the keypad
map ⏎ newline
map ⌤ newline
map S-⏎ newline no-indent
map S-⌤ newline no-indent
map C-⏎ execute-line
map C-⌤ execute-line
map CA-⏎ execute-line clear-errors
map CA-⌤ execute-line clear-errors
# Alternate?
map CA-⌤ clear-errors; execute-line


map C-W push window

keymap window

map → focus-window-right

*/
