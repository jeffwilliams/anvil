package main

import (
	"fmt"
	"os/exec"
	"syscall"
)

func newCmd(cmd string) *exec.Cmd {
	c := exec.Command("cmd")
	args := fmt.Sprintf("/C %s", cmd)
	c.SysProcAttr = &syscall.SysProcAttr{
		HideWindow: true,
		CmdLine:    args,
	}
	return c
}
