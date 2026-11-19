package app

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSampleProcessPrivateMemory(t *testing.T) {
	if runtime.GOOS == "windows" || runtime.GOOS == "linux" || runtime.GOOS == "darwin" {
		if got := sampleProcessPrivateMemory(os.Getpid()); got <= 0 {
			t.Fatalf("expected positive private memory on %s, got %d", runtime.GOOS, got)
		}
	}
	if got := sampleAppMemory(); got <= 0 {
		t.Fatalf("expected positive sampleAppMemory, got %d", got)
	}
}

func TestIsManagedCoreExecutablePath(t *testing.T) {
	binDir := filepath.Join(t.TempDir(), "bin")
	mihomoInBin := filepath.Join(binDir, "mihomo.exe")
	singBoxInBin := filepath.Join(binDir, "sing-box.exe")
	otherMihomo := filepath.Join(t.TempDir(), "other", "mihomo.exe")
	unrelatedTest := filepath.Join(binDir, "main.test.exe")

	if !isManagedCoreExecutablePath(mihomoInBin, binDir, "") {
		t.Fatalf("expected %q inside binDir to be recognized as managed core", mihomoInBin)
	}
	if !isManagedCoreExecutablePath(singBoxInBin, binDir, "") {
		t.Fatalf("expected %q inside binDir to be recognized as managed core", singBoxInBin)
	}
	if isManagedCoreExecutablePath(otherMihomo, binDir, "") {
		t.Fatalf("expected %q outside binDir without configuredPath to not be managed", otherMihomo)
	}
	if !isManagedCoreExecutablePath(otherMihomo, binDir, otherMihomo) {
		t.Fatalf("expected explicitly configured %q to be recognized as managed core", otherMihomo)
	}
	if isManagedCoreExecutablePath(unrelatedTest, binDir, "") {
		t.Fatalf("expected non-core binary %q to be rejected", unrelatedTest)
	}
}
