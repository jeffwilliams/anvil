package main

import (
	"bytes"
	"fmt"
)

type Packer struct {
	all          []Packable
	headerHeight float32
	maxSpace     float32
}

type pos struct {
	coord    float32
	oldCoord float32
	p        Packable
}

func NewPacker(headerHeight, maxSpace float32, all []Packable) Packer {
	return Packer{
		all:          all,
		headerHeight: headerHeight,
		maxSpace:     maxSpace,
	}

}

// MoveTo moves or adds `changed` to the right position in the ordered list `all`, first changing the coordinate
// of `changed` to `newCoord`. If the headers of items in `all` (which have height `headerHeight`) would now
// overlap `changed` in it's new position, they are moved if possible, however their coordinates after moving
// must fall within the range of [0, `maxSpace`] or the packing fails.
func (p Packer) MoveTo(changed Packable, newCoord float32) []Packable {
	if newCoord < 0 {
		newCoord = 0
	}
	if newCoord > p.maxSpace {
		newCoord = p.maxSpace
	}

	above, below := p.itemsAboveAndBelow(changed, newCoord)

	// Bubble down (if needed and possible)
	thresh := newCoord + p.headerHeight
	thresh = p.bubbleDown(thresh, below)

	if thresh >= p.maxSpace {
		log(LogCatgPack, "window too low, moving up\n")
		// TODO: the window can't fit here without being moved up slighylu. Bubble it up along with the others.
		if len(below) == 0 {
			// packing failed
			return p.all
		}
		p.undo(below)
		newCoord = below[0].p.PackingCoord() - p.headerHeight
	} else {
		thresh = newCoord
	}

	// bubbling up
	thresh = newCoord

	thresh = p.bubbleUp(thresh, above)

	if thresh >= 0 {
		// success!
		chpos := pos{newCoord, newCoord, changed}
		p.concat(chpos, above, below)
		p.all[0].SetPackingCoord(0)

		// hres
		return p.all
	}

	log(LogCatgPack, "packing failed\n")
	return p.all
}

func (p Packer) itemsAboveAndBelow(changed Packable, newCoord float32) (above, below []pos) {
	above = make([]pos, 0, len(p.all))
	below = make([]pos, 0, len(p.all))

	//for _, win := range all {
	for _, n := range p.all {
		if changed == n {
			continue
		}

		if n.PackingCoord() < newCoord {
			above = append(above, pos{n.PackingCoord(), n.PackingCoord(), n})
		} else {
			below = append(below, pos{n.PackingCoord(), n.PackingCoord(), n})
		}
	}
	return
}

// bubbleDown adjusts the items in `below` such that they are all lower than thresh
// and don't overlap. Items are pushed lower and if that causes an overlap, the next item is
// moved lower and so on. The coordinate of the highest item after bubbling up is returned; if
// this is more than the available space, there is no way to repack all the items to fit in
// the available space.
func (p Packer) bubbleDown(thresh float32, below []pos) (newThresh float32) {
	for i, pos := range below {
		if pos.coord < thresh {
			below[i].coord = thresh
			thresh += p.headerHeight
		} else {
			break
		}
	}
	return thresh
}

// bubbleDown adjusts the items in `above` such that they are all higher than thresh
// and don't overlap. Items are pushed higher and if that causes an overlap, the next item is
// moved higher and so on. The coordinate of the highest item after bubbling up is returned; if
// this is less than zero, there is no way to repack all the items to fit in the available space.
func (p Packer) bubbleUp(thresh float32, above []pos) (newThresh float32) {

	for i := len(above) - 1; i >= 0; i-- {
		pos := above[i]
		if pos.coord+p.headerHeight > thresh {
			above[i].coord = thresh - p.headerHeight
			thresh -= p.headerHeight
		} else {
			break
		}
	}
	return thresh
}

