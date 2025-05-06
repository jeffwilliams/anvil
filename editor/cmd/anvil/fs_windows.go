package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"syscall"
)

// WindowsCmd builds an exec.Cmd that runs 'cmd.exe /C' with the passed arguments.
func WindowsCmd(arg string) *exec.Cmd {
	cmd := exec.Command("cmd")
	args := fmt.Sprintf("/C %s", arg)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow: true,
		CmdLine:    args,
	}
	return cmd
}

func KillProcess(p *os.Process) error {
	kill := exec.Command("TASKKILL", "/T", "/F", "/PID", strconv.Itoa(p.Pid))
	err := kill.Run()
	if err != nil {
		return err
	}
	return p.Kill()
}
