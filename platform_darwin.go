package main

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

const platformID = "macos-arm64"
const platformArch = "arm64"
const engineBinaryName = "llama-server"
const supportsMetal = true

func prepareEngine(ctx context.Context, root string, offline bool) error {
	return checkEngine(ctx, root)
}

func checkOSVersion() error {
	version, err := syscall.Sysctl("kern.osproductversion")
	if err != nil {
		return fmt.Errorf("could not read the macOS version (13.3 or later is required): %w", err)
	}
	majorText, minorAndPatch, _ := strings.Cut(strings.TrimSpace(version), ".")
	minorText, _, _ := strings.Cut(minorAndPatch, ".")
	major, err := strconv.Atoi(majorText)
	if err != nil || major < 0 {
		return fmt.Errorf("unrecognized macOS version %q", version)
	}
	minor := 0
	if minorText != "" {
		minor, err = strconv.Atoi(minorText)
		if err != nil || minor < 0 {
			return fmt.Errorf("unrecognized macOS version %q", version)
		}
	}
	if major < 13 || (major == 13 && minor < 3) {
		return fmt.Errorf("the bundled engine requires macOS 13.3 or later (detected %s)", version)
	}
	return nil
}

func configureEngine(cmd *exec.Cmd)       {}
func browserCommand(url string) *exec.Cmd { return exec.Command("/usr/bin/open", url) }
