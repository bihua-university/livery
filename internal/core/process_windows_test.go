//go:build windows

package core

import (
	"os"
	"os/exec"
	"runtime"
	"testing"
	"time"
	"unsafe"
)

func TestWindowsProcessGuardKillsChildOnJobClose(t *testing.T) {
	if runtime.GOARCH == "amd64" || runtime.GOARCH == "arm64" {
		if got := unsafe.Sizeof(jobObjectExtendedLimitInformation{}); got != 144 {
			t.Fatalf("expected JOBOBJECT_EXTENDED_LIMIT_INFORMATION size 144 on 64-bit Windows, got %d", got)
		}
	}

	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}

	cmd := exec.Command(exe, "run")
	cmd.Env = append(os.Environ(), "LIVERY_TEST_HELPER=1")
	configureCommand(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start helper child: %v", err)
	}
	defer func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	}()

	guard := attachProcessGuard(cmd)
	if guard == nil {
		t.Fatal("expected non-nil Windows Job Object processGuard")
	}

	waitCh := make(chan error, 1)
	go func() {
		waitCh <- cmd.Wait()
	}()

	// Closing the job handle without explicitly calling Process.Kill simulates
	// the OS closing Livery's handles on crash, Task Manager kill, or updater exit.
	guard.Close()

	select {
	case <-waitCh:
		// Child was immediately terminated by Windows kernel via JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE.
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for Windows Job Object to terminate child process on job close")
	}
}
