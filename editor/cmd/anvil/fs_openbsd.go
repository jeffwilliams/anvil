package main

import (
	"os"
	"os/exec"
)

func WindowsCmd(arg string) *exec.Cmd {
	return nil
}

func KillProcess(p *os.Process) error {
	return p.Kill()
}
