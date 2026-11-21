package clashapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/bihua-university/livery/internal/model"
)

func newMockClient(handler http.Handler) (*Client, func()) {
	srv := httptest.NewServer(handler)
	host, portStr, _ := net.SplitHostPort(srv.Listener.Addr().String())
	port, _ := strconv.Atoi(portStr)
	return New(host, port, "secret123"), srv.Close
}

func TestDelayRequestsUseConfiguredTimeout(t *testing.T) {
	c, closeServer := newMockClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("timeout") != "12000" {
			t.Errorf("unexpected timeout: %s", r.URL.RawQuery)
		}
		fmt.Fprint(w, `{"delay": 12}`)
	}))
	defer closeServer()
	c.http.Timeout = time.Nanosecond
	if _, err := c.TestDelay(t.Context(), "node", "", 12000); err != nil {
		t.Fatal(err)
	}
	if _, err := c.TestGroupDelay(t.Context(), "group", "", 12000); err != nil {
		t.Fatal(err)
	}
}

func TestClientProxiesAndSelection(t *testing.T) {
	selected := "HK-01"
	mux := http.NewServeMux()
	mux.HandleFunc("/proxies", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret123" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"proxies": map[string]any{
				"Proxy": map[string]any{
					"name": "Proxy",
					"type": "Selector",
					"now":  selected,
					"all":  []string{"HK-01", "JP-01"},
				},
				"HK-01": map[string]any{
					"name":    "HK-01",
					"type":    "VLESS",
					"udp":     true,
					"history": []map[string]int{{"delay": 42}},
				},
				"JP-01": map[string]any{
					"name":    "JP-01",
					"type":    "Trojan",
					"udp":     true,
					"history": []map[string]int{{"delay": 88}},
				},
			},
		})
	})
	mux.HandleFunc("/proxies/Proxy", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			http.Error(w, "method", http.StatusMethodNotAllowed)
			return
		}
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		selected = body["name"]
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/proxies/HK-01/delay", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]int{"delay": 37})
	})

	c, closeFn := newMockClient(mux)
	defer closeFn()

	ctx := context.Background()
	groups, nodes, err := c.GetProxies(ctx)
	if err != nil {
		t.Fatalf("GetProxies: %v", err)
	}
	if len(groups) != 1 || groups[0].Now != "HK-01" {
		t.Fatalf("unexpected groups: %+v", groups)
	}
	if nodes["HK-01"].Delay != 42 {
		t.Fatalf("unexpected delay: %+v", nodes["HK-01"])
	}

	if err := c.SelectProxy(ctx, "Proxy", "JP-01"); err != nil {
		t.Fatalf("SelectProxy: %v", err)
	}
	groups, _, err = c.GetProxies(ctx)
	if err != nil || groups[0].Now != "JP-01" {
		t.Fatalf("expected JP-01 selected, got %+v (err=%v)", groups, err)
	}

	delay, err := c.TestDelay(ctx, "HK-01", "", 2000)
	if err != nil || delay != 37 {
		t.Fatalf("TestDelay: got %d, err=%v", delay, err)
	}
}

func TestProxyURLUsesControllerListener(t *testing.T) {
	for _, tc := range []struct{ config, want string }{
		{`{"mixed-port":8123,"port":8124}`, "http://127.0.0.1:8123"},
		{`{"mixed-port":0,"port":8124}`, "http://127.0.0.1:8124"},
		{`{"socks-port":8125}`, "socks5://127.0.0.1:8125"},
		{`{"mixed-port":0,"port":0,"socks-port":0}`, ""},
	} {
		t.Run(tc.config, func(t *testing.T) {
			client, closeServer := newMockClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, tc.config) }))
			defer closeServer()
			got, err := client.ProxyURL(context.Background(), model.CoreMihomo, 7890)
			if tc.want == "" {
				if err == nil {
					t.Fatalf("unexpected fallback: %s", got)
				}
			} else if err != nil || got.String() != tc.want {
				t.Fatalf("got %v, %v; want %s", got, err, tc.want)
			}
		})
	}
}

func TestClientModeAndRules(t *testing.T) {
	mode := model.ModeRule
	mux := http.NewServeMux()
	mux.HandleFunc("/configs", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			var b map[string]string
			_ = json.NewDecoder(r.Body).Decode(&b)
			mode = model.ProxyMode(b["mode"])
			w.WriteHeader(http.StatusNoContent)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"mode": string(mode)})
	})
	mux.HandleFunc("/rules", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"rules": []model.RuleEntry{
				{Type: "domain_suffix", Payload: "google.com", Proxy: "Proxy"},
			},
		})
	})

	c, closeFn := newMockClient(mux)
	defer closeFn()

	ctx := context.Background()
	if err := c.SetMode(ctx, model.ModeGlobal); err != nil {
		t.Fatalf("SetMode: %v", err)
	}
	got, err := c.GetMode(ctx)
	if err != nil || got != model.ModeGlobal {
		t.Fatalf("GetMode: got %q, err=%v", got, err)
	}
	rules, err := c.GetRules(ctx)
	if err != nil || len(rules) != 1 || rules[0].Payload != "google.com" {
		t.Fatalf("GetRules: got %+v, err=%v", rules, err)
	}
}

