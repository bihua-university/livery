//go:build windows

package app

import (
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

const (
	processQueryLimitedInformation = 0x1000
	afInet                         = 2
	afInet6                        = 23
	tcpTableOwnerPidListener       = 3
)

var (
	memKernel32                  = syscall.NewLazyDLL("kernel32.dll")
	memGetCurrentProcess         = memKernel32.NewProc("GetCurrentProcess")
	memOpenProcess               = memKernel32.NewProc("OpenProcess")
	memCloseHandle               = memKernel32.NewProc("CloseHandle")
	memQueryFullProcessImageName = memKernel32.NewProc("QueryFullProcessImageNameW")
	memK32GetProcessMem          = memKernel32.NewProc("K32GetProcessMemoryInfo")
	memSetWorkingSetSizeEx       = memKernel32.NewProc("SetProcessWorkingSetSizeEx")
	memIphlpapi                  = syscall.NewLazyDLL("iphlpapi.dll")
	memGetExtendedTcpTable       = memIphlpapi.NewProc("GetExtendedTcpTable")
)

type processMemoryCountersEx2 struct {
	cb                         uint32
	pageFaultCount             uint32
	peakWorkingSetSize         uintptr
	workingSetSize             uintptr
	quotaPeakPagedPoolUsage    uintptr
	quotaPagedPoolUsage        uintptr
	quotaPeakNonPagedPoolUsage uintptr
	quotaNonPagedPoolUsage     uintptr
	pagefileUsage              uintptr
	peakPagefileUsage          uintptr
	privateUsage               uintptr
	privateWorkingSetSize      uintptr
	sharedCommitUsage          uint64
}

func sampleProcessPrivateMemory(pid int) int64 {
	if pid <= 0 {
		return 0
	}
	var proc uintptr
	if pid == os.Getpid() {
		proc, _, _ = memGetCurrentProcess.Call()
	} else {
		proc, _, _ = memOpenProcess.Call(processQueryLimitedInformation, 0, uintptr(pid))
		if proc != 0 {
			defer memCloseHandle.Call(proc)
		}
	}
	if proc == 0 {
		return 0
	}

	var ex2 processMemoryCountersEx2
	ex2.cb = uint32(unsafe.Sizeof(ex2))
	if r, _, _ := memK32GetProcessMem.Call(proc, uintptr(unsafe.Pointer(&ex2)), uintptr(ex2.cb)); r != 0 {
		return int64(ex2.privateWorkingSetSize)
	}
	return 0
}

func findCoreListenerPID(port int, configuredPath string) int {
	for _, spec := range []struct {
		af      uintptr
		rowSize uintptr
		portOff uintptr
		pidOff  uintptr
	}{
		{afInet, 24, 8, 20},   // MIB_TCPROW_OWNER_PID
		{afInet6, 56, 20, 52}, // MIB_TCP6ROW_OWNER_PID
	} {
		var size uint32
		memGetExtendedTcpTable.Call(0, uintptr(unsafe.Pointer(&size)), 0, spec.af, tcpTableOwnerPidListener, 0)
		if size < 4 {
			continue
		}
		buf := make([]byte, size)
		if r, _, _ := memGetExtendedTcpTable.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)), 0, spec.af, tcpTableOwnerPidListener, 0); r != 0 {
			continue
		}
		count := *(*uint32)(unsafe.Pointer(&buf[0]))
		for i := uint32(0); i < count; i++ {
			off := 4 + uintptr(i)*spec.rowSize
			if off+spec.rowSize > uintptr(len(buf)) {
				break
			}
			rawPort := *(*uint32)(unsafe.Pointer(&buf[off+spec.portOff]))
			p := int((rawPort&0xff)<<8 | (rawPort>>8)&0xff)
			pid := int(*(*uint32)(unsafe.Pointer(&buf[off+spec.pidOff])))
			if p == port && pid > 0 && isKnownCoreExecutable(processExeBaseName(pid), configuredPath) {
				return pid
			}
		}
	}
	return 0
}

func processExePath(pid int) string {
	h, _, _ := memOpenProcess.Call(processQueryLimitedInformation, 0, uintptr(pid))
	if h == 0 {
		return ""
	}
	defer memCloseHandle.Call(h)

	var buf [512]uint16
	size := uint32(len(buf))
	if r, _, _ := memQueryFullProcessImageName.Call(h, 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size))); r == 0 || size == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf[:size])
}

func processExeBaseName(pid int) string {
	p := processExePath(pid)
	if p == "" {
		return ""
	}
	return filepath.Base(p)
}

func trimProcessMemory() {
	proc, _, _ := memGetCurrentProcess.Call()
	if proc == 0 || memSetWorkingSetSizeEx.Find() != nil {
		return
	}
	allOnes := ^uintptr(0)
	_, _, _ = memSetWorkingSetSizeEx.Call(proc, allOnes, allOnes, 0)
}

