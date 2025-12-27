package main

// Select keyActions slice definition:
// ◊!/^var.keyActions.=/;/\n}/◊

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"github.com/jeffwilliams/anvil/editor/internal/keymap"
	"github.com/jeffwilliams/anvil/editor/internal/runes"
)

type keyHandler func(gtx layout.Context, e *editable, ev *key.Event, args []string, state actionProcessingState) (result keyHandlerResult)

type keyHandlerResult struct {
	resetWordCompletions       Trilean
	resetFileCompletions       Trilean
	resetCommandCompletions    Trilean
	clearRecentlyTypedText     Trilean
	clearLastKeypressWasSearch Trilean
	handled                    bool
	stopProcessingActions      bool
}

type actionProcessingState struct {
	lastHandlerReportedHandled bool
}

type Trilean struct {
	val uint8
}

func (t Trilean) IsSet() bool {
	return t.val != 0
}

func (t *Trilean) Set(b bool) {
	t.val = 1
	if b {
		t.val = 2
	}
}

func (t Trilean) Val() (bool, error) {
	if !t.IsSet() {
		return false, fmt.Errorf("trilean is not set")
	}

	return t.val == 2, nil
}

func (t Trilean) MustVal() bool {
	return t.val == 2
}

type keyAction struct {
	name        string
	desc        string
	paramLabels []string
	handler     keyHandler
}

func (ka keyAction) String() string {
	return fmt.Sprintf("%s %s", ka.name, strings.Join(ka.paramLabels, " "))
}

