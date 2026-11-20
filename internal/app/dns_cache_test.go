package app

import (
	"testing"
	"time"
)

func TestDNSNetworkHandoff(t *testing.T) {
	var w dnsNetworkWatch
	start := time.Now()
	for i, step := range []struct {
		network            string
		reconnected, flush bool
	}{
		{"wifi", false, false}, {"wifi", false, false}, {"", false, false}, {"", false, false},
		{"ethernet", true, false}, {"ethernet-with-dns", false, false}, {"ethernet-with-dns", false, true},
		{"ethernet-with-dns", false, false}, {"ethernet-with-dns", true, false}, {"ethernet-with-dns", false, true},
	} {
		if got := w.changed(start, step.network, step.reconnected); got != step.flush {
			t.Fatalf("sample %d: flush=%v, want %v", i, got, step.flush)
		}
	}
	if w.changed(start.Add(time.Minute), "wifi", true) {
		t.Fatal("new session flushed at startup")
	}
}
