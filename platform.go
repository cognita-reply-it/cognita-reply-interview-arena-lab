package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

func checkPlatform() error {
	if runtime.GOARCH != platformArch {
		return fmt.Errorf("this kit requires %s", platformID)
	}
	return checkOSVersion()
}

func enginePath(root string) string {
	return filepath.Join(root, "runtime", platformID, "llama-b11233", engineBinaryName)
}

func engineCommand(ctx context.Context, root string, args ...string) *exec.Cmd {
	binary := enginePath(root)
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = filepath.Dir(binary)
	configureEngine(cmd)
	return cmd
}

// Catch missing libraries and incompatible runtimes before downloading model weights.
func checkEngine(ctx context.Context, root string) error {
	probeCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := engineCommand(probeCtx, root, "--version")
	if _, err := os.Stat(cmd.Path); err != nil {
		return fmt.Errorf("engine missing or inaccessible: %w. Extract the complete %s kit", err, platformID)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("the %s engine could not start: %w. See the platform requirements in README.md. %s", platformID, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func lockKit(dir string) (func(), error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	return lockFile(filepath.Join(dir, "instance.lock"))
}

func openBrowser(url string) {
	if err := browserCommand(url).Run(); err != nil {
		fmt.Println("Open this link in your browser:", url)
	}
}

func startEngine(ctx context.Context, root, model, key string, cpu bool) (string, func(), <-chan error, error) {
	probe, e := net.Listen("tcp4", "127.0.0.1:0")
	if e != nil {
		return "", nil, nil, e
	}
	addr := probe.Addr().String()
	probe.Close()
	_, port, _ := net.SplitHostPort(addr)
	gpu := "99"
	mode := "Metal"
	if cpu || !supportsMetal {
		gpu = "0"
		mode = "CPU"
	}
	logPath := filepath.Join(root, ".arena", fmt.Sprintf("engine-%s.log", time.Now().Format("20060102-150405.000000000")))
	log, e := os.OpenFile(logPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return "", nil, nil, e
	}
	args := []string{"--model", model, "--host", "127.0.0.1", "--port", port, "--api-key", key, "--ctx-size", "8192", "--parallel", "1", "--n-gpu-layers", gpu, "--threads", "8", "--jinja", "--reasoning-format", "deepseek", "--reasoning-budget", fmt.Sprint(coachReasoningBudget), "--no-webui", "--no-warmup"}
	if gpu == "0" {
		args = append(args, "--device", "none")
	}
	cmd := engineCommand(context.Background(), root, args...)
	cmd.Stdout = log
	cmd.Stderr = log
	start := time.Now()
	stopProcess, e := launchEngine(cmd)
	if e != nil {
		log.Close()
		return "", nil, nil, fmt.Errorf("could not start the %s engine: %w. See the platform requirements in README.md", platformID, e)
	}
	// finished is a broadcast for cleanup; exited has exactly one consumer
	// (startup before readiness, main after readiness). cmd.Wait runs only once.
	finished := make(chan struct{})
	exited := make(chan error, 1)
	go func() {
		err := cmd.Wait()
		close(finished)
		if err != nil {
			exited <- fmt.Errorf("local engine exited: %w. See %s", err, logPath)
		} else {
			exited <- fmt.Errorf("local engine exited. See %s", logPath)
		}
		close(exited)
	}()
	var once sync.Once
	stop := func() {
		once.Do(func() {
			stopProcess(finished)
			log.Close()
			fmt.Println("Local engine stopped.")
		})
	}
	url := "http://" + addr
	client := &http.Client{Timeout: time.Second, Transport: &http.Transport{Proxy: nil}}
	fmt.Println("Loading model ·", mode, "· log:", logPath)
	deadline := time.NewTimer(120 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			stop()
			return "", nil, nil, ctx.Err()
		case e := <-exited:
			stop()
			return "", nil, nil, e
		case <-deadline.C:
			stop()
			return "", nil, nil, fmt.Errorf("model loading exceeded 120 seconds. See %s", logPath)
		case <-ticker.C:
			req, _ := http.NewRequest("GET", url+"/health", nil)
			req.Header.Set("Authorization", "Bearer "+key)
			resp, e := client.Do(req)
			if e == nil {
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
				if resp.StatusCode == 200 {
					fmt.Printf("Loaded %s in %.2f s · PID %d\n", mode, time.Since(start).Seconds(), cmd.Process.Pid)
					return url, stop, exited, nil
				}
			}
		}
	}
}
