package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

// Probe the shipped engine first. Only a dynamic-loader error for these two
// Ubuntu runtime dependencies warrants an installation attempt.
func prepareEngine(ctx context.Context, root string, offline bool) error {
	probeErr := checkEngine(ctx, root)
	if probeErr == nil {
		return nil
	}
	packages := map[string]string{"libgomp.so.1": "libgomp1", "libstdc++.so.6": "libstdc++6"}
	missingDependency := false
	for library := range packages {
		if strings.Contains(probeErr.Error(), "error while loading shared libraries: "+library+": cannot open shared object file: No such file or directory") {
			missingDependency = true
		}
	}
	if !missingDependency {
		return probeErr
	}

	// The loader reports only its first missing library. Ask ldd for all
	// missing dependencies, with the exact same environment as the engine.
	engine := enginePath(root)
	list := exec.CommandContext(ctx, "/usr/bin/ldd", engine)
	list.Dir = filepath.Dir(engine)
	configureEngine(list)
	list.Env = append(list.Env, "LC_ALL=C")
	output, err := list.Output()
	if err != nil {
		return probeErr
	}
	var missing []string
	for _, line := range strings.Split(string(output), "\n") {
		name, absent := strings.CutSuffix(strings.TrimSpace(line), " => not found")
		if !absent {
			continue
		}
		pkg, known := packages[name]
		if !known {
			return fmt.Errorf("%w. Also missing %s. Extract the complete kit and check the system libraries", probeErr, name)
		}
		if !slices.Contains(missing, pkg) {
			missing = append(missing, pkg)
		}
	}
	if len(missing) == 0 {
		return probeErr
	}
	names := strings.Join(missing, " ")
	if offline {
		return fmt.Errorf("offline mode skips package installation. Install the missing Ubuntu packages (%s) before launching offline, or relaunch without --offline", names)
	}
	release, _ := os.ReadFile("/etc/os-release")
	if !strings.Contains("\n"+string(release)+"\n", "\nID=ubuntu\n") && !strings.Contains("\n"+string(release)+"\n", "\nID=\"ubuntu\"\n") {
		return fmt.Errorf("%w. Install the missing runtime packages (%s) using your system package manager", probeErr, names)
	}
	command := "/usr/bin/apt-get"
	var prefix []string
	if os.Geteuid() != 0 {
		if _, err := os.Stat("/usr/bin/sudo"); err != nil {
			return fmt.Errorf("ask an administrator to install the missing Ubuntu packages (%s), then relaunch", names)
		}
		prefix = []string{command}
		command = "/usr/bin/sudo"
	}
	fmt.Printf("Installing missing Ubuntu packages: %s. Ubuntu may ask for administrator authorization.\n", names)
	for _, args := range [][]string{{"update"}, append([]string{"install", "-y", "--no-install-recommends", "--no-remove"}, missing...)} {
		cmd := exec.CommandContext(ctx, command, append(prefix, args...)...)
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("Ubuntu package setup failed: %w. Check administrator access and your Ubuntu package sources, then run sudo apt-get update and sudo apt-get install %s", err, names)
		}
	}
	return checkEngine(ctx, root)
}
