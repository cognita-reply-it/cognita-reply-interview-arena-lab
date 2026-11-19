package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"unsafe"
)

const platformID = "windows-x64"
const platformArch = "amd64"
const engineBinaryName = "llama-server.exe"
const supportsMetal = false

var stopSignals = []os.Signal{os.Interrupt, syscall.SIGTERM}

func checkOSVersion() error         { return nil }
func configureEngine(cmd *exec.Cmd) {}
func browserCommand(url string) *exec.Cmd {
	return exec.Command(filepath.Join(os.Getenv("SystemRoot"), "System32", "rundll32.exe"), "url.dll,FileProtocolHandler", url)
}

func lockFile(path string) (func(), error) {
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	// Share mode zero reserves the file until Windows closes this process's handle.
	h, err := syscall.CreateFile(name, syscall.GENERIC_READ|syscall.GENERIC_WRITE, 0, nil, syscall.OPEN_ALWAYS, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err == syscall.Errno(32) { // ERROR_SHARING_VIOLATION
		return nil, fmt.Errorf("the kit is already running: use the open window or stop it before restarting")
	}
	if err != nil {
		return nil, err
	}
	return func() { syscall.CloseHandle(h) }, nil
}

// Win32 JOBOBJECT_EXTENDED_LIMIT_INFORMATION on Windows x64.
type jobExtendedLimits struct {
	processTime, jobTime           int64
	limitFlags                     uint32
	minWorkingSet, maxWorkingSet   uintptr
	activeProcessLimit             uint32
	affinity                       uintptr
	priorityClass, schedulingClass uint32
	ioCounters                     [6]uint64
	memoryLimitsAndPeaks           [4]uintptr
}

var (
	kernel32   = syscall.NewLazyDLL("kernel32.dll")
	createJob  = kernel32.NewProc("CreateJobObjectW")
	setJobInfo = kernel32.NewProc("SetInformationJobObject")
	assignJob  = kernel32.NewProc("AssignProcessToJobObject")
)

func launchEngine(cmd *exec.Cmd) (func(<-chan struct{}), error) {
	job, _, err := createJob.Call(0, 0)
	if job == 0 {
		return nil, fmt.Errorf("creating Windows job: %w", err)
	}
	closeJob := func() { syscall.CloseHandle(syscall.Handle(job)) }
	const jobObjectExtendedLimitInformation = 9
	const jobObjectLimitKillOnJobClose = 0x2000
	limits := jobExtendedLimits{limitFlags: jobObjectLimitKillOnJobClose}
	ok, _, err := setJobInfo.Call(job, jobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), unsafe.Sizeof(limits))
	if ok == 0 {
		closeJob()
		return nil, fmt.Errorf("configuring Windows job: %w", err)
	}
	// Let the launcher handle console Ctrl+C. Closing the job terminates the
	// engine and its descendants even if the launcher is forcibly terminated.
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
	if err := cmd.Start(); err != nil {
		closeJob()
		return nil, err
	}
	var assignErr error
	err = cmd.Process.WithHandle(func(h uintptr) {
		ok, _, e := assignJob.Call(job, h)
		if ok == 0 {
			assignErr = e
		}
	})
	if err == nil {
		err = assignErr
	}
	if err != nil {
		cmd.Process.Kill()
		cmd.Wait()
		closeJob()
		return nil, fmt.Errorf("assigning engine to Windows job: %w", err)
	}
	return func(finished <-chan struct{}) {
		closeJob()
		<-finished
	}, nil
}
