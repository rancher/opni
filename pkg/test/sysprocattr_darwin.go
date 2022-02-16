//go:build darwin

package test

import (
	"os"
	"os/exec"
	"os/signal"
	"syscall"
)

func ConfigureSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
}

func init() {
	c := make(chan os.Signal, 4)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM, syscall.SIGKILL)
	pid := os.Getpid()
	pgid, err := syscall.Getpgid(pid)
	if err != nil {
		panic(err)
	}
	go func() {
		<-c
		syscall.Kill(-pgid, syscall.SIGKILL)
		os.Exit(1)
	}()
}
