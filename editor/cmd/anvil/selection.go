package main

import (
	"fmt"
	"sort"

	"github.com/jeffwilliams/anvil/editor/internal/intvl"
	"github.com/jeffwilliams/anvil/editor/internal/runes"
	"github.com/jeffwilliams/anvil/editor/internal/slice"
)

// textRange represents a segment of text between [start, end).
type textRange struct {
	start, end int
}

// selection represents a selected segment of text between [start, end).
type selection struct {
	textRange
	// adjustSide is the side of the selection that gets adjusted when a keyboard motion
	// occurs
	adjustSide horizontalDirection
}

type selectionPurpose int

const (
	SelectionPurposeSelect selectionPurpose = iota
	SelectionPurposeExecute
)

func NewTextRange(start, end int) textRange {
	r := textRange{start, end}
	r.Reorient()
	return r
}

func NewSelection(start, end int, adjustSide horizontalDirection) selection {
	s := selection{textRange{start, end}, adjustSide}
	s.Reorient()
	return s
}

func NewSelectionPtr(start, end int, adjustSide horizontalDirection) *selection {
	s := &selection{textRange{start, end}, adjustSide}
	s.Reorient()
	return s
}

func (s textRange) String() string {
	return fmt.Sprintf("[%d,%d)", s.start, s.end)
}

func (s textRange) Start() int {
	return s.start
}

func (s textRange) End() int {
	return s.end
}

func (s textRange) Overlaps(o *textRange) bool {
	return intvl.Overlaps(s, o)
}

func (s textRange) Valid() bool {
	return s.end >= 0 && s.start >= 0 && s.end >= s.start
}

// Cut removes the part of 's' that overlaps with 'r'.
func (s *textRange) Cut(r *textRange) {
	if s.start < r.end && s.end >= r.end {
		s.start = r.end
	}

	if s.end > r.start && s.start <= r.start {
		s.end = r.start
	}
}

// Reorient ensures that the start of the range is before the end of the range, and if not swaps
// the start and end.
func (s *textRange) Reorient() (swapped bool) {
	if s.end < s.start {
		swapped = true
		s.start, s.end = s.end, s.start
	}
	return
}

func (s *textRange) Len() int {
	return s.end - s.start
}

// Reorient ensures that the start of the range is before the end of the range, and if not swaps
// the start and end.
func (s *selection) Reorient() {
	swapped := s.textRange.Reorient()
	if swapped {
		if s.adjustSide == Left {
			s.adjustSide = Right
		} else {
			s.adjustSide = Left
		}
	}
}

func (e *editableModel) clearSelections() {
	if e.selections != nil {
		e.selections = e.selections[:0]
	}
	e.primarySel = nil
	e.primarySelPurpose = SelectionPurposeSelect
	e.selectionBeingBuilt = nil
}

func (e *editable) clearSelections() {
	e.editableModel.clearSelections()
	editor.clearLastSelectionIfOwnedBy(e)
}

func (e *editableModel) addSelection(s *selection) {
	e.removeSelectionsOverlappingWith(s)
	e.selections = append(e.selections, s)
	e.selectionsModified()
}

func (e *editableModel) removeSelectionsOverlappingWith(s *selection) {
	var remove []*selection
	for _, o := range e.selections {
		if o != s && o.Overlaps(&s.textRange) {
			remove = append(remove, o)
		}
	}

	for _, o := range remove {
		e.removeSelection(o)
	}
}

func (e *editableModel) numberOfSelections() int {
	return len(e.selections)
}

func (e *editableModel) SelectionsPresent() bool {
	return e.numberOfSelections() > 0
}

func (e *editable) selectionStartingFirst() *selection {
	if len(e.selections) == 0 {
		return nil
	}

	min := e.selections[0]
	for _, s := range e.selections {
		if s.start < min.start {
			min = s
		}
	}
	return min
}

func (e *editableModel) removeSelection(sel *selection) {
	match := func(i int) bool {
		return e.selections[i] == sel
	}
	s := slice.RemoveFirstMatchFromSlice(e.selections, match)
	e.selections = s.([]*selection)
	if sel == e.primarySel {
		e.primarySel = nil
	}
	e.selectionsModified()
}

func (e *editable) extendSelectionBeingBuilt(rank SelectionRank, index int) {
	if e.selectionBeingBuilt == nil {
		e.startBuildingSelection(rank)
	}

	e.removeSelectionsOverlappingWith(e.selectionBeingBuilt)

	ci := e.lastCursorIndex()

	if index <= ci {
		e.selectionBeingBuilt.start = index
		e.selectionBeingBuilt.end = ci
	} else {
		e.selectionBeingBuilt.start = ci
		e.selectionBeingBuilt.end = index
	}

	if len(e.selections) == 1 {
		e.primarySel = e.selectionBeingBuilt
	}
	e.selectionsModified()
}