// concat replaces the set of all packables with the concatenation
// of the lists above and below (in that order), and also sets the packing
// coordinates of the items to the values from the lists.
func (p *Packer) concat(changed pos, above, below []pos) {
	// Bubbling succeeded!
	if len(p.all) < len(above)+len(below)+1 {
		p.all = append(p.all, nil)
	}
	log(LogCatgPack, "Set the packing coord to %f\n", changed.coord)

	log(LogCatgPack, "pack: above:\n")
	i := 0
	for _, pos := range above {
		p.all[i] = pos.p
		if pos.p == nil {
			log(LogCatgPack, "pos window %d is nil\n", i)
		}
		p.all[i].SetPackingCoord(pos.coord)
		log(LogCatgPack, "  %p coord: %f\n", p.all[i], p.all[i].PackingCoord())
		i++
	}
	p.all[i] = changed.p
	p.all[i].SetPackingCoord(changed.coord)
	i++
	log(LogCatgPack, "pack: below:\n")
	for _, pos := range below {
		p.all[i] = pos.p
		if pos.p == nil {
			log(LogCatgPack, "pos window %d is nil\n", i)
		}
		p.all[i].SetPackingCoord(pos.coord)
		log(LogCatgPack, "  %p coord: %f\n", p.all[i], p.all[i].PackingCoord())
		i++
	}

}

func (p *Packer) undo(s []pos) {
	for i, pos := range s {
		s[i].coord = pos.oldCoord
	}
}

type Packable interface {
	PackingCoord() float32
	SetPackingCoord(x float32)
}

// Pack finds an appropriate place to put the passed Packables. This is different from MoveTo
// in that the items passed here don't have a pre-determined place to be put.
func (p *Packer) Pack(items []Packable) []Packable {
	log(LogCatgPack, "Pack: when called: %s", p)
	//r.printWindowPositions()
	for _, w := range items {
		tallest, height := p.tallestItem()
		if tallest == nil {
			log(LogCatgPack, "When positioning: no tallest window\n")
			p.all = append(p.all, w)
			continue
		}
		log(LogCatgPack, "Tallest window: %p y=%f height=%v\n", tallest, tallest.PackingCoord(), height)

		// The text rendering looks better if we make sure to render on integer pixels
		w.SetPackingCoord(round(tallest.PackingCoord() + height/2))
		p.insertItem(w)
		log(LogCatgPack, "positionWindows: after inserting %p windows are: %s\n", w, p)

		//r.printWindowPositions()
	}
	log(LogCatgPack, "Pack: when done: %s", p)
	return p.all
}

func (p Packer) tallestItem() (item Packable, height float32) {
	if len(p.all) == 0 {
		return
	}

	item = p.all[0]
	height = p.ItemSize(0)
	for i := 1; i < len(p.all); i++ {
		if h := p.ItemSize(i); h > height {
			item = p.all[i]
			height = h
		}
	}
	return
}

func (p Packer) ItemSize(index int) float32 {
	if index < 0 || index >= len(p.all) {
		return 0
	}
	y := p.all[index].PackingCoord()
	if p.hasItemBelow(index) {
		//log(LogCatgPack,"windowHeight: Window %p has window below. Height is %f\n", r.Windows[winIndex], r.windowBelow(winIndex).TopY-y)
		return p.itemBelow(index).PackingCoord() - y
	}
	//log(LogCatgPack,"windowHeight: Window %p has no window below. Height is %f\n", r.Windows[winIndex], r.vspace-y)
	return p.maxSpace - y
}

func (p *Packer) insertItem(w Packable) {
	p.all = append(p.all, w)
	bubble := Packable(nil)
	for i, win := range p.all {
		if bubble != nil {
			bubble, p.all[i] = p.all[i], bubble
		} else if win.PackingCoord() > w.PackingCoord() {
			// Insert here, and bubble down later ones
			bubble = p.all[i]
			p.all[i] = w
		}
	}
}

func (p Packer) hasItemBelow(index int) bool {
	return index < len(p.all)-1
}

func (p Packer) itemBelow(index int) Packable {
	return p.all[index+1]
}

func (p Packer) String() string {
	var buf bytes.Buffer

	fmt.Fprintf(&buf, "Packer items:\n")
	for i, x := range p.all {
		fmt.Fprintf(&buf, "  %d) %p coord: %f\n", i, x, x.PackingCoord())
	}
	fmt.Fprintf(&buf, "header height: %f max space: %f\n", p.headerHeight, p.maxSpace)

	return buf.String()
}