var keyActions = []keyAction{
	{
		"newline", "Insert a newline at the current cursor position.",
		[]string{"no-indent"},
		func(gtx layout.Context, e *editable, ev *key.Event, args []string, state actionProcessingState) (result keyHandlerResult) {
			if slices.Contains(args, "no-indent") {
				e.InsertText("\n")
				return
			}

			if len(e.CursorIndices) == 1 {
				e.autoIndent()
			} else {
				e.InsertText("\n")
			}
			return
		},
	},
	{
		"execute-line", "Execute the line with the cursor as a command",
		[]string{},
		func(gtx layout.Context, e *editable, ev *key.Event, args []string, state actionProcessingState) (result keyHandlerResult) {
			w := runes.NewWalker(e.Bytes())
			w.SetRunePosCache(e.firstCursorIndex(), &e.runeOffsetCache)
			start, end := w.CurrentLineBounds()
			text := string(w.TextBetweenRuneIndices(start, end))
			text = strings.TrimSpace(text)
			if strings.HasPrefix(text, "◊") && strings.HasSuffix(text, "◊") {
				l := utf8.RuneLen('◊')
				text = text[l : len(text)-l]
			}

			if e.adapter.displayPath() != nil && IsErrorsWindow(e.adapter.displayPath().String()) {
				w := runes.NewWalker(e.Bytes())
				w.SetRunePosCache(e.firstCursorIndex(), &e.runeOffsetCache)
				if w.AtEnd() {
					e.InsertText("\n")
				}
			}

			e.adapter.execute(e, gtx, text, nil)
			return
		},
	},
	{
		"clear-errors", "Clear the errors window associated with the current window",
		[]string{},
		func(gtx layout.Context, e *editable, ev *key.Event, args []string, state actionProcessingState) (result keyHandlerResult) {
			e.adapter.clearErrors()
			return
		},
	},
	{
		"backspace", "Delete the rune before each cursor and move the cursors backwards one rune",
		[]string{},
		func(gtx layout.Context, e *editable, ev *key.Event, args []string, state actionProcessingState) (result keyHandlerResult) {
			if e.SelectionsPresent() {
				e.InsertText("")
				return
			}

			if len(e.CursorIndices) > 1 {
				e.SetSaveDeletes(false)
			}
			e.text.StartTransaction()
			for i, ndx := range e.CursorIndices {
				if ndx > 0 {
					e.CursorIndices[i]--
					e.deleteFromPieceTable(e.CursorIndices[i], 1)
					log(LogCatgEd, "Delete at %d of length %d\n", e.CursorIndices[i], 1)
				}
			}
			e.text.EndTransaction()
			e.SetSaveDeletes(true)
			return
		},
	},
	{
		"delete", "Delete the rune after each cursor",
		[]string{},
		func(gtx layout.Context, e *editable, ev *key.Event, args []string, state actionProcessingState) (result keyHandlerResult) {
			if e.SelectionsPresent() {
				e.InsertText("")
				return
			}

			for _, ndx := range e.CursorIndices {
				if ndx < e.text.Len() {
					e.deleteFromPieceTable(ndx, 1)
				}
			}
			return
		},
	},
	{
		"tab", "Insert the current tab string at each cursor position",
		[]string{},
		func(gtx layout.Context, e *editable, ev *key.Event, args []string, state actionProcessingState) (result keyHandlerResult) {
			e.InsertText(e.adapter.insertWhenTabPressed())
			return
		},
	},
	{
		"move-left-rune", "Move each cursor left one rune",
		[]string{},
		func(gtx layout.Context, e *editable, ev *key.Event, args []string, state actionProcessingState) (result keyHandlerResult) {
			if e.SelectionsPresent() {
				e.changeSelectionsToCursors(Left)
				return
			}

			var mis motionItems
			mis = newCursorsMotionItems(e)

			for _, mi := range mis.items() {
				if mi.position() > 0 {
					p := mi.position()
					p--
					mi.setPosition(p)
				}
			}
			mis.doneAdjusting(gtx)
			return
		},
	},
	{
		"select-left-rune", "Extend each selection to the left one rune",
		[]string{},
		func(gtx layout.Context, e *editable, ev *key.Event, args []string, state actionProcessingState) (result keyHandlerResult) {
			var mis motionItems
			mis = newSelectionMotionItems(e, Left)

			for _, mi := range mis.items() {
				if mi.position() > 0 {
					p := mi.position()
					p--
					mi.setPosition(p)
				}
			}
			mis.doneAdjusting(gtx)
			return
		},
	},
	{
		"move-left-word", "Move each cursor left one space-separated word",
		[]string{},
		func(gtx layout.Context, e *editable, ev *key.Event, args []string, state actionProcessingState) (result keyHandlerResult) {
			if e.SelectionsPresent() && !ev.Modifiers.Contain(key.ModShift) {
				e.changeSelectionsToCursors(Left)
				return
			}

			var mis motionItems
			mis = newCursorsMotionItems(e)

			if e.text.Len() > 0 {
				w := runes.NewWalker(e.Bytes())
				for _, mi := range mis.items() {
					w.SetRunePosCache(mi.position(), &e.runeOffsetCache)
					w.BackwardToWordStart()
					mi.setPosition(w.RunePos())
				}
				mis.doneAdjusting(gtx)
				return
			}

			for _, mi := range mis.items() {
				if mi.position() > 0 {
					p := mi.position()
					p--
					mi.setPosition(p)
				}
			}
			mis.doneAdjusting(gtx)
			return
		},
	},
	{
		"move-left-chunk", "Move each cursor left one chunk",
		[]string{},
		func(gtx layout.Context, e *editable, ev *key.Event, args []string, state actionProcessingState) (result keyHandlerResult) {
			if e.SelectionsPresent() && !ev.Modifiers.Contain(key.ModShift) {
				e.changeSelectionsToCursors(Left)
				return
			}

			var mis motionItems
			mis = newCursorsMotionItems(e)

			if e.text.Len() > 0 {
				w := runes.NewWalker(e.Bytes())
				for _, mi := range mis.items() {
					w.SetRunePosCache(mi.position(), &e.runeOffsetCache)
					w.BackwardToChunkStart()
					mi.setPosition(w.RunePos())
				}
				mis.doneAdjusting(gtx)
				return
			}

			for _, mi := range mis.items() {
				if mi.position() > 0 {
					p := mi.position()
					p--
					mi.setPosition(p)
				}
			}
			mis.doneAdjusting(gtx)
			return
		},
	},
	{
		"select-left-word", "Extend each selection to the left one word",
		[]string{},
		func(gtx layout.Context, e *editable, ev *key.Event, args []string, state actionProcessingState) (result keyHandlerResult) {
			var mis motionItems
			mis = newSelectionMotionItems(e, Left)

			if e.text.Len() > 0 {
				w := runes.NewWalker(e.Bytes())
				for _, mi := range mis.items() {
					w.SetRunePosCache(mi.position(), &e.runeOffsetCache)
					w.BackwardToWordStart()
					mi.setPosition(w.RunePos())
				}
				mis.doneAdjusting(gtx)
				return
			}

			for _, mi := range mis.items() {
				if mi.position() > 0 {
					p := mi.position()
					p--
					mi.setPosition(p)
				}
			}
			mis.doneAdjusting(gtx)
			return
		},
	},
	{
		"select-left-chunk", "Extend each selection to the left one chunk",
		[]string{},
		func(gtx layout.Context, e *editable, ev *key.Event, args []string, state actionProcessingState) (result keyHandlerResult) {
			var mis motionItems
			mis = newSelectionMotionItems(e, Left)

			if e.text.Len() > 0 {
				w := runes.NewWalker(e.Bytes())
				for _, mi := range mis.items() {
					w.SetRunePosCache(mi.position(), &e.runeOffsetCache)
					w.BackwardToChunkStart()
					mi.setPosition(w.RunePos())
				}
				mis.doneAdjusting(gtx)
				return
			}

			for _, mi := range mis.items() {
				if mi.position() > 0 {
					p := mi.position()
					p--
					mi.setPosition(p)
				}
			}
			mis.doneAdjusting(gtx)
			return
		},
	},
	{
		"move-right-rune", "Move each cursor right one rune",
		[]string{},
		func(gtx layout.Context, e *editable, ev *key.Event, args []string, state actionProcessingState) (result keyHandlerResult) {
			if e.SelectionsPresent() {
				e.changeSelectionsToCursors(Right)
				return
			}

			var mis motionItems
			mis = newCursorsMotionItems(e)

			for _, mi := range mis.items() {
				if mi.position() < e.text.Len() {
					p := mi.position()
					p++
					mi.setPosition(p)
				}
			}
			mis.doneAdjusting(gtx)
			return
		},
	},
	{
		"select-right-rune", "Extend each selection to the right one rune",
		[]string{},
		func(gtx layout.Context, e *editable, ev *key.Event, args []string, state actionProcessingState) (result keyHandlerResult) {
			var mis motionItems
			mis = newSelectionMotionItems(e, Right)

			for _, mi := range mis.items() {
				if mi.position() < e.text.Len() {
					p := mi.position()
					p++
					mi.setPosition(p)
				}
			}
			mis.doneAdjusting(gtx)
			return
		},
	},
	{
		"move-right-word", "Move each cursor right one space-separated word",
		[]string{},
		func(gtx layout.Context, e *editable, ev *key.Event, args []string, state actionProcessingState) (result keyHandlerResult) {
			if e.SelectionsPresent() && !ev.Modifiers.Contain(key.ModShift) {
				e.changeSelectionsToCursors(Right)
				return
			}

			var mis motionItems
			mis = newCursorsMotionItems(e)

			if e.text.Len() > 0 {
				w := runes.NewWalker(e.Bytes())
				for _, mi := range mis.items() {
					w.SetRunePosCache(mi.position(), &e.runeOffsetCache)
					w.ForwardToStartOfNextWord()
					mi.setPosition(w.RunePos())
				}
				mis.doneAdjusting(gtx)
				return
			}

			for _, mi := range mis.items() {
				if mi.position() < e.text.Len() {
					p := mi.position()
					p++
					mi.setPosition(p)
				}
			}
			mis.doneAdjusting(gtx)
			return
		},
	},
	{
		"move-right-chunk", "Move each cursor right one chunk",
		[]string{},
		func(gtx layout.Context, e *editable, ev *key.Event, args []string, state actionProcessingState) (result keyHandlerResult) {
			if e.SelectionsPresent() && !ev.Modifiers.Contain(key.ModShift) {
				e.changeSelectionsToCursors(Left)
				return
			}

			var mis motionItems
			mis = newCursorsMotionItems(e)

			if e.text.Len() > 0 {
				w := runes.NewWalker(e.Bytes())
				for _, mi := range mis.items() {
					w.SetRunePosCache(mi.position(), &e.runeOffsetCache)
					w.ForwardToChunkEnd()
					mi.setPosition(w.RunePos())
				}
				mis.doneAdjusting(gtx)
				return
			}

			for _, mi := range mis.items() {
				if mi.position() > 0 {
					p := mi.position()
					p--
					mi.setPosition(p)
				}
			}
			mis.doneAdjusting(gtx)
			return
		},
	},
	{
		"select-right-word", "Extend each selection to the right one word",
		[]string{},
		func(gtx layout.Context, e *editable, ev *key.Event, args []string, state actionProcessingState) (result keyHandlerResult) {
			var mis motionItems
			mis = newSelectionMotionItems(e, Right)

			if e.text.Len() > 0 {
				w := runes.NewWalker(e.Bytes())
				for _, mi := range mis.items() {
					w.SetRunePosCache(mi.position(), &e.runeOffsetCache)
					w.ForwardToStartOfNextWord()
					mi.setPosition(w.RunePos())
				}
				mis.doneAdjusting(gtx)
				return
			}

			for _, mi := range mis.items() {
				if mi.position() < e.text.Len() {
					p := mi.position()
					p++
					mi.setPosition(p)
				}
			}
			mis.doneAdjusting(gtx)
			return
		},
	},
	{
		"select-right-chunk", "Extend each selection to the right one chunk",
		[]string{},
		func(gtx layout.Context, e *editable, ev *key.Event, args []string, state actionProcessingState) (result keyHandlerResult) {
			var mis motionItems
			mis = newSelectionMotionItems(e, Right)

			if e.text.Len() > 0 {
				w := runes.NewWalker(e.Bytes())
				for _, mi := range mis.items() {
					w.SetRunePosCache(mi.position(), &e.runeOffsetCache)
					w.ForwardToChunkEnd()
					mi.setPosition(w.RunePos())
				}
				mis.doneAdjusting(gtx)
				return
			}

			for _, mi := range mis.items() {
				if mi.position() < e.text.Len() {
					p := mi.position()
					p++
					mi.setPosition(p)
				}
			}
			mis.doneAdjusting(gtx)
			return
		},
	},
	{
		"move-up", "Move each cursor up one line",
		[]string{},
		func(gtx layout.Context, e *editable, ev *key.Event, args []string, state actionProcessingState) (result keyHandlerResult) {
			if e.SelectionsPresent() {
				e.changeSelectionsToCursors(Left)
				return
			}

			var mis motionItems
			mis = newCursorsMotionItems(e)

			w := runes.NewWalker(e.Bytes())
			for _, mi := range mis.items() {
				w.SetRunePosCache(mi.position(), &e.runeOffsetCache)
				li := w.IndexInLine()
				w.BackwardToStartOfLine()
				w.Backward(1)
				w.BackwardToStartOfLine()
				if li >= w.LineLen() {
					li = w.LineLen() - 1
				}
				w.Forward(li)
				mi.setPosition(w.RunePos())
			}
			mis.doneAdjusting(gtx)
			return
		},
	},
	{
		"select-up", "Extend each selection up one line",
		[]string{},
		func(gtx layout.Context, e *editable, ev *key.Event, args []string, state actionProcessingState) (result keyHandlerResult) {
			var mis motionItems
			mis = newSelectionMotionItems(e, Right)

			w := runes.NewWalker(e.Bytes())
			for _, mi := range mis.items() {
				w.SetRunePosCache(mi.position(), &e.runeOffsetCache)
				li := w.IndexInLine()
				w.BackwardToStartOfLine()
				w.Backward(1)
				w.BackwardToStartOfLine()
				if li >= w.LineLen() {
					li = w.LineLen() - 1
				}
				w.Forward(li)
				mi.setPosition(w.RunePos())
			}
			mis.doneAdjusting(gtx)
			return
		},
	},
	{
		"new-cursor-above-first", "Create a new cursor above the highest cursor",
		[]string{},
		func(gtx layout.Context, e *editable, ev *key.Event, args []string, state actionProcessingState) (result keyHandlerResult) {
			e.AddNewCursorAboveFirst()
			return
		},
	},
	{
		"move-down", "Move each cursor down one line",
		[]string{},
		func(gtx layout.Context, e *editable, ev *key.Event, args []string, state actionProcessingState) (result keyHandlerResult) {
			if e.SelectionsPresent() {
				e.changeSelectionsToCursors(Right)
				return
			}

			var mis motionItems
			mis = newCursorsMotionItems(e)

			w := runes.NewWalker(e.Bytes())
			for _, mi := range mis.items() {
				w.SetRunePosCache(mi.position(), &e.runeOffsetCache)
				li := w.IndexInLine()
				w.ForwardToEndOfLine()
				w.Forward(1)
				if li >= w.LineLen() {
					li = w.LineLen() - 1
				}
				w.Forward(li)
				mi.setPosition(w.RunePos())
			}
			mis.doneAdjusting(gtx)
			return
		},
	},
	{
		"select-down", "Extend each selection down one line",
		[]string{},
		func(gtx layout.Context, e *editable, ev *key.Event, args []string, state actionProcessingState) (result keyHandlerResult) {
			var mis motionItems
			mis = newSelectionMotionItems(e, Right)

			w := runes.NewWalker(e.Bytes())
			for _, mi := range mis.items() {
				w.SetRunePosCache(mi.position(), &e.runeOffsetCache)
				li := w.IndexInLine()
				w.ForwardToEndOfLine()
				w.Forward(1)
				if li >= w.LineLen() {
					li = w.LineLen() - 1
				}
				w.Forward(li)
				mi.setPosition(w.RunePos())
			}
			mis.doneAdjusting(gtx)
			return
		},
	},
	{
		"new-cursor-below-last", "Create a new cursor below the lowest cursor",
		[]string{},
		func(gtx layout.Context, e *editable, ev *key.Event, args []string, state actionProcessingState) (result keyHandlerResult) {
			e.AddNewCursorBelowLast()
			return
		},
	},
	{
		"move-to-eol", "Move each cursor to the end of the line",
		[]string{},
		func(gtx layout.Context, e *editable, ev *key.Event, args []string, state actionProcessingState) (result keyHandlerResult) {
			if e.SelectionsPresent() {
				e.clearSelections()
			}

			var mis motionItems
			mis = newCursorsMotionItems(e)

			w := runes.NewWalker(e.Bytes())
			for _, mi := range mis.items() {
				w.SetRunePosCache(mi.position(), &e.runeOffsetCache)
				w.ForwardToEndOfLine()
				mi.setPosition(w.RunePos())
			}
			mis.doneAdjusting(gtx)
			return
		},
	},
	{
		"select-to-eol", "Extend each selection to the end of the line",
		[]string{},
		func(gtx layout.Context, e *editable, ev *key.Event, args []string, state actionProcessingState) (result keyHandlerResult) {

			var mis motionItems
			mis = newSelectionMotionItems(e, Right)

			w := runes.NewWalker(e.Bytes())
			for _, mi := range mis.items() {
				w.SetRunePosCache(mi.position(), &e.runeOffsetCache)
				w.ForwardToEndOfLine()
				mi.setPosition(w.RunePos())
			}
			mis.doneAdjusting(gtx)
			return
		},
	},
	{
		"move-to-eof", "Reduce cursors to one and move it to the end of the file",
		[]string{},
		func(gtx layout.Context, e *editable, ev *key.Event, args []string, state actionProcessingState) (result keyHandlerResult) {
			if e.text.Len() > 0 {
				e.moveToEndOfDoc(gtx)
			}
			return
		},
	},
	{
		"select-to-eof", "Reduce cursors to one and select to the end of the file",
		[]string{},
		func(gtx layout.Context, e *editable, ev *key.Event, args []string, state actionProcessingState) (result keyHandlerResult) {
			if e.text.Len() > 0 {
				from := e.firstCursorIndex()
				e.moveToEndOfDoc(gtx)
				e.addSecondarySelection(e.firstCursorIndex(), from, Right)
			}
			return
		},
	},
	{
		"move-to-sol", "Reduce cursors to one and move it to the start of the file",
		[]string{},
		func(gtx layout.Context, e *editable, ev *key.Event, args []string, state actionProcessingState) (result keyHandlerResult) {
			if e.SelectionsPresent() {
				e.clearSelections()
			}

			var mis motionItems
			mis = newCursorsMotionItems(e)

			w := runes.NewWalker(e.Bytes())
			for _, mi := range mis.items() {
				w.SetRunePosCache(mi.position(), &e.runeOffsetCache)
				w.BackwardToStartOfLine()
				mi.setPosition(w.RunePos())
			}
			mis.doneAdjusting(gtx)
			return
		},
	},
	{
		"select-to-sol", "Extend each selection to the start of the line",
		[]string{},
		func(gtx layout.Context, e *editable, ev *key.Event, args []string, state actionProcessingState) (result keyHandlerResult) {

			var mis motionItems
			mis = newSelectionMotionItems(e, Left)

			w := runes.NewWalker(e.Bytes())
			for _, mi := range mis.items() {
				w.SetRunePosCache(mi.position(), &e.runeOffsetCache)
				w.BackwardToStartOfLine()
				mi.setPosition(w.RunePos())
			}
			mis.doneAdjusting(gtx)
			return
		},
	},
	{
		"move-to-sof", "Reduce cursors to one and move it to the start of the file",
		[]string{},
		func(gtx layout.Context, e *editable, ev *key.Event, args []string, state actionProcessingState) (result keyHandlerResult) {
			if e.text.Len() > 0 {
				e.setToOneCursorIndex(0)
				e.makeCursorVisibleByScrolling(gtx)
			}
			return
		},
	},
	{
		"select-to-sof", "Reduce cursors to one and select to the start of the file",
		[]string{},
		func(gtx layout.Context, e *editable, ev *key.Event, args []string, state actionProcessingState) (result keyHandlerResult) {
			if e.text.Len() > 0 {
				from := e.firstCursorIndex()
				e.setToOneCursorIndex(0)
				e.makeCursorVisibleByScrolling(gtx)
				e.addSecondarySelection(e.firstCursorIndex(), from, Left)
			}
			return
		},
	},
	{
		"move-page-down", "Move the viewport down one page of text",
		[]string{},
		func(gtx layout.Context, e *editable, ev *key.Event, args []string, state actionProcessingState) (result keyHandlerResult) {
			e.ScrollOnePage(gtx, Down)
			return
		},
	},
	{
		"move-page-up", "Move the viewport up one page of text",
		[]string{},
		func(gtx layout.Context, e *editable, ev *key.Event, args []string, state actionProcessingState) (result keyHandlerResult) {
			e.ScrollOnePage(gtx, Up)
			return
		},
	},
	{
		"undo", "Undo the last change",
		[]string{},
		func(gtx layout.Context, e *editable, ev *key.Event, args []string, state actionProcessingState) (result keyHandlerResult) {
			if e.matchingBracketInsertion.Undo(gtx, e) {
				return
			}
			e.Undo(gtx)
			return
		},
	},
	{
		"redo", "Redo the last undone change",
		[]string{},
		func(gtx layout.Context, e *editable, ev *key.Event, args []string, state actionProcessingState) (result keyHandlerResult) {
			e.Redo(gtx)
			result.clearRecentlyTypedText.Set(true)
			return
		},
	},
	{
		"scroll-line-up", "Move the viewport up one line",
		[]string{},
		func(gtx layout.Context, e *editable, ev *key.Event, args []string, state actionProcessingState) (result keyHandlerResult) {
			e.ScrollOneLine(gtx, Up)
			return
		},
	},
	{
		"scroll-line-down", "Move the viewport down one line",
		[]string{},
		func(gtx layout.Context, e *editable, ev *key.Event, args []string, state actionProcessingState) (result keyHandlerResult) {
			e.ScrollOneLine(gtx, Down)
			return
		},
	},
	{
		"complete-word", "Reduce cursors to one and complete the word ending at the cursor",
		[]string{},
		func(gtx layout.Context, e *editable, ev *key.Event, args []string, state actionProcessingState) (result keyHandlerResult) {
			if len(e.CursorIndices) != 1 {
				return
			}
			result.resetWordCompletions.Set(false)
			ndx := e.firstCursorIndex()
			ctx := e.wordObjectToComplete(ndx)
			e.doWordCompletion(ctx, Forward)
			result.clearRecentlyTypedText.Set(true)
			return
		},
	},
	{
		"previous-completion", "If the last action was a completion, move to the previous completion in the list",
		[]string{},
		func(gtx layout.Context, e *editable, ev *key.Event, args []string, state actionProcessingState) (result keyHandlerResult) {
			if len(e.CursorIndices) != 1 {
				return
			}
			if e.wordCompletion.isCompletionInProgress() {
				result.resetWordCompletions.Set(false)
				ndx := e.firstCursorIndex()
				ctx := e.wordObjectToComplete(ndx)
				e.doWordCompletion(ctx, Reverse)
			}

			if e.fileCompletion.isCompletionInProgress() {
				result.resetFileCompletions.Set(false)
				ndx := e.firstCursorIndex()
				ctx := e.filenameObjectToComplete(ndx)
				e.doFilenameCompletion(ctx, Reverse)
			}

			if e.commandCompletion.isCompletionInProgress() {
				result.resetCommandCompletions.Set(false)
				ndx := e.firstCursorIndex()
				ctx := e.filenameObjectToComplete(ndx)
				e.doCommandCompletion(ctx, Reverse)
			}
			result.clearRecentlyTypedText.Set(true)
			return
		},
	},
	{
		"complete-file", "Reduce cursors to one and complete the file path ending at the cursor",
		[]string{},
		func(gtx layout.Context, e *editable, ev *key.Event, args []string, state actionProcessingState) (result keyHandlerResult) {
			if len(e.CursorIndices) != 1 {
				return
			}
			result.resetFileCompletions.Set(false)
			ndx := e.firstCursorIndex()
			ctx := e.filenameObjectToComplete(ndx)
			e.doFilenameCompletion(ctx, Forward)
			result.clearRecentlyTypedText.Set(true)
			return
		},
	},
	{
		"complete-command", "Reduce cursors to one and complete the command ending at the cursor",
		[]string{},
		func(gtx layout.Context, e *editable, ev *key.Event, args []string, state actionProcessingState) (result keyHandlerResult) {
			if len(e.CursorIndices) != 1 {
				return
			}
			result.resetCommandCompletions.Set(false)
			ndx := e.firstCursorIndex()
			ctx := e.filenameObjectToComplete(ndx)
			e.doCommandCompletion(ctx, Forward)
			result.clearRecentlyTypedText.Set(true)
			return
		},
	},
	{
		"put", "Put the current file",
		[]string{},
		func(gtx layout.Context, e *editable, ev *key.Event, args []string, state actionProcessingState) (result keyHandlerResult) {
			e.adapter.put()
			return
		},
	},
	{
		"get", "Get the current file",
		[]string{},
		func(gtx layout.Context, e *editable, ev *key.Event, args []string, state actionProcessingState) (result keyHandlerResult) {
			e.adapter.get()
			return
		},
	},
	{
		"acquire", "Acquire the word under the cursor, the lozenge-delimited text surrounding the cursor, or the current selection. If the first argument is set to 'same-window' then the item is acquired in the same window instead of creating a new window",
		[]string{"location"},
		func(gtx layout.Context, e *editable, ev *key.Event, args []string, state actionProcessingState) (result keyHandlerResult) {
			ndx := e.firstCursorIndex()
			if e.primarySel != nil && ndx == e.primarySel.End() {
				// As a special case, if the cursor is just after the end of the primary
				// selection likely the user wants to execute the primary selection. They
				// might have just typed some text, hit Escape to select it, and are using
				// Enter to execute it.
				ndx--
			}
			howToLoad := loadFileInSeparateWindow
			if slices.Contains(args, "same-window") {
				howToLoad = loadFileInCurrentWindow
			}
			e.acquire(gtx, ndx, howToLoad)

			return
		},
	},
	{
		"copy", "Copy the selected text to the clipboard",
		[]string{},
		func(gtx layout.Context, e *editable, ev *key.Event, args []string, state actionProcessingState) (result keyHandlerResult) {
			e.adapter.copyAllSelectionsFromLastSelectedEditable(gtx)
			return
		},
	},
	{
		"cut", "Copy the selected text to the clipboard then delete the selected text",
		[]string{},
		func(gtx layout.Context, e *editable, ev *key.Event, args []string, state actionProcessingState) (result keyHandlerResult) {
			e.adapter.cutAllSelectionsFromLastSelectedEditable(gtx)
			result.clearRecentlyTypedText.Set(true)
			return
		},
	},
	{
		"paste", "Insert the text from the clipboard at each cursor position or selection",
		[]string{},
		func(gtx layout.Context, e *editable, ev *key.Event, args []string, state actionProcessingState) (result keyHandlerResult) {
			e.adapter.pasteToFocusedEditable(gtx)
			return
		},
	},
	{
		"add-lozenge", "Insert a lozenge character at each cursor, or surround each selection with a lozenge",
		[]string{},
		func(gtx layout.Context, e *editable, ev *key.Event, args []string, state actionProcessingState) (result keyHandlerResult) {
			e.InsertLozenge()
			return
		},
	},
	{
		"execute", "Execute the word under the cursor, the lozenge-delimited text surrounding the cursor, or the current selection",
		[]string{},
		func(gtx layout.Context, e *editable, ev *key.Event, args []string, state actionProcessingState) (result keyHandlerResult) {
			ndx := e.firstCursorIndex()
			if e.primarySel != nil && ndx == e.primarySel.End() {
				// As a special case, if the cursor is just after the end of the primary
				// selection likely the user wants to execute the primary selection. They
				// might have just typed some text, hit Escape to select it, and are using
				// Enter to execute it.
				ndx--
			}
			t, _ := e.textObjectForExecutionAt(ndx)
			if t != "" {
				if ev.Modifiers.Contain(key.ModAlt) {
					e.adapter.clearErrors()
				}
				e.adapter.execute(e, gtx, t, nil)
			}
			result.clearRecentlyTypedText.Set(true)

			return
		},
	},
	{
		"search", "Search for the word under the cursor, the lozenge-delimited text surrounding the cursor, or the current selection. If the first argument is set to 'reverse' then the search is performed backwards",
		[]string{"direction"},
		func(gtx layout.Context, e *editable, ev *key.Event, args []string, state actionProcessingState) (result keyHandlerResult) {
			ndx := e.firstCursorIndex()
			if e.primarySel != nil && ndx == e.primarySel.End() {
				// As a special case, if the cursor is just after the end of the primary
				// selection likely the user wants to execute the primary selection. They
				// might have just typed some text, hit Escape to select it, and are using
				// Enter to execute it.
				ndx--
			}

			dir := Forward
			if slices.Contains(args, "reverse") {
				dir = Reverse
			}

			if e.lastKeypressWasSearch {
				e.ContinueSearch(gtx, dir)
			} else {
				t, _ := e.textObjectForSearchAt(ndx)
				if t != "" {

					// The behavour here is subtle. Imagine the user entered a regex in the tag to search for, and hit CTRL-/ multiple times.
					// We want it to behave like the right clicked multiple times: find the first match of the regex and select it, then
					// find the next match and select that as well, and so on. We also want the keyboard focus to shift to the Body so once
					// they have selected the items they want they can manipulate them with the keyboard.
					//
					// So the first time the user hits CTRL-/ in the Tag, and we start a new search, select the match, set the keyboard
					// focus to the body, and record in the body the search term and flag that a search is in progress. The next time CTRL-/
					// is pressed, the event is processed by the body, which realizes a search is in progress and continues the search by
					// finding the next match. The body handles the remaining keypresses in this way.
					//
					// In the Shift keypress handler below, we don't clear the flag that the last keypress was a search. This is so
					// the user can search forwards with CTRL-/ and then backwards for the same term with CTRL-SHIFT-/ (aka ?): pressing
					// the shift key alone must _not_ reset the search.
					e.SearchAndUpdateEditable(gtx, t, e.executeOn.firstCursorIndex(), dir)
					e.executeOn.lastSearchTerm = t
				}
			}
			e.executeOn.lastKeypressWasSearch = true
			result.clearLastKeypressWasSearch.Set(false)
			result.clearRecentlyTypedText.Set(true)
			return
		},
	},
	{
		"select-all", "Select all the text in the window body",
		[]string{},
		func(gtx layout.Context, e *editable, ev *key.Event, args []string, state actionProcessingState) (result keyHandlerResult) {
			e.selectAll()
			result.clearRecentlyTypedText.Set(true)
			return
		},
	},
	{
		"delimit-with-cursors", "Selimit each selection with cursors",
		[]string{},
		func(gtx layout.Context, e *editable, ev *key.Event, args []string, state actionProcessingState) (result keyHandlerResult) {
			e.DelimitSelectionsWithCursors()
			return
		},
	},
	{
		"delete-line", "Delete the line containing each cursor",
		[]string{},
		func(gtx layout.Context, e *editable, ev *key.Event, args []string, state actionProcessingState) (result keyHandlerResult) {
			if e.SelectionsPresent() {
				return
			}

			e.text.StartTransaction()
			for i, ndx := range e.CursorIndices {
				w := runes.NewWalker(e.Bytes())
				w.SetRunePosCache(ndx, &e.runeOffsetCache)
				start, end := w.CurrentLineBounds()
				if start != end {
					e.CursorIndices[i] = start
					e.deleteFromPieceTableUndoIndex(start, end-start, ndx)
				}
			}
			e.text.EndTransaction()
			result.clearRecentlyTypedText.Set(true)

			return
		},
	},
	{
		"delete-to-eol", "Delete from each cursor to the end of the line",
		[]string{},
		func(gtx layout.Context, e *editable, ev *key.Event, args []string, state actionProcessingState) (result keyHandlerResult) {
			if e.SelectionsPresent() {
				return
			}

			e.text.StartTransaction()
			for _, ndx := range e.CursorIndices {
				w := runes.NewWalker(e.Bytes())
				w.SetRunePosCache(ndx, &e.runeOffsetCache)
				w.ForwardToEndOfLine()
				p := w.RunePos()
				if ndx != p {
					e.deleteFromPieceTableUndoIndex(ndx, p-ndx, ndx)
				}
			}
			e.text.EndTransaction()
			result.clearRecentlyTypedText.Set(true)

			return
		},
	},
	{
		"pointer-cut-or-exec-with-arg", "If the right pointer button is currently held down after making a selection, cut that selection. If the middle pointer button is currently held down after making a selection, execute that selection with the last selected text as an argument",
		[]string{},
		func(gtx layout.Context, e *editable, ev *key.Event, args []string, state actionProcessingState) (result keyHandlerResult) {
			result.resetWordCompletions.Set(false)
			result.resetFileCompletions.Set(false)
			if e.pointerState.pressedButtons.Contain(pointer.ButtonPrimary) {
				e.adapter.cutAllSelectionsFromLastSelectedEditable(gtx)
				return
			}

			/* This code is written this way to handle a specific corner case. Imagine this sequence:
			   1. The user selects text in window 1. The keyboard focus is changed to window 1.
				 2. The user middle-clicks a word or selection in window 2. The keyboard focus remains in window 1.
				 3. The user clicks Ctrl. The keypress is handled by window 1.
				 Thus, when handling the Ctrl keypress in window 1, we need to find out which window
				 the middle-click occurred in (window 2), and also the information about that past middle-click
				 (i.e. the location) and execute the word or selection in window 2 where that middle-click
				 occurred.
			*/
			if ed := e.adapter.getEditableWhereTertiaryButtonHoldStarted(); ed != nil {
				log(LogCatgEd, "Ctrl was pressed while tertiary mouse button was pressed\n")
				ed.executeSelectedWithAllSelectionsInLastSelectedEditable(&ed.pointerState)
				ed.ignoreTertiaryRelease = true
			}
			return
		},
	},
	{
		"pointer-paste", "If the left pointer button is held down, paste at that position",
		[]string{},
		func(gtx layout.Context, e *editable, ev *key.Event, args []string, state actionProcessingState) (result keyHandlerResult) {
			if e.pointerState.pressedButtons.Contain(pointer.ButtonPrimary) {
				e.adapter.pasteToFocusedEditable(gtx)
			}
			result.clearLastKeypressWasSearch.Set(false)
			return
		},
	},
	{
		"pointer-save-or-goto-mark", "If the left pointer button is held down, save a mark named after the current key. Otherwise, goto to the mark named after the current key",
		[]string{},
		func(gtx layout.Context, e *editable, ev *key.Event, args []string, state actionProcessingState) (result keyHandlerResult) {
			tgt := e.executeOn
			if tgt.adapter.displayPath() == nil {
				return
			}
			markName := fmt.Sprintf("%s@%s", tgt.adapter.displayPath().String(), ev.Name)
			if e.pointerState.pressedButtons.Contain(pointer.ButtonPrimary) {
				tgt.adapter.mark(markName, tgt.adapter.displayPath(), tgt.adapter.loadPath(), tgt.firstCursorIndex())
			} else {
				tgt.adapter.gotoMark(markName)
			}
			return
		},
	},
	{
		"cursor-at-selections-lines", "Make a cursor at the start of each selection and remove the selections",
		[]string{},
		func(gtx layout.Context, e *editable, ev *key.Event, args []string, state actionProcessingState) (result keyHandlerResult) {
			if e.SelectionsPresent() {
				e.makeCursorAtEachLineInSelections()
				result.handled = true
			}
			return
		},
	},
	{
		"reduce-cursors-to-one", "If there are multiple cursors, reduce to one cursor",
		[]string{},
		func(gtx layout.Context, e *editable, ev *key.Event, args []string, state actionProcessingState) (result keyHandlerResult) {
			if len(e.CursorIndices) > 1 {
				e.reduceCursorsToOne()
				result.handled = true
			}
			return
		},
	},
	{
		"select-recently-typed", "Select the recently typed text",
		[]string{},
		func(gtx layout.Context, e *editable, ev *key.Event, args []string, state actionProcessingState) (result keyHandlerResult) {
			e.selectRecentlyTypedText()
			return
		},
	},
	{
		"halt-if-handled", "Some actions such as reduce-cursors-to-one specifically indicate whether or not they handled an action (i.e. if they actually performed some action). This action will halt if the previous action executed in the mapping indicated they handled an action.",
		[]string{},
		func(gtx layout.Context, e *editable, ev *key.Event, args []string, state actionProcessingState) (result keyHandlerResult) {
			if state.lastHandlerReportedHandled {
				result.stopProcessingActions = true
			}
			return
		},
	},
	{
		"pop", "Pop the keymap stack: remove the top active keymap from the stack",
		[]string{},
		func(gtx layout.Context, e *editable, ev *key.Event, args []string, state actionProcessingState) (result keyHandlerResult) {
			e.keys.Pop()
			return
		},
	},
	{
		"push", "Push the loaded keymap with the specified name onto the top of the stack",
		[]string{"keymap-name"},
		func(gtx layout.Context, e *editable, ev *key.Event, args []string, state actionProcessingState) (result keyHandlerResult) {
			if len(args) < 1 {
				return
			}
			km, ok := keymaps[args[0]]
			if !ok {
				log(LogCatgEd, "action 'push': no keymap defined with name %s", args[0])
				return
			}

			e.executeOn.keys.Push(km)
			return
		},
	},
	{
		"noop", "Do nothing",
		[]string{},
		func(gtx layout.Context, e *editable, ev *key.Event, args []string, state actionProcessingState) (result keyHandlerResult) {
			return
		},
	},
	{
		"execute-args", "Execute the arguments of the action as a command",
		[]string{"command..."},
		func(gtx layout.Context, e *editable, ev *key.Event, args []string, state actionProcessingState) (result keyHandlerResult) {
			text := strings.Join(args, " ")
			e.adapter.execute(e, gtx, text, nil)
			return
		},
	},
	{
		"move-to-window-on-left", "Move the keyboard focus to the window left of the current one",
		[]string{},
		func(gtx layout.Context, e *editable, ev *key.Event, args []string, state actionProcessingState) (result keyHandlerResult) {
			col := e.adapter.column().left()
			if col != nil {
				topY := 0
				w := e.adapter.window()
				if w != nil {
					topY = w.TopY
				}
				w = col.windowAt(topY)
				if w != nil {
					w.SetFocus(gtx)
				} else {
					col.Tag.SetFocus(gtx)
				}
			} /* else if len(editor.Cols) > 0 {
				editor.Cols[0].Tag.SetFocus(gtx)
			} else {
				editor.Tag.SetFocus(gtx)
			}*/
			return
		},
	},
	{
		"move-to-window-on-right", "Move the keyboard focus to the window right of the current one",
		[]string{},
		func(gtx layout.Context, e *editable, ev *key.Event, args []string, state actionProcessingState) (result keyHandlerResult) {
			col := e.adapter.column().right()
			if col != nil {
				topY := 0
				w := e.adapter.window()
				if w != nil {
					topY = w.TopY
				}
				w = col.windowAt(topY)
				if w != nil {
					w.SetFocus(gtx)
				} else {
					col.Tag.SetFocus(gtx)
				}
			} /* else if len(editor.Cols) > 0 {
				editor.Cols[len(editor.Cols)-1].Tag.SetFocus(gtx)
			} else {
				editor.Tag.SetFocus(gtx)
			}*/
			return
		},
	},
	{
		"move-to-window-above", "Move the keyboard focus to the window above the current one",
		[]string{},
		func(gtx layout.Context, e *editable, ev *key.Event, args []string, state actionProcessingState) (result keyHandlerResult) {
			w := e.adapter.window()
			o := w.above()
			if o != nil {
				o.SetFocus(gtx)
			} else if w != nil {
				// TODO: if focus is already on the window tag, go to the column tag
				w.Tag.SetFocus(gtx)
			}
			return
		},
	},
	{
		"move-to-window-below", "Move the keyboard focus to the window below the current one",
		[]string{},
		func(gtx layout.Context, e *editable, ev *key.Event, args []string, state actionProcessingState) (result keyHandlerResult) {
			w := e.adapter.window()
			if w != nil {
				w := w.below()
				if w != nil {
					w.SetFocus(gtx)
				}
			} /*else if len(editor.Cols) > 0 {
				editor.Cols[0].Tag.SetFocus(gtx)
			}*/
			return
		},
	},
	{
		"move-to-window-body", "Move the keyboard focus to the window body",
		[]string{},
		func(gtx layout.Context, e *editable, ev *key.Event, args []string, state actionProcessingState) (result keyHandlerResult) {
			w := e.adapter.window()
			if w != nil {
				w.SetFocus(gtx)
			}
			return
		},
	},
	{
		"move-to-window-tag", "Move the keyboard focus to the window tag",
		[]string{},
		func(gtx layout.Context, e *editable, ev *key.Event, args []string, state actionProcessingState) (result keyHandlerResult) {
			w := e.adapter.window()
			if w != nil {
				w.Tag.SetFocus(gtx)
			}
			return
		},
	},
	{
		"move-to-column-tag", "Move the keyboard focus to the column tag",
		[]string{},
		func(gtx layout.Context, e *editable, ev *key.Event, args []string, state actionProcessingState) (result keyHandlerResult) {
			c := e.adapter.column()
			if c != nil {
				c.Tag.SetFocus(gtx)
			}
			return
		},
	},
	{
		"move-to-editor-tag", "Move the keyboard focus to the editor tag",
		[]string{},
		func(gtx layout.Context, e *editable, ev *key.Event, args []string, state actionProcessingState) (result keyHandlerResult) {
			editor.Tag.SetFocus(gtx)
			return
		},
	},
	{
		"insert-text", "Insert the arguments at each cursor position. The arguments are joined using a single space.",
		[]string{"text"},
		func(gtx layout.Context, e *editable, ev *key.Event, args []string, state actionProcessingState) (result keyHandlerResult) {
			s := strings.Join(args, " ")
			if e.writeLock.isLocked() {
				return
			}

			e.InsertText(s)
			return
		},
	},
	{
		"move-to-layer-above", "Make the layer above the current one active",
		[]string{},
		func(gtx layout.Context, e *editable, ev *key.Event, args []string, state actionProcessingState) (result keyHandlerResult) {
			editor.ActivateLayerRelativeToCurrent(+1)
			editor.SignalRedrawRequired()
			return
		},
	},
	{
		"move-to-layer-below", "Make the layer below the current one active",
		[]string{},
		func(gtx layout.Context, e *editable, ev *key.Event, args []string, state actionProcessingState) (result keyHandlerResult) {
			editor.ActivateLayerRelativeToCurrent(-1)
			editor.SignalRedrawRequired()
			return
		},
	},
	{
		"move-to-highest-layer", "Make the highest layer active",
		[]string{},
		func(gtx layout.Context, e *editable, ev *key.Event, args []string, state actionProcessingState) (result keyHandlerResult) {
			editor.ActivateHighestLayer()
			editor.SignalRedrawRequired()
			return
		},
	},
	{
		"new-layer", "Make a new layer and switch to it",
		[]string{},
		func(gtx layout.Context, e *editable, ev *key.Event, args []string, state actionProcessingState) (result keyHandlerResult) {
			editor.AddLayer()
			editor.ActivateLayer(len(editor.Layers) - 1)
			editor.SignalRedrawRequired()
			return
		},
	},
	{
		"delete-layer", "Delete the current layer",
		[]string{},
		func(gtx layout.Context, e *editable, ev *key.Event, args []string, state actionProcessingState) (result keyHandlerResult) {
			editor.DelActiveLayer()
			editor.SignalRedrawRequired()
			return
		},
	},
	{
		"set-layer", "Set the current active layer to the argument",
		[]string{"index"},
		func(gtx layout.Context, e *editable, ev *key.Event, args []string, state actionProcessingState) (result keyHandlerResult) {
			index := 0
			if len(args) < 1 {
				index = 0
			}

			index, err := strconv.Atoi(args[0])
			if err != nil {
				log(LogCatgEd, "action 'set-layer': cannot convert argument '%s' to int", args[0])
				return
			}

			editor.ActivateLayer(index)
			editor.SignalRedrawRequired()
			return
		},
	},
}

