package runes

import (
	"fmt"
	"unicode/utf8"
)

// OffsetCache caches the byte offsets associated with rune offsets in a document.
// The cache is valid as long as the document is only appended to. If the contents change
// then the cache must be cleared.
type OffsetCache struct {
	intvl      int
	vals       []offsetCacheEntry
	lastDocLen int
}

type offsetCacheEntry struct {
	runeOffset int
	byteOffset int
}

func NewOffsetCache(cacheInterval int) OffsetCache {
	if cacheInterval == 0 {
		cacheInterval = 1048576
	}
	return OffsetCache{
		intvl: cacheInterval,
	}
}

func (c *OffsetCache) Clear() {
	c.vals = c.vals[:0]
}

func (c *OffsetCache) ClearAfter(runeOffset int) {
	if runeOffset < 0 {
		c.Clear()
		return
	}

	ndx := runeOffset / c.intvl
	mod := runeOffset % c.intvl

	if mod != 0 {
		ndx++
	}

	if ndx >= len(c.vals) {
		return
	}

	c.vals = c.vals[:ndx]
}

type invalidUTF8Offset struct {
	byteOffset int
	runeOffset int
	byt        byte
}

func (e invalidUTF8Offset) Error() string {
	return fmt.Sprintf("invalid UTF-8 sequence at byte offset %d, rune offset %d. First byte of invalid sequence is %d\n",
		e.byteOffset, e.runeOffset, e.byt)
}

// Update updates the cache so that it contains entries every cacheInterval number of runes.
func (c *OffsetCache) Update(doc []byte) (warn error) {
	// Go to the last entry in vals. Then move forward each interval and
	// add a new entry to the cache

	c.addInitialZeroEntryIfNeeded()

	byteOff := 0
	runeOff := 0
	if c.vals != nil && len(c.vals) > 0 {
		byteOff = c.vals[len(c.vals)-1].byteOffset
		runeOff = c.vals[len(c.vals)-1].runeOffset
	}

OUT:
	for {
		for i := 0; i < c.intvl; i++ {
			if byteOff >= len(doc) {
				break OUT
			}

			r, size := utf8.DecodeRune(doc[byteOff:])
			if r == utf8.RuneError && warn == nil {
				warn = &invalidUTF8Offset{
					byteOffset: byteOff,
					runeOffset: runeOff,
					byt:        doc[byteOff],
				}
			}

			byteOff += size
		}
		runeOff += c.intvl

		c.add(runeOff, byteOff)
	}
	return
}

func (c *OffsetCache) shouldUpdateBecauseDocChanged(doc []byte) bool {
	b := c.lastDocLen != len(doc)
	if b {
		c.lastDocLen = len(doc)
	}
	return b
}

func (c *OffsetCache) addInitialZeroEntryIfNeeded() {
	if c.vals == nil || len(c.vals) == 0 {
		c.add(0, 0)
	}
}

func (c *OffsetCache) add(runeOffset int, byteOffset int) {
	c.vals = append(c.vals, offsetCacheEntry{
		runeOffset: runeOffset,
		byteOffset: byteOffset,
	})
}

// If an invalid UTF-8 sequence is encountered, error will be set, but this function will continue
// processing the text regardless so the result should still be usable.
func (c *OffsetCache) Get(doc []byte, runeOffset int) (byteOffset int, err error, runeCount int) {
	ndx := runeOffset / c.intvl

	if ndx >= len(c.vals) {
		err = c.Update(doc)
	}

	if ndx >= len(c.vals) {
		ndx = len(c.vals) - 1
	}

	// ndx on success is the smallest index of the entry who's runeOffset is greater than what we want.
	// So we need to take the previous one.
	curRuneOffset := c.vals[ndx].runeOffset
	byteOffset = c.vals[ndx].byteOffset

	for curRuneOffset < runeOffset {
		if byteOffset >= len(doc) {
			break
		}

		r, size := utf8.DecodeRune(doc[byteOffset:])
		if r == utf8.RuneError && err == nil {
			err = &invalidUTF8Offset{
				byteOffset: byteOffset,
				runeOffset: curRuneOffset,
				byt:        doc[byteOffset],
			}
		}

		byteOffset += size
		curRuneOffset++
	}

	runeCount = curRuneOffset
	return
}