// Grow increases the size of the specified packable. It moves items below and
// above if necessary.
func (p Packer) Grow(change Packable, extra float32) []Packable {

	curCoord := change.PackingCoord()

	above, below := p.itemsAboveAndBelow(change, curCoord)

	min := func(a, b float32) float32 {
		if a > b {
			return b
		} else {
			return a
		}
	}

	// Move the items below
	if len(below) > 0 {
		b := below[0]

		max := p.maxSpace - b.p.PackingCoord()
		amt := p.availableSpaceIn(below, max)
		log(LogCatgPack, "Available space below: %f\n", amt)
		amt = min(amt, extra/2)
		thresh := b.p.PackingCoord() + amt
		thresh = p.bubbleDown(thresh, below)
		log(LogCatgPack, "new thresh after bubble down: %f. Max = %f\n", thresh, p.maxSpace)
		if thresh >= p.maxSpace {
			p.undo(below)
		}
		extra = extra / 2
	}

	// Move the items above
	max := curCoord
	amt := p.availableSpaceIn(above, max)
	amt = min(amt, extra)

	log(LogCatgPack, "Available space above: %f\n", amt)

	newCoord := curCoord - amt
	thresh := p.bubbleUp(newCoord, above)
	log(LogCatgPack, "new thresh after bubble up: %f\n", thresh)
	if thresh < 0 {
		p.undo(above)
		newCoord = curCoord
	}
	chpos := pos{newCoord, newCoord, change}
	p.concat(chpos, above, below)
	p.all[0].SetPackingCoord(0)

	log(LogCatgPack, "packing failed\n")
	return p.all
}

func (pk Packer) itemIndex(p Packable) int {
	for i, n := range pk.all {
		if p == n {
			return i
		}
	}
	return -1
}

func (p Packer) availableSpaceIn(items []pos, max float32) float32 {
	return max - float32(len(items))*p.headerHeight
}

// Move item to the top, and make all others only their header height tall.
func (p *Packer) MinimizeAllExcept(item Packable) []Packable {
	above, below := p.itemsAboveAndBelow(item, item.PackingCoord())

	coord := float32(0)
	for _, w := range above {
		w.p.SetPackingCoord(coord)
		coord += p.headerHeight
	}

	item.SetPackingCoord(coord)

	coord = p.maxSpace - p.headerHeight*float32(len(below))
	for _, w := range below {
		w.p.SetPackingCoord(coord)
		coord += p.headerHeight
	}

	return p.all
}

func (p *Packer) Maximize(item Packable) []Packable {

	p.moveToFirst(p.all, item)

	for _, w := range p.all {
		if w == item {
			w.SetPackingCoord(0)
		} else {
			w.SetPackingCoord(p.maxSpace)
		}
	}

	return p.all
}

func (p *Packer) moveToFirst(items []Packable, item Packable) {
	for i, w := range items {
		if w == item {
			items[0], items[i] = items[i], items[0]
			break
		}
	}
}

func (p *Packer) SpaceEvenly() []Packable {
	l := float32(len(p.all))
	sep := (p.maxSpace - p.headerHeight*l) / l

	for i, w := range p.all {
		coord := float32(i) * (sep + p.headerHeight)
		w.SetPackingCoord(round(coord))
	}

	return p.all
}

// RepackItemsBelowLimit adjusts the packables so that any that are
// not visible because their coordinate is below the max space for packing
// are moved up so they are visible. Other items that would then overlap
// are moved up as well, iteratively.
func (p *Packer) RepackItemsBelowLimit() []Packable {

	pos := p.positions()
	thresh := p.bubbleUp(p.maxSpace, pos)

	if thresh < 0 {
		// packing failed
		log(LogCatgPack, "Packer.RepackItemsBelowLimit: packing failed. Not enough space to fit all windows.\n")
		return p.all
	}

	for i := 0; i < len(p.all); i++ {
		p.all[i].SetPackingCoord(pos[i].coord)
	}

	return p.all
}

func (p *Packer) positions() []pos {
	pn := make([]pos, len(p.all))

	for i, n := range p.all {
		pn[i] = pos{n.PackingCoord(), n.PackingCoord(), n}
	}

	return pn
}
