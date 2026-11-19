package main

import (
	"os"
	"os/exec"
)

const platformID = "linux-x64"
const platformArch = "amd64"
const engineBinaryName = "llama-server"
const supportsMetal = false

func checkOSVersion() error { return nil }
func configureEngine(cmd *exec.Cmd) {
	path := cmd.Dir
	if extra := os.Getenv("LD_LIBRARY_PATH"); extra != "" {
		path += ":" + extra
	}
	cmd.Env = append(cmd.Environ(), "LD_LIBRARY_PATH="+path)
}
func browserCommand(url string) *exec.Cmd { return exec.Command("xdg-open", url) }
