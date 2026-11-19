//go:build windows

package app

import (
	"syscall"
	"unsafe"
)

var (
	memKernel32            = syscall.NewLazyDLL("kernel32.dll")
	memGetCurrentProcess   = memKernel32.NewProc("GetCurrentProcess")
	memK32GetProcessMem    = memKernel32.NewProc("K32GetProcessMemoryInfo")
	memPsapi               = syscall.NewLazyDLL("psapi.dll")
	memGetProcessMemLegacy = memPsapi.NewProc("GetProcessMemoryInfo")
)

type processMemoryCounters struct {
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
}

func sampleProcessRSS() int64 {
	proc, _, _ := memGetCurrentProcess.Call()
	if proc == 0 {
		return 0
	}
	var counters processMemoryCounters
	counters.cb = uint32(unsafe.Sizeof(counters))

	if memK32GetProcessMem.Find() == nil {
		r, _, _ := memK32GetProcessMem.Call(
			proc,
			uintptr(unsafe.Pointer(&counters)),
			uintptr(counters.cb),
		)
		if r != 0 {
			return int64(counters.workingSetSize)
		}
	}
	if memGetProcessMemLegacy.Find() == nil {
		r, _, _ := memGetProcessMemLegacy.Call(
			proc,
			uintptr(unsafe.Pointer(&counters)),
			uintptr(counters.cb),
		)
		if r != 0 {
			return int64(counters.workingSetSize)
		}
	}
	return 0
}
