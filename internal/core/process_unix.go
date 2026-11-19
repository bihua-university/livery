//go:build unix

package core

import (
	"errors"
	"os"
	"os/exec"
	"syscall"

	"github.com/bihua-university/livery/internal/model"
)

type processGuard struct{}

func attachProcessGuard(_ *exec.Cmd) *processGuard { return nil }
func (*processGuard) Terminate()                   {}
func (*processGuard) Close()                       {}

func configureCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
}

func terminateProcessGraceful(cmd *exec.Cmd) bool {
	if cmd == nil || cmd.Process == nil || cmd.Process.Pid <= 0 {
		return false
	}
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM); err == nil {
		return true
	}
	return cmd.Process.Signal(syscall.SIGTERM) == nil
}

func terminateProcessForce(cmd *exec.Cmd, _ *processGuard) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	if cmd.Process.Pid > 0 {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	_ = cmd.Process.Kill()
}

// IsElevated reports whether the current process runs as root.
func IsElevated() bool {
	return os.Geteuid() == 0
}

// RelaunchElevated is only implemented via UAC on Windows.
func RelaunchElevated() error {
	return errors.New("please restart Livery with administrator/root privileges for TUN mode")
}

func binaryNames(coreType model.CoreType) []string {
	if model.NormalizeCoreType(coreType) == model.CoreSingBox {
		return []string{"sing-box"}
	}
	return []string{"mihomo", "clash-meta"}
}
