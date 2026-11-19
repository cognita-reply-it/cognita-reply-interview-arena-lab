package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"time"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "\nAI Interview Arena:", err)
		os.Exit(1)
	}
}
func run() error {
	root := flag.String("root", "", "kit directory (defaults to the directory containing the executable)")
	model := flag.String("model", "", "existing local or USB GGUF file to verify and use in place")
	offline := flag.Bool("offline", false, "disable downloads")
	cpu := flag.Bool("cpu", false, "use CPU inference (the default on Ubuntu and Windows)")
	prepare := flag.Bool("prepare-only", false, "verify or download the model, then exit")
	noBrowser := flag.Bool("no-browser", false, "skip opening the browser")
	port := flag.Int("port", 0, "browser port (0 selects an available port)")
	budget := flag.Int("reasoning-budget", coachReasoningBudget, "maximum reasoning tokens for a controlled comparison")
	capture := flag.String("capture-dir", "", "private tests: capture engine requests under .local-tests")
	maxTokens := flag.Int("max-tokens", coachMaxTokens, "total token budget, including final feedback")
	replyTimeout := flag.Duration("reply-timeout", defaultReplyTimeout(), "maximum time for a complete reply (e.g. 60s or 5m)")
	flag.Parse()
	if *replyTimeout <= 0 {
		return fmt.Errorf("--reply-timeout must be positive")
	}
	if *budget < 0 || *budget > 2048 || *maxTokens < *budget+256 || *maxTokens > 4096 {
		return fmt.Errorf("invalid token budget: reasoning must be 0–2048 and the total must be between reasoning+256 and 4096")
	}
	coachReasoningBudget, coachMaxTokens = *budget, *maxTokens
	if err := checkPlatform(); err != nil {
		return err
	}
	if *root == "" {
		exe, e := os.Executable()
		if e != nil {
			return e
		}
		*root = filepath.Dir(exe)
	}
	abs, e := filepath.Abs(*root)
	if e != nil {
		return e
	}
	*root = abs
	raw, e := os.ReadFile(filepath.Join(*root, "manifest.json"))
	if e != nil {
		return e
	}
	var manifest struct {
		Model ModelSpec `json:"model"`
	}
	if e = json.Unmarshal(raw, &manifest); e != nil {
		return e
	}
	if manifest.Model.Size <= 0 || len(manifest.Model.SHA256) != 64 {
		return fmt.Errorf("invalid model manifest")
	}
	ctx, cancel := signal.NotifyContext(context.Background(), stopSignals...)
	defer cancel()
	// The advisory OS lock is released even after a forced application exit.
	unlock, e := lockKit(filepath.Join(*root, ".arena"))
	if e != nil {
		return e
	}
	defer unlock()
	fmt.Println("AI Interview Arena · Cognita Reply / Innovator Bootcamp\nRecognise your strengths and describe them in your own words, drawing on what you actually did.")
	start := time.Now()
	if !*prepare {
		if err := prepareEngine(ctx, *root, *offline); err != nil {
			return err
		}
	}
	path, e := ensureModel(ctx, manifest.Model, filepath.Join(*root, "models"), *model, *offline)
	if e != nil {
		return e
	}
	fmt.Printf("\nModel preparation and verification: %.2f s\n", time.Since(start).Seconds())
	if *prepare {
		return nil
	}
	token := make([]byte, 24)
	if _, e = rand.Read(token); e != nil {
		return e
	}
	key := hex.EncodeToString(token)
	engine, stop, engineDone, e := startEngine(ctx, *root, path, key, *cpu)
	if e != nil {
		return e
	}
	defer stop()
	listener, e := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", *port))
	if e != nil {
		return e
	}
	app, e := newApp(engine, key, filepath.Join(*root, "configs"), cancel)
	if e != nil {
		listener.Close()
		return e
	}
	app.(*arenaApp).client.Timeout = *replyTimeout
	if *capture != "" {
		if e = app.(*arenaApp).setCaptureDir(*capture); e != nil {
			listener.Close()
			return e
		}
	}
	server := &http.Server{Handler: app, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}
	url := "http://" + listener.Addr().String()
	fmt.Printf("Ready: %s\nTotal startup time: %.2f s. Press Ctrl+C or choose Settings > Stop the app in the browser to quit.\n", url, time.Since(start).Seconds())
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	if !*noBrowser {
		go openBrowser(url)
	}
	var runErr error
	select {
	case e := <-done:
		if e != http.ErrServerClosed {
			runErr = e
		}
		cancel()
	case e := <-engineDone:
		// Cancellation from Ctrl+C, SIGHUP or the UI is an intentional stop.
		if ctx.Err() == nil {
			runErr = e
		}
		cancel()
	case <-ctx.Done():
	}
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer shutdownCancel()
	server.Shutdown(shutdownCtx)
	return runErr
}
