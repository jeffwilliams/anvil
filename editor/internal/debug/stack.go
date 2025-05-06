package debug

import "runtime"

// Stack returns a stacktrace of the current goroutine
func Stack() string {
	buf := make([]byte, 3000)
	sz := runtime.Stack(buf, true)
	buf = buf[0:sz]
	return string(buf)
}
