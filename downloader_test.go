package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fixtureDir(t *testing.T) string {
	t.Helper()
	os.MkdirAll(".cache", 0755)
	d, e := os.MkdirTemp(".cache", "download-test-")
	if e != nil {
		t.Fatal(e)
	}
	return d
} // Deliberately retained; no permanent deletion.
func specFor(b []byte, url string) ModelSpec {
	h := sha256.Sum256(b)
	return ModelSpec{Name: "fixture.gguf", URL: url, Size: int64(len(b)), SHA256: hex.EncodeToString(h[:])}
}
func TestDownloadResumeIntegrityReuseAndUSB(t *testing.T) {
	b := []byte(strings.Repeat("GGUF-integrity-fixture", 4096))
	calls := 0
	ranges := []string{}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		ranges = append(ranges, r.Header.Get("Range"))
		if calls == 1 {
			w.Header().Set("Content-Length", fmt.Sprint(len(b)))
			w.Write(b[:100])
			return
		}
		offset := 100
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", offset, len(b)-1, len(b)))
		w.Header().Set("Content-Length", fmt.Sprint(len(b)-offset))
		w.WriteHeader(206)
		w.Write(b[offset:])
	}))
	defer s.Close()
	dir := fixtureDir(t)
	spec := specFor(b, s.URL)
	p, e := ensureModel(context.Background(), spec, dir, "", false)
	if e != nil {
		t.Fatal(e)
	}
	if calls != 2 || ranges[1] != "bytes=100-" {
		t.Fatalf("resume not used: %v", ranges)
	}
	s.Close()
	if _, e = ensureModel(context.Background(), spec, dir, "", true); e != nil {
		t.Fatal(e)
	}
	other := fixtureDir(t)
	if _, e = ensureModel(context.Background(), spec, other, p, true); e != nil {
		t.Fatal(e)
	}
	entries, _ := os.ReadDir(other)
	if len(entries) != 0 {
		t.Fatal("USB model unexpectedly copied")
	}
}
func TestInvalidModelAndBadRangePreserved(t *testing.T) {
	b := []byte("GGUF-correct-model")
	dir := fixtureDir(t)
	spec := specFor(b, "http://invalid.local")
	path := filepath.Join(dir, spec.Name)
	os.WriteFile(path, []byte("GGUF-corrupt-model"), 0644)
	if _, e := ensureModel(context.Background(), spec, dir, "", true); e == nil {
		t.Fatal("accepted invalid hash")
	}
	if _, e := os.Stat(path); e != nil {
		t.Fatal("original file removed")
	}
	dir = fixtureDir(t)
	part := filepath.Join(dir, spec.Name+".part")
	os.WriteFile(part, b[:4], 0644)
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(b) }))
	defer s.Close()
	spec.URL = s.URL
	if _, e := ensureModel(context.Background(), spec, dir, "", false); e == nil {
		t.Fatal("accepted ignored Range")
	}
	got, _ := os.ReadFile(part)
	if string(got) != string(b[:4]) {
		t.Fatal("damaged partial file")
	}
}

// A stalled body must time out without discarding the bytes already received.
// The next real downloader call then resumes against the same HTTP server.
func TestDownloadBodyInactivityTimeoutAndResume(t *testing.T) {
	b := []byte(strings.Repeat("GGUF-stall-resume", 1024))
	calls := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.Header().Set("Content-Length", fmt.Sprint(len(b)))
			w.Write(b[:64])
			w.(http.Flusher).Flush()
			<-r.Context().Done()
			return
		}
		if r.Header.Get("Range") != "bytes=64-" {
			t.Errorf("resume Range: %q", r.Header.Get("Range"))
		}
		w.Header().Set("Content-Length", fmt.Sprint(len(b)-64))
		w.Header().Set("Content-Range", fmt.Sprintf("bytes 64-%d/%d", len(b)-1, len(b)))
		w.WriteHeader(http.StatusPartialContent)
		w.Write(b[64:])
	}))
	defer s.Close()
	client := downloadHTTPClient(100 * time.Millisecond)
	defer client.CloseIdleConnections()
	response, err := client.Get(s.URL)
	if err != nil {
		t.Fatal(err)
	}
	dir := fixtureDir(t)
	spec := specFor(b, s.URL)
	f, err := os.Create(filepath.Join(dir, spec.Name+".part"))
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	n, copyErr := io.Copy(f, response.Body)
	f.Close()
	response.Body.Close()
	if copyErr == nil || n != 64 {
		t.Fatalf("stalled body: bytes=%d, error=%v", n, copyErr)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("stalled body was not interrupted promptly")
	}
	if _, err = ensureModel(context.Background(), spec, dir, "", false); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("expected resumed second request, got %d", calls)
	}
}

func TestDownloadReadDeadlineIsNotATotalLimit(t *testing.T) {
	const chunks = 8
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprint(chunks))
		for i := 0; i < chunks; i++ {
			w.Write([]byte("x"))
			w.(http.Flusher).Flush()
			time.Sleep(40 * time.Millisecond)
		}
	}))
	defer s.Close()
	client := downloadHTTPClient(150 * time.Millisecond)
	defer client.CloseIdleConnections()
	start := time.Now()
	response, err := client.Get(s.URL)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || len(data) != chunks {
		t.Fatalf("slow ongoing transfer failed: bytes=%d error=%v", len(data), err)
	}
	if time.Since(start) < 150*time.Millisecond {
		t.Fatal("fixture did not exceed the inactivity interval")
	}
}

func TestOfflinePartialRecoveryRequiresFullValidModel(t *testing.T) {
	b := []byte("GGUF-complete-download-awaiting-checksum")
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("offline recovery contacted the download source")
		w.WriteHeader(500)
	}))
	defer s.Close()
	spec := specFor(b, s.URL)
	for _, tc := range []struct {
		name        string
		data        []byte
		wantSuccess bool
	}{
		{"complete", b, true},
		{"incomplete", b[:12], false},
		{"complete-but-corrupt", []byte(strings.Repeat("x", len(b))), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := fixtureDir(t)
			part := filepath.Join(dir, spec.Name+".part")
			if err := os.WriteFile(part, tc.data, 0644); err != nil {
				t.Fatal(err)
			}
			path, err := ensureModel(context.Background(), spec, dir, "", true)
			if tc.wantSuccess {
				if err != nil {
					t.Fatal(err)
				}
				if err := verifyModel(context.Background(), path, spec); err != nil {
					t.Fatal(err)
				}
				if _, err := os.Stat(part); !os.IsNotExist(err) {
					t.Fatalf("complete .part was not promoted: %v", err)
				}
			} else {
				if err == nil {
					t.Fatal("accepted partial or corrupt model")
				}
				retained, readErr := os.ReadFile(part)
				if readErr != nil || string(retained) != string(tc.data) {
					t.Fatalf("partial bytes not preserved: %v", readErr)
				}
				if _, err := os.Stat(filepath.Join(dir, spec.Name)); !os.IsNotExist(err) {
					t.Fatal("invalid model was promoted")
				}
			}
		})
	}
}
