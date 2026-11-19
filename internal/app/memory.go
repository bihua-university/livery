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

// isManagedCoreExecutablePath reports whether exePath points to a mihomo or sing-box
// binary owned by Livery (under binDir or matching the configured core binary path).
func isManagedCoreExecutablePath(exePath, binDir, configuredPath string) bool {
	exePath = strings.TrimSpace(exePath)
	if exePath == "" || !isKnownCoreExecutable(exePath, configuredPath) {
		return false
	}
	cleanExe := filepath.Clean(exePath)
	if cfg := strings.TrimSpace(configuredPath); cfg != "" {
		if strings.EqualFold(cleanExe, filepath.Clean(cfg)) {
			return true
		}
	}
	if bin := strings.TrimSpace(binDir); bin != "" {
		rel, err := filepath.Rel(filepath.Clean(bin), cleanExe)
		if err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func terminateOwnedExternalCore(port int, binDir, configuredPath string) {
	if port <= 0 {
		return
	}
	pid := findCoreListenerPID(port, configuredPath)
	if pid <= 0 || pid == os.Getpid() {
		return
	}
	if !isManagedCoreExecutablePath(processExePath(pid), binDir, configuredPath) {
		return
	}
	if proc, err := os.FindProcess(pid); err == nil && proc != nil {
		_ = proc.Kill()
	}
}
