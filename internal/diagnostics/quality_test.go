package diagnostics

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return fn(r) }

func TestQualityCachesOnlyReputationForMatchingExit(t *testing.T) {
	dir := t.TempDir()
	var lookups, probes atomic.Int32
	ip := "203.0.113.19"
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body, status := "", http.StatusOK
		switch req.URL.Host {
		case "api64.ipify.org":
			body = ip
		case "api.ipquery.io":
			lookups.Add(1)
			body = `{"ip":"` + ip + `","risk":{"risk_score":0},"location":{"country":"Japan"}}`
		default:
			probes.Add(1)
			if req.URL.Host == "chatgpt.com" {
				status = http.StatusForbidden
			}
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	checker := New(dir)
	check := func() {
		t.Helper()
		report := checker.check(context.Background(), client)
		if report.IPError != nil || report.IP != ip || report.Quality.Risk.Score == nil || *report.Quality.Risk.Score != 0 || report.Quality.Location.Country != "Japan" {
			t.Fatalf("lookup: %+v", report)
		}
		if len(report.Services) != 4 || report.Services[3].Status != http.StatusForbidden {
			t.Fatalf("service results: %+v", report.Services)
		}
	}
	check()
	checker = New(dir)
	check()
	if lookups.Load() != 1 || probes.Load() != 8 {
		t.Fatalf("cached check: lookups=%d probes=%d", lookups.Load(), probes.Load())
	}
	entry := checker.entries[ip]
	entry.Expires = time.Now().Add(-time.Second)
	checker.entries[ip] = entry
	check()
	ip = "2001:db8::1"
	check()
	if lookups.Load() != 3 {
		t.Fatalf("expired or different exit reused cached risk: %d lookups", lookups.Load())
	}
}
