package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/image/math/fixed"

	"math"

	"gioui.org/f32"
	"gioui.org/io/clipboard"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"github.com/jeffwilliams/anvil/editor/internal/ansi"
	"github.com/jeffwilliams/anvil/editor/internal/expr"
	"github.com/jeffwilliams/anvil/editor/internal/intvl"
	"github.com/jeffwilliams/anvil/editor/internal/pctbl"
	"github.com/jeffwilliams/anvil/editor/internal/regex"
	"github.com/jeffwilliams/anvil/editor/internal/runes"
	"github.com/jeffwilliams/anvil/editor/internal/slice"
	"github.com/jeffwilliams/anvil/editor/internal/typeset"
	"github.com/jeffwilliams/anvil/editor/internal/words"
	"github.com/sarpdag/boyermoore"
)

// editable is a widget that can draw and edit text.
type editable struct {
	layouter
	editableModel
	PreventScrolling bool
	style            editableStyle
	textRender       *TextRenderer
	styleSeq         intvl.IntervalSequence
	styleChanges     intvl.IntervalIter

	layedoutText *typeset.Text

	selectionBeingBuilt   *selection
	lastSearchResult      *selection
	lastSearchTerm        string
	lastKeypressWasSearch bool
	// executeOn is used by some operations to specify which editable to
	// actually act on. For example right clicking in a Tag should do the
	// search in the Body not the tag.
	executeOn             *editable
	preDrawHook           func(e *editable, gtx layout.Context)
	tag                   event.Tag
	pointerState          PointerState
	ignoreTertiaryRelease bool
	opsForNextLayout      OpsForNextLayout
	syntaxHighlighter     Highlighter
	asyncHighlighter      *AsyncHighlighter
	syntaxMaxDocSize      int
	Scheduler             *Scheduler
	maxSizeLastLayout     image.Point
	// label is a name for this editable used for debugging
	label                  string
	completionSource       string
	completionMaxDocSize   int
	colorizeAnsiEscapes    bool
	textChangedListeners   []func(*TextChange)
	adapter                adapter
	syntaxHighlightDelay   time.Duration
	draggingTertiaryButton bool
	wrap                   bool
	elastic                bool
}

type editableStyle struct {
	Fonts       []FontStyle
	FgColor     Color
	BgColor     Color
	LineSpacing unit.Dp

	PrimarySelection   textStyle
	SecondarySelection textStyle
	ExecutionSelection textStyle

	TabStopInterval unit.Dp
	TabStopPadding  unit.Dp
	TextLeftPadding unit.Dp
}

func (e *editable) Init(style editableStyle) {
	e.SetAdapter(nilAdapter{})
	e.text = pctbl.Optimize(pctbl.NewPieceTable([]byte{}))
	e.style = style
	e.layouter.setFontStyles(style.Fonts)
	e.initTextRenderer()
	e.runeOffsetCache = runes.NewOffsetCache(0)
	e.completionMaxDocSize = 2 * 1024 * 1024
	e.syntaxMaxDocSize = 2 * 1024 * 1024
	e.syntaxHighlightDelay = 1 * time.Millisecond
	e.CursorIndices = []int{0}
	e.wordCompletion = NewCompletion(e)
	e.fileCompletion = NewCompletion(e)
	e.recentlyTypedText.start = -1
	e.wrap = true
}

func (e *editable) SetAdapter(a adapter) {
	e.adapter = a
	e.editableModel.adapter = a
}

func (e *editable) initTextRenderer() {
	e.textRender = NewTextRenderer(e.layouter.curFont(), e.layouter.curFontSize(), e.lineSpacingScaled, e.style.FgColor, e.lineHeight)
}

func (e *editable) InitPointerEventHandlers() {
	if e.pointerState.LenHandlers() > 0 {
		return
	}

	// Clicks
	e.pointerState.Handler(PointerEventMatch{pointer.Press, pointer.ButtonPrimary}, e.onPointerPrimaryButtonPress)
	e.pointerState.Handler(PointerEventMatch{pointer.Press, pointer.ButtonTertiary}, e.onPointerTertiaryButtonPress)
	e.pointerState.Handler(PointerEventMatch{pointer.Release, pointer.ButtonTertiary}, e.onPointerTertiaryButtonRelease)
	e.pointerState.Handler(PointerEventMatch{pointer.Press, pointer.ButtonSecondary}, e.onPointerSecondaryButtonPress)

	e.pointerState.Handler(PointerEventMatch{pointer.Drag, pointer.ButtonPrimary}, e.onPointerPrimaryButtonDrag)
	e.pointerState.Handler(PointerEventMatch{pointer.Drag, pointer.ButtonTertiary}, e.onPointerTertiaryButtonDrag)
	e.pointerState.Handler(PointerEventMatch{typ: pointer.Scroll}, e.onPointerScroll)
	e.pointerState.Handler(PointerEventMatch{pointer.Release, pointer.ButtonPrimary}, e.onPointerRelease)
	//e.pointerState.Handler(PointerEventMatch{pointer.Release, pointer.ButtonTertiary}, e.onPointerRelease)
	e.pointerState.Handler(PointerEventMatch{pointer.Release, pointer.ButtonSecondary}, e.onPointerRelease)
}

func (e *editable) SetTextString(s string) {
	e.editableModel.SetTextString(s)
	e.invalidateLayedoutText()
	e.textChanged(fireListeners, TextChange{})
}

func (e *editable) SetTextStringNoUndo(s string) {
	e.editableModel.SetTextStringNoUndo(s)
	e.textChanged(fireListeners, TextChange{})
}

func (e *editable) SetText(b []byte) {
	e.editableModel.SetText(b)
	e.invalidateLayedoutText()
	e.textChanged(fireListeners, TextChange{})
}

func (e *editable) SetTextStringNoReset(s string) {
	e.editableModel.SetTextStringNoReset(s)
	e.invalidateLayedoutText()
}

func (e *editable) Append(b []byte) {
	e.editableModel.Append(b)

	e.invalidateLayedoutText()
	// Since we only appended text we don't need to invalidate the rune offset cache
	e.textChangedButDontClearRuneOffsetCache(fireListeners, TextChange{})
}

func (e *editable) Key(gtx layout.Context, ev *key.Event) {
	e.invalidateLayedoutText()
	if ev.State == key.Release {
		e.KeyRelease(gtx, ev)
		return
	}

	e.KeyPress(gtx, ev)
}

func (e *editable) KeyRelease(gtx layout.Context, ev *key.Event) {
}

