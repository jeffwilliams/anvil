//go:build !windows

package main

import "os/exec"

func newCmd(cmd string) *exec.Cmd {
	c := exec.Command("bash", "-c", cmd)
	return c
}
