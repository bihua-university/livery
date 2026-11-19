//go:build darwin

package app

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
)

const rusageInfoV2 = 2

type rusageInfoV2Struct struct {
	UUID              [16]byte
	UserTime          uint64
	SystemTime        uint64
	PkgIdleWkups      uint64
	InterruptWkups    uint64
	Pageins           uint64
	WiredSize         uint64
	ResidentSize      uint64
	PhysFootprint     uint64
	ProcStartAbstime  uint64
	ProcExitAbstime   uint64
	ChildUserTime     uint64
	ChildSystemTime   uint64
	ChildPkgIdleWkups uint64
	ChildIntWkups     uint64
	ChildPageins      uint64
	ChildElapsedTime  uint64
	DiskIOReadBytes   uint64
	DiskIOWriteBytes  uint64
}

var (
	darwinProcOnce     sync.Once
	darwinProcPidUsage func(pid int32, flavor int32, buffer uintptr) int32
)

func sampleProcessPrivateMemory(pid int) int64 {
	if pid <= 0 {
		return 0
	}
	darwinProcOnce.Do(func() {
		if lib, err := purego.Dlopen("/usr/lib/libSystem.B.dylib", purego.RTLD_LAZY|purego.RTLD_GLOBAL); err == nil {
			purego.RegisterLibFunc(&darwinProcPidUsage, lib, "proc_pid_rusage")
		}
	})
	if darwinProcPidUsage == nil {
		return 0
	}
	var ri rusageInfoV2Struct
	if darwinProcPidUsage(int32(pid), rusageInfoV2, uintptr(unsafe.Pointer(&ri))) == 0 {
		return int64(ri.PhysFootprint)
	}
	return 0
}

func findCoreListenerPID(port int, configuredPath string) int {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	out, err := exec.CommandContext(ctx, "lsof", "-nP", "-iTCP:"+strconv.Itoa(port), "-sTCP:LISTEN", "-Fp", "-Fc").Output()
	if err != nil {
		return 0
	}
	var curPID int
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "p") {
			curPID, _ = strconv.Atoi(line[1:])
		} else if strings.HasPrefix(line, "c") && curPID > 0 && isKnownCoreExecutable(line[1:], configuredPath) {
			return curPID
		}
	}
	return 0
}

func processExePath(pid int) string {
	if pid <= 0 {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	out, err := exec.CommandContext(ctx, "ps", "-o", "comm=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func trimProcessMemory() {}