type namedActionBinding struct {
	actionName string
	params     []string
}

//go:embed default-keymaps.dat
var defaultKeymapDefinitions []byte

// buildKeymap creates a new keymap.Keymap with the given name. It fills the map using the list of mappings
// defined in `nameMap`, but converts the names of actions and parameters in the `nameMap` (the placeholders) with
// the actual functions in `actions`. The values in the resulting keymap, then, are of type `boundKeyAction`; so the
// iterator returned by the keymap's Get function iterates over items of type `boundKeyAction`.
func buildKeymap(def keymap.Definition, actions []keyAction) keymap.Keymap {
	actionsMap := make(map[string]keyAction)
	for _, v := range actions {
		actionsMap[v.name] = v
	}

	m := keymap.NewKeymap(def.Name)
	m.Fallthrough = def.Fallthrough

	for k, l := range def.Keys {
		for _, actionDefn := range l {
			action, ok := actionsMap[actionDefn.ActionName]
			if !ok {
				log(LogCatgEd, "buildKeymap: undefined action '%s'\n", actionDefn.ActionName)
				continue
			}

			boundAction := boundKeyAction{
				keyAction: action,
				params:    actionDefn.Params,
			}

			if k.Name == "default" && k.Modifiers == 0 {
				m.Default = append(m.Default, boundAction)
				continue
			}
			m.Append(k, boundAction)
		}
	}
	return m
}