func (e *editable) startBuildingSelection(rank SelectionRank) {
	sel := NewSelectionPtr(e.firstCursorIndex(), e.firstCursorIndex(), Right)
	e.addSelection(sel)
	e.selectionBeingBuilt = sel
	if rank == PrimarySelection {
		e.primarySel = sel
		e.primarySelPurpose = SelectionPurposeSelect
		editor.setLastSelection(e, e.primarySel)
	}
}

func (e *editable) setPrimarySelection(start, end int) {
	if e.primarySel != nil {
		e.primarySel.start = start
		e.primarySel.end = end
		editor.setLastSelection(e, e.primarySel)
		return
	}

	e.addPrimarySelection(start, end)
}

func (e *editable) addPrimarySelection(start, end int) {
	sel := NewSelectionPtr(start, end, Right)
	e.addSelection(sel)
	e.primarySel = sel
	e.primarySelPurpose = SelectionPurposeSelect
	editor.setLastSelection(e, sel)
	e.selectionsModified()
}

func (e *editableModel) selectionsModified() {
	e.typingInSelectedTextAction = replaceSelectionsWithText
}

func (e *editableModel) addSecondarySelection(start, end int, adjustSide horizontalDirection) (setPrimary bool) {
	sel := NewSelectionPtr(start, end, adjustSide)
	e.addSelection(sel)
	if e.primarySel == nil {
		e.primarySel = sel
		setPrimary = true
	}
	return
}

func (e *editable) addSecondarySelection(start, end int, adjustSide horizontalDirection) {
	setPrimary := e.editableModel.addSecondarySelection(start, end, adjustSide)
	if setPrimary {
		editor.setLastSelection(e, e.primarySel)
	}
}

func (e *editableModel) selectionContaining(runeIndex int) *selection {
	for _, s := range e.selections {
		if s.start <= runeIndex && s.end > runeIndex {
			return s
		}
	}
	return nil
}

func (e *editable) dumpSelections() {
	sort.Slice(e.selections, func(a, b int) bool {
		return e.selections[a].start < e.selections[b].start
	})

	for _, sel := range e.selections {
		if e.primarySel == sel {
			log(LogCatgEd, "primary selection [%d,%d]\n", sel.start, sel.end)
		} else {
			log(LogCatgEd, "secondary selection [%d,%d]\n", sel.start, sel.end)
		}
	}
}

func (e *editableModel) textOfSelection(s *selection) string {

	w := runes.NewWalker(e.Bytes())
	if s != nil {
		return string(w.TextBetweenRuneIndices(s.start, s.end))
	}
	return ""
}

func (e *editable) textOfPrimarySelection() (text string, ok bool) {
	if e.primarySel == nil {
		return
	}

	text = e.textOfSelection(e.primarySel)
	ok = true
	return
}

func (e *editableModel) shiftSelectionsDueToTextModification(startOfChange, lengthOfChange int) {
	for _, s := range e.selections {
		e.shiftTextRangeDueToTextModification(&s.textRange, startOfChange, lengthOfChange)
	}

	if e.immutableRange.Len() != 0 {
		// We know that the user can't be typing within the immutable range, so they must be instead typing just
		// before the beginning of the immutable range
		e.shiftTextRangeDueToTextModificationBounds(&e.immutableRange, startOfChange, lengthOfChange, changeAtBoundsIsNotWithinSelection)
	}
}

func (e *editableModel) shiftTextRangeDueToTextModification(r *textRange, startOfChange, lengthOfChange int) {
	r.start, r.end = computeShiftNeededDueToTextModification(r, startOfChange, lengthOfChange)
}

func (e *editableModel) shiftTextRangeDueToTextModificationBounds(r *textRange, startOfChange, lengthOfChange int, b changeAtSelectionBoundsBehaviour) {
	r.start, r.end = computeShiftNeededDueToTextModificationBounds(r, startOfChange, lengthOfChange, b)
}

func (e *editable) stopBuildingSelection() {
	e.selectionBeingBuilt = nil

	newSel := make([]*selection, 0, len(e.selections))

	for _, s := range e.selections {
		if s.Len() == 0 {
			if s == e.primarySel {
				e.primarySel = nil
			}
			continue
		}
		newSel = append(newSel, s)
	}
	e.selections = newSel
}

type SelectionRank int

const (
	PrimarySelection SelectionRank = iota
	SecondarySelection
)

func (e *editable) copySelections() []*selection {
	rc := make([]*selection, len(e.selections))

	for i, s := range e.selections {
		newS := &selection{}
		*newS = *s
		rc[i] = newS
	}
	return rc
}

func (e *editable) selectionsInDisplayOrder() []*selection {
	ordered := make([]*selection, len(e.selections))
	copy(ordered, e.selections)

	sort.Slice(ordered, func(i, j int) bool {
		return ordered[i].Start() < ordered[j].Start()
	})

	return ordered
}

func (e *editable) contractSelectionsOnLeftBy(amt int) {
	for i, s := range e.selections {
		if s.Len() < amt+1 {
			continue
		}

		e.selections[i].start += amt
	}
}

func (e *editable) selectAll() {
	e.clearSelections()
	e.setPrimarySelection(0, e.text.Len())
}
