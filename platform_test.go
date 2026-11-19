package main

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// Run the current test executable as a tiny HTTP engine, so process lifecycle
// checks do not load the real model or disturb a running benchmark.
func TestMain(m *testing.M) {
	if filepath.Base(os.Args[0]) != engineBinaryName {
		os.Exit(m.Run())
	}
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		os.Exit(0)
	}
	port, mode, device := "", "", ""
	for i, arg := range os.Args {
		if i+1 >= len(os.Args) {
			continue
		}
		switch arg {
		case "--port":
			port = os.Args[i+1]
		case "--model":
			mode = os.Args[i+1]
		case "--device":
			device = os.Args[i+1]
		}
	}
	if port == "" {
		os.Exit(8)
	}
	if mode == "require-cpu" && device != "none" {
		os.Exit(9)
	}
	var once sync.Once
	server := http.Server{Addr: "127.0.0.1:" + port, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(`{"status":"ok"}`))
		if mode == "exit-after-ready" {
			once.Do(func() { time.AfterFunc(100*time.Millisecond, func() { os.Exit(7) }) })
		}
	})}
	if err := server.ListenAndServe(); err != nil {
		os.Exit(8)
	}
}

func engineFixture(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(fixtureDir(t))
	if err != nil {
		t.Fatal(err)
	}
	target := enginePath(root)
	binDir := filepath.Dir(target)
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".arena"), 0755); err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	src, err := os.Open(executable)
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0755)
	if err != nil {
		t.Fatal(err)
	}
	_, err = io.Copy(out, src)
	closeErr := out.Close()
	if err != nil {
		t.Fatal(err)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	return root
}

func TestEngineExitIsObservableAndCleanupIdempotent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, stop, exited, err := startEngine(ctx, engineFixture(t), "exit-after-ready", "test-key", false)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	select {
	case err := <-exited:
		if err == nil || !strings.Contains(err.Error(), "exit status 7") || !strings.Contains(err.Error(), "engine-") {
			t.Fatalf("missing engine exit diagnostic: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("engine exit was not observed")
	}
	stop()
	stop()
}

func TestEngineStopAndCPUDeviceSelection(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, stop, exited, err := startEngine(ctx, engineFixture(t), "require-cpu", "test-key", true)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	stopped := make(chan struct{})
	go func() { stop(); close(stopped) }()
	select {
	case <-stopped:
	case <-time.After(4 * time.Second):
		t.Fatal("engine stop did not finish")
	}
	select {
	case <-exited:
	case <-time.After(time.Second):
		t.Fatal("engine child did not exit")
	}
	stop()
}

func TestBundledEnginePreflight(t *testing.T) {
	ctx := context.Background()
	root := engineFixture(t)
	if err := checkEngine(ctx, root); err != nil {
		t.Fatal(err)
	}
	if err := checkEngine(ctx, filepath.Join(root, "missing")); err == nil {
		t.Fatal("missing engine passed startup check")
	}
}

func TestConcurrentKitLock(t *testing.T) {
	dir := fixtureDir(t)
	unlock, err := lockKit(dir)
	if err != nil {
		t.Fatal(err)
	}
	if extra, err := lockKit(dir); err == nil {
		extra()
		unlock()
		t.Fatal("concurrent launch acquired kit lock")
	}
	unlock()
	unlock, err = lockKit(dir)
	if err != nil {
		t.Fatal(err)
	}
	unlock()
}
