package regex

type reFlags uint32
type reFlag uint32

const (
	iFlag reFlag = 1 << iota
	mFlag
	sFlag
	uFlag
)

func (r *reFlags) set(f reFlag) {
	*r |= reFlags(f)
}

func (r *reFlags) unset(f reFlag) {
	*r &= reFlags(^f)
}

func (r reFlags) test(f reFlag) bool {
	return (r & reFlags(f)) > 0
}

func (r reFlags) diff(f reFlags) reFlags {
	return r ^ f
}

func (f reFlag) Rune() rune {
	switch f {
	case iFlag:
		return 'i'
	case mFlag:
		return 'm'
	case sFlag:
		return 's'
	case uFlag:
		return 'U'
	}
	return '%'
}

// parseTokenFlags parses the flags in a flags token, i.e. (?i).
// It returns two values: modified has a bit set if and only if that flag
// is mentioned in the token, either as a set or unset; values
// has a bit set if and only if it is modified and the token sets
// that flag on. All bits in `values` that are not set in `modified` are 0.
func parseTokenFlags(tok *token) (modified, values reFlags) {
	if tok.typ != flagsTok {
		return
	}

	if len(tok.value) < 4 {
		return
	}

	flagCmd := tok.value[2 : len(tok.value)-1]

	sawHyphen := false

	setOrUnsetFlag := func(f reFlag) {
		modified.set(f)
		if !sawHyphen {
			values.set(f)
		}
	}

	for _, r := range flagCmd {
		switch r {
		case 'i':
			setOrUnsetFlag(iFlag)
		case 'm':
			setOrUnsetFlag(mFlag)
		case 's':
			setOrUnsetFlag(sFlag)
		case 'U':
			setOrUnsetFlag(uFlag)
		case '-':
			sawHyphen = true
		}
	}

	return
}

// changedFlagsWhenTokenAppled computes the new state of the global regex flags after a flags token is processed.
// The parameter `current` is the current global state of the flags. tokenModified and tokenValues are the modified
// and values of the flags that applying a flags token would affect. This function returns which flags were actually
// modified and the new values of the flags after applying the token. "actually modified" means, for example, if a flag
// was already set globally and a token tries to turn it on, there is no actual change to the flag value and so modified
// would not include that change.
func changedFlagsWhenTokenApplied(current, tokenModified, tokenValues reFlags) (modified, values reFlags) {

	// Examples:
	//
	// current: 0101
	// tokenModified: 0001
	// tokenValues: 0000
	// modified: 0001
	// values: 0100

	// current: 0101
	// tokenModified: 0001
	// tokenValues: 0001
	// modified: 0000
	// values: 0101

	modified = (current & tokenModified) ^ tokenValues
	//values = (take tokenValues and for the non-token-modified bits, or it with current)
	values = (tokenValues & tokenModified) | (current & ^tokenModified)
	return
}

// invertModifiedFlags changes the flags in values that are modified (have their bits
// set to 1 in `modified`) so that they are inverted.
func invertModifiedFlags(modified, values reFlags) reFlags {
	return ^values & modified
}

func flagsString(flags reFlags, changed reFlags) []rune {
	var set []rune
	var unset []rune
	for f := iFlag; f <= uFlag; f = f << 1 {
		if !changed.test(f) {
			continue
		}

		if flags.test(f) {
			set = append(set, f.Rune())
		} else {
			unset = append(unset, f.Rune())
		}
	}

	res := make([]rune, 0, len(set)+len(unset)+1)

	for _, r := range set {
		res = append(res, r)
	}
	if len(unset) > 0 {
		res = append(res, '-')
	}
	for _, r := range unset {
		res = append(res, r)
	}
	return res
}
