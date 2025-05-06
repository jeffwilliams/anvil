package main

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func setNoEcho(tty *os.File) {

	fd := int(tty.Fd())

	termios, err := unix.IoctlGetTermios(fd, unix.TCGETS)
	if err != nil {
		fmt.Printf("awin: Getting terminal state failed: %s\n", err)
		return
	}

	newState := *termios
	newState.Lflag &^= unix.ECHO
	newState.Lflag |= unix.ICANON | unix.ISIG
	newState.Iflag |= unix.ICRNL
	if err := unix.IoctlSetTermios(fd, unix.TCSETS, &newState); err != nil {
		fmt.Printf("awin: Setting terminal state failed: %s\n", err)
		return
	}
}
