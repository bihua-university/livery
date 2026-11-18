//go:build windows

package core

import (
	"os"
	"os/exec"
	"syscall"
	"unsafe"

	"github.com/bihua-university/livery/internal/model"
)

const (
	createNoWindow  = 0x08000000
	tokenQuery      = 0x0008
	tokenElevation  = 20
	swShowNormal    = 1
)

var (
	shell32             = syscall.NewLazyDLL("shell32.dll")
	shellExecuteW       = shell32.NewProc("ShellExecuteW")
	advapi32            = syscall.NewLazyDLL("advapi32.dll")
	openProcessToken    = advapi32.NewProc("OpenProcessToken")
	getTokenInformation = advapi32.NewProc("GetTokenInformation")
	kernel32            = syscall.NewLazyDLL("kernel32.dll")
	getCurrentProcess   = kernel32.NewProc("GetCurrentProcess")
	closeHandle         = kernel32.NewProc("CloseHandle")
)

func configureCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow,
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
