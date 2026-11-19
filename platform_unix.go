//go:build linux || darwin

package main

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"
)

var stopSignals = []os.Signal{os.Interrupt, syscall.SIGTERM, syscall.SIGHUP}

func lockFile(path string) (func(), error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		if err == syscall.EWOULDBLOCK {
			return nil, fmt.Errorf("the kit is already running: use the open window or stop it before restarting")
		}
		return nil, err
	}
	return func() { f.Close() }, nil
}

func launchEngine(cmd *exec.Cmd) (func(<-chan struct{}), error) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return func(finished <-chan struct{}) {
		select {
		case <-finished:
			return
		default:
		}
		syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
		select {
		case <-finished:
		case <-time.After(3 * time.Second):
			syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			<-finished
		}
	}, nil
}