var keymaps = map[string]keymap.Keymap{}
var keymapDefs = map[string]keymap.Definition{}

var keymapsLoadedFromFile bool

func initKeymaps() error {

	log(LogCatgApp, "Loading built-in keymaps\n")
	buf := bytes.NewBuffer(defaultKeymapDefinitions)
	defs, err := keymap.LoadDefinitions(buf)
	if err != nil {
		return err
	}
	buildAndInstallKeymapsFromDefs(defs)
	err = pushBaseKeymap()
	if err != nil {
		return err
	}

	defs, err = keymap.LoadDefinitionsFromFile(KeymapConfigFile())
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			log(LogCatgApp, "Not loading keymaps from config file %s: %v\n", KeymapConfigFile(), err)
			return nil
		}

		log(LogCatgApp, "Loading keymaps from config file %s failed: %v\n", KeymapConfigFile(), err)
		return err
	}
	buildAndInstallKeymapsFromDefs(defs)
	keymapsLoadedFromFile = true

	return nil
}

func pushBaseKeymap() error {
	km, ok := keymaps["base"]
	if !ok {
		return fmt.Errorf("No keymap named 'base' is defined")
	}
	globalKeymapStack.Push(km)

	return nil
}

func buildAndInstallKeymapsFromDefs(defs []keymap.Definition) error {
	for _, def := range defs {
		log(LogCatgEd, "%s\n", def.String())
	}

	for _, def := range defs {
		buildAndInstallKeymap(def, keyActions)
	}

	//	km, ok := keymaps["base"]
	//	if !ok {
	//		return fmt.Errorf("No keymap named 'base' is defined")
	//	}
	//globalKeymapStack.Push(km)

	return nil
}

