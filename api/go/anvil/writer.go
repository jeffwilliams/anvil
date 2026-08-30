package anvil

import (
	"bytes"
	"fmt"
)

type bodyWriter struct {
	winId int
	a     Anvil
	buf   bytes.Buffer
}

func newBodyWriter(a Anvil, winId int) bodyWriter {
	return bodyWriter{
		a:     a,
		winId: winId,
	}
}

func (w bodyWriter) Write(p []byte) (n int, err error) {
	w.buf.Reset()
	w.buf.Write(p)
	_, err = w.a.Post(fmt.Sprintf("/wins/%d/body", w.winId), &w.buf)
	if err == nil {
		n = len(p)
	}
	return
}
