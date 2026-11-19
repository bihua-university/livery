//go:build windows

package core

import (
	"os"
	"os/exec"
	"sync"
	"syscall"
	"unsafe"

	"github.com/bihua-university/livery/internal/model"
)

const (
	createNoWindow                    = 0x08000000
	tokenQuery                        = 0x0008
	tokenElevation                    = 20
	swShowNormal                      = 1
	jobObjectExtendedLimitInfoClass = 9
	jobObjectLimitKillOnJobClose    = 0x00002000
	processSetQuota                   = 0x0100
	processTerminate                  = 0x0001
)

var (
	shell32                  = syscall.NewLazyDLL("shell32.dll")
	shellExecuteW            = shell32.NewProc("ShellExecuteW")
	advapi32                 = syscall.NewLazyDLL("advapi32.dll")
	openProcessToken         = advapi32.NewProc("OpenProcessToken")
	getTokenInformation      = advapi32.NewProc("GetTokenInformation")
	kernel32                 = syscall.NewLazyDLL("kernel32.dll")
	getCurrentProcess        = kernel32.NewProc("GetCurrentProcess")
	openProcess              = kernel32.NewProc("OpenProcess")
	closeHandle              = kernel32.NewProc("CloseHandle")
	createJobObjectW         = kernel32.NewProc("CreateJobObjectW")
	setInformationJobObject  = kernel32.NewProc("SetInformationJobObject")
	assignProcessToJobObject = kernel32.NewProc("AssignProcessToJobObject")
	terminateJobObject       = kernel32.NewProc("TerminateJobObject")
)

type jobObjectBasicLimitInformation struct {
	PerProcessUserTimeLimit int64
	PerJobUserTimeLimit     int64
	LimitFlags              uint32
	MinimumWorkingSetSize   uintptr
	MaximumWorkingSetSize   uintptr
	ActiveProcessLimit      uint32
	Affinity                uintptr
	PriorityClass           uint32
	SchedulingClass         uint32
}

type ioCounters struct {
	ReadOperationCount  uint64
	WriteOperationCount uint64
	OtherOperationCount uint64
	ReadTransferCount   uint64
	WriteTransferCount  uint64
	OtherTransferCount  uint64
}

type jobObjectExtendedLimitInformation struct {
	BasicLimitInformation jobObjectBasicLimitInformation
	IoInfo                ioCounters
	ProcessMemoryLimit    uintptr
	JobMemoryLimit        uintptr
	PeakProcessMemoryUsed uintptr
	PeakJobMemoryUsed     uintptr
}

// processGuard binds a spawned core process to a Windows Job Object configured with
// JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE so Windows automatically terminates the core
// (and any child processes it spawned) whenever Livery exits, crashes, or is killed.
type processGuard struct {
	mu  sync.Mutex
	job syscall.Handle
}

func configureCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow,
	}
}

func attachProcessGuard(cmd *exec.Cmd) *processGuard {
	if cmd == nil || cmd.Process == nil || cmd.Process.Pid <= 0 {
		return nil
	}
	job, _, _ := createJobObjectW.Call(0, 0)
	if job == 0 {
		return nil
	}
	var info jobObjectExtendedLimitInformation
	info.BasicLimitInformation.LimitFlags = jobObjectLimitKillOnJobClose
	r, _, _ := setInformationJobObject.Call(
		job,
		jobObjectExtendedLimitInfoClass,
		uintptr(unsafe.Pointer(&info)),
		unsafe.Sizeof(info),
	)
	if r == 0 {
		closeHandle.Call(job)
		return nil
	}
	proc, _, _ := openProcess.Call(processSetQuota|processTerminate, 0, uintptr(cmd.Process.Pid))
	if proc == 0 {
		closeHandle.Call(job)
		return nil
	}
	assigned, _, _ := assignProcessToJobObject.Call(job, proc)
	closeHandle.Call(proc)
	if assigned == 0 {
		closeHandle.Call(job)
		return nil
	}
	return &processGuard{job: syscall.Handle(job)}
}

func (g *processGuard) Terminate() {
	if g == nil {
		return
	}
	g.mu.Lock()
	job := g.job
	g.job = 0
	g.mu.Unlock()
	if job != 0 {
		terminateJobObject.Call(uintptr(job), 1)
		closeHandle.Call(uintptr(job))
	}
}

func (g *processGuard) Close() {
	if g == nil {
		return
	}
	g.mu.Lock()
	job := g.job
	g.job = 0
	g.mu.Unlock()
	if job != 0 {
		closeHandle.Call(uintptr(job))
	}
}

func terminateProcessGraceful(_ *exec.Cmd) bool {
	// Windows console-less child processes do not support POSIX SIGINT.
	return false
}

func terminateProcessForce(cmd *exec.Cmd, guard *processGuard) {
	if guard != nil {
		guard.Terminate()
	}
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}

// IsElevated reports whether the current process has Administrator privileges.
func IsElevated() bool {
	proc, _, _ := getCurrentProcess.Call()
	var token uintptr
	r, _, _ := openProcessToken.Call(proc, tokenQuery, uintptr(unsafe.Pointer(&token)))
	if r == 0 {
		return false
	}
	defer closeHandle.Call(token)

	var elevation uint32
	var retLen uint32
	r, _, _ = getTokenInformation.Call(
		token,
		tokenElevation,
		uintptr(unsafe.Pointer(&elevation)),
		unsafe.Sizeof(elevation),
		uintptr(unsafe.Pointer(&retLen)),
	)
	return r != 0 && elevation != 0
}

// RelaunchElevated prompts Windows UAC to restart the current executable as Administrator.
func RelaunchElevated() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	verb, _ := syscall.UTF16PtrFromString("runas")
	file, _ := syscall.UTF16PtrFromString(exe)
	r, _, callErr := shellExecuteW.Call(0, uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(file)), 0, 0, swShowNormal)
	if r <= 32 {
		return callErr
	}
	return nil
}

func binaryNames(coreType model.CoreType) []string {
	if model.NormalizeCoreType(coreType) == model.CoreSingBox {
		return []string{"sing-box.exe", "sing-box"}
	}
	return []string{"mihomo.exe", "mihomo", "clash-meta.exe", "clash-meta"}
}