func TestLiveConfigAndCacheErrors(t *testing.T) {
	const payload = "dns:\n  enable: true\n  enhanced-mode: redir-host\n"
	port := 7890
	var patches []int
	var flushed []string
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /configs", func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Payload string }
		if json.NewDecoder(r.Body).Decode(&body) != nil || body.Payload != payload || r.URL.Query().Get("force") != "true" {
			t.Error("missing compiled configuration")
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /configs", func(w http.ResponseWriter, r *http.Request) { fmt.Fprintf(w, `{"mixed-port":%d}`, port) })
	mux.HandleFunc("PATCH /configs", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]int
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		port = body["mixed-port"]
		patches = append(patches, port)
		if port == 8888 {
			port = 0
		} // Failed bind still returns 204.
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /cache/{name}/flush", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		flushed = append(flushed, name)
		if name == "dns" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	client, closeServer := newMockClient(mux)
	defer closeServer()
	ctx := context.Background()
	if err := client.ReloadConfig(ctx, []byte(payload)); err != nil {
		t.Fatal(err)
	}
	if err := client.PatchInbound(ctx, "mixed-port", 8888); err == nil || !slices.Equal(patches, []int{8888, 7890}) {
		t.Fatalf("failed port change: err=%v, patches=%v", err, patches)
	}
	if err := client.FlushDNSCaches(ctx); err == nil || !slices.Equal(flushed, []string{"dns", "fakeip"}) {
		t.Fatalf("partial flush: err=%v, calls=%v", err, flushed)
	}
}

func TestClientUpdateGeo(t *testing.T) {
	called := false
	mux := http.NewServeMux()
	mux.HandleFunc("/configs/geo", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method", http.StatusMethodNotAllowed)
			return
		}
		if r.Header.Get("Authorization") != "Bearer secret123" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		called = true
		w.WriteHeader(http.StatusNoContent)
	})

	c, closeFn := newMockClient(mux)
	defer closeFn()

	if err := c.UpdateGeo(context.Background()); err != nil {
		t.Fatalf("UpdateGeo: %v", err)
	}
	if !called {
		t.Fatal("expected POST /configs/geo to be called")
	}
}

func TestClientStreamMemory(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/memory", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret123" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintln(w, `{"inuse":0,"oslimit":0}`)
		_, _ = fmt.Fprintln(w, `{"inuse":53420032,"oslimit":0}`)
		_, _ = fmt.Fprintln(w, `{"inuse":0,"oslimit":0,"footprint":12345678}`)
	})

	c, closeFn := newMockClient(mux)
	defer closeFn()

	var got []int64
	if err := c.StreamMemory(context.Background(), func(mem int64) {
		got = append(got, mem)
	}); err != nil {
		t.Fatalf("StreamMemory: %v", err)
	}
	if len(got) != 2 || got[0] != 53420032 || got[1] != 12345678 {
		t.Fatalf("unexpected memory samples: %v", got)
	}

	mux404 := http.NewServeMux()
	mux404.HandleFunc("/memory", http.NotFound)
	c404, close404 := newMockClient(mux404)
	defer close404()
	if err := c404.StreamMemory(context.Background(), func(int64) {}); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("expected ErrUnsupported on 404, got %v", err)
	}
}

func TestClientStreamsTrafficAndLogs(t *testing.T) {
	longMsg := strings.Repeat("a", 80*1024)

	mux := http.NewServeMux()
	mux.HandleFunc("/traffic", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, `{"up":123,"down":456}`)
		_, _ = fmt.Fprintln(w, ``)
		_, _ = fmt.Fprintln(w, `not-json`)
	})
	mux.HandleFunc("/logs", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("level") != "info" {
			t.Errorf("expected default level=info, got %q", r.URL.Query().Get("level"))
		}
		_, _ = fmt.Fprintln(w, `{"type":"INFO","payload":""}`)
		_, _ = fmt.Fprintf(w, `{"type":"WARNING","payload":%q}`+"\n", longMsg)
	})

	c, closeFn := newMockClient(mux)
	defer closeFn()

	var samples []model.TrafficSample
	if err := c.StreamTraffic(context.Background(), func(s model.TrafficSample) {
		samples = append(samples, s)
	}); err != nil {
		t.Fatalf("StreamTraffic: %v", err)
	}
	if len(samples) != 1 || samples[0].Up != 123 || samples[0].Down != 456 {
		t.Fatalf("unexpected traffic samples: %+v", samples)
	}

	var logs []model.LogEntry
	if err := c.StreamLogs(context.Background(), "", func(e model.LogEntry) {
		logs = append(logs, e)
	}); err != nil {
		t.Fatalf("StreamLogs: %v", err)
	}
	if len(logs) != 1 || logs[0].Level != "warning" || len(logs[0].Message) != len(longMsg) {
		t.Fatalf("unexpected logs: count=%d", len(logs))
	}
}

func TestGetProxiesGroupOrdering(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/proxies", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"proxies": map[string]any{
				"GLOBAL": map[string]any{"name": "GLOBAL", "type": "Selector", "all": []string{"Second", "First"}},
				"First":  map[string]any{"name": "First", "type": "URLTest", "all": []string{"n1"}},
				"Second": map[string]any{"name": "Second", "type": "Selector", "all": []string{"n1"}},
				"ExtraB": map[string]any{"name": "ExtraB", "type": "URLTest", "all": []string{"n1"}},
				"ExtraA": map[string]any{"name": "ExtraA", "type": "Selector", "all": []string{"n1"}},
				"ExtraC": map[string]any{"name": "ExtraC", "type": "URLTest", "all": []string{"n1"}},
			},
		})
	})

	c, closeFn := newMockClient(mux)
	defer closeFn()

	groups, _, err := c.GetProxies(context.Background())
	if err != nil {
		t.Fatalf("GetProxies: %v", err)
	}
	got := make([]string, len(groups))
	for i, g := range groups {
		got[i] = g.Name
	}
	want := []string{"Second", "First", "ExtraA", "ExtraB", "ExtraC", "GLOBAL"}
	if !slices.Equal(got, want) {
		t.Fatalf("group order = %v, want %v", got, want)
	}
}