func buildAndInstallKeymap(def keymap.Definition, actions []keyAction) {
	def = duplicateKeysUsingCtrlAsCommand(def)
	newMap := buildKeymap(def, actions)

	switch def.Op {
	case keymap.Update:
		oldMap, ok := keymaps[def.Name]
		oldDef, ok2 := keymapDefs[def.Name]
		if !ok || !ok2 {
			keymaps[def.Name] = newMap
			keymapDefs[def.Name] = def
			break
		}

		for k, v := range newMap.Keys {
			oldMap.Keys[k] = v
			oldDef.Keys[k] = def.Keys[k]
		}
		keymaps[def.Name] = oldMap
		keymapDefs[def.Name] = oldDef

	case keymap.Replace:
		keymaps[def.Name] = newMap
		keymapDefs[def.Name] = def
	default:
		log(LogCatgEd, "buildAndInstallKeymap: invalid operation %v\n", def.Op)
		return
	}
	log(LogCatgEd, "buildAndInstallKeymap: installed keymap:\n%s\n", keymaps[def.Name])
}

func duplicateKeysUsingCtrlAsCommand(def keymap.Definition) keymap.Definition {
	cpy := def
	for k, v := range cpy.Keys {
		if k.Modifiers&key.ModCtrl > 0 {
			k.Modifiers = k.Modifiers&^key.ModCtrl | key.ModCommand
			cpy.Keys[k] = v
		}
	}
	return cpy
}

type boundKeyAction struct {
	keyAction
	params []string
}

func (bka boundKeyAction) String() string {
	var parms string
	if len(bka.params) > 0 {
		parms = " " + strings.Join(bka.params, " ")
	}
	return fmt.Sprintf("%s%s", bka.keyAction.name, parms)
}

var globalKeymapStack keymap.Stack
