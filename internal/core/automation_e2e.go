//go:build e2e

package core

import "os"

func init() {
	// Automated sessions attach to a mock controller, even when the host
	// has real core binaries available for version probes or startup.
	disableLocalBinaries = os.Getenv("MYGO_AUTOMATION") != ""
}
