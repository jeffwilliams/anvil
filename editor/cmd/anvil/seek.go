package main

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/jeffwilliams/anvil/editor/internal/expr"
)

type seek struct {
	seekType  seekType
	line, col int
	runePos   int
	regex     *regexp.Regexp
}

type seekType int

const (
	seekToLineAndCol seekType = iota
	seekToRunePos
	seekToRegex
)

func parseSeekFromFilename(path string) (seeklessPath string, seek seek, err error) {
	/*
	   file
	   file#rune
	   file!regex

	   file:line
	   host:file
	   host:file#rune
	   host:file!regex

	   file:line:col
	   host:file:line
	   host:port:/file
	   host:port:file#rune
	   host:port:file!regex

	   host:file:line:col
	   host:port:file:line

	   host:port:file:line:col
	*/

	parts := strings.SplitN(path, ":", 5)

	parseRuneIndexOrRegex := func(path string) {
		seeklessPath = path
		i := strings.IndexAny(path, "#!")
		if i >= 1 && len(path) > i+1 {
			seeklessPath = path[:i]
			if path[i] == '#' {
				seek.runePos, _ = strconv.Atoi(path[i+1:])
				seek.seekType = seekToRunePos
			} else if path[i] == '!' {
				regex := path[i+1:]
				seek.regex, err = expr.CompileRegexpWithMultiline(regex)
				if err != nil {
					err = fmt.Errorf("Parsing regular expression in seek at end of filename failed: %v", err)
					return
				}
				seek.seekType = seekToRegex
			}
		}
	}

	onePart := func(parts []string) {
		parseRuneIndexOrRegex(parts[0])
	}

	twoParts := func(parts []string) {
		line, err := strconv.Atoi(parts[1])
		if err == nil {
			// file:line
			seeklessPath = parts[0]
			seek.line = line
			return
		}

		parseRuneIndexOrRegex(path)
	}

	threeParts := func(parts []string) {
		n1, err := strconv.Atoi(parts[1])
		if err != nil {
			// host:file:line
			seeklessPath = fmt.Sprintf("%s:%s", parts[0], parts[1])
			seek.line, err = strconv.Atoi(parts[2])
			if err != nil {
				// Assume file after the host contains a : in it.
				seeklessPath = fmt.Sprintf("%s:%s:%s", parts[0], parts[1], parts[2])
			}
			return
		}

		n2, err := strconv.Atoi(parts[2])
		if err == nil {
			// file:line:col
			seeklessPath = parts[0]
			seek.line = n1
			seek.col = n2
			return
		}

		// One of:
		// host:port:file
		// host:port:file#rune
		// host:port:file!regex
		parseRuneIndexOrRegex(path)
	}

	fourParts := func(parts []string) {
		n3, err := strconv.Atoi(parts[3])
		if err != nil {
			err = fmt.Errorf("Parsing number at end of filename failed: %v", err)
			return
		}

		n2, err := strconv.Atoi(parts[2])
		if err == nil {
			// host:file:line:col
			seeklessPath = fmt.Sprintf("%s:%s", parts[0], parts[1])
			seek.line = n2
			seek.col = n3
			return
		}

		//host:port:file:line
		seeklessPath = fmt.Sprintf("%s:%s:%s", parts[0], parts[1], parts[2])
		seek.line = n3
	}

	fiveParts := func(parts []string) {
		// host:port:file:line:col
		seeklessPath = fmt.Sprintf("%s:%s:%s", parts[0], parts[1], parts[2])

		n3, err := strconv.Atoi(parts[3])
		if err != nil {
			err = fmt.Errorf("Parsing line number failed: %v", err)
			return
		}

		n4, err := strconv.Atoi(parts[4])
		if err != nil {
			err = fmt.Errorf("Parsing column number failed: %v", err)
			return
		}

		seek.line = n3
		seek.col = n4
	}

	removeEmpty := func(parts []string) []string {
		j := 0
		for i := 0; i < len(parts); i++ {
			if parts[i] == "" {
				continue
			}
			parts[j] = parts[i]
			j++
		}
		return parts[:j]
	}

	parts = removeEmpty(parts)

	switch len(parts) {
	case 1:
		onePart(parts)
	case 2:
		twoParts(parts)
	case 3:
		threeParts(parts)
	case 4:
		fourParts(parts)
	case 5:
		fiveParts(parts)
	}

	return
}

func (s seek) empty() bool {
	return s.line == 0 && s.col == 0 && s.seekType == 0
}
