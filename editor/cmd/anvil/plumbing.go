package main

import (
	"io"
	"regexp"
)

/*
Plumbing file format:

		match <regex>
		do <command>

		match <regex>
		do <command>


<regex> can contain submatches which can be referenced by $0 (the entire match), $1 (first group), $2, etc.
<command> is an anvil or shell command.

*/

type Plumber struct {
	rules []PlumbingRule
}

func NewPlumber(rules []PlumbingRule) *Plumber {
	return &Plumber{
		rules,
	}
}

func (p Plumber) Plumb(obj string, executor *CommandExecutor, ctx *CmdContext) (ok bool, err error) {
	for _, rule := range p.rules {
		if rule.Try(obj, executor, ctx) {
			ok = true
			return
		}
	}
	return false, nil
}

type PlumbingRule struct {
	Match *regexp.Regexp
	Do    string
}

func (rule PlumbingRule) Try(obj string, executor *CommandExecutor, ctx *CmdContext) (matched bool) {
	submatches := rule.Match.FindStringSubmatchIndex(obj)
	if submatches == nil {
		return
	}

	matched = true
	cmd := []byte{}
	cmd = rule.Match.Expand(cmd, []byte(rule.Do), []byte(obj), submatches)

	log(LogCatgPlumb, "Plumber: executing '%s'\n", cmd)

	executor.Do(string(cmd), ctx)
	return
}

func ParsePlumbingRules(f io.Reader) (rules []PlumbingRule, err error) {
	var rule PlumbingRule

	onMatch := func(re *regexp.Regexp) {
		rule.Match = re
	}

	onDo := func(do string) {
		rule.Do = do
		rules = append(rules, rule)
	}

	err = parseDoMatchConfigFile(f, onMatch, onDo)
	return
}
