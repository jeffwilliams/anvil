package escape

import (
	"bytes"
	"fmt"
	"strings"
)

// ExpandEscapes interprets backslash-escapes in a string, replacing them with the actual character
// they represent. It supports the following:
//   - \n
//   - \t
//   - \r
//   - \\
//   - \'
//   - \"
func ExpandEscapes(s string) string {
	var buf bytes.Buffer

	const (
		normal = iota
		escape
	)
	state := normal
	for _, rn := range s {
		switch state {
		case normal:
			if rn == '\\' {
				state = escape
				continue
			}
			buf.WriteRune(rn)
		case escape:
			switch rn {
			case 'n':
				buf.WriteRune('\n')
			case 'r':
				buf.WriteRune('\r')
			case 't':
				buf.WriteRune('\t')
			case '\\':
				buf.WriteRune('\\')
			case '\'':
				buf.WriteRune('\'')
			case '"':
				buf.WriteRune('"')
			default:
				buf.WriteRune('\\')
				buf.WriteRune(rn)
			}
			state = normal
		}
	}

	return buf.String()
}

// Try and expand a string of the form "..." or '...' that might contain
// escapes for \" and \'.
func ExpandEscapesAndUnquote(s string) (string, error) {
	s = strings.TrimSpace(s)

	if len(s) == 0 {
		return "", nil
	}

	quote := s[0]
	if quote != '\'' && quote != '"' {
		return "", fmt.Errorf("invalid quote character: %c", quote)
	}

	if len(s) == 1 {
		return "", fmt.Errorf("missing end quote")
	}

	if s[len(s)-1] != quote {
		return "", fmt.Errorf("missing end quote")
	}

	return ExpandEscapes(s[1 : len(s)-1]), nil
}
