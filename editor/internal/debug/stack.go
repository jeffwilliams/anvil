package debug

import (
	"fmt"
	"runtime"
	"strings"
)

// Stack returns a stacktrace of the current goroutine
func Stack() string {
	return stack()
}

// PrefixedStack returns a stacktrace of the current goroutine with `prefix` at the beginning of each line
func PrefixedStack(pfx string) string {
	st := stack()
	if pfx == "" {
		return st
	}

	parts := strings.Split(st, "\n")
	st = strings.Join(parts, fmt.Sprintf("\n%s", pfx))
	st = pfx + st

	return st
}

func stack() string {
	buf := make([]byte, 3000)
	sz := runtime.Stack(buf, true)
	buf = buf[0:sz]
	return string(buf)

}
