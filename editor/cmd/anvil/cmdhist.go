package main

import (
	"bytes"
	"fmt"
	"github.com/jeffwilliams/anvil/editor/internal/circ"
	"sort"
	"sync"
	"time"
)

type CommandHistory struct {
	cmds circ.Circ[*CommandHistoryEntry]
	lock sync.Mutex
	max  int
}

type CommandHistoryEntry struct {
	cmd         string
	started     time.Time
	ended       time.Time
	state       RunState
	dir         string
	exitCode    int
	exitCodeSet bool
}

type RunState int

const (
	Running RunState = iota
	Completed
	Orphaned
)

type Verbosity int

const (
	NotVerbose = iota
	Verbose
)

func (r RunState) String() string {
	switch r {
	case Running:
		return "running"
	case Completed:
		return "complete"
	case Orphaned:
		return "orphaned"
	default:
		return "unknown state"
	}
}

func NewCommandHistory(max int) *CommandHistory {
	return &CommandHistory{cmds: circ.New[*CommandHistoryEntry](max), max: max}
}

func (ch *CommandHistory) Started(dir, cmd string) *CommandHistoryEntry {
	ch.lock.Lock()
	defer ch.lock.Unlock()
	e := &CommandHistoryEntry{
		cmd:     cmd,
		started: time.Now(),
		state:   Running,
		dir:     dir,
	}
	ch.cmds.Add(e)
	return e
}

func (ch *CommandHistory) Completed(e *CommandHistoryEntry) {
	ch.lock.Lock()
	defer ch.lock.Unlock()
	e.ended = time.Now()
	e.state = Completed
}

func (ch *CommandHistory) SetExitCode(e *CommandHistoryEntry, c int) {
	ch.lock.Lock()
	defer ch.lock.Unlock()
	e.exitCode = c
	e.exitCodeSet = true
}

func (ch *CommandHistory) String(verbosity Verbosity) string {
	var buf bytes.Buffer

	ch.lock.Lock()
	defer ch.lock.Unlock()
	ch.cmds.Each(func(e *CommandHistoryEntry) {
		ss, es := ch.formatTimes(e.started, e.ended)
		dirString := ""
		cmdSuffix := ""
		if verbosity == Verbose && e.dir != "" {
			dirString = fmt.Sprintf("◊On %s ", e.dir)
			cmdSuffix = "◊"
		}
		exitCode := ""
		if e.exitCodeSet {
			exitCode = fmt.Sprintf("(exit %d)", e.exitCode)
		}

		switch e.state {
		case Completed:
			fmt.Fprintf(&buf, "[%s – %s]%s %s%s%s\n", ss, es, exitCode, dirString, e.cmd, cmdSuffix)
		case Orphaned:
			fmt.Fprintf(&buf, "[%s %s] %s%s%s\n", ss, e.state, dirString, e.cmd, cmdSuffix)
		case Running:
			fmt.Fprintf(&buf, "[%s %s] %s%s%s\n", ss, e.state, dirString, e.cmd, cmdSuffix)
		default:
			fmt.Fprintf(&buf, "[? ?] %s%s%s\n", dirString, e.cmd, cmdSuffix)
		}
	})

	return buf.String()
}

func (ch *CommandHistory) formatTimes(start, end time.Time) (startStr, endStr string) {
	var format string

	if ch.sameDate(start, time.Now()) {
		format = "15:04:05"
	} else {
		format = "2006-01-02 15:04:05"
	}

	startStr = start.Format(format)

	if ch.sameDate(start, end) {
		format = "15:04:05"
	} else {
		format = "2006-01-02 15:04:05"
	}

	endStr = end.Format(format)
	return
}

func (ch *CommandHistory) sameDate(t1, t2 time.Time) bool {
	return t1.YearDay() == t2.YearDay()
}

func (ch *CommandHistory) Merge(ch2 *CommandHistory) *CommandHistory {
	result := NewCommandHistory(ch.max)

	var sorted1, sorted2 []*CommandHistoryEntry

	ch.cmds.Each(func(v *CommandHistoryEntry) {
		sorted1 = append(sorted1, v)
	})

	ch2.cmds.Each(func(v *CommandHistoryEntry) {
		sorted2 = append(sorted2, v)
	})

	sort.Slice(sorted1, func(i, j int) bool {
		return sorted1[i].started.Before(sorted1[j].started)
	})

	sort.Slice(sorted2, func(i, j int) bool {
		return sorted2[i].started.Before(sorted2[j].started)
	})

	var i, j int
	for i < len(sorted1) && j < len(sorted2) {
		a := sorted1[i]
		b := sorted2[j]

		switch {
		case a.started.Before(b.started):
			result.cmds.Add(a)
			i++
		case b.started.Before(a.started):
			result.cmds.Add(b)
			j++
		case b.started.Equal(a.started):
			result.cmds.Add(b)
			i++
		}
	}

	for i < len(sorted1) {
		result.cmds.Add(sorted1[i])
		i++
	}

	for j < len(sorted2) {
		result.cmds.Add(sorted2[j])
		j++
	}

	return result
}
