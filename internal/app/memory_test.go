package app

import (
	"runtime"
	"testing"
)

func TestSampleProcessRSSAndAppMemory(t *testing.T) {
	rss := sampleProcessRSS()
	switch runtime.GOOS {
	case "windows", "linux", "darwin":
		if rss <= 0 {
			t.Fatalf("expected positive process RSS on %s, got %d", runtime.GOOS, rss)
		}
		appMem := sampleAppMemory()
		if appMem != rss {
			// On live processes two consecutive samples can differ slightly by a few pages; verify it's in the RSS ballpark, not tiny heap.
			diff := appMem - rss
			if diff < 0 {
				diff = -diff
			}
			if diff > 16<<20 {
				t.Fatalf("expected sampleAppMemory (%d) to track process RSS (%d)", appMem, rss)
			}
		}
		t.Logf("GOOS=%s processRSS=%d runtimeMem=%d", runtime.GOOS, rss, sampleRuntimeMemory())
	default:
		if appMem := sampleAppMemory(); appMem <= 0 {
			t.Fatalf("expected positive fallback runtime memory, got %d", appMem)
		}
	}
}

func TestParseProcStatmRSS(t *testing.T) {
	if got := parseProcStatmRSS([]byte("312456 15432 4512 1024 0 12345 0\n"), 4096); got != 15432*4096 {
		t.Fatalf("unexpected statm RSS: got %d, want %d", got, 15432*4096)
	}
	if got := parseProcStatmRSS([]byte("invalid"), 4096); got != 0 {
		t.Fatalf("expected 0 for malformed statm, got %d", got)
	}
	if got := parseProcStatmRSS([]byte("100 0 0"), 4096); got != 0 {
		t.Fatalf("expected 0 for zero resident pages, got %d", got)
	}
	if got := parseProcStatmRSS([]byte("100 10 0"), 0); got != 0 {
		t.Fatalf("expected 0 for invalid page size, got %d", got)
	}
}

func TestParsePSRssKiB(t *testing.T) {
	if got := parsePSRssKiB([]byte("  65432 \n")); got != 65432*1024 {
		t.Fatalf("unexpected ps RSS: got %d, want %d", got, 65432*1024)
	}
	if got := parsePSRssKiB([]byte("")); got != 0 {
		t.Fatalf("expected 0 for empty ps output, got %d", got)
	}
	if got := parsePSRssKiB([]byte("0")); got != 0 {
		t.Fatalf("expected 0 for zero ps RSS, got %d", got)
	}
	if got := parsePSRssKiB([]byte("not-a-number")); got != 0 {
		t.Fatalf("expected 0 for non-numeric ps output, got %d", got)
	}
}
