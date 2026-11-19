package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type ModelSpec struct {
	Name   string `json:"name"`
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

func verifyModel(ctx context.Context, path string, spec ModelSpec) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() != spec.Size {
		return fmt.Errorf("invalid model size: got %d bytes, expected %d", info.Size(), spec.Size)
	}
	h := sha256.New()
	buf := make([]byte, 4*1024*1024)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, e := f.Read(buf)
		if n > 0 {
			h.Write(buf[:n])
		}
		if e == io.EOF {
			break
		}
		if e != nil {
			return e
		}
	}
	if hex.EncodeToString(h.Sum(nil)) != spec.SHA256 {
		return fmt.Errorf("SHA-256 mismatch: the file is incomplete or differs from the expected model. Move it elsewhere or to the Trash before trying again")
	}
	return nil
}

func ensureModel(ctx context.Context, spec ModelSpec, dir, local string, offline bool) (string, error) {
	if local != "" {
		abs, e := filepath.Abs(local)
		if e != nil {
			return "", e
		}
		fmt.Println("Verifying local model in place:", abs)
		fmt.Println("Download: 0.00 s (local file, no network access).")
		return abs, verifyModelTimed(ctx, abs, spec)
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, spec.Name)
	if _, err := os.Stat(path); err == nil {
		fmt.Println("Found the model. Checking its integrity without downloading it again.")
		fmt.Println("Download: 0.00 s (existing model, no network access).")
		return path, verifyModelTimed(ctx, path, spec)
	} else if !os.IsNotExist(err) {
		return "", err
	}
	part := path + ".part"
	partial, statErr := os.Stat(part)
	if statErr != nil && !os.IsNotExist(statErr) {
		return "", statErr
	}
	// An interruption during the final checksum can leave a complete .part.
	// It can be verified and promoted without Internet, including in offline mode.
	if statErr == nil && partial.Size() == spec.Size {
		fmt.Println("Found a complete .part file. Recovering it without network access.")
		fmt.Println("Download: 0.00 s (transfer completed on a previous run).")
		if err := verifyModelTimed(ctx, part, spec); err != nil {
			return "", err
		}
		if err := os.Rename(part, path); err != nil {
			return "", err
		}
		fmt.Println("Model verified and ready.")
		return path, nil
	}
	if offline {
		if statErr == nil {
			return "", fmt.Errorf("incomplete download in offline mode: received %d of %d bytes. The .part file has been kept. Restart with internet access to resume, or use --model with a complete file", partial.Size(), spec.Size)
		}
		return "", fmt.Errorf("model missing in offline mode. Start once with internet access or use --model /path/to/%s", spec.Name)
	}
	client := downloadHTTPClient(45 * time.Second)
	defer client.CloseIdleConnections()
	downloadStart := time.Now()
	var last error
	for attempt := 0; attempt < 4; attempt++ {
		offset := int64(0)
		if st, e := os.Stat(part); e == nil {
			offset = st.Size()
		} else if !os.IsNotExist(e) {
			return "", e
		}
		if offset > spec.Size {
			return "", fmt.Errorf("partial download is too large: move %s to the Trash and try again", part)
		}
		if offset == spec.Size {
			last = nil
			break
		}
		fmt.Printf("Model download: %.1f / %.1f MB. Attempt %d/4. Press Ctrl+C to pause and keep the partial file.\n", float64(offset)/1e6, float64(spec.Size)/1e6, attempt+1)
		req, e := http.NewRequestWithContext(ctx, "GET", spec.URL, nil)
		if e != nil {
			return "", e
		}
		req.Header.Set("Accept-Encoding", "identity")
		if offset > 0 {
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
		}
		resp, e := client.Do(req)
		if e == nil {
			if (offset == 0 && resp.StatusCode != 200) || (offset > 0 && resp.StatusCode != 206) {
				resp.Body.Close()
				return "", fmt.Errorf("download source returned HTTP %d. The partial file has been kept. Try again with a stable connection. If the source does not support resuming, use --model", resp.StatusCode)
			}
			if offset > 0 {
				expected := fmt.Sprintf("bytes %d-", offset)
				if !strings.HasPrefix(resp.Header.Get("Content-Range"), expected) || !strings.HasSuffix(resp.Header.Get("Content-Range"), "/"+strconv.FormatInt(spec.Size, 10)) {
					resp.Body.Close()
					return "", fmt.Errorf("invalid response to resume request. The partial file has been kept")
				}
			}
			if resp.ContentLength >= 0 && resp.ContentLength != spec.Size-offset {
				resp.Body.Close()
				return "", fmt.Errorf("unexpected download size: %d", resp.ContentLength)
			}
			f, openErr := os.OpenFile(part, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
			if openErr != nil {
				resp.Body.Close()
				return "", openErr
			}
			progress := &progressWriter{out: f, total: spec.Size, current: offset, last: time.Now()}
			// Read one byte beyond the expected end to detect an invalid/changing source.
			n, copyErr := io.Copy(progress, io.LimitReader(resp.Body, spec.Size-offset+1))
			resp.Body.Close()
			syncErr := f.Sync()
			closeErr := f.Close()
			if copyErr == nil {
				copyErr = syncErr
			}
			if copyErr == nil {
				copyErr = closeErr
			}
			if copyErr == nil && n != spec.Size-offset {
				copyErr = fmt.Errorf("connection interrupted after %d bytes", n)
			}
			e = copyErr
		}
		if e == nil {
			last = nil
			break
		}
		last = e
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		fmt.Println("Download interrupted. Resuming from the partial file:", e)
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(time.Duration(attempt+1) * time.Second):
		}
	}
	if last != nil {
		return "", fmt.Errorf("download incomplete: %w. The .part file has been kept. Restart to resume", last)
	}
	fmt.Printf("\nDownload and resume: %.2f s (includes retries and disk writes, excludes SHA-256 verification).\n", time.Since(downloadStart).Seconds())
	fmt.Println("Download complete. Verifying SHA-256...")
	if err := verifyModelTimed(ctx, part, spec); err != nil {
		return "", err
	}
	if err := os.Rename(part, path); err != nil {
		return "", err
	}
	fmt.Println("Model verified and ready.")
	return path, nil
}

