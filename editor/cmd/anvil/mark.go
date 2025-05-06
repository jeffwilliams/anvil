package main

import (
	"bytes"
	"fmt"
	"sort"
)

type Marks struct {
	marks map[string]*MarkPosition
}

type MarkPosition struct {
	DisplayPath *GlobalPath
	LoadPath    *GlobalPath
	Index       int
}

func (m *Marks) Set(markName string, displayPath, loadPath *GlobalPath, index int) {
	if m.marks == nil {
		m.marks = make(map[string]*MarkPosition)
	}
	m.marks[markName] = &MarkPosition{displayPath, loadPath, index}
}

func (m *Marks) Unset(name string) {
	if m.marks == nil {
		return
	}
	delete(m.marks, name)
}

func (m *Marks) Clear() {
	if m.marks == nil {
		return
	}
	m.marks = make(map[string]*MarkPosition)
}

func (m *Marks) Seek(name string) (displayPath, loadPath *GlobalPath, goTo seek, ok bool) {
	if m.marks == nil {
		return
	}

	var pos *MarkPosition
	pos, ok = m.marks[name]
	if !ok {
		return
	}

	displayPath = pos.DisplayPath
	loadPath = pos.LoadPath
	goTo = seek{
		seekType: seekToRunePos,
		runePos:  pos.Index,
	}

	return
}

func (m *Marks) String() string {
	if m.marks == nil {
		return ""
	}

	keys := make([][2]string, len(m.marks))

	i := 0
	for k, v := range m.marks {
		keys[i][0] = k
		keys[i][1] = fmt.Sprintf("%s#%d", v.DisplayPath, v.Index)
		i++
	}

	sort.Slice(keys, func(a, b int) bool {
		return keys[a][0] < keys[b][0]
	})

	var buf bytes.Buffer
	for _, v := range keys {
		fmt.Fprintf(&buf, "Goto %s\n\t%s\n", v[0], v[1])
	}

	return buf.String()
}

type MarkState struct {
	Marks map[string]*MarkPositionState
}

type MarkPositionState struct {
	LoadPath    string
	DisplayPath string
	DirState    GlobalPathDirState
	Index       int
}

func (m *Marks) State() MarkState {
	var ms MarkState
	ms.Marks = make(map[string]*MarkPositionState)

	for k, v := range m.marks {
		ms.Marks[k] = &MarkPositionState{
			LoadPath:    v.LoadPath.String(),
			DisplayPath: v.DisplayPath.String(),
			DirState:    v.LoadPath.DirState(),
			Index:       v.Index,
		}
	}

	return ms
}

func (m *Marks) SetState(state MarkState) {
	m.marks = make(map[string]*MarkPosition)
	for k, v := range state.Marks {
		displayPath := NewGlobalPath(v.DisplayPath, v.DirState)
		loadPath := NewGlobalPath(v.LoadPath, v.DirState)

		m.marks[k] = &MarkPosition{
			DisplayPath: displayPath,
			LoadPath:    loadPath,
			Index:       v.Index,
		}
	}
}

func (m *Marks) ShiftDueToTextModification(loadPath *GlobalPath, startOfChange, lengthOfChange int) {
	if loadPath == nil {
		return
	}
	lp := loadPath.String()
	for _, mark := range m.marks {
		if mark.LoadPath.String() == lp {
			if mark.Index >= startOfChange {
				mark.Index += lengthOfChange
			}
		}
	}
}
