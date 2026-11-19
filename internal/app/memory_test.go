package app

import (
	"os"
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
