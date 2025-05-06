package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"sort"
	"unicode/utf8"
)

type TagsIndex struct {
	// offset is the offset in the sorted tags file of the section at which the tags starting with the specified letter (rune) begin.
	offset map[rune]int64
}

func BuildTagsIndex(r io.Reader) TagsIndex {

	ti := TagsIndex{offset: make(map[rune]int64)}

	offset := int64(0)
	s := bufio.NewScanner(r)
	var lastRune rune
	for s.Scan() {
		l := s.Text()
		r, _ := utf8.DecodeRuneInString(l)
		if len(l) > 0 && lastRune != r {
			lastRune = r
			ti.offset[lastRune] = offset
		}
		offset += int64(len(l)) + 1 // This only works for Unix format files.
	}

	return ti
}

func (ti *TagsIndex) String() string {
	var buf bytes.Buffer
	keys := make([]rune, len(ti.offset))

	var i int
	for k := range ti.offset {
		keys[i] = k
		i++
	}

	sort.Slice(keys, func(a, b int) bool {
		return keys[a] < keys[b]
	})

	for _, k := range keys {
		fmt.Fprintf(&buf, "%c: %d\n", k, ti.offset[k])
	}

	return buf.String()
}
