//go:build !windows

package core

import (
	"errors"
	"os"
	"os/exec"

	"github.com/bihua-university/livery/internal/model"
)

func configureCommand(_ *exec.Cmd) {}

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
