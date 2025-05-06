//go:build !windows

package main

import (
	"io"
	"os/exec"
	"syscall"

	"github.com/creack/pty"
)

func startCmdUsingPty(argv []string) (stdin io.Writer, stdout io.Reader, terminated func() bool, err error) {
	//fmt.Printf("Running command %s %s\n", os.Args[1], strings.Join(args, " "))

	c := exec.Command(argv[0], argv[1:]...)

	tty, err := pty.Start(c)
	setNoEcho(tty)

	if err != nil {
		return
	}

	stdin = tty
	stdout = tty

	ch := make(chan struct{})
	go func() {
		c.Process.Wait()
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
	return syscall.Read(int(fd), p)
}
