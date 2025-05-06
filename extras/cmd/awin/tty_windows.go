package main

import (
	"context"
	"io"
	"strings"
	"syscall"

	"github.com/UserExistsError/conpty"
)

func startCmdUsingPty(argv []string) (stdin io.Writer, stdout io.Reader, terminated func() bool, err error) {
	c := strings.Join(argv, " ")
	debug("awin: running command '%s'\n", c)

	var tty *conpty.ConPty
	tty, err = conpty.Start(c)
	if err != nil {
		return
	}

	stdin = tty
	stdout = tty

	ch := make(chan struct{})
	go func() {
		//time.Sleep(1000 * time.Millisecond)
		code, err := tty.Wait(context.Background())
		if err != nil {
			debug("awin: Wait returned with error: %v\n", err)
		}
		debug("awin: Wait returned with exit code: %d\n", code)
		close(ch)
	}()

	terminated = func() bool {
		select {
		case <-ch:
			return true
		default:
		}
		return false
	}

	return
}

func readFromFd(fd uintptr, p []byte) (n int, err error) {
	return syscall.Read(syscall.Handle(fd), p)
}
