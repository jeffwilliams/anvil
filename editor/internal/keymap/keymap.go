package keymap

import (
	"bytes"
	"fmt"

	"gioui.org/io/key"
)

type Key struct {
	Name      string
	Modifiers key.Modifiers
}

type Action interface{}

type SpecialAction int

const (
	Pop SpecialAction = iota
	Halt
	HaltIfNoAction
)

type Keymap struct {
	Name        string
	Fallthrough bool
	Keys        map[Key][]Action
	// Default, when not nil, is executed if no other key matches
	Default []Action
}

func NewKeymap(name string) Keymap {
	return Keymap{
		Name: name,
		Keys: make(map[Key][]Action),
	}
}

func (k Keymap) String() string {
	var buf bytes.Buffer

	fmt.Fprintf(&buf, "keymap %s fallthrough: %v default: %s\n", k.Name, k.Fallthrough, k.Default)
	for k, list := range k.Keys {
		fmt.Fprintf(&buf, "  map %s ", k)
		for i, action := range list {
			if i > 0 {
				fmt.Fprintf(&buf, "; ")
			}
			fmt.Fprintf(&buf, "%s", action)
		}
		fmt.Fprintf(&buf, "\n")
	}
	return buf.String()
}

func (km *Keymap) Append(k Key, a Action) {
	list, ok := km.Keys[k]
	if !ok {
		list = []Action{}
	}
	list = append(list, a)
	km.Keys[k] = list
}

type Stack struct {
	stack []Keymap
}

// Push a new Keymap onto the stack. When a key is pressed, the top of the
// stack is checked first, and if it doesn't have a mapping then the next lower
// element is checked for a mapping
func (st *Stack) Push(m Keymap) {
	st.stack = append(st.stack, m)
}

// Pop element off the stack. If there is only one element in the stack
// this function does not remove it.
func (st *Stack) Pop() {
	if len(st.stack) == 1 {
		return
	}

	st.stack = st.stack[:len(st.stack)-1]
}

func (st *Stack) Top() *Keymap {
	if len(st.stack) == 0 {
		return nil
	}

	return &st.stack[len(st.stack)-1]
}

func (st *Stack) Get(name string, modifiers key.Modifiers) (iter ActionIter, found bool) {

	for i := len(st.stack) - 1; i >= 0; i-- {
		list, ok := st.stack[i].Keys[Key{name, modifiers}]
		if ok {
			iter = ActionIter{stack: st, actions: list}
			found = true
			return
		}

		if st.stack[i].Default != nil {
			iter = ActionIter{stack: st, actions: st.stack[i].Default}
			found = true
			return
		}

		if !st.stack[i].Fallthrough {
			return
		}
	}
	return
}

func (st *Stack) String() string {
	var buf bytes.Buffer
	for i, k := range st.stack {
		fmt.Fprintf(&buf, "Keymap %d: %s\n", i, k)
	}
	return buf.String()
}

func (st *Stack) Clone() *Stack {
	result := new(Stack)

	result.stack = make([]Keymap, len(st.stack))
	copy(result.stack, st.stack)
	return result
}

// expand converts a single key event into a series of key events with
// each combination of modifiers. For example, if
//func (st *Stack) expand(name string, modifiers key.Modifiers) []Key {
//
//}

type ActionIter struct {
	stack   *Stack
	actions []Action
	// index - 1 points to current item in actions
	index int
}

// Move to the next item in the iterator. The iterator starts before the first item, so this
// must be called before calling Item() the first time. Internally this function
// handles the special actions.
func (iter *ActionIter) Next() bool {
	iter.index++
	if iter.index > len(iter.actions) {
		return false
	}

	if iter.Item() == Pop {
		iter.stack.Pop()
		return iter.Next()
	}

	if iter.Item() == Halt {
		iter.index--
		return false
	}
	return true
}

func (iter ActionIter) Item() Action {
	if iter.index < 1 {
		return nil
	}
	return iter.actions[iter.index-1]
}

func (k Key) String() string {
	var buf bytes.Buffer

	if k.Modifiers.Contain(key.ModCtrl) {
		buf.WriteRune('C')
	}
	if k.Modifiers.Contain(key.ModShift) {
		buf.WriteRune('S')
	}
	if k.Modifiers.Contain(key.ModAlt) {
		buf.WriteRune('A')
	}
	if k.Modifiers.Contain(key.ModCommand) {
		buf.WriteRune('K')
	}

	if buf.Len() > 0 {
		buf.WriteRune('-')
	}

	buf.WriteString(k.Name)
	return buf.String()
}
