package runes

func IsABracket(r rune) bool {
	switch r {
	case '{', '[', '(', '<', '>', ')', ']', '}':
		return true
	}
	return false
}

func MatchingBracket(r rune) (opener, closer rune) {
	switch r {
	case '{':
		opener = '{'
		closer = '}'
	case '[':
		opener = '['
		closer = ']'
	case '(':
		opener = '('
		closer = ')'
	case '<':
		opener = '<'
		closer = '>'
	case '}':
		opener = '}'
		closer = '{'
	case ']':
		opener = ']'
		closer = '['
	case ')':
		opener = ')'
		closer = '('
	case '>':
		opener = '>'
		closer = '<'
	}
	return
}
