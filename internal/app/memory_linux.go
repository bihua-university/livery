//go:build linux

package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func sampleProcessPrivateMemory(pid int) int64 {
	if pid <= 0 {
		return 0
	}
	path := fmt.Sprintf("/proc/%d/smaps_rollup", pid)
	if pid == os.Getpid() {
		path = "/proc/self/smaps_rollup"
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	var totalKiB int64
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "Private_Clean:") && !strings.HasPrefix(line, "Private_Dirty:") {
			continue
		}
		if fields := strings.Fields(line); len(fields) >= 2 {
			if val, err := strconv.ParseInt(fields[1], 10, 64); err == nil && val > 0 {
				totalKiB += val
			}
		}
	}
	return totalKiB * 1024
}

func findCoreListenerPID(port int, configuredPath string) int {
	wantPortHex := fmt.Sprintf(":%04X", port)
	inodes := make(map[string]struct{})
	for _, tcpFile := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		data, err := os.ReadFile(tcpFile)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n")[1:] {
			fields := strings.Fields(line)
			if len(fields) >= 10 && fields[3] == "0A" && strings.HasSuffix(strings.ToUpper(fields[1]), wantPortHex) && fields[9] != "0" {
				inodes["socket:["+fields[9]+"]"] = struct{}{}
			}
		}
	}
	if len(inodes) == 0 {
		return 0
	}

	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0
	}
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if !entry.IsDir() || err != nil || pid <= 0 {
			continue
		}
		comm, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "comm"))
		if err != nil || !isKnownCoreExecutable(string(comm), configuredPath) {
			continue
		}
		fdDir := filepath.Join("/proc", entry.Name(), "fd")
		fds, _ := os.ReadDir(fdDir)
		for _, fd := range fds {
			if target, err := os.Readlink(filepath.Join(fdDir, fd.Name())); err == nil {
				if _, ok := inodes[target]; ok {
					return pid
				}
			}
		}
	}
	return 0
}

func trimProcessMemory() {}