type progressWriter struct {
	out            io.Writer
	total, current int64
	last           time.Time
}

func (p *progressWriter) Write(b []byte) (int, error) {
	n, e := p.out.Write(b)
	p.current += int64(n)
	if time.Since(p.last) > time.Second {
		fmt.Printf("\rModel: %.1f%% · %.0f / %.0f MB", 100*float64(p.current)/float64(p.total), float64(p.current)/1e6, float64(p.total)/1e6)
		p.last = time.Now()
	}
	return n, e
}

// A fresh deadline on every socket read limits network inactivity, not the
// complete download duration. The timeout propagates to io.Copy, which keeps
// the received bytes and retries with Range. No watchdog goroutine is needed.
type downloadIdleConn struct {
	net.Conn
	idle time.Duration
}

func (c *downloadIdleConn) Read(p []byte) (int, error) {
	if err := c.Conn.SetReadDeadline(time.Now().Add(c.idle)); err != nil {
		return 0, err
	}
	return c.Conn.Read(p)
}

func downloadHTTPClient(idle time.Duration) *http.Client {
	dialer := &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			conn, err := dialer.DialContext(ctx, network, address)
			if err != nil {
				return nil, err
			}
			return &downloadIdleConn{Conn: conn, idle: idle}, nil
		},
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout: 45 * time.Second,
	}
	return &http.Client{Transport: transport}
}

func verifyModelTimed(ctx context.Context, path string, spec ModelSpec) error {
	start := time.Now()
	err := verifyModel(ctx, path, spec)
	if err != nil {
		fmt.Printf("Integrity check did not finish: %.2f s.\n", time.Since(start).Seconds())
	} else {
		fmt.Printf("SHA-256 verification: %.2f s.\n", time.Since(start).Seconds())
	}
	return err
}
