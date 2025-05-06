package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type Hook struct {
	Match *regexp.Regexp
	Do    []string
}

func parseConfigFile() (hooks []Hook, err error) {
	d := os.Getenv("ANVIL_CFG_DIR")
	if d == "" {
		err = fmt.Errorf("Can't find the Anvil config dir; ANVIL_CFG_DIR is not set")
		return
	}

	name := filepath.Join(d, "filehooks")
	var f *os.File
	f, err = os.Open(name)
	if err != nil {
		err = fmt.Errorf("Can't open config file %s: %v", name, err)
		return
	}
	defer f.Close()

	return parseConfigFileFrom(f)
}

func parseConfigFileFrom(f io.Reader) (hooks []Hook, err error) {

	var hook Hook
	onMatch := func(re *regexp.Regexp) {
		if hook.Match != nil {
			hooks = append(hooks, hook)
			hook.Do = nil
		}
		hook.Match = re
	}

	onDo := func(do string) {
		hook.Do = append(hook.Do, do)
	}

	err = ParseMatchDoConfigFile(f, onMatch, onDo)

	if err == nil && len(hook.Do) > 0 {
		hooks = append(hooks, hook)
	}

	return
}

func ParseMatchDoConfigFile(f io.Reader, onMatch func(re *regexp.Regexp), onDo func(do string)) (err error) {

	s := bufio.NewScanner(f)

	const (
		stateNormal = iota
		stateSawMatch
		stateSawDo
	)

	state := stateNormal

	handleMatch := func(line, data string) {
		re, err2 := regexp.Compile(data)
		if err2 != nil {
			err = fmt.Errorf("Parsing regexp for line '%s' failed: %v", line, err)
			return
		}
		onMatch(re)
		state = stateSawMatch
	}

	handleDo := func(data string) {
		onDo(data)
		//rules = append(rules, rule)
		state = stateSawDo
	}

	for s.Scan() {
		line := s.Text()
		line = strings.TrimSpace(line)
		if line == "" || line[0] == '#' {
			continue
		}

		toks := strings.SplitN(line, " ", 2)
		if len(toks) < 2 {
			err = fmt.Errorf("Invalid line: expected a word, a space, then a string. Line is '%s'", line)
			return
		}

		switch state {
		case stateSawDo:
			if toks[0] == "do" {
				handleDo(toks[1])
				continue
			}
			if toks[0] != "match" {
				err = fmt.Errorf("Expected line beginning with 'match' but got '%s'", line)
				return
			}

			handleMatch(line, toks[1])
		case stateNormal:
			if toks[0] != "match" {
				err = fmt.Errorf("Expected line beginning with 'match' but got '%s'", line)
				return
			}

			handleMatch(line, toks[1])
		case stateSawMatch:
			if toks[0] != "do" {
				err = fmt.Errorf("Expected line beginning with 'do' but got '%s'", line)
				return
			}
			handleDo(toks[1])
		}
	}
	return
}
