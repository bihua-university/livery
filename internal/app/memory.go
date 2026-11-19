package app

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/bihua-university/livery/internal/model"
)

// sampleAppMemory returns this process's private physical memory (PrivateWorkingSetSize on Windows,
// Private_Clean+Private_Dirty on Linux, phys_footprint on macOS), falling back to Go heap alloc.
func sampleAppMemory() int64 {
	if mem := sampleProcessPrivateMemory(os.Getpid()); mem > 0 {
		return mem
	}
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return int64(ms.HeapAlloc)
}

// sampleCoreProcessMemory returns the running core's private physical memory when its PID is known
// or resolvable from the local Clash API port.
func sampleCoreProcessMemory(info model.CoreInfo, apiPort int) int64 {
	pid := info.PID
	if pid <= 0 && info.External && apiPort > 0 {
		pid = findCoreListenerPID(apiPort, info.BinaryPath)
	}
	if pid <= 0 {
		return 0
	}
	return sampleProcessPrivateMemory(pid)
}

func isKnownCoreExecutable(name, configuredPath string) bool {
	base := strings.ToLower(strings.TrimSpace(filepath.Base(name)))
	switch base {
	case "mihomo", "mihomo.exe", "clash-meta", "clash-meta.exe", "sing-box", "sing-box.exe":
		return true
	}
	if cfg := strings.TrimSpace(configuredPath); cfg != "" {
		return base == strings.ToLower(strings.TrimSpace(filepath.Base(cfg)))
	}
	return false
}