func (e *editable) KeyPress(gtx layout.Context, ev *key.Event) {
	log(LogCatgEd, "%s: keypress: %#v\n", e.label, ev)

	resetWordCompletions := true
	resetFileCompletions := true
	clearRecentlyTypedText := false
	clearLastKeypressWasSearch := true

	switch ev.Name {
	case "⏎", "⌤":
		// Enter, Numpad Enter
		if ev.Modifiers.Contain(key.ModCtrl) {

			w := runes.NewWalker(e.Bytes())
			w.SetRunePosCache(e.firstCursorIndex(), &e.runeOffsetCache)
			start, end := w.CurrentLineBounds()
			text := string(w.TextBetweenRuneIndices(start, end))
			text = strings.TrimSpace(text)
			if strings.HasPrefix(text, "◊") && strings.HasSuffix(text, "◊") {
				l := utf8.RuneLen('◊')
				text = text[l : len(text)-l]
			}

			if IsErrorsWindow(e.adapter.displayPath().String()) {
				w := runes.NewWalker(e.Bytes())
				w.SetRunePosCache(e.firstCursorIndex(), &e.runeOffsetCache)
				if w.AtEnd() {
					e.InsertText("\n")
				}
			}

			if ev.Modifiers.Contain(key.ModAlt) {
				e.adapter.clearErrors()
			}
			e.adapter.execute(e, gtx, text, nil)
			break
		}

		if len(e.CursorIndices) == 1 && !ev.Modifiers.Contain(key.ModShift) {
			e.autoIndent()
		} else {
			e.InsertText("\n")
		}

	case "⌫":
		// Backspace
		if e.SelectionsPresent() {
			e.InsertText("")
			break
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
	case "⌦":
		// Delete
		if e.SelectionsPresent() {
			e.InsertText("")
			break
		}

		for _, ndx := range e.CursorIndices {
			if ndx < e.text.Len() {
				e.deleteFromPieceTable(ndx, 1)
			}
		}
	case "Tab":
		// Tab
		e.InsertText(e.adapter.insertWhenTabPressed())
	case "←":
		// Left
		if e.SelectionsPresent() && !ev.Modifiers.Contain(key.ModShift) {
			e.changeSelectionsToCursors(Left)
			return
		}

		var mis motionItems
		if ev.Modifiers.Contain(key.ModShift) {
			mis = newSelectionMotionItems(e, Left)
		} else {
			mis = newCursorsMotionItems(e)
		}

		if ev.Modifiers.Contain(key.ModCtrl) && e.text.Len() > 0 {
			w := runes.NewWalker(e.Bytes())
			for _, mi := range mis.items() {
				w.SetRunePosCache(mi.position(), &e.runeOffsetCache)
				w.BackwardToWordStart()
				mi.setPosition(w.RunePos())
			}
			mis.doneAdjusting(gtx)
			break
		}

		for _, mi := range mis.items() {
			if mi.position() > 0 {
				p := mi.position()
				p--
				mi.setPosition(p)
			}
		}
		mis.doneAdjusting(gtx)
	case "→":
		// Right
		if e.SelectionsPresent() && !ev.Modifiers.Contain(key.ModShift) {
			e.changeSelectionsToCursors(Right)
			return
		}

		var mis motionItems
		if ev.Modifiers.Contain(key.ModShift) {
			mis = newSelectionMotionItems(e, Right)
		} else {
			mis = newCursorsMotionItems(e)
		}

		if ev.Modifiers.Contain(key.ModCtrl) && e.text.Len() > 0 {
			w := runes.NewWalker(e.Bytes())
			for _, mi := range mis.items() {
				w.SetRunePosCache(mi.position(), &e.runeOffsetCache)
				w.ForwardToStartOfNextWord()
				mi.setPosition(w.RunePos())
			}
			mis.doneAdjusting(gtx)
			break
		}

		for _, mi := range mis.items() {
			if mi.position() < e.text.Len() {
				p := mi.position()
				p++
				mi.setPosition(p)
			}
		}
		mis.doneAdjusting(gtx)
	case "↑":
		// Up
		if e.SelectionsPresent() && !ev.Modifiers.Contain(key.ModShift) {
			e.changeSelectionsToCursors(Left)
			return
		}

		var mis motionItems
		if ev.Modifiers.Contain(key.ModShift) {
			mis = newSelectionMotionItems(e, Right)
		} else {
			mis = newCursorsMotionItems(e)
		}

		if ev.Modifiers.Contain(key.ModAlt) && !e.SelectionsPresent() && len(e.CursorIndices) > 0 {
			e.AddNewCursorAboveFirst()
			break
		}

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
	case "↓":
		// Down
		if e.SelectionsPresent() && !ev.Modifiers.Contain(key.ModShift) {
			e.changeSelectionsToCursors(Right)
			return
		}

		var mis motionItems
		if ev.Modifiers.Contain(key.ModShift) {
			mis = newSelectionMotionItems(e, Right)
		} else {
			mis = newCursorsMotionItems(e)
		}

		if ev.Modifiers.Contain(key.ModAlt) && !e.SelectionsPresent() && len(e.CursorIndices) > 0 {
			e.AddNewCursorBelowLast()
			break
		}

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
	case "⇲":
		// End
		if e.SelectionsPresent() && !ev.Modifiers.Contain(key.ModShift) {
			e.clearSelections()
		}

		if ev.Modifiers.Contain(key.ModCtrl) && e.text.Len() > 0 {
			from := e.firstCursorIndex()
			e.moveToEndOfDoc(gtx)
			if ev.Modifiers.Contain(key.ModShift) {
				e.addSecondarySelection(from, e.firstCursorIndex(), Right)
			}
			break
		}

		var mis motionItems
		if ev.Modifiers.Contain(key.ModShift) {
			mis = newSelectionMotionItems(e, Right)
		} else {
			mis = newCursorsMotionItems(e)
		}

		w := runes.NewWalker(e.Bytes())
		for _, mi := range mis.items() {
			w.SetRunePosCache(mi.position(), &e.runeOffsetCache)
			w.ForwardToEndOfLine()
			mi.setPosition(w.RunePos())
		}
		mis.doneAdjusting(gtx)
	case "⇱":
		// Home
		if e.SelectionsPresent() && !ev.Modifiers.Contain(key.ModShift) {
			e.clearSelections()
		}

		if ev.Modifiers.Contain(key.ModCtrl) {
			from := e.firstCursorIndex()
			e.setToOneCursorIndex(0)
			e.makeCursorVisibleByScrolling(gtx)
			if ev.Modifiers.Contain(key.ModShift) {
				e.addSecondarySelection(e.firstCursorIndex(), from, Left)
			}
			break
		}

		var mis motionItems
		if ev.Modifiers.Contain(key.ModShift) {
			mis = newSelectionMotionItems(e, Left)
		} else {
			mis = newCursorsMotionItems(e)
		}

		w := runes.NewWalker(e.Bytes())
		for _, mi := range mis.items() {
			w.SetRunePosCache(mi.position(), &e.runeOffsetCache)
			w.BackwardToStartOfLine()
			mi.setPosition(w.RunePos())
		}
		mis.doneAdjusting(gtx)
	case "⇟":
		// Page down
		e.ScrollOnePage(gtx, Down)
	case "⇞":
		// Page up
		e.ScrollOnePage(gtx, Up)
	case "Z":
		if ev.Modifiers.Contain(key.ModCtrl) || ev.Modifiers.Contain(key.ModCommand) {
			if e.matchingBracketInsertion.Undo(gtx, e) {
				break
			}
			e.Undo(gtx)
		}
	case "R":
		if ev.Modifiers.Contain(key.ModCtrl) || ev.Modifiers.Contain(key.ModCommand) {
			e.Redo(gtx)
			clearRecentlyTypedText = true
		}
	case "E":
		if ev.Modifiers.Contain(key.ModCtrl) {
			e.ScrollOneLine(gtx, Up)
		}
	case "Y":
		if ev.Modifiers.Contain(key.ModCtrl) {
			e.ScrollOneLine(gtx, Down)
		}
	case "N":
		if ev.Modifiers.Contain(key.ModCtrl) && len(e.CursorIndices) == 1 {
			resetWordCompletions = false
			ndx := e.firstCursorIndex()
			ctx := e.wordObjectToComplete(ndx)
			e.doWordCompletion(ctx, Forward)
			clearRecentlyTypedText = true
		}
	case "P":
		if ev.Modifiers.Contain(key.ModCtrl) && len(e.CursorIndices) == 1 {
			if e.wordCompletion.isCompletionInProgress() {
				resetWordCompletions = false
				ndx := e.firstCursorIndex()
				ctx := e.wordObjectToComplete(ndx)
				e.doWordCompletion(ctx, Reverse)
			}

			if e.fileCompletion.isCompletionInProgress() {
				resetFileCompletions = false
				ndx := e.firstCursorIndex()
				ctx := e.filenameObjectToComplete(ndx)
				e.doFilenameCompletion(ctx, Reverse)
			}
			clearRecentlyTypedText = true
		}
	case "F":
		if ev.Modifiers.Contain(key.ModCtrl) {
			resetFileCompletions = false
			ndx := e.firstCursorIndex()
			ctx := e.filenameObjectToComplete(ndx)
			e.doFilenameCompletion(ctx, Forward)
			clearRecentlyTypedText = true
		}
	case "S":
		if ev.Modifiers.Contain(key.ModCtrl) || ev.Modifiers.Contain(key.ModCommand) {
			e.adapter.put()
		}
	case "G":
		if ev.Modifiers.Contain(key.ModCtrl) {
			e.adapter.get()
		}
	case "Q":
		if ev.Modifiers.Contain(key.ModCtrl) {
			ndx := e.firstCursorIndex()
			if e.primarySel != nil && ndx == e.primarySel.End() {
				// As a special case, if the cursor is just after the end of the primary
				// selection likely the user wants to execute the primary selection. They
				// might have just typed some text, hit Escape to select it, and are using
				// Enter to execute it.
				ndx--
			}

			howToLoad := loadFileInSeparateWindow
			if ev.Modifiers.Contain(key.ModAlt) {
				howToLoad = loadFileInCurrentWindow
			}
			e.acquire(gtx, ndx, howToLoad)

			return
		}
	case "C":
		if ev.Modifiers.Contain(key.ModCtrl) || ev.Modifiers.Contain(key.ModCommand) {
			e.adapter.copyAllSelectionsFromLastSelectedEditable(gtx)
		}
	case "X":
		if ev.Modifiers.Contain(key.ModCtrl) || ev.Modifiers.Contain(key.ModCommand) {
			e.adapter.cutAllSelectionsFromLastSelectedEditable(gtx)
			clearRecentlyTypedText = true
		}
	case "V":
		if ev.Modifiers.Contain(key.ModCtrl) || ev.Modifiers.Contain(key.ModCommand) {
			e.adapter.pasteToFocusedEditable(gtx)
			clearRecentlyTypedText = true
		}
	case "L":
		if ev.Modifiers.Contain(key.ModCtrl) {
			e.InsertLozenge()
		}
	case "T":
		if ev.Modifiers.Contain(key.ModCtrl) {
			ndx := e.firstCursorIndex()
			if e.primarySel != nil && ndx == e.primarySel.End() {
				// As a special case, if the cursor is just after the end of the primary
				// selection likely the user wants to execute the primary selection. They
				// might have just typed some text, hit Escape to select it, and are using
				// Enter to execute it.
				ndx--
			}
			t := e.textObjectForExecutionAt(ndx)
			if t != "" {
				if ev.Modifiers.Contain(key.ModAlt) {
					e.adapter.clearErrors()
				}
				e.adapter.execute(e, gtx, t, nil)
			}
			clearRecentlyTypedText = true
		}
	case "/", "?":
		if ev.Modifiers.Contain(key.ModCtrl) {
			ndx := e.firstCursorIndex()
			if e.primarySel != nil && ndx == e.primarySel.End() {
				// As a special case, if the cursor is just after the end of the primary
				// selection likely the user wants to execute the primary selection. They
				// might have just typed some text, hit Escape to select it, and are using
				// Enter to execute it.
				ndx--
			}

			// GIO behaves differently here between Linux and Windows for the keystroke we want to use to search backwards.
			// In Linux we get CTRL-?. In Windows we get CTRL-SHIFT-/.
			dir := Forward
			if ev.Name == "?" || ev.Modifiers.Contain(key.ModShift) {
				dir = Reverse
			}

			if e.lastKeypressWasSearch {
				e.ContinueSearch(gtx, dir)
			} else {
				t := e.textObjectForSearchAt(ndx)
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
			clearLastKeypressWasSearch = false
			clearRecentlyTypedText = true
		}
	case "A":
		if ev.Modifiers.Contain(key.ModCtrl) || ev.Modifiers.Contain(key.ModCommand) {
			e.selectAll()
			clearRecentlyTypedText = true
		}
	case "D":
		if ev.Modifiers.Contain(key.ModCtrl) {
			e.DelimitSelectionsWithCursors()
		}
	case "U":
		if e.SelectionsPresent() {
			return
		}

		if ev.Modifiers.Contain(key.ModCtrl) {
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
			clearRecentlyTypedText = true
		}
	case "K":
		if e.SelectionsPresent() {
			return
		}

		if ev.Modifiers.Contain(key.ModCtrl) {
			e.text.StartTransaction()
			for _, ndx := range e.CursorIndices {
				w := runes.NewWalker(e.Bytes())
				w.SetRunePosCache(ndx, &e.runeOffsetCache)
				w.ForwardToEndOfLine()
				p := w.RunePos()
				//start, end := w.CurrentLineBounds()
				if ndx != p {
					e.deleteFromPieceTableUndoIndex(ndx, p-ndx, ndx)
				}
			}
			e.text.EndTransaction()
			clearRecentlyTypedText = true
		}
	case "Ctrl":
		// Ctrl
		resetWordCompletions = false
		resetFileCompletions = false
		if e.pointerState.pressedButtons.Contain(pointer.ButtonPrimary) {
			e.adapter.cutAllSelectionsFromLastSelectedEditable(gtx)
			break
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

	case "Shift":
		// Shift
		if e.pointerState.pressedButtons.Contain(pointer.ButtonPrimary) {
			e.adapter.pasteToFocusedEditable(gtx)
		}
		clearLastKeypressWasSearch = false
	case "F1", "F2", "F3", "F4", "F5", "F6", "F7", "F8", "F9", "F10", "F11", "F12":
		tgt := e.executeOn
		markName := fmt.Sprintf("%s@%s", tgt.adapter.displayPath().String(), ev.Name)
		if e.pointerState.pressedButtons.Contain(pointer.ButtonPrimary) {
			tgt.adapter.mark(markName, tgt.adapter.displayPath(), tgt.adapter.loadPath(), tgt.firstCursorIndex())
		} else {
			tgt.adapter.gotoMark(markName)
		}

	case "⎋":
		// Escape
		if e.SelectionsPresent() {
			e.makeCursorAtEachLineInSelections()
		} else if len(e.CursorIndices) > 1 {
			e.reduceCursorsToOne()
		} else {
			e.selectRecentlyTypedText()
		}

	default:
		log(LogCatgEd, "Key %s pressed\n", ev.Name)
	}

	if resetWordCompletions {
		e.wordCompletion.Reset()
	}
	if resetFileCompletions {
		e.fileCompletion.Reset()
	}
	if clearRecentlyTypedText {
		e.ClearRecentlyTypedText()
	}
	if clearLastKeypressWasSearch {
		e.lastKeypressWasSearch = false
		e.executeOn.lastKeypressWasSearch = false
	}
}

// AddNewCursorBelowLast adds a new cursor in the line below the last cursor in the
// body at the same column as the last cursor
func (e *editable) AddNewCursorBelowLast() {
	e.sortCursorIndicesByDisplayOrder()
	last := e.CursorIndices[len(e.CursorIndices)-1]
	w := runes.NewWalker(e.Bytes())
	w.SetRunePosCache(last, &e.runeOffsetCache)
	li := w.IndexInLine()
	w.ForwardToEndOfLine()
	w.Forward(1)
	if li >= w.LineLen() {
		li = w.LineLen() - 1
	}
	w.Forward(li)
	e.CursorIndices = append(e.CursorIndices, w.RunePos())
	e.removeDuplicateCursors()
}

func (e *editable) AddNewCursorAboveFirst() {
	e.sortCursorIndicesByDisplayOrder()
	first := e.CursorIndices[0]
	w := runes.NewWalker(e.Bytes())
	w.SetRunePosCache(first, &e.runeOffsetCache)
	li := w.IndexInLine()
	w.BackwardToStartOfLine()
	w.Backward(1)
	w.BackwardToStartOfLine()
	if li >= w.LineLen() {
		li = w.LineLen() - 1
	}
	w.Forward(li)
	e.CursorIndices = append(e.CursorIndices, w.RunePos())
	e.removeDuplicateCursors()
}

func (e *editable) Undo(gtx layout.Context) {
	e.undoOrRedo(gtx, e.text.Undo, -1)
}

func (e *editable) Redo(gtx layout.Context) {
	e.undoOrRedo(gtx, e.text.Redo, 1)
}

func mergeConsecutiveUndoData(undoDatas []interface{}) (result undoData, err error) {
	for i, intf := range undoDatas {
		ud, ok := intf.(*undoData)
		if !ok {
			err = fmt.Errorf("mergeConsecutiveUndoData was passed something that is not an undoData (it is a %T)", intf)
			return
		}

		if i == 0 {
			result.startOfChange = ud.startOfChange
			result.lengthOfChange = ud.lengthOfChange
			result.cursorIndex = ud.cursorIndex
			continue
		}

		if ud.startOfChange < result.startOfChange {
			ud.startOfChange = result.startOfChange
		}
		if ud.cursorIndex < result.cursorIndex {
			ud.cursorIndex = result.cursorIndex
		}
		result.lengthOfChange += ud.lengthOfChange

	}
	return
}

func (e *editable) autoIndent() {
	// Autoindenting with multiple cursors is tricky since InsertText applies the change
	// for multiple cursors
	w := runes.NewWalker(e.Bytes())
	w.SetRunePosCache(e.firstCursorIndex(), &e.runeOffsetCache)

	if w.IsInRunOfSpaces() {
		// If the user is hitting enter somewhere within the leading spaces of the line,
		// then don't insert the full leading spaces, only insert up to the position where
		// the cursor is.
		spaceStart, _ := w.CurrentRunOfSpacesBounds()
		lineStart, _ := w.CurrentLineBounds()
		if spaceStart == lineStart {
			count := w.IndexInLine()
			w.BackwardToStartOfLine()
			var buf bytes.Buffer
			for ; count > 0; count-- {
				buf.WriteRune(w.Rune())
				w.Forward(1)
			}
			e.InsertText("\n")
			if buf.Len() > 0 {
				e.InsertText(buf.String())
			}
			return
		}
	}

	w.BackwardToStartOfLine()
	space := w.CurrentRunOfSpaces()
	e.InsertText("\n")
	if space != "" {
		e.InsertText(space)
	}
}

func (e *editable) undoOrRedo(gtx layout.Context, undoOrRedo func() []interface{}, shiftDirection int) {
	if e.writeLock.isLocked() {
		return
	}

	if e.SelectionsPresent() {
		e.clearSelections()
	}

	e.invalidateLayedoutText()
	e.textChanged(fireListeners, TextChange{})

	uds := undoOrRedo()

	if uds != nil {
		ud, err := mergeConsecutiveUndoData(uds)
		if err != nil {
			log(LogCatgEd, "editable.undoOrRedo: %v\n", err)
			return
		}
		e.setToOneCursorIndex(ud.cursorIndex)
		e.shiftItemsDueToTextModification(ud.startOfChange, shiftDirection*ud.lengthOfChange)
		// We fire the text-change listeners so that cloned windows can adjust their top-left
		e.notifyTextChangeListeners(NewTextChange(ud.startOfChange, shiftDirection*ud.lengthOfChange))
	}

	e.makeCursorVisibleByScrolling(gtx)
	return
}

func (e *editable) moveToEndOfDoc(gtx layout.Context) {
	e.setToOneCursorIndex(e.text.Len())
	e.makeCursorVisibleByScrolling(gtx)
}

func (e *editable) cursorVisible(gtx layout.Context) bool {
	ltext, err := e.getOrBuildLayedoutText(gtx, e.visibleText(gtx))
	if err != nil {
		return false
	}

	pos := e.findFirstCursorIn(gtx, ltext)
	if pos == nil || len(pos) == 0 {
		return false
	}
	y := pos[0].Y

	if y < 0 || int(y+e.lineHeight()) > gtx.Constraints.Max.Y {
		return false
	}

	return true
}

func (e *editable) makeCursorVisibleByScrolling(gtx layout.Context) {
	if e.PreventScrolling {
		return
	}

	cursorIndex := e.firstCursorIndex()

	w := runes.NewWalker(e.Bytes())

	cursorAboveViewport := func() bool {
		return e.TopLeftIndex > cursorIndex
	}

	if cursorAboveViewport() {
		w.SetRunePosCache(cursorIndex, &e.runeOffsetCache)
		w.BackwardToStartOfLine()
		e.TopLeftIndex = w.RunePos()

		// Move a few extra lines if we can
		lines := 0
		for w.RunePos() > 0 && lines < 3 {
			e.TopLeftIndex = w.RunePos()
			w.Backward(1)
			w.BackwardToStartOfLine()
			lines++
		}
	} else {
		if e.cursorVisible(gtx) {
			return
		}

		w.SetRunePosCache(cursorIndex, &e.runeOffsetCache)
		w.BackwardToStartOfLine()
		topLeft := e.TopLeftIndex
		pageLenInRunes := e.layoutPreviousPageBackwardsFrom(gtx, w.RunePos())

		w.Backward(pageLenInRunes)
		e.TopLeftIndex = w.RunePos()
		if e.TopLeftIndex <= topLeft {
			e.TopLeftIndex = topLeft
		}
	}
	e.invalidateLayedoutText()
}

func (e *editable) makeCursorVisibleByMovingCursor(gtx layout.Context) {
	/*
		In general, we usually don't want to use this. It breaks some workflows. For example, suppose
		you want to use multiple cursors to make the same change in different places. If you place one cursor,
		then scroll off the viewport to the place where you want to make the second change, if we used this
		function to ensure the cursor is visible while scrolling then the change the user wanted to make
		at the first cursor position will be made in the wrong place.
	*/
	if e.SelectionsPresent() {
		log(LogCatgEd, "makeCursorVisibleByMovingCursor: selections present so no cursors to move")
		return
	}

	if e.cursorVisible(gtx) {
		log(LogCatgEd, "makeCursorVisibleByMovingCursor: cursor already visible")
		return
	}

	if e.CursorIndices[0] < e.TopLeftIndex {
		log(LogCatgEd, "makeCursorVisibleByMovingCursor: pull down")
		e.setToOneCursorIndex(e.TopLeftIndex)
	} else {
		log(LogCatgEd, "makeCursorVisibleByMovingCursor: pull up")
		ltext, err := e.getOrBuildLayedoutText(gtx, e.visibleText(gtx))
		if err != nil {
			e.setToOneCursorIndex(e.TopLeftIndex)
			return
		}

		if ltext.LineCount() == 0 {
			return
		}

		n := e.TopLeftIndex
		log(LogCatgEd, "makeCursorVisibleByMovingCursor: ltext.LineCount() = %d", ltext.LineCount())
		log(LogCatgEd, "makeCursorVisibleByMovingCursor: len(ltext.Lines()) = %d", len(ltext.Lines()))
		for _, line := range ltext.Lines()[:ltext.LineCount()-1] {
			n += line.RuneCount()
		}
		e.setToOneCursorIndex(n)
	}
}

func (e *editable) centerOnFirstCursorOrPrimarySelection(gtx layout.Context) {

	if e.cursorVisible(gtx) {
		return
	}

	var index int
	if e.SelectionsPresent() {
		if e.primarySel == nil {
			return
		}

		index = e.primarySel.start
	} else {
		index = e.CursorIndices[0]
	}

	windowHeightInLines := e.heightInLines(gtx)

	doc := e.Bytes()

	// As a special case, if the cursor is at the end of the window, scroll so
	// as much text is shown as possible.
	if index >= e.text.Len()-1 {
		e.makeCursorVisibleByScrolling(gtx)
		return
	}

	doc, runeIndex := e.firstNRunes(doc, index)
	w := runes.NewWalker(doc)
	w.SetRunePosCache(index, &e.runeOffsetCache)
	//w.BackwardToStartOfLine()

	if windowHeightInLines <= 2 {
		e.SetTopLeft(w.RunePos())
		return
	}

	constraints := e.textLayoutConstraints(gtx)
	bl := NewBackwardsLayouter(doc, runeIndex, &e.runeOffsetCache, constraints)

	numberOfLinesToMoveBack := windowHeightInLines/2 - 1
	linesMovedBack := 0
	for {
		eof, wrappedCount, lineLenInRunes := bl.Next()
		if linesMovedBack+wrappedCount > numberOfLinesToMoveBack || eof {
			break
		}

		w.Backward(lineLenInRunes)
		linesMovedBack += wrappedCount
	}

	e.SetTopLeft(w.RunePos())
}

func (e *editable) unwrappedLineCount(gtx layout.Context) int {
	windowHeightInLines := 0
	ltext, err := e.getOrBuildLayedoutText(gtx, e.visibleText(gtx))
	if err == nil {
		windowHeightInLines = ltext.SourceLineCount()
	} else {
		// This is not accurate when lines are wrapped
		windowHeightInLines = e.heightInLines(gtx)
	}

	return windowHeightInLines
}

func (e *editable) wrappedLineCount(gtx layout.Context) int {
	windowHeightInLines := 0
	ltext, err := e.getOrBuildLayedoutText(gtx, e.visibleText(gtx))
	if err == nil {
		windowHeightInLines = ltext.LineCount()
	} else {
		// This is not accurate when lines are wrapped
		windowHeightInLines = e.heightInLines(gtx)
	}

	return windowHeightInLines
}

func (e *editable) prepareForLayout() {
	e.pointerState.currentPointerEvent.set = false
	//e.invalidateLayedoutText()
}

func (e *editable) Pointer(gtx layout.Context, ev *pointer.Event) {
	log(LogCatgEd, "%s: pointer event: %s\n", e.label, e.pointerEventString(ev))
	e.wordCompletion.Reset()
	e.fileCompletion.Reset()
	e.invalidateLayedoutText()
	e.InitPointerEventHandlers()
	e.pointerState.Event(ev, gtx)
	e.ClearRecentlyTypedText()
}

func (e *editable) pointerEventString(ev *pointer.Event) string {
	var buf bytes.Buffer

	fmt.Fprintf(&buf, "kind: %s buttons: %s position: %s modifiers: %s", ev.Kind, ev.Buttons, ev.Position, ev.Modifiers)
	return buf.String()
}

func (e *editable) runeIndexOfPointerEvent(ev *pointer.Event, text typeset.Text) int {
	runeIndex := text.IndexOfPixelCoord(ev.Position)
	runeIndex += e.TopLeftIndex
	return runeIndex
}

type verticalDirection int

const (
	Down verticalDirection = iota
	Up
)

type horizontalDirection int

const (
	Left horizontalDirection = iota
	Right
)

func (e *editable) ScrollOneLine(gtx layout.Context, d verticalDirection) {
	if e.PreventScrolling {
		return
	}

	w := runes.NewWalker(e.Bytes())
	_ = w.SetRunePosCache(e.TopLeftIndex, &e.runeOffsetCache)
	posBefore := w.RunePos()
	if d == Down {
		w.ForwardToEndOfLine()
		w.Forward(1)
	} else {
		w.BackwardToStartOfLine()
		w.Backward(1)
		w.BackwardToStartOfLine()
	}
	posAfter := w.RunePos()
	if posBefore == posAfter {
		return
	}

	e.TopLeftIndex = w.RunePos()
	e.invalidateLayedoutText()
}

func (e *editable) ScrollOnePage(gtx layout.Context, d verticalDirection) {
	if e.PreventScrolling {
		return
	}

	w := runes.NewWalker(e.Bytes())
	err := w.SetRunePosCache(e.TopLeftIndex, &e.runeOffsetCache)
	if err != nil {
		log(LogCatgEd, "editable.ScrollOnePage: %v\n", err)
	}
	n := 0

	if d == Down {
		// Page down

		max := e.unwrappedLineCount(gtx) - 1
		for !w.AtEnd() {
			e.TopLeftIndex++
			w.Forward(1)
			if w.Rune() == '\n' {
				n++
			}
			if n >= max {
				// Move past the last newline
				if w.Rune() == '\n' {
					e.TopLeftIndex++
				}
				break
			}
		}
	} else {
		/*
			To calculate how many lines we need to move back depends on how the lines are layed out when wrapped.
			We know we how many wrapped lines we can display; it is the height of the viewport in lines (height of the font).
			But we need to know how many unwrapped lines that translates to.
			We basically start laying out lines backwards starting from the current TopLeftIndex, one at a time. Each unwrapped
			line results in one or more wrapped lines. When we reach the max number of wrapped lines that can fit in the viewport
			we have our count of unwrapped lines, and we can then move walk backwards that many lines using the RuneWalker.
		*/
		pageLenInRunes := e.layoutPreviousPageBackwardsFrom(gtx, e.TopLeftIndex)
		log(LogCatgEd, "editable.ScrollOnePage: pageLenInRunes: %d\n", pageLenInRunes)
		w.Backward(pageLenInRunes)
		e.TopLeftIndex = w.RunePos()
	}
	e.invalidateLayedoutText()
}

func (e *editable) layoutPreviousPageBackwardsFrom(gtx layout.Context, runeIndex int) (pageLenInRunes int) {
	maxWrapped := e.heightInLines(gtx)
	doc := e.Bytes()
	doc, runeCount := e.firstNRunes(doc, runeIndex)

	// Give one-line's worth of grace in case the last line doesn't fully fit on the screen.
	maxWrapped -= 1

	constraints := e.textLayoutConstraints(gtx)
	bl := NewBackwardsLayouter(doc, runeCount, &e.runeOffsetCache, constraints)

	linesMovedBack := 0
	for {
		eof, wrappedCount, lineLenInRunes := bl.Next()
		if linesMovedBack+wrappedCount > maxWrapped || eof {
			break
		}
		linesMovedBack += wrappedCount
		pageLenInRunes += lineLenInRunes
	}
	return
}

func (e *editable) relayout(gtx layout.Context) {
	e.initPreDrawState(gtx)

	if e.preDrawHook != nil {
		e.preDrawHook(e, gtx)
	}

	// Events have already been processed and the text contents updated.
	// Anything that can't be immedately applied is handled after
	e.opsForNextLayout.Perform(gtx)

	// layout the text into lines. Don't bother styling it.

	_, err := e.getOrBuildLayedoutText(gtx, e.visibleText(gtx))
	if err != nil {
		e.adapter.appendError("", err.Error())
		return
	}

	// Mouse clicks and other stuff that needs resolution from pixel coords to character index
	// is processed here
	e.processDeferredEventsAndApplyTo(gtx, *e.layedoutText)
	return
}

func (e *editable) draw(gtx layout.Context) layout.Dimensions {
	defer e.indentOnLeft(&gtx).Pop()
	defer e.postDraw(gtx)

	// Now that we've finished handling all events, prepare the styles.
	e.prepareStylesChanges(gtx)

	_, err := e.getOrBuildLayedoutText(gtx, e.visibleText(gtx))
	if err != nil {
		e.adapter.appendError("", err.Error())
		return layout.Dimensions{Size: image.Point{X: gtx.Constraints.Max.X, Y: 0}}
	}
	height := e.renderTextWithStyles(gtx, *e.layedoutText)

	e.drawCursorIn(gtx, *e.layedoutText)

	//e.postDraw(gtx)

	return layout.Dimensions{Size: image.Point{X: gtx.Constraints.Max.X, Y: int(height)}}
}

func (e *editable) indentOnLeft(gtx *layout.Context) op.TransformStack {
	return op.Offset(image.Point{gtx.Metric.Dp(e.style.TextLeftPadding), 0}).Push(gtx.Ops)
}

func (e *editable) initPreDrawState(gtx layout.Context) {
	if e.maxSizeLastLayout != gtx.Constraints.Max {
		e.invalidateLayedoutText()
	}
}

func (e *editable) postDraw(gtx layout.Context) {
	e.maxSizeLastLayout = gtx.Constraints.Max
	e.pointerState.FreeLayoutContext()
}

func (e *editable) visibleText(gtx layout.Context) []byte {
	doc := e.Bytes()

	doc, _ = e.removeFirstNRunes(doc, e.TopLeftIndex)

	h := e.heightInLines(gtx)
	w := runes.NewWalker(doc)
	w.ForwardLines(h + 1)
	p := w.BytePos()

	doc = doc[:p]
	return doc
}

func (e *editable) SetPreDrawHook(preDrawHook func(e *editable, gtx layout.Context)) {
	e.preDrawHook = preDrawHook
}

func (e *editable) heightInLines(gtx layout.Context) int {
	pixelHeight := gtx.Constraints.Max.Y
	lineHeight := e.lineHeight()

	return int(math.Floor(float64(pixelHeight) / float64(lineHeight)))
}

func (e *editable) layoutText(gtx layout.Context, doc []byte) (text *typeset.Text, err error) {

	//log(LogCatgEd,"editable.layoutText: for %s: called for doc %s\n", e.label, doc)

	constraints := e.textLayoutConstraints(gtx)

	t, errs := typeset.Layout(doc, constraints)
	text = &t

	for _, e := range errs {
		log(LogCatgEd, "typeset.Layout error: %v\n", e)
	}

	return
}

func (e *editable) textLayoutConstraints(gtx layout.Context) typeset.Constraints {
	var wrapWidth int
	if e.wrap {
		wrapWidth = gtx.Constraints.Max.X - gtx.Metric.Dp(e.style.TextLeftPadding)
	}

	return typeset.Constraints{
		FontFaceId:        e.curFontName(),
		FontSize:          e.curFontSize(),
		FontFace:          e.curFont(),
		WrapWidth:         wrapWidth,
		TabStopInterval:   gtx.Metric.Dp(e.style.TabStopInterval),
		MaxHeight:         gtx.Constraints.Max.Y,
		ExtraLineGap:      gtx.Metric.Dp(e.style.LineSpacing),
		ReplaceCRWithTofu: e.adapter.replaceCrWithTofu(),
		ElasticTabStops:   e.elastic,
		ElasticMinWidth:   gtx.Metric.Dp(e.style.TabStopInterval),
		ElasticPadding:    gtx.Metric.Dp(e.style.TabStopPadding),
	}
}

func (e *editable) invalidateLayedoutText() {
	e.layedoutText = nil
}

type fireListenersBehaviour int

const (
	fireListeners fireListenersBehaviour = iota
	dontFireListeners
)

type TextChange struct {
	Offset, Length int
}

func (t TextChange) IsZero() bool {
	return t.Offset == 0 && t.Length == 0
}

func NewTextChange(offset, length int) TextChange {
	return TextChange{
		Offset: offset,
		Length: length,
	}
}

// textChanged is called when the text in the editable has changed. Parameter textChange describes the
// change: Offset and Length are positive for an insert, Length is negative for a delete.
// textChange is the zero TextChange if it is unclear what text was changed.
func (e *editable) textChanged(b fireListenersBehaviour, textChange TextChange) {
	e.textChangedButDontClearRuneOffsetCache(b, textChange)
	if textChange.IsZero() {
		e.runeOffsetCache.Clear()
		return
	}

	if textChange.Length > 0 {
		e.runeOffsetCache.ClearAfter(textChange.Offset)
		return
	}

	if textChange.Length < 0 {
		e.runeOffsetCache.ClearAfter(textChange.Offset + textChange.Length - 1)
		return
	}
}

func (e *editable) textChangedButDontClearRuneOffsetCache(b fireListenersBehaviour, textChange TextChange) {
	if e.asyncHighlighter != nil {
		e.asyncHighlighter.Cancel()
	}
	e.schedule("highlight-syntax", e.syntaxHighlightDelay, func() { e.HighlightSyntax() })

	e.schedule("build-completions", 300*time.Millisecond, e.BuildCompletions)

	if b == dontFireListeners {
		return
	}

	e.notifyTextChangeListeners(textChange)
}

func (e *editable) notifyTextChangeListeners(textChange TextChange) {
	for _, l := range e.textChangedListeners {
		l(&textChange)
	}
}

func (e *editable) LenOfDisplayedTextInBytes(gtx layout.Context) (ln int, err error) {
	if e.layedoutText != nil {
		ln = e.layedoutText.ByteCount()
		return
	}

	ltext, err := e.getOrBuildLayedoutText(gtx, e.visibleText(gtx))
	if err != nil {
		return
	}
	ln = ltext.ByteCount()
	return
}

func (e *editable) getOrBuildLayedoutText(gtx layout.Context, doc []byte) (l *typeset.Text, err error) {
	if e.layedoutText != nil {
		return e.layedoutText, nil
	}

	e.layedoutText, err = e.layoutText(gtx, doc)
	l = e.layedoutText
	return
}

func (e *editable) processDeferredEventsAndApplyTo(gtx layout.Context, text typeset.Text) {
	if e.pointerState.currentPointerEvent.set {
		log(LogCatgEd, "%s: processing deferred pointer event: %s\n", e.label, e.pointerEventString(&e.pointerState.currentPointerEvent.Event))
		ri := e.runeIndexOfPointerEvent(&e.pointerState.currentPointerEvent.Event, text)
		e.pointerState.SetRuneIndexOfCurrentEvent(ri)
		e.pointerState.InvokeHandlers()
	}
}

func (e *editable) executeSelectedWithLastSelectedArg(ps *PointerState) {
	arg := e.adapter.textOfLastSelectionInEditor()
	e.executeSelected(ps, arg)
}

func (e *editable) executeSelectedWithAllSelectionsInLastSelectedEditable(ps *PointerState) {
	args := e.adapter.textOfAllSelectionsInLastSelectedEditable()
	e.executeSelected(ps, args...)
}

func (e *editable) onPointerPrimaryButtonPress(ps *PointerState) {
	if ps.currentPointerEvent.Modifiers&key.ModCommand > 0 {
		// Treat as a tertiary press
		ps.pressedButtons = ps.pressedButtons & (^pointer.ButtonPrimary)
		ps.pressedButtons = ps.pressedButtons | pointer.ButtonTertiary
		e.onPointerTertiaryButtonPress(ps)
		return
	}

	if e.pointerState.pressedButtons.Contain(pointer.ButtonTertiary) {
		e.ignoreTertiaryRelease = true

		e.executeSelectedWithAllSelectionsInLastSelectedEditable(ps)
		return
	}

	ev := ps.currentPointerEvent
	runeIndex := ev.runeIndex

	e.SetFocus(ps.gtx)

	if ps.consecutiveClicks == 1 {
		// Single click
		e.lastSearchResult = nil
		if ev.Modifiers&key.ModAlt == 0 {
			e.setToOneCursorIndex(runeIndex)
			e.clearSelections()
		} else {
			if e.removeCursorAt(runeIndex) {
				return
			}
			if sel := e.selectionContaining(runeIndex); sel != nil && e.numberOfSelections() > 1 {
				e.removeSelection(sel)
				return
			}
			e.CursorIndices = append(e.CursorIndices, runeIndex)
			e.removeDuplicateCursors()
		}

	} else {
		w := runes.NewWalker(e.Bytes())
		w.SetRunePosCache(runeIndex, &e.runeOffsetCache)
		var l, r int
		if ps.consecutiveClicks == 2 {
			// Double click: select identifier, or string of spaces, or until next bracket
			l, r = e.boundsToSelectOnDoubleClick(w)
		} else if ps.consecutiveClicks == 3 {
			// Triple click: select word, or bracketed/quoted text including bracket/quote
			l, r = e.boundsToSelectOnTripleClick(w)
		} else if ps.consecutiveClicks >= 4 {
			// Quad click: select line
			l, r = w.CurrentLineBounds()
		}

		if ev.Modifiers&key.ModAlt == 0 {
			e.setPrimarySelection(l, r)
		} else {
			e.addSecondarySelection(l, r, Right)
		}

		ndx := e.cursorIndexWithin(l, r)
		if ndx != -1 {
			e.CursorIndices[ndx] = r
		}

	}
}

func (e *editable) boundsToSelectOnDoubleClick(w runes.Walker) (l, r int) {
	var err error
	if w.IsAtBracket() {
		l, r, err = w.TextWithinBracketsBounds()
		if err == nil {
			return
		}
	}

	if w.IsAtStartOfLine() {
		l, r = w.CurrentLineBoundsIncludingNl()
		return
	}

	if w.IsAtQuote() {
		l, r, err = w.TextWithinQuotesInCurrentLine()
		if err == nil {
			return
		}
	}

	if w.IsInRunOfSpaces() {
		l, r = w.CurrentRunOfSpacesBounds()
	} else if w.IsInRunOfSymbols() {
		l, r = w.CurrentRunOfSymbolsBounds()
	} else {
		l, r = w.CurrentIdentifierBounds()
	}

	return
}

func (e *editable) boundsToSelectOnTripleClick(w runes.Walker) (l, r int) {

	if w.IsAtBracket() {
		var err error
		l, r, err = w.TextWithinBracketsBounds()
		if err != nil {
			return
		}
		l--
		r++
		return
	}

	if w.IsAtQuote() {
		var err error
		l, r, err = w.TextWithinQuotesInCurrentLine()
		if err != nil {
			return
		}
		l--
		r++
		return
	}

	l, r = w.CurrentWordBounds()
	return
}

func (e *editable) onPointerTertiaryButtonPress(ps *PointerState) {
	//e.SetFocus(ps.gtx)

	ev := ps.currentPointerEvent
	runeIndex := ev.runeIndex

	e.overridingCursorIndices = []int{runeIndex}
	e.adapter.setEditableWhereTertiaryButtonHoldStarted(e)
	log(LogCatgEd, "Setting editable where tertiary button was pressed\n")
}

func (e *editable) onPointerTertiaryButtonRelease(ps *PointerState) {
	e.overridingCursorIndices = nil
	log(LogCatgEd, "Clearing editable where tertiary button was pressed\n")
	e.adapter.clearEditableWhereTertiaryButtonHoldStarted()

	if e.ignoreTertiaryRelease {
		e.ignoreTertiaryRelease = false
		return
	}

	if e.pointerState.pressedButtons.Contain(pointer.ButtonPrimary) {
		e.adapter.cutAllSelectionsFromLastSelectedEditable(ps.gtx)
		return
	}

	if e.draggingTertiaryButton {
		e.stopBuildingSelection()
		e.draggingTertiaryButton = false
		cmd, ok := e.textOfPrimarySelection()
		if ok {
			e.adapter.execute(e, ps.gtx, cmd, nil)
		}
		e.lastSearchResult = nil
		e.clearSelections()

		return
	}

	if ps.currentPointerEvent.Modifiers&key.ModAlt > 0 {
		e.adapter.clearErrors()
	}

	e.executeSelected(ps)
}

func (e *editable) executeSelected(ps *PointerState, args ...string) {
	runeIndex := ps.currentPointerEvent.runeIndex
	cmd := e.textObjectForExecutionAt(runeIndex)

	e.adapter.execute(e, ps.gtx, cmd, args)
	e.lastSearchResult = nil
}

func (e *editable) plumb(gtx layout.Context, obj string) (plumbed bool) {
	return e.adapter.plumb(e, gtx, obj)
}

func (e *editable) onPointerSecondaryButtonPress(ps *PointerState) {
	if e.pointerState.pressedButtons.Contain(pointer.ButtonPrimary) {
		e.adapter.pasteToFocusedEditable(ps.gtx)
		return
	}

	const (
		acquire = iota
		continuePreviousSearch
		newSearch
		noop
	)

	var obj string

	determineAction := func() (action int) {

		action = acquire
		if pointerEventsOccurredAtAlmostSamePlace(&ps.currentPointerEvent, &ps.lastPointerEvent) &&
			ps.lastPressEvent.set &&
			!ps.currentPointerEvent.Modifiers.Contain(key.ModAlt) &&
			ps.lastPressEvent.button == pointer.ButtonSecondary {

			action = continuePreviousSearch
			obj = e.textObjectForAcquireAt(ps.lastPressEvent.runeIndex)
		}

		if action == acquire {
			if ps.currentPointerEvent.Modifiers.Contain(key.ModAlt) {
				howToLoad := loadFileInSeparateWindow
				if ps.currentPointerEvent.Modifiers.Contain(key.ModCtrl) {
					howToLoad = loadFileInCurrentWindow
				}

				e.acquire(ps.gtx, ps.currentPointerEvent.runeIndex, howToLoad)
				return
			}

			action = newSearch
			obj = e.textObjectForSearchAt(ps.currentPointerEvent.runeIndex)
		}

		return
	}

	action := determineAction()

	switch action {
	case newSearch:
		log(LogCatgEd, "new search for '%s'\n", obj)
		searchAt := ps.currentPointerEvent.runeIndex
		if e.executeOn != e {
			searchAt = e.executeOn.firstCursorIndex()
			e.executeOn.clearSelections()
		}
		direction := Forward
		if ps.currentPointerEvent.Modifiers.Contain(key.ModShift) {
			direction = Reverse
		}
		e.SearchAndUpdateEditable(ps.gtx, obj, searchAt, direction)
	case continuePreviousSearch:
		log(LogCatgEd, "continue search\n")
		direction := Forward
		if ps.currentPointerEvent.Modifiers.Contain(key.ModShift) {
			direction = Reverse
		}
		e.ContinueSearch(ps.gtx, direction)
	}
}

func (e *editable) acquire(gtx layout.Context, runeIndex int, howToLoad fileLoadArrangement) {
	obj := e.textObjectForAcquireAt(runeIndex)

	if e.plumb(gtx, obj) {
		return
	}

	var err error
	var seek seek

	obj, seek, err = parseSeekFromFilename(obj)
	if err != nil {
		e.adapter.appendError("", fmt.Sprintf("Can't acquire: %v", err))
		return
	}

	e.determineFilePathAndLoadFile(obj, seek, howToLoad)
}

func (e *editable) determineFilePathAndLoadFile(partialFilePath string, seek seek, how fileLoadArrangement) {
	j := NewNamedJob(filepath.Base(partialFilePath))
	e.adapter.addJob(j)
	go func() {
		// The call to Complete can block if the file is remote and there is already an ssh connection pending for it.
		// If there is, we would block on the ssh cache mutex, which would freeze the UI.
		// Hence we are doing the call to Complete in a new goroutine
		displayPath, loadPath := completeDisplayAndLoadPaths(e.adapter.completer(), partialFilePath)

		w := determineFilePathAndLoadFileWork{
			job:                 j,
			adapter:             e.adapter,
			fileLoadArrangement: how,
			displayPath:         displayPath,
			loadPath:            loadPath,
			seek:                seek,
		}
		e.adapter.doWork(w)
	}()
}

type namedJob struct {
	name string
}

func NewNamedJob(name string) Job {
	return &namedJob{name}
}

func (j namedJob) Name() string {
	return j.name
}

func (j namedJob) Kill() {
}

type determineFilePathAndLoadFileWork struct {
	adapter             adapter
	fileLoadArrangement fileLoadArrangement
	displayPath         *GlobalPath
	loadPath            *GlobalPath
	seek                seek
	job                 Job
}

type fileLoadArrangement int

const (
	loadFileInSeparateWindow fileLoadArrangement = iota
	loadFileInCurrentWindow
)

func (w determineFilePathAndLoadFileWork) Service() (done bool) {
	if w.displayPath.String() == "" {
		return true
	}

	w.adapter.addOpForNextLayout(func(gtx layout.Context) {
		var opts LoadFileOpts
		if !w.seek.empty() {
			opts.GoTo = w.seek
		}

		log(LogCatgEd, "Load the file %s (%s)\n", w.displayPath, w.loadPath)
		switch w.fileLoadArrangement {
		case loadFileInSeparateWindow:
			w.adapter.loadFile(gtx, w.displayPath, w.loadPath, opts)
		case loadFileInCurrentWindow:
			w.adapter.loadFileInPlace(gtx, w.displayPath, w.loadPath, opts)
		}
	})

	return true
}

func (w determineFilePathAndLoadFileWork) Job() Job {
	return w.job
}

func (w determineFilePathAndLoadFileWork) Name() string {
	return w.displayPath.Base()
}

func (w determineFilePathAndLoadFileWork) Kill() {
}

type selectBehaviour int

const (
	selectText selectBehaviour = iota
	dontSelectText
)

func (e *editable) moveCursorTo(gtx layout.Context, seek seek, selectBehaviour selectBehaviour) {
	doc := e.Bytes()
	w := runes.NewWalker(doc)

	var l, r int
	if seek.seekType == seekToRegex {
		loc := seek.regex.FindIndex(doc)
		if loc == nil {
			return
		}
		w.ForwardBytes(loc[0])
		l = w.RunePos()
		w.ForwardBytes(loc[1] - loc[0])
		r = w.RunePos()
	} else {
		if seek.seekType == seekToLineAndCol {
			w.GoToLineAndCol(seek.line, seek.col)
			if seek.col != 0 {
				l, r = w.RunePos(), w.RunePos()+1
			} else {
				l, r = w.CurrentLineBoundsIncludingNl()
			}
		} else {
			w.Forward(seek.runePos)
			l, r = w.RunePos(), w.RunePos()+1
		}
	}
	e.setToOneCursorIndex(l)
	if selectBehaviour == selectText {
		e.setPrimarySelection(l, r)
	}
	e.makeCursorVisibleByScrolling(gtx)
}

// SearchAndUpdateEditable clears the current selections and begins a new search for `needle` starting from `searchAt`.
func (e *editable) SearchAndUpdateEditable(gtx layout.Context, needle string, searchAt int, direction direction) {
	e.executeOn.lastSearchResult = nil
	e.searchAndUpdateEditable(gtx, searchAt, needle, direction)
}

func (e *editable) ContinueSearch(gtx layout.Context, direction direction) {
	if e.executeOn.lastSearchResult == nil {
		return
	}

	searchAt := e.executeOn.lastSearchResult.End()
	if direction == Reverse {
		searchAt = e.executeOn.lastSearchResult.Start()
	}
	sel := e.executeOn.primarySel
	if sel == nil {
		return
	}

	needle := e.executeOn.lastSearchTerm
	e.searchAndUpdateEditable(gtx, searchAt, needle, direction)
}

func (e *editable) searchAndUpdateEditable(gtx layout.Context, searchAt int, needle string, direction direction) {
	pos, end := e.executeOn.Search(searchAt, needle, direction)

	if pos == searchAt {
		if direction == Forward {
			pos, end = e.executeOn.Search(searchAt+1, needle, direction)
		} else {
			pos, end = e.executeOn.Search(searchAt-1, needle, direction)
		}
	}

	if pos == -1 {
		// Wrap the search
		if direction == Forward {
			pos, end = e.executeOn.Search(0, needle, direction)
		} else {
			pos, end = e.executeOn.Search(len(e.executeOn.Bytes())-1, needle, direction)
		}
	}

	if pos < 0 {
		return
	}

	e.executeOn.setToOneCursorIndex(pos)
	e.executeOn.addPrimarySelection(pos, end)
	e.executeOn.lastSearchResult = e.executeOn.primarySel
	e.executeOn.lastSearchTerm = needle

	if e.executeOn != e {
		// This handles a corner case. If you right click to search from the tag of a window,
		// that click event is processed when the tag is layed out, which is before the body is layed out.
		// We only adjust the height constraint (MaxY) in the layout context to account for the height of the tag
		// _after_ we finish laying out the tag. But here we are going to try to do a search in the body when laying out
		// the tag and will attempt to make the cursor visible in the body, but the height constraint of the body is too
		// large and so if the search term is off the bottom of the body viewport we won't make it visible since we think
		// the body is larger than it is (and we make it just barely visible by scrolling the least amount).
		//
		// To fix that we instead defer making the cursor visible in the body until it is being layed out itself, at
		// which time the constraints are correct.
		e.executeOn.AddOpForNextLayout(func(gtx layout.Context) {
			e.executeOn.makeCursorVisibleByScrolling(gtx)
			e.executeOn.SetFocus(gtx)
		})
	} else {
		e.executeOn.makeCursorVisibleByScrolling(gtx)
		e.executeOn.SetFocus(gtx)
	}
}

func (e *editable) onPointerPrimaryButtonDrag(ps *PointerState) {
	// Extend the selection from the start to here
	rank := PrimarySelection
	if ps.currentPointerEvent.Modifiers&key.ModAlt > 0 && e.SelectionsPresent() {
		rank = SecondarySelection
	}

	e.scrollIfPointerEventNearEdge(ps)

	e.extendSelectionBeingBuilt(rank, ps.currentPointerEvent.runeIndex)
	e.lastSearchResult = nil
}

func (e *editable) onPointerTertiaryButtonDrag(ps *PointerState) {
	if !e.draggingTertiaryButton && len(e.overridingCursorIndices) > 0 {
		e.draggingTertiaryButton = true
		e.clearSelections()
		//TODO: set focus as in Primary Press?
		e.setToOneCursorIndex(e.overridingCursorIndices[0])
		e.overridingCursorIndices = nil
	}

	// Extend the selection from the start to here
	rank := PrimarySelection

	e.scrollIfPointerEventNearEdge(ps)

	e.extendSelectionBeingBuilt(rank, ps.currentPointerEvent.runeIndex)
	e.primarySelPurpose = SelectionPurposeExecute
}

func (e *editable) scrollIfPointerEventNearEdge(ps *PointerState) {
	if ps.currentPointerEvent.Position.Y < float32(e.lineHeight()) {
		e.ScrollOneLine(ps.gtx, Up)
	} else if ps.currentPointerEvent.Position.Y > float32((e.heightInLines(ps.gtx)-1)*e.lineHeight()) {
		e.ScrollOneLine(ps.gtx, Down)
	}
}

func (e *editable) onPointerRelease(ps *PointerState) {
	if ps.currentPointerEvent.Modifiers&key.ModCommand > 0 {
		// Treat as a tertiary release
		ps.pressedButtons = ps.pressedButtons & (^pointer.ButtonPrimary)
		ps.pressedButtons = ps.pressedButtons | pointer.ButtonTertiary
		e.onPointerTertiaryButtonRelease(ps)
		return
	}
	e.stopBuildingSelection()
}

func (e *editable) onPointerScroll(ps *PointerState) {
	direction := Down
	if ps.currentPointerEvent.Scroll.Y > 0 {
		direction = Down
	} else {
		direction = Up
	}

	if ps.currentPointerEvent.Modifiers&key.ModCtrl > 0 {
		e.adjustFontSizeOnScroll(direction)
		return
	}

	for i := 0; i < 3; i++ {
		e.ScrollOneLine(ps.gtx, direction)
	}
}

func (e *editable) adjustFontSizeOnScroll(direction verticalDirection) {
	style := e.adapter.style()
	for i := range style.Fonts {
		d := 1
		if direction == Down {
			d = -1
		}
		style.Fonts[i].FontSize += unit.Sp(d)
		if style.Fonts[i].FontSize < 1 {
			style.Fonts[i].FontSize = 1
		}
	}
	e.adapter.setStyle(style)

}

func (e *editable) prepareStylesChanges(gtx layout.Context) {
	e.styleSeq.Reset()
	e.initStyleChangesFromSelections(gtx)
	e.initStyleChangesFromSyntax(gtx)
	e.initStyleChangesFromManualHighlighting(gtx)
	e.styleSeq.Sort()
	e.styleChanges = e.styleSeq.Iter()
	e.styleChanges.ForwardTo(e.TopLeftIndex)
}

func (e *editable) renderTextWithStyles(gtx layout.Context, ltext typeset.Text) int {
	yoffset := 0
	lineStartIndex := e.TopLeftIndex

	stack := op.Offset(image.Point{}).Push(gtx.Ops)

	//log(LogCatgEd,"editable.renderTextWithStyles: for %s got %d lines of text\n", e.label, len(ltext.GetLines()))

	for i, line := range ltext.Lines() {
		e.renderLineWithStyles(gtx, &ltext, &line, &lineStartIndex, i == len(ltext.Lines())-1)

		yoffset += ltext.LineHeight()
		if yoffset > gtx.Constraints.Max.Y {
			yoffset = gtx.Constraints.Max.Y
			break
		}

		op.Offset(image.Point{0, e.lineHeight()}).Add(gtx.Ops)
	}
	stack.Pop()

	if ltext.EndsWith('\n') {
		yoffset += ltext.LineHeight()
	}

	return yoffset
}

func (e *editable) renderLineWithStyles(gtx layout.Context, ltext *typeset.Text, line *typeset.Line, lineStartIndex *int, isLastLine bool) {
	xoffset := 0

	moveRightBy := func(line *typeset.Line) {
		d := line.Width().Round()
		op.Offset(image.Point{d, 0}).Add(gtx.Ops)
		xoffset += d
	}

	stack := op.Offset(image.Point{}).Push(gtx.Ops)
	e.applyStyleFor(e.styleChanges.Active())
	lineLen := line.RuneCount()
	lastSplitIndex := *lineStartIndex

	for chg := e.styleChanges.Next(); chg != nil && line != nil; chg = e.styleChanges.Next() {
		nxt := chg.AbsolutePosition

		if nxt >= *lineStartIndex+lineLen {
			break
		}

		rel := nxt - lastSplitIndex
		lastSplitIndex = nxt
		first, rest := line.Split(rel)

		e.textRender.DrawTextline(gtx, first)
		e.styleChanges.ForwardTo(nxt)
		e.applyStyleFor(e.styleChanges.Active())
		moveRightBy(first)
		line = rest
	}

	// In case we are in a selection, draw the text background all the way to the right margin
	if !isLastLine {
		e.textRender.DrawTextBgRect(gtx, gtx.Constraints.Max.X-xoffset)
	}

	*lineStartIndex += lineLen

	if line != nil {
		e.textRender.DrawTextline(gtx, line)
	}

	stack.Pop()
}

func (e *editable) drawCursorIn(gtx layout.Context, ltext typeset.Text) {

	if e.adapter.focusedEditable() != e && e.overridingCursorIndices == nil {
		return
	}

	var pos []image.Point
	if e.overridingCursorIndices != nil {
		pos = e.findCursorsInSlice(gtx, &ltext, e.overridingCursorIndices, -1, -1)
	} else {
		pos = e.findCursorsIn(gtx, &ltext, -1, -1)
		//pos = e.findCursorsInSlice(gtx, &ltext, e.CursorIndices, -1, -1)
	}

	for _, pt := range pos {
		stack := op.Offset(pt).Push(gtx.Ops)
		e.drawCursor(gtx)
		stack.Pop()
	}
}

/*
findCursorsIn finds the screen coordinates of the cursors between [minCursor:maxCursor]. Use -1,-1 for the full range.
*/
func (e *editable) findCursorsIn(gtx layout.Context, ltext *typeset.Text, minCursor, maxCursor int) (positions []image.Point) {
	return e.findCursorsInSliceUnlessSelections(gtx, ltext, e.CursorIndices, minCursor, maxCursor)
}

func (e *editable) findCursorsInSliceUnlessSelections(gtx layout.Context, ltext *typeset.Text, cursorIndices []int, minCursor, maxCursor int) (positions []image.Point) {
	if e.SelectionsPresent() {
		return
	}
	return e.findCursorsInSlice(gtx, ltext, cursorIndices, minCursor, maxCursor)
}

func (e *editable) findCursorsInSlice(gtx layout.Context, ltext *typeset.Text, cursorIndices []int, minCursor, maxCursor int) (positions []image.Point) {
	if ltext.Empty() {
		positions = []image.Point{{0, 0}}
		return
	}

	const maxInt = int(^uint(0) >> 1)
	const invalidCursorIndex = maxInt

	cursorIndicesWithinLine := make([]int, len(cursorIndices))
	for i, v := range cursorIndices {
		if v < e.TopLeftIndex {
			cursorIndicesWithinLine[i] = invalidCursorIndex
		} else {
			cursorIndicesWithinLine[i] = v - e.TopLeftIndex
		}
	}

	forEachCursor := func(f func(index, position int) (stop bool)) (stop bool) {
		for j, cursorIndexWithinLine := range cursorIndicesWithinLine {
			if j < minCursor || j >= maxCursor {
				continue
			}
			if cursorIndicesWithinLine[j] == invalidCursorIndex {
				continue
			}

			stop = f(j, cursorIndexWithinLine)
			if stop {
				return
			}
		}
		return
	}

	lines := ltext.Lines()
	lastLineIndex := len(lines) - 1

	determineXWithinLine := func(line *typeset.Line, cursorIndexWithinLine int) (x int) {
		runeIndex := 0
		var xp fixed.Int26_6
		glyphs := line.Glyphs()
		for i := range line.Runes() {
			if cursorIndexWithinLine <= runeIndex {
				break
			}
			runeIndex++
			if i < len(glyphs) {
				xp += glyphs[i].Advance
			}
		}
		x = xp.Round()
		return
	}

	if minCursor == -1 {
		minCursor = 0
	}
	if maxCursor == -1 {
		maxCursor = len(cursorIndicesWithinLine)
	}

	y := 0
	for i, line := range lines {
		isLastLine := i == lastLineIndex
		lineLen := line.RuneCount()

		stop := forEachCursor(func(j, cursorIndexWithinLine int) (stop bool) {
			if cursorIndexWithinLine < lineLen || (cursorIndexWithinLine == lineLen && isLastLine && !line.EndsWith('\n')) {
				// This is the line
				x := determineXWithinLine(&line, cursorIndexWithinLine)
				// Draw a cursor here
				positions = append(positions, image.Point{x, y})

				if len(positions) >= len(cursorIndicesWithinLine) {
					stop = true
					return
				}
				cursorIndicesWithinLine[j] = invalidCursorIndex // Mark as completed
			}

			if cursorIndicesWithinLine[j] != invalidCursorIndex {
				cursorIndexWithinLine -= lineLen
				cursorIndicesWithinLine[j] = cursorIndexWithinLine
			}
			return
		})

		if stop {
			return
		}
		y += ltext.LineHeight()
	}

	forEachCursor(func(j, cursorIndexWithinLine int) bool {
		if cursorIndexWithinLine == 0 {
			x := 0
			positions = append(positions, image.Point{x, y})
		}
		return false
	})
	return
}

func (e *editable) findFirstCursorIn(gtx layout.Context, ltext *typeset.Text) (positions []image.Point) {
	return e.findCursorsIn(gtx, ltext, 0, 1)
}

func (e *editable) initStyleChangesFromSelections(gtx layout.Context) {
	for _, s := range e.selections {
		e.styleSeq.AddWithoutSort(s)
	}
}

func (e *editable) initStyleChangesFromSyntax(gtx layout.Context) {
	if e.syntaxTokens != nil {
		for _, i := range e.syntaxTokens {
			e.styleSeq.AddWithoutSort(i)
		}
	}

	e.addStyleChangesDueToAnsiColorEscapeSequences(gtx)
}

func (e *editable) initStyleChangesFromManualHighlighting(gtx layout.Context) {
	for _, i := range e.manualHighlighting {
		e.styleSeq.AddWithoutSort(i)
	}
}

func (e *editable) addStyleChangesDueToAnsiColorEscapeSequences(gtx layout.Context) {
	if !e.colorizeAnsiEscapes {
		return
	}

	txt := e.visibleText(gtx)

	if !ansi.HasEscapeCodes(txt) {
		return
	}

	newIntvl := func(start, end int, color color.NRGBA) intvl.Interval {
		return NewSyntaxInterval(start, end, Color(color))
	}

	seqs, err := ansi.HighlightColorEscapeSequences(txt, e.TopLeftIndex, newIntvl)
	if err != nil {
		log(LogCatgEd, "editable.addStyleChangesDueToAnsiColorEscapeSequences: error: %v\n", err)
	}

	for _, s := range seqs {
		e.styleSeq.AddWithoutSort(s)
	}
}

func (e *editable) HighlightSyntax() {
	// Since syntax highlighting the whole document is slow and CPU intensive there are a few
	// mechanisms to alleviate the issue.
	//
	// First, for simple changes the existing syntax tokens are
	// simply shifted (instead of being recomputed) and then the syntax highlighting is re-executed.
	// The shifting will give a mostly accurate result that is later corrected.
	//
	// Second, when text is changed there is a delay before running the syntax highlighting (see
	// editable.textChanged). If the text is repeatedly changed, the delay keeps getting restarted
	// so that we don't do any work for that long. This is to alleviate CPU usage. The delay begins
	// very low, since we optimistically assume this is a small document, but of highlighting
	// when measured takes a long time the delay is set to a higher value. This helps to make the
	// highlighting feel snappy for small documents.
	//
	// Third, when the highlighting is actually executed, if it doesn't complete within a timeout
	// it is cancelled and run in the background asynchronously. This is so that typing in a large
	// document doesn't seem to lag since the highlighting doesn't appear to take so long when it
	// does run.

	if e.syntaxHighlighter != nil && e.text.Len() < e.syntaxMaxDocSize {
		var err error
		toks, err := e.asyncHighlighter.Highlight(string(e.Bytes()))
		e.syntaxHighlightDelay = 1 * time.Millisecond
		if err != nil {
			log(LogCatgSyntax, "syntax highlighting failed: %v\n", err)
			// Keep existing tokens if it is a timeout, since we will get the tokens eventually.
			if err == ErrTimeout {
				toks = e.syntaxTokens
				e.syntaxHighlightDelay = 100 * time.Millisecond
			}
		}
		//log(LogCatgEd,"setting syntax tokens to %p after highlighting\n", toks)
		e.syntaxTokens = toks
	} else {
		//log(LogCatgSyntax,"%s: setting syntax tokens to nil\n", e.label)
		e.syntaxTokens = nil
	}
}

func (e *editable) BuildCompletions() {
	if e.completer != nil && e.text.Len() < e.completionMaxDocSize {
		e.completer.Build(e.completionSource, e.Bytes())
	}
}

func (e *editable) applyStyleFor(c []intvl.Interval) {
	e.textRender.SetDrawBg(false)

	if c == nil || len(c) == 0 {
		// Use the default style.
		e.textRender.SetFgColor(e.style.FgColor)
		return
	}

	// Process selections first. If there are any selections active, don't do syntax
	// highlighting.
	foundSel := false
	for _, intvl := range c {
		sel, ok := intvl.(*selection)
		if ok {
			foundSel = true
			if sel == e.primarySel {
				if e.primarySelPurpose == SelectionPurposeExecute {
					//e.textRender.SetFgColor(MustParseHexColor("#000000"))
					//e.textRender.SetBgColor(MustParseHexColor("#9b2226"))
					e.textRender.SetFgColor(e.style.ExecutionSelection.FgColor)
					e.textRender.SetBgColor(e.style.ExecutionSelection.BgColor)
				} else {
					e.textRender.SetFgColor(e.style.PrimarySelection.FgColor)
					e.textRender.SetBgColor(e.style.PrimarySelection.BgColor)
				}
			} else {
				e.textRender.SetFgColor(e.style.SecondarySelection.FgColor)
				e.textRender.SetBgColor(e.style.SecondarySelection.BgColor)
			}
			e.textRender.SetDrawBg(true)
		}
	}

	if !foundSel {
		for _, intvl := range c {
			syn, ok := intvl.(*SyntaxInterval)
			if ok {
				e.textRender.SetFgColor(syn.Color())
			}
		}
	}
}

func (e *editable) drawCursor(gtx layout.Context) {
	var path clip.Path

	lh := float32(e.lineHeight())

	pt := func(x, y int) f32.Point {
		xi := gtx.Metric.Dp(unit.Dp(x))
		yi := gtx.Metric.Dp(unit.Dp(y))
		return f32.Pt(float32(xi), float32(yi))
	}

	// Outer path
	path.Begin(gtx.Ops)
	path.Move(pt(-3, 0))

	path.Line(pt(7, 0))
	path.Line(pt(0, 3))
	path.Line(pt(-2, 0))
	// Move down line height less 6
	path.Line(f32.Pt(0, lh))
	path.Line(pt(0, -6))
	path.Line(pt(2, 0))
	path.Line(pt(0, 3))
	path.Line(pt(-7, 0))
	path.Line(pt(0, -3))
	path.Line(pt(2, 0))
	// Move up line height less 6
	path.Line(f32.Pt(0, -lh))
	path.Line(pt(0, 6))
	path.Line(pt(-2, 0))
	path.Line(pt(0, -3))
	path.Close()

	stack := clip.Outline{Path: path.End()}.Op().Push(gtx.Ops)

	paint.ColorOp{Color: color.NRGBA{A: 0xff}}.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)

	stack.Pop()

	// Inner path
	path.Begin(gtx.Ops)
	path.Move(pt(-2, 1))

	path.Line(pt(5, 0))
	path.Line(pt(0, 1))
	path.Line(pt(-2, 0))
	path.Line(f32.Pt(0, lh))
	path.Line(pt(0, -4))
	path.Line(pt(2, 0))
	path.Line(pt(0, 1))

	path.Line(pt(-5, 0))
	path.Line(pt(0, -1))
	path.Line(pt(2, 0))
	path.Line(f32.Pt(0, -lh))
	path.Line(pt(0, 4))
	path.Line(pt(-2, 0))
	path.Line(pt(0, 1))

	path.Close()

	stack = clip.Outline{Path: path.End()}.Op().Push(gtx.Ops)

	paint.ColorOp{Color: color.NRGBA{R: 0xf0, G: 0xf0, B: 0xf0, A: 0xff}}.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)

	stack.Pop()
}

func (e *editable) InsertText(text string) {
	e.invalidateLayedoutText()

	startTransaction := func() {
		if len(text) == 1 && len(e.CursorIndices) == 1 {
			// This is likely the user typing just after matching-bracket insertion.
			// Don't undo both changes together; treat this separately
			return
		}
		e.text.StartTransaction()
		e.SetSaveDeletes(false)
	}

	endTransaction := func() {
		if len(text) == 1 && len(e.CursorIndices) == 1 {
			return
		}
		e.SetSaveDeletes(true)
		e.text.EndTransaction()
	}

	if e.SelectionsPresent() {
		startTransaction()
		e.replaceAllSelectionsWith(text)
		endTransaction()
		e.changeSelectionsToCursors(Right)
		return
	}

	if e.matchingBracketInsertion.InsertMatchingBrackets(e, text) {
		return
	}

	startTransaction()
	e.InsertTextAtEachCursor(text)
	endTransaction()
}

func (e *editable) InsertTextAtEachCursor(text string) {
	for i, ndx := range e.CursorIndices {
		e.insertToPieceTable(ndx, text)
		e.CursorIndices[i] += utf8.RuneCountInString(text)
	}
}

func (e *editable) InsertTextAtCursors(text []string) {
	e.text.StartTransaction()
	e.SetSaveDeletes(false)

	if len(text) > len(e.CursorIndices) {
		for i := len(e.CursorIndices); i < len(text); i++ {
			e.AddNewCursorBelowLast()
		}
	}

	for i, ndx := range e.CursorIndices {
		if i < len(text) {
			e.insertToPieceTable(ndx, text[i])
			e.CursorIndices[i] += utf8.RuneCountInString(text[i])
		}
	}

	e.SetSaveDeletes(true)
	e.text.EndTransaction()
}

func (e *editable) DelimitSelectionsWithCursors() {

	var cursors []int

	for _, sel := range e.selections {
		cursors = append(cursors, sel.start)
		cursors = append(cursors, sel.end)
	}

	e.SetCursorIndices(cursors)
}

func (e *editable) InsertTextAndSelect(text string) {
	if e.SelectionsPresent() {
		e.InsertText(text)
		return
	}

	l := utf8.RuneCountInString(text)
	e.InsertText(text)

	e.clearSelections()
	for _, ndx := range e.CursorIndices {
		e.addSecondarySelection(ndx-l, ndx, Right)
	}
}

// Returns (-1,-1) if not found.
func (e *editable) Search(startRuneIndex int, needle string, direction direction) (start, end int) {
	if len(needle) > 2 && needle[0] == '/' && needle[len(needle)-1] == '/' {
		return e.SearchForRegexp(startRuneIndex, needle[1:len(needle)-1], direction)
	} else {
		return e.SearchForLiteral(startRuneIndex, needle, direction)
	}
}

func (e *editable) SearchForLiteral(startRuneIndex int, needle string, direction direction) (start, end int) {
	b := e.Bytes()
	w := runes.NewWalker(b)
	_ = w.SetRunePosCache(startRuneIndex, &e.runeOffsetCache)
	nb := []byte(needle)

	var byteIndex int
	if direction == Forward {
		byteIndex = boyermoore.Index(b[w.BytePos():], nb)
	} else {
		byteIndex = boyermoore.IndexRev(b[:w.BytePos()], nb)
	}

	if byteIndex < 0 {
		return -1, -1
	}
	if direction == Reverse {
		w.SetRunePos(0)
	}
	w.ForwardBytes(byteIndex)

	//return boyermoore.IndexWithTable(&tbl, string(b[startRuneIndex:]), needle) + startRuneIndex
	return w.RunePos(), w.RunePos() + utf8.RuneCountInString(needle)
}

func (e *editable) SearchForRegexp(startRuneIndex int, needle string, direction direction) (start, end int) {
	b := e.Bytes()
	w := runes.NewWalker(b)
	_ = w.SetRunePosCache(startRuneIndex, &e.runeOffsetCache)

	var err error
	if direction == Reverse {
		needle, err = regex.ReverseRegex(needle)
		if err != nil {
			e.adapter.appendError("", err.Error())
			return -1, -1
		}
	}

	re, err := expr.CompileRegexpWithMultiline(needle)
	if err != nil {
		e.adapter.appendError("", err.Error())
		return -1, -1
	}

	log(LogCatgEd, "editable.SearchForRegexp: searching %s for regex %s\n", direction, needle)

	var loc []int
	if direction == Forward {
		b = b[w.BytePos():]
		loc = re.FindIndex(b)
	} else {
		b = b[:w.BytePos()]
		// TODO: this could be rediculously slow.
		r := make([]byte, len(b))
		copy(r, b)
		reverse(r)
		b = r
		loc = re.FindIndex(b)
	}

	if loc == nil {
		return -1, -1
	}

	if direction == Reverse {
		loc[1], loc[0] = loc[0], loc[1]
		loc[0] = len(b) - loc[0]
		loc[1] = len(b) - loc[1]
	}

	match := b[loc[0]:loc[1]]

	if direction == Reverse {
		w.SetRunePos(0)
	}
	w.ForwardBytes(loc[0])

	//return boyermoore.IndexWithTable(&tbl, string(b[startRuneIndex:]), needle) + startRuneIndex
	return w.RunePos(), w.RunePos() + utf8.RuneCount(match)
}

func (e *editable) cutText(gtx layout.Context, sel *selection) {
	log(LogCatgEd, "editable.cutText: selection: %v\n", sel)
	ci := sel.start
	e.copyText(gtx, sel)
	e.deleteFromPieceTableUndoIndex(sel.start, sel.Len(), e.firstCursorIndex())
	e.clearSelections()
	e.setToOneCursorIndex(ci)
	e.makeCursorVisibleByScrolling(gtx)
}

func (e *editable) copyText(gtx layout.Context, sel *selection) {
	text := e.textOfSelection(sel)
	log(LogCatgEd, "Setting clipboard to '%s'\n", string(text))
	e.writeTextToClipboard(gtx, text)
}

func (e *editable) writeTextToClipboard(gtx layout.Context, text string) {
	buf := bytes.NewBufferString(text)
	data := nopCloser{buf}
	cmd := clipboard.WriteCmd{Type: "text/plain", Data: data}
	gtx.Execute(cmd)
}

type nopCloser struct {
	io.Reader
}

func (nopCloser) Close() error {
	return nil
}

func (e *editable) cutAllSelectedText(gtx layout.Context) {
	var buf bytes.Buffer
	sels := e.selectionsInDisplayOrder()

	ci := 0
	if e.primarySel != nil {
		ci = e.primarySel.Start()
	} else if len(sels) > 0 {
		ci = sels[0].Start()
	}

	var selTexts []string
	for _, s := range sels {
		t := e.textOfSelection(s)
		selTexts = append(selTexts, t)
		buf.WriteString(t)
	}
	editor.SetLastSelectionsWrittenToClipboard(selTexts)

	e.StartTransaction()
	for _, s := range sels {
		e.deleteFromPieceTableUndoIndex(s.Start(), s.Len(), e.firstCursorIndex())
	}
	e.EndTransaction()

	e.clearSelections()
	e.setToOneCursorIndex(ci)
	e.makeCursorVisibleByScrolling(gtx)

	e.writeTextToClipboard(gtx, buf.String())
}

func (e *editable) copyAllSelectedText(gtx layout.Context) {
	var buf bytes.Buffer
	sels := e.selectionsInDisplayOrder()

	var selTexts []string
	for _, s := range sels {
		t := e.textOfSelection(s)
		selTexts = append(selTexts, t)
		buf.WriteString(t)
	}
	editor.SetLastSelectionsWrittenToClipboard(selTexts)

	log(LogCatgEd, "%s: copying this text to clipboard: '%s'\n", e.label, buf.String())

	e.writeTextToClipboard(gtx, buf.String())
}

func (e *editable) Tag() event.Tag {
	if e.tag != nil {
		return e.tag
	}
	return e
}

func (e *editable) insertToPieceTable(index int, text string) {
	e.editableModel.insertToPieceTableUndoIndex(index, text, index)
	e.invalidateLayedoutText()
	l := utf8.RuneCountInString(text)
	e.textChanged(fireListeners, NewTextChange(index, l))
	e.updateRecentlyTypedTextWithInsertion(index, l)
}

func (e *editable) insertToPieceTableUndoIndex(index int, text string, undoIndex int) {
	e.editableModel.insertToPieceTableUndoIndex(index, text, undoIndex)
	e.invalidateLayedoutText()
	l := utf8.RuneCountInString(text)
	e.textChanged(fireListeners, NewTextChange(index, l))
	e.updateRecentlyTypedTextWithInsertion(index, l)
}

func (e *editable) deleteFromPieceTable(index, length int) {
	e.deleteFromPieceTableUndoIndex(index, length, index)
	e.invalidateLayedoutText()
	e.textChanged(fireListeners, NewTextChange(index, -length))
}

func (e *editable) deleteFromPieceTableUndoIndex(index, length, undoIndex int) {
	e.editableModel.deleteFromPieceTableUndoIndex(index, length, undoIndex)
	e.invalidateLayedoutText()
	e.textChanged(fireListeners, NewTextChange(index, -length))
	e.updateRecentlyTypedTextWithDeletion(index, length)
}

type typingInSelectedTextAction int

const (
	replaceSelectionsWithText typingInSelectedTextAction = iota
	appendTextToSelections
)

func (t typingInSelectedTextAction) String() string {
	switch t {
	case replaceSelectionsWithText:
		return "replaceSelectionsWithText"
	case appendTextToSelections:
		return "appendTextToSelections"
	default:
		return "unknown"
	}
}

func (e *editable) AddOpForNextLayout(op LayoutOp) {
	e.opsForNextLayout.Add(op)
}

func (e *editable) SetTopLeft(topLeft int) {
	e.editableModel.SetTopLeft(topLeft)
	e.invalidateLayedoutText()
}

func (e *editable) SetFocus(gtx layout.Context) {
	if gtx.Ops == nil {
		return
	}
	log(LogCatgEd, "Setting focus to editable %s\n", e.label)
	gtx.Execute(key.FocusCmd{Tag: e.Tag()})
	e.adapter.setFocusedEditable(e)
	e.adapter.clearEditableWhereTertiaryButtonHoldStarted()
}

func (e *editable) schedule(id string, d time.Duration, f func()) {
	if e.Scheduler == nil {
		log(LogCatgEd, "editable: can't schedule %s: scheduler is nil\n", id)
		return
	}

	e.Scheduler.AfterFunc(id, d, f)
}

func (e *editable) doWordCompletion(ctx completionContext, direction direction) {

	moveCurrentWordToEndOfCompletions := func(comps []words.Completion) {
		slice.FindAndMoveToEnd(comps, func(i int) bool { return comps[i].Word() == ctx.word })
	}

	if e.completer != nil {
		if e.wordCompletion.NeedCompletions() {
			comps, _ := e.completer.Completions(ctx.prefix)
			moveCurrentWordToEndOfCompletions(comps)
			e.wordCompletion.SetCompletions(e.convertCompletionsToWorders(comps))
		}
		e.wordCompletion.ApplyCompletion(ctx, direction)
	}
}

func (e *editable) convertCompletionsToWorders(comps []words.Completion) []Worder {
	var w []Worder
	for _, c := range comps {
		w = append(w, c)
	}
	return w
}

type fileCompletion string

func (fc fileCompletion) Word() string {
	return string(fc)
}

func (e *editable) convertStringsToWorders(comps []string) []Worder {
	converted := make([]Worder, len(comps))
	for i, e := range comps {
		converted[i] = fileCompletion(e)
	}
	return converted
}

func (e *editable) showCompletions(dir string, comps []Worder) {
	var text bytes.Buffer

	fmt.Fprintf(&text, "Completions:\n")
	for _, c := range comps {
		sourcer, ok := c.(Sourcer)
		if ok {
			fmt.Fprintf(&text, "%s  %s\n", c.Word(), sourcer.Sources())
		} else {
			fmt.Fprintf(&text, "%s\n", c.Word())
		}

	}
	e.adapter.appendError(dir, text.String())
}

func (e *editable) doFilenameCompletion(ctx completionContext, direction direction) {
	cb := func(completions []string) {
		ndx := e.firstCursorIndex()
		ctx := e.filenameObjectToComplete(ndx)
		e.applyFilenameCompletions(completions, ctx, direction)
	}
	e.adapter.completeFilename(ctx.prefix, cb)
}

func (e *editable) applyFilenameCompletions(comps []string, ctx completionContext, direction direction) {

	moveCurrentWordToEndOfCompletions := func(comps []string) {
		slice.FindAndMoveToEnd(comps, func(i int) bool { return comps[i] == ctx.word })
	}

	if e.fileCompletion.NeedCompletions() {
		moveCurrentWordToEndOfCompletions(comps)
		e.fileCompletion.SetCompletions(e.convertStringsToWorders(comps))
	}
	e.fileCompletion.ApplyCompletion(ctx, direction)
}

func (e *editable) makeExprHandler() *ExprHandler {
	file := e.adapter.displayPath().String()
	dir := e.adapter.dir()

	data := e.text.Bytes()

	afterChanged := func() {
		e.moveCursorToStartOfPrimarySelection()
		e.invalidateLayedoutText()
		e.textChanged(fireListeners, TextChange{})
		e.AddOpForNextLayout(func(gtx layout.Context) {
			e.makeCursorVisibleByScrolling(gtx)
			e.SetFocus(gtx)
		})
	}

	return &ExprHandler{
		pieceTable:   e.text,
		file:         file,
		dir:          dir,
		data:         data,
		editable:     e,
		afterChanged: afterChanged,
		cursorIndex:  e.firstCursorIndex(),
	}
}

func (e *editable) ColorizeAnsiEscapes(b bool) {
	e.colorizeAnsiEscapes = b
}

func (e *editable) NextFont() {
	e.nextFont()
	e.invalidateLayedoutText()
	e.initTextRenderer()
}

func (e *editable) AddTextChangeListener(f func(*TextChange)) {
	e.textChangedListeners = append(e.textChangedListeners, f)
}

type TextChangeListener interface {
	TextChanged(c *TextChange)
}

func (e *editable) asyncSyntaxHighlightingDone(tokens []intvl.Interval, err error) {
	if err != nil {
		log(LogCatgSyntax, "asyncSyntaxHighlightingDone: Error highlighting: %v\n", err)
		return
	}

	e.adapter.doWork(setSyntaxTokens{e, tokens})
}

func (e *editable) SetCursorIndices(cursors []int) {

	if len(cursors) == 0 {
		return
	}

	for _, c := range cursors {
		if c < 0 || c > e.Len() {
			return
		}
	}

	if e.SelectionsPresent() {
		e.clearSelections()
	}

	e.CursorIndices = cursors

	e.removeDuplicateCursors()
	e.AddOpForNextLayout(func(gtx layout.Context) {
		e.makeCursorVisibleByScrolling(gtx)
	})
}

func (e *editable) cursorIndexWithin(startIndex, endIndex int) int {
	for i, c := range e.CursorIndices {
		if c >= startIndex && c < endIndex {
			return i
		}
	}
	return -1
}

func (e *editable) FocusChanged(gtx layout.Context, ev *key.FocusEvent) {
	e.overridingCursorIndices = nil
}

func (e *editable) SetStyle(style editableStyle) {
	e.style = style
	e.layouter.setFontStyles(style.Fonts)
	e.layouter.lineSpacing = style.LineSpacing
	e.initTextRenderer()
	e.invalidateLayedoutText()
}

func (e *editable) ReplaceSelectionWith(sel *selection, text string) {
	e.replaceSelectionWith(sel, text)
	e.invalidateLayedoutText()
	l := utf8.RuneCountInString(text)
	e.notifyTextChangeListeners(NewTextChange(sel.Start(), l))
}

func (e *editable) AppendToSelection(sel *selection, text string) {
	e.appendToSelection(sel, text)
	e.invalidateLayedoutText()
	l := utf8.RuneCountInString(text)
	e.notifyTextChangeListeners(NewTextChange(sel.Start()+sel.Len(), l))
}

func (e *editable) attemptBlockPaste(text string) (success bool) {
	if len(editor.LastSelectionsWrittenToClipboard()) < 2 || e.SelectionsPresent() {
		return
	}

	var buf bytes.Buffer
	for _, t := range editor.LastSelectionsWrittenToClipboard() {
		buf.WriteString(t)
	}

	if buf.String() != text {
		return
	}

	lt := editor.LastSelectionsWrittenToClipboard()
	e.InsertTextAtCursors(lt)

	e.clearSelections()
	for i, ndx := range e.CursorIndices {
		if i >= len(lt) {
			break
		}
		l := utf8.RuneCountInString(lt[i])
		e.addSecondarySelection(ndx-l, ndx, Right)
	}

	return true
}

type setSyntaxTokens struct {
	e      *editable
	tokens []intvl.Interval
	//syntaxTokens               []intvl.Interval
}

func (s setSyntaxTokens) Job() Job {
	return nil
}

func (s setSyntaxTokens) Service() (done bool) {
	log(LogCatgSyntax, "Setting syntax tokens from background\n")
	s.e.syntaxTokens = s.tokens
	return true
}

func reverse(s []byte) {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
}

type matchingBracketInsertion struct {
	active             bool
	opener, closer     rune
	savedCursorIndices []int
}

func (m *matchingBracketInsertion) InsertMatchingBrackets(e *editable, textEntered string) (inserted bool) {
	// Enter in the matching brackets by alternating
	m.active = false

	if len(e.CursorIndices) == 0 || len(textEntered) == 0 || len(e.CursorIndices)%2 != 0 {
		log(LogCatgEd, "InsertMatchingBrackets: no cursor indices, no text, or cursor indices is not even. Cursor indices: %d text: '%s'\n",
			len(e.CursorIndices), textEntered)
		return
	}

	r, _ := utf8.DecodeRuneInString(textEntered)
	if !runes.IsABracket(r) {
		log(LogCatgEd, "InsertMatchingBrackets: A bracket was not entered (got '%c')\n", r)
		return
	}

	m.opener, m.closer = runes.MatchingBracket(r)
	if r != m.opener {
		log(LogCatgEd, "InsertMatchingBrackets: A bracket was entered, but it is not an opening bracket (got '%c')\n", r)
		return
	}

	m.active = true

	m.insertMatchingBrackets(e)
	return true
}

func (m *matchingBracketInsertion) insertMatchingBrackets(e *editable) {
	m.savedCursorIndices = make([]int, len(e.CursorIndices))
	copy(m.savedCursorIndices, e.CursorIndices)

	e.text.StartTransaction()
	even := true
	sort.Ints(e.CursorIndices)
	for i, ndx := range e.CursorIndices {
		t := m.closer
		if even {
			t = m.opener
		}

		text := string(t)
		e.insertToPieceTable(ndx, text)
		e.CursorIndices[i] += utf8.RuneCountInString(text)
		even = !even
	}
	e.text.EndTransaction()
}

func (m *matchingBracketInsertion) Undo(gtx layout.Context, e *editable) (undone bool) {
	// Undo the last change, and enter in the brackets the user really wanted (stored in m.bracket)
	if !m.active {
		return
	}

	log(LogCatgEd, "InsertMatchingBrackets: handling undo\n")
	e.Undo(gtx)
	m.closer = m.opener
	e.CursorIndices = m.savedCursorIndices
	m.insertMatchingBrackets(e)

	m.active = false
	return true
}

type Worder interface {
	Word() string
}

type Sourcer interface {
	Sources() []string
}

type completion struct {
	context              completionContext
	completions          []Worder
	editable             *editable
	dir                  string
	completionInProgress bool
	completionToShow     int
}

func NewCompletion(e *editable) completion {
	return completion{editable: e}
}

func (c *completion) Reset() {
	c.completionInProgress = false
}

func (c *completion) NeedCompletions() bool {
	return !c.isCompletionInProgress()
}

func (c *completion) SetCompletions(completions []Worder) {
	c.completions = completions
}

func (c *completion) ApplyCompletion(ctx completionContext, direction direction) {
	if c.isCompletionInProgress() {
		delta := 1
		if direction == Reverse {
			delta = len(c.completions) - 1
		}
		c.completionToShow = (c.completionToShow + delta) % len(c.completions)
		c.applyNextCompletion()
		return
	}

	c.beginNewCompletion(ctx)
}

func (c *completion) isCompletionInProgress() bool {
	return c.completionInProgress
}

func (c *completion) beginNewCompletion(ctx completionContext) {
	if len(c.completions) == 0 {
		return
	}

	c.completionInProgress = true
	c.completionToShow = 0
	c.context = ctx

	if len(c.completions) > 1 {
		c.editable.showCompletions(c.dir, c.completions)
	}
	c.replaceWordWithCurrentCompletion()
}

func (c *completion) replaceWordWithCurrentCompletion() {
	c.editable.deleteFromPieceTableUndoIndex(c.context.wordStartIndex, c.context.wordEndIndex-c.context.wordStartIndex, c.context.prefixEndIndex)
	s := c.completions[c.completionToShow].Word()
	l := utf8.RuneCountInString(s)
	c.editable.insertToPieceTable(c.context.wordStartIndex, s)
	c.context.wordEndIndex = c.context.wordStartIndex + l
	c.editable.clearSelections()
	//c.editable.SetCursorIndex(0, c.context.prefixEndIndex/)
	c.editable.SetCursorIndex(0, c.context.wordEndIndex)
}

/*
func (c *completion) replaceWordWithCurrentCompletion() {
	// The cursor is at the end of the prefix to be completed.
	c.editable.deleteFromPieceTableUndoIndex(c.context.wordStartIndex, c.context.wordEndIndex-c.context.wordStartIndex, c.context.prefixEndIndex)
	s := c.completions[c.completionToShow].Word()
	l := utf8.RuneCountInString(s)
	c.editable.insertToPieceTable(c.context.wordStartIndex, s)
	c.context.wordEndIndex = c.context.wordStartIndex + l
	c.editable.clearSelections()
	c.editable.SetCursorIndex(0, c.context.prefixEndIndex)
}
*/

func (c *completion) applyNextCompletion() {
	c.replaceWordWithCurrentCompletion()
}

func (c *completion) shiftDueToTextModification(startOfChange, lenOfChange int) {
	if startOfChange >= c.context.prefixStartIndex {
		return
	}

	c.context.prefixStartIndex += lenOfChange
	c.context.prefixEndIndex += lenOfChange
	c.context.wordStartIndex += lenOfChange
	c.context.wordEndIndex += lenOfChange
}

type editableWriteLock struct {
	locked bool
}

func (e *editableWriteLock) lock() {
	e.locked = true
}

func (e *editableWriteLock) unlock() {
	e.locked = false
}

func (e *editableWriteLock) isLocked() bool {
	return e.locked
}

// motionItem is an item that we want to adjust when an arrow key, or home or end is pressed.
// They are either cursors we want to move, or a selection that we want to extend.
type motionItem interface {
	position() int
	setPosition(int)
}

type cursorMotionItem struct {
	e           *editable
	cursorIndex int
}

func (m cursorMotionItem) position() int {
	return m.e.CursorIndices[m.cursorIndex]
}

func (m cursorMotionItem) setPosition(v int) {
	m.e.CursorIndices[m.cursorIndex] = v
}

type selectionMotionItem struct {
	e   *editable
	sel *selection
}

func (m selectionMotionItem) position() int {
	if m.sel.adjustSide == Left {
		return m.sel.Start()
	}
	return m.sel.End()
}

func (m selectionMotionItem) setPosition(v int) {
	if m.sel.adjustSide == Left {
		m.sel.textRange.start = v
	} else {
		m.sel.textRange.end = v
	}
	m.sel.Reorient()

	// If the selection now overlaps with another selection, truncate it.
	for _, s := range m.e.selections {
		if s != m.sel {
			m.sel.Cut(&s.textRange)
		}
	}
}

type motionItems interface {
	items() []motionItem
	doneAdjusting(gtx layout.Context)
}

type cursorsMotionItems struct {
	e *editable
}

func newCursorsMotionItems(e *editable) cursorsMotionItems {
	r := cursorsMotionItems{e: e}
	return r
}

func (m cursorsMotionItems) items() []motionItem {
	r := []motionItem{}
	for i := range m.e.CursorIndices {
		r = append(r, cursorMotionItem{m.e, i})
	}
	return r
}

func (m cursorsMotionItems) doneAdjusting(gtx layout.Context) {
	m.e.removeDuplicateCursors()
	m.e.makeCursorVisibleByScrolling(gtx)
}

type selectionsMotionItems struct {
	e *editable
}

func newSelectionMotionItems(e *editable, adjustSide horizontalDirection) selectionsMotionItems {
	if !e.SelectionsPresent() {
		// Make a new selection for each cursor
		for _, c := range e.CursorIndices {
			e.addSecondarySelection(c, c, adjustSide)
		}
	}

	r := selectionsMotionItems{e: e}
	return r
}

func (m selectionsMotionItems) items() []motionItem {
	r := []motionItem{}
	for _, s := range m.e.selections {
		r = append(r, selectionMotionItem{m.e, s})
	}
	return r
}

func (m selectionsMotionItems) doneAdjusting(gtx layout.Context) {
	if len(m.e.selections) > 0 {
		editor.setLastSelection(m.e, m.e.selections[len(m.e.selections)-1])
	}
	m.e.selectionsModified()
}

func (e *editable) RotateSelections() {
	e.editableModel.RotateSelections()
	e.invalidateLayedoutText()
	e.textChanged(fireListeners, TextChange{})
}

func (e *editable) SetElastic(b bool) {
	e.elastic = b
}
