package main

import (
	"context"
	"debug/pe"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const redistName = "VC_redist.x64.exe"
const redistURL = "https://aka.ms/vc14/vc_redist.x64.exe"

func prepareEngine(ctx context.Context, root string, offline bool) error {
	probeErr := checkEngine(ctx, root)
	if probeErr == nil || !windowsLoaderError(probeErr) {
		return probeErr
	}
	// Only repair the VC runtime if a DLL or export used by this engine is missing.
	missing := missingVCRuntime(filepath.Dir(enginePath(root)))
	if missing == "" {
		return probeErr
	}
	fmt.Println("The engine requires Microsoft Visual C++ v14 (x64):", missing)
	if err := installVCRuntime(ctx, root, offline); err != nil {
		return err
	}
	return checkEngine(ctx, root)
}

func windowsLoaderError(err error) bool {
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		switch uint32(exit.ExitCode()) {
		case 0xc0000135, 0xc0000138, 0xc0000139: // DLL, ordinal or entry point not found
			return true
		}
	}
	return errors.Is(err, syscall.Errno(126)) || errors.Is(err, syscall.Errno(127))
}

func missingVCRuntime(dir string) string {
	system := filepath.Join(os.Getenv("SystemRoot"), "System32")
	// These are the non-system modules imported by the pinned server.
	runtimeDLLs := make(map[string]*syscall.DLL)
	defer func() {
		for _, dll := range runtimeDLLs {
			dll.Release()
		}
	}()
	for _, module := range []string{engineBinaryName, "llama-server-impl.dll", "llama-common.dll", "llama.dll", "mtmd.dll", "ggml.dll", "ggml-base.dll", "libomp.dll"} {
		f, err := pe.Open(filepath.Join(dir, module))
		if err != nil {
			return "" // The kit itself is incomplete or damaged.
		}
		imports, err := f.ImportedSymbols()
		f.Close()
		if err != nil {
			return ""
		}
		for _, item := range imports {
			sep := strings.LastIndexByte(item, ':')
			if sep < 0 {
				continue
			}
			name := strings.ToLower(item[sep+1:])
			switch name {
			case "msvcp140.dll", "vcruntime140.dll", "vcruntime140_1.dll":
			default:
				continue
			}
			dll := runtimeDLLs[name]
			if dll == nil {
				dll, err = syscall.LoadDLL(filepath.Join(system, name))
				if err != nil {
					return name
				}
				runtimeDLLs[name] = dll
			}
			if _, err := dll.FindProc(item[:sep]); err != nil {
				return name + " requires an update"
			}
		}
	}
	return ""
}

func installVCRuntime(ctx context.Context, root string, offline bool) error {
	local, err := os.Open(filepath.Join(root, redistName))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	var source io.ReadCloser
	if local != nil {
		source = local
	} else {
		if offline {
			return fmt.Errorf("offline mode: place Microsoft's %s beside the launcher, or ask IT to install the Visual C++ v14 x64 Redistributable", redistName)
		}
		fmt.Println("Downloading the Microsoft Visual C++ x64 Redistributable...")
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, redistURL, nil)
		if err != nil {
			return err
		}
		client := http.Client{Timeout: 5 * time.Minute, CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if req.URL.Scheme != "https" || len(via) >= 10 {
				return fmt.Errorf("invalid Microsoft download redirect")
			}
			return nil
		}}
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("Microsoft runtime download: %w", err)
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return fmt.Errorf("Microsoft runtime download: %s", resp.Status)
		}
		source = resp.Body
	}
	defer source.Close()

	dir, err := os.MkdirTemp("", "arena-vcredist-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir) // Only the launcher's private staging directory.
	path := filepath.Join(dir, redistName)
	dest, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	const limit = 128 << 20
	size, err := io.Copy(dest, io.LimitReader(source, limit+1))
	closeErr := dest.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if size == 0 || size > limit {
		return fmt.Errorf("invalid Microsoft runtime installer size")
	}

	system := filepath.Join(os.Getenv("SystemRoot"), "System32")
	ps := filepath.Join(system, "WindowsPowerShell", "v1.0", "powershell.exe")
	cmd := exec.CommandContext(ctx, ps, "-NoProfile", "-NonInteractive", "-Command", verifyAndInstallRedist)
	cmd.Dir = system
	cmd.Env = append(cmd.Environ(), "ARENA_REDIST_PATH="+path)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	fmt.Println("Verifying the Microsoft installer. Windows may ask for administrator approval.")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("Visual C++ prerequisite setup: %w. See the message above or ask IT to install the x64 redistributable", err)
	}
	return nil
}

// The command is fixed. The file path is data and never becomes PowerShell source.
// Hold the staged file against replacement between signature verification and setup.
const verifyAndInstallRedist = `
$ErrorActionPreference = 'Stop'
$p = $env:ARENA_REDIST_PATH
$lock = [IO.File]::Open($p, 'Open', 'Read', 'Read')
try {
    $signature = Get-AuthenticodeSignature -LiteralPath $p
    $info = [Diagnostics.FileVersionInfo]::GetVersionInfo($p)
    if ($signature.Status -ne 'Valid' -or
        $signature.SignerCertificate.GetNameInfo('SimpleName', $false) -cne 'Microsoft Corporation' -or
        $info.OriginalFilename -ine 'VC_redist.x64.exe' -or
        $info.ProductName -notlike 'Microsoft Visual C++*Redistributable (x64)*' -or
        $info.FileMajorPart -ne 14) {
        throw 'Installer verification failed. Use the official Microsoft Visual C++ v14 x64 Redistributable.'
    }
    try {
        $result = Start-Process -FilePath $p -ArgumentList '/install','/passive','/norestart' -WorkingDirectory (Split-Path -LiteralPath $p) -Verb RunAs -Wait -PassThru
    } catch {
        if ($_.Exception.GetBaseException().NativeErrorCode -eq 1223) {
            throw 'Administrator approval was cancelled. Reopen the launcher or ask IT to install the runtime.'
        }
        throw
    }
    # Burn may return installer errors as HRESULT_FROM_WIN32.
    $code = $result.ExitCode
    if (($code -shr 16) -eq -32761) { $code = $code -band 65535 }
    switch ($code) {
        0 { exit 0 }
        1638 { exit 0 } # A newer version exists. Repeat the engine probe.
        3010 { throw 'Restart Windows, then reopen the launcher to complete setup.' }
        1641 { throw 'Windows is restarting. Reopen the launcher after the restart.' }
        1602 { throw 'Setup was cancelled. Reopen the launcher when ready.' }
        default { throw "Microsoft Visual C++ setup returned $($result.ExitCode)." }
    }
} finally {
    $lock.Dispose()
}
`
