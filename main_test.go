package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bihua-university/livery/internal/model"
)

var (
	e2eBuildOnce sync.Once
	e2eBinDir    string
	e2eBinPath   string
	e2eBuildErr  error
)

func TestMain(m *testing.M) {
	code := m.Run()
	if e2eBinDir != "" {
		_ = os.RemoveAll(e2eBinDir)
	}
	os.Exit(code)
}

func skipIfNoE2E(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}
	if runtime.GOOS == "linux" && os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
		t.Skip("skipping E2E test on headless Linux without DISPLAY or WAYLAND_DISPLAY")
	}
}

func ensureE2EBinary(t *testing.T) string {
	t.Helper()
	e2eBuildOnce.Do(func() {
		dir, err := os.MkdirTemp("", "livery-e2e-bin-*")
		if err != nil {
			e2eBuildErr = err
			return
		}
		e2eBinDir = dir
		name := "livery-e2e"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		outPath := filepath.Join(dir, name)
		cmd := exec.Command("go", "build", "-tags", "e2e", "-o", outPath, ".")
		if out, err := cmd.CombinedOutput(); err != nil {
			e2eBuildErr = fmt.Errorf("go build -tags e2e failed: %w\n%s", err, string(out))
			return
		}
		e2eBinPath = outPath
	})
	if e2eBuildErr != nil {
		t.Fatal(e2eBuildErr)
	}
	return e2eBinPath
}

func freeTCPPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("allocate free tcp port: %v", err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

type autoWindow struct {
	ID      int64  `json:"id"`
	Title   string `json:"title"`
	Kind    string `json:"kind"`
	Focused bool   `json:"focused"`
	Visible bool   `json:"visible"`
}

type autoNode struct {
	ID      string   `json:"id"`
	Parent  string   `json:"parent"`
	Role    string   `json:"role"`
	Key     any      `json:"key"`
	Label   string   `json:"label"`
	Value   string   `json:"value"`
	States  []string `json:"states"`
	Actions []string `json:"actions"`
}

func (n autoNode) keyString() string {
	if n.Key == nil {
		return ""
	}
	return fmt.Sprint(n.Key)
}

func (n autoNode) hasAction(act string) bool {
	return slices.Contains(n.Actions, act)
}

type e2eHarness struct {
	t       *testing.T
	dataDir string
	cmd     *exec.Cmd
	conn    net.Conn
	reader  *bufio.Reader
	nextID  atomic.Int64
	winID   int64
}

func launchE2EApp(t *testing.T, settings model.AppSettings) *e2eHarness {
	t.Helper()
	skipIfNoE2E(t)
	binPath := ensureE2EBinary(t)

	dataDir := t.TempDir()
	// Keep Unix socket path short for Windows AF_UNIX sun_path (108-byte limit).
	sockDir, err := os.MkdirTemp("", "liv-*")
	if err != nil {
		t.Fatalf("create socket temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(sockDir) })
	sockPath := filepath.Join(sockDir, "a.sock")

	settingsBytes, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		t.Fatalf("marshal settings: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "settings.json"), settingsBytes, 0o600); err != nil {
		t.Fatalf("write settings.json: %v", err)
	}

	cmd := exec.Command(binPath)
	cmd.Env = append(os.Environ(),
		"LIVERY_DATA_DIR="+dataDir,
		"MYGO_AUTOMATION="+sockPath,
	)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start livery e2e binary: %v", err)
	}

	h := &e2eHarness{
		t:       t,
		dataDir: dataDir,
		cmd:     cmd,
	}
	t.Cleanup(func() {
		if h.conn != nil {
			_ = h.conn.Close()
		}
		if h.cmd != nil && h.cmd.Process != nil {
			_ = h.cmd.Process.Kill()
			_, _ = h.cmd.Process.Wait()
		}
	})

	deadline := time.Now().Add(15 * time.Second)
	for {
		conn, err := net.Dial("unix", sockPath)
		if err == nil {
			h.conn = conn
			h.reader = bufio.NewReader(conn)
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for automation socket %s: %v", sockPath, err)
		}
		time.Sleep(50 * time.Millisecond)
	}

	for {
		wins := h.windows()
		for _, w := range wins {
			if w.Title == "Livery" && w.Kind == "content" {
				h.winID = w.ID
				return h
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for Livery window, saw %+v", wins)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func (h *e2eHarness) call(method string, params any) (json.RawMessage, error) {
	h.t.Helper()
	id := h.nextID.Add(1)
	req := map[string]any{
		"id":     id,
		"method": method,
	}
	if params != nil {
		req["params"] = params
	}
	line, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	_ = h.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	if _, err := h.conn.Write(append(line, '\n')); err != nil {
		return nil, err
	}
	_ = h.conn.SetReadDeadline(time.Now().Add(15 * time.Second))
	respLine, err := h.reader.ReadBytes('\n')
	if err != nil {
		return nil, err
	}
	var resp struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(respLine, &resp); err != nil {
		return nil, fmt.Errorf("decode %s response: %w (%s)", method, err, strings.TrimSpace(string(respLine)))
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("%s (%s): %s", method, resp.Error.Code, resp.Error.Message)
	}
	return resp.Result, nil
}

func (h *e2eHarness) mustCall(method string, params any) json.RawMessage {
	h.t.Helper()
	res, err := h.call(method, params)
	if err != nil {
		h.t.Fatalf("%v", err)
	}
	return res
}

func (h *e2eHarness) windows() []autoWindow {
	h.t.Helper()
	raw := h.mustCall("windows", nil)
	var res struct {
		Windows []autoWindow `json:"windows"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		h.t.Fatalf("decode windows: %v", err)
	}
	return res.Windows
}

func (h *e2eHarness) tree() []autoNode {
	h.t.Helper()
	raw := h.mustCall("tree", map[string]any{"window": h.winID})
	var res struct {
		Nodes []autoNode `json:"nodes"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		h.t.Fatalf("decode tree: %v", err)
	}
	return res.Nodes
}

func (h *e2eHarness) idle(timeoutMs int) {
	h.t.Helper()
	_, _ = h.call("idle", map[string]any{"window": h.winID, "timeout": timeoutMs})
}

func (h *e2eHarness) pressKey(key string) {
	h.t.Helper()
	h.mustCall("act", map[string]any{
		"window": h.winID,
		"key":    key,
		"action": "press",
	})
	h.idle(2000)
}

func (h *e2eHarness) setValueKey(key, text string) {
	h.t.Helper()
	h.mustCall("act", map[string]any{
		"window": h.winID,
		"key":    key,
		"action": "setValue",
		"text":   text,
	})
	h.idle(2000)
}

func (h *e2eHarness) pressLabel(label string) {
	h.t.Helper()
	nodes := h.waitUntil(5*time.Second, fmt.Sprintf("pressable label %q", label), func(nodes []autoNode) bool {
		_, ok := findPressableByLabel(nodes, label)
		return ok
	})
	node, _ := findPressableByLabel(nodes, label)
	_, _ = h.call("act", map[string]any{
		"window": h.winID,
		"id":     node.ID,
		"action": "scrollIntoView",
	})
	h.mustCall("act", map[string]any{
		"window": h.winID,
		"id":     node.ID,
		"action": "press",
	})
	h.idle(2000)
}

func (h *e2eHarness) waitUntil(timeout time.Duration, desc string, pred func([]autoNode) bool) []autoNode {
	h.t.Helper()
	deadline := time.Now().Add(timeout)
	var last []autoNode
	for {
		last = h.tree()
		if pred(last) {
			return last
		}
		if time.Now().After(deadline) {
			h.t.Fatalf("timed out waiting for %s; visible texts: %v", desc, collectTexts(last))
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func (h *e2eHarness) readSettings() model.AppSettings {
	h.t.Helper()
	b, err := os.ReadFile(filepath.Join(h.dataDir, "settings.json"))
	if err != nil {
		h.t.Fatalf("read settings.json: %v", err)
	}
	var s model.AppSettings
	if err := json.Unmarshal(b, &s); err != nil {
		h.t.Fatalf("unmarshal settings.json: %v", err)
	}
	return s
}

func hasNodeText(nodes []autoNode, substr string) bool {
	for _, n := range nodes {
		if strings.Contains(n.Label, substr) || strings.Contains(n.Value, substr) {
			return true
		}
	}
	return false
}

func findByKey(nodes []autoNode, key string) (autoNode, bool) {
	for _, n := range nodes {
		if n.keyString() == key {
			return n, true
		}
	}
	return autoNode{}, false
}

func findPressableByLabel(nodes []autoNode, label string) (autoNode, bool) {
	for _, n := range nodes {
		if n.Label == label && n.hasAction("press") {
			return n, true
		}
	}
	return autoNode{}, false
}

func collectTexts(nodes []autoNode) []string {
	var out []string
	for _, n := range nodes {
		if n.Label != "" {
			out = append(out, fmt.Sprintf("%s[%s]", n.Role, n.Label))
		} else if n.Value != "" {
			out = append(out, fmt.Sprintf("%s=%s", n.Role, n.Value))
		}
	}
	return out
}

func baseTestSettings(apiPort int) model.AppSettings {
	s := model.DefaultSettings()
	s.CoreType = model.CoreMihomo
	s.ClashAPIHost = "127.0.0.1"
	s.ClashAPIPort = apiPort
	s.AutoStartCore = false
	s.ProxyGuard = false
	s.SystemProxyEnabled = false
	s.TUNEnabled = false
	s.Language = model.LangZH
	s.ThemeMode = model.ThemeLight
	s.CloseAction = model.CloseQuit
	return s
}

type mockMihomo struct {
	server *httptest.Server
	port   int

	mu            sync.Mutex
	mode          string
	groupSelected map[string]string
}

func startMockMihomo(t *testing.T) *mockMihomo {
	t.Helper()
	m := &mockMihomo{
		mode: "rule",
		groupSelected: map[string]string{
			"选择节点":      "HK-01",
			"Streaming": "US-01",
			"GLOBAL":    "选择节点",
		},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/version", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"meta":    true,
			"version": "v1.19.34",
		})
	})
	mux.HandleFunc("/configs", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			var body struct {
				Mode string `json:"mode"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body.Mode != "" {
				m.mu.Lock()
				m.mode = body.Mode
				m.mu.Unlock()
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		m.mu.Lock()
		mode := m.mode
		m.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"mode": mode})
	})
	mux.HandleFunc("/proxies", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		selMain := m.groupSelected["选择节点"]
		selStream := m.groupSelected["Streaming"]
		selGlobal := m.groupSelected["GLOBAL"]
		m.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"proxies": map[string]any{
				"GLOBAL": map[string]any{
					"name": "GLOBAL",
					"type": "Selector",
					"now":  selGlobal,
					"all":  []string{"选择节点", "Streaming"},
				},
				"选择节点": map[string]any{
					"name": "选择节点",
					"type": "Selector",
					"now":  selMain,
					"all":  []string{"HK-01", "JP-01"},
				},
				"Streaming": map[string]any{
					"name": "Streaming",
					"type": "Selector",
					"now":  selStream,
					"all":  []string{"US-01", "SG-01"},
				},
				"HK-01": map[string]any{
					"name":    "HK-01",
					"type":    "VLESS",
					"udp":     true,
					"history": []map[string]any{{"delay": 45}},
				},
				"JP-01": map[string]any{
					"name":    "JP-01",
					"type":    "Trojan",
					"udp":     true,
					"history": []map[string]any{{"delay": 180}},
				},
				"US-01": map[string]any{
					"name":    "US-01",
					"type":    "VLESS",
					"udp":     false,
					"history": []map[string]any{{"delay": 210}},
				},
				"SG-01": map[string]any{
					"name":    "SG-01",
					"type":    "Hysteria2",
					"udp":     true,
					"history": []map[string]any{{"delay": 65}},
				},
			},
		})
	})
	mux.HandleFunc("/proxies/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		groupName, _ := url.PathUnescape(strings.TrimPrefix(r.URL.Path, "/proxies/"))
		var body struct {
			Name string `json:"name"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		m.mu.Lock()
		m.groupSelected[groupName] = body.Name
		m.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/connections", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Mimic real Mihomo: /connections returns memory: 0 while real memory arrives via /memory stream.
		_ = json.NewEncoder(w).Encode(map[string]any{
			"downloadTotal": 204800,
			"uploadTotal":   40960,
			"memory":        0,
			"connections": []map[string]any{
				{
					"id": "conn-1",
					"metadata": map[string]any{
						"network":         "tcp",
						"type":            "mixed",
						"sourceIP":        "127.0.0.1",
						"sourcePort":      "54321",
						"destinationIP":   "93.184.216.34",
						"destinationPort": "443",
						"host":            "example.com",
					},
					"upload":   512,
					"download": 2048,
					"start":    time.Now().Add(-10 * time.Second).Format(time.RFC3339),
					"chains":   []string{"HK-01", "选择节点"},
					"rule":     "DomainSuffix",
				},
			},
		})
	})
	mux.HandleFunc("/memory", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		flusher, _ := w.(http.Flusher)
		_, _ = w.Write([]byte("{\"inuse\":0,\"oslimit\":0}\n"))
		if flusher != nil {
			flusher.Flush()
		}
		select {
		case <-r.Context().Done():
			return
		case <-time.After(40 * time.Millisecond):
		}
		_, _ = w.Write([]byte("{\"inuse\":53420032,\"oslimit\":0}\n"))
		if flusher != nil {
			flusher.Flush()
		}
		<-r.Context().Done()
	})
	mux.HandleFunc("/traffic", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		flusher, _ := w.(http.Flusher)
		_, _ = w.Write([]byte("{\"up\":1024,\"down\":4096}\n"))
		if flusher != nil {
			flusher.Flush()
		}
		<-r.Context().Done()
	})
	mux.HandleFunc("/rules", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"rules": []map[string]any{
				{
					"type":    "DomainSuffix",
					"payload": "google.com",
					"proxy":   "选择节点",
				},
			},
		})
	})

	m.server = httptest.NewServer(mux)
	t.Cleanup(m.server.Close)

	u, err := url.Parse(m.server.URL)
	if err != nil {
		t.Fatalf("parse mock server url: %v", err)
	}
	m.port, _ = strconv.Atoi(u.Port())
	return m
}

func TestE2ENavigationStoppedStateAndQuit(t *testing.T) {
	unusedPort := freeTCPPort(t)
	h := launchE2EApp(t, baseTestSettings(unusedPort))

	h.waitUntil(5*time.Second, "overview stopped state", func(nodes []autoNode) bool {
		return hasNodeText(nodes, "Livery") &&
			hasNodeText(nodes, "未运行") &&
			hasNodeText(nodes, "实时流量趋势 (60s)")
	})

	pages := []struct {
		navKey     string
		expectText string
	}{
		{"nav-proxies", "Mihomo 核心未运行"},
		{"nav-profiles", "Default Profile"},
		{"nav-connections", "活跃连接"},
		{"nav-rules", "生效路由规则"},
		{"nav-logs", "Livery initialized"},
		{"nav-settings", "外观与桌面行为"},
	}

	for _, tc := range pages {
		h.pressKey(tc.navKey)
		h.waitUntil(5*time.Second, fmt.Sprintf("page %s contains %q", tc.navKey, tc.expectText), func(nodes []autoNode) bool {
			return hasNodeText(nodes, tc.expectText)
		})
	}

	if !hasNodeText(h.tree(), "应用更新") {
		t.Fatalf("expected OTA update card on settings page")
	}

	// Verify closing window in close_action=quit cleanly exits the process.
	_, _ = h.call("act", map[string]any{"window": h.winID, "key": "win-close", "action": "press"})
	done := make(chan error, 1)
	go func() { done <- h.cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expected clean app exit on close, got: %v", err)
		}
		h.cmd = nil // already waited
	case <-time.After(10 * time.Second):
		t.Fatalf("timed out waiting for app to exit after pressing win-close")
	}
}

func TestE2EMihomoAttachedMemoryProxiesRulesAndSettings(t *testing.T) {
	mock := startMockMihomo(t)
	h := launchE2EApp(t, baseTestSettings(mock.port))

	// 1. Overview attaches to external mock Mihomo and renders streamed /memory (50.95 MB) despite /connections memory=0.
	h.waitUntil(8*time.Second, "attached Mihomo overview with streamed core memory", func(nodes []autoNode) bool {
		return hasNodeText(nodes, "已接管") &&
			hasNodeText(nodes, "Mihomo v1.19.34") &&
			hasNodeText(nodes, "核心 50.95 MB")
	})

	// Switch proxy mode on Overview and verify PATCH /configs hits mock Mihomo.
	h.pressLabel("全局")
	deadline := time.Now().Add(5 * time.Second)
	for {
		mock.mu.Lock()
		mode := mock.mode
		mock.mu.Unlock()
		if mode == "global" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected mock Mihomo mode updated to global, got %q", mode)
		}
		time.Sleep(50 * time.Millisecond)
	}

	// 2. Proxies: group ordering, accordion expand/collapse, node selection, Collapse All.
	h.pressKey("nav-proxies")
	nodes := h.waitUntil(8*time.Second, "proxies groups and nodes loaded", func(nodes []autoNode) bool {
		_, hasHK := findByKey(nodes, "node-选择节点-HK-01")
		_, hasJP := findByKey(nodes, "node-选择节点-JP-01")
		_, hasMain := findByKey(nodes, "grp-toggle-选择节点")
		_, hasStream := findByKey(nodes, "grp-toggle-Streaming")
		return hasMain && hasStream && hasHK && hasJP
	})
	if _, hasSG := findByKey(nodes, "node-Streaming-SG-01"); hasSG {
		t.Fatalf("expected collapsed second group node SG-01 to be hidden initially")
	}

	h.pressKey("node-选择节点-JP-01")
	deadline = time.Now().Add(5 * time.Second)
	for {
		mock.mu.Lock()
		sel := mock.groupSelected["选择节点"]
		mock.mu.Unlock()
		if sel == "JP-01" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected 选择节点 selected node updated to JP-01, got %q", sel)
		}
		time.Sleep(50 * time.Millisecond)
	}

	h.pressKey("grp-toggle-Streaming")
	h.waitUntil(5*time.Second, "Streaming group expanded showing SG-01", func(nodes []autoNode) bool {
		_, ok := findByKey(nodes, "node-Streaming-SG-01")
		return ok
	})
	h.pressKey("node-Streaming-SG-01")
	deadline = time.Now().Add(5 * time.Second)
	for {
		mock.mu.Lock()
		sel := mock.groupSelected["Streaming"]
		mock.mu.Unlock()
		if sel == "SG-01" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected Streaming selected node updated to SG-01, got %q", sel)
		}
		time.Sleep(50 * time.Millisecond)
	}

	h.pressLabel("全部展开")
	h.waitUntil(5*time.Second, "all proxy groups expanded including GLOBAL", func(nodes []autoNode) bool {
		_, hasGlobalNode := findByKey(nodes, "node-GLOBAL-选择节点")
		return hasGlobalNode
	})

	h.pressLabel("全部折叠")
	h.waitUntil(5*time.Second, "all proxy groups collapsed", func(nodes []autoNode) bool {
		_, hasHK := findByKey(nodes, "node-选择节点-HK-01")
		_, hasSG := findByKey(nodes, "node-Streaming-SG-01")
		_, hasGlobalNode := findByKey(nodes, "node-GLOBAL-选择节点")
		_, hasMain := findByKey(nodes, "grp-toggle-选择节点")
		_, hasStream := findByKey(nodes, "grp-toggle-Streaming")
		return !hasHK && !hasSG && !hasGlobalNode && hasMain && hasStream
	})

	// 3. Connections page displays mocked active connection.
	h.pressKey("nav-connections")
	h.waitUntil(5*time.Second, "active connection rendered", func(nodes []autoNode) bool {
		return hasNodeText(nodes, "example.com:443")
	})

	// 4. Rules page displays mock Mihomo rules and persists a custom rule via modal.
	h.pressKey("nav-rules")
	h.waitUntil(5*time.Second, "mock Mihomo rule rendered", func(nodes []autoNode) bool {
		return hasNodeText(nodes, "google.com")
	})
	h.pressLabel("添加自定义规则")
	h.waitUntil(5*time.Second, "rule modal input visible", func(nodes []autoNode) bool {
		_, ok := findByKey(nodes, "rule-pat-in")
		return ok
	})
	h.setValueKey("rule-pat-in", "openai.com")
	h.pressLabel("添加并应用")
	h.waitUntil(5*time.Second, "custom rule rendered in rules page", func(nodes []autoNode) bool {
		return hasNodeText(nodes, "openai.com")
	})
	s := h.readSettings()
	if len(s.CustomRules) != 1 || s.CustomRules[0].Pattern != "openai.com" {
		t.Fatalf("expected custom rule openai.com persisted in settings.json, got %+v", s.CustomRules)
	}

	// 5. Settings page: switch language to English and theme to Dark, verify persistence.
	h.pressKey("nav-settings")
	h.pressLabel("English")
	h.waitUntil(5*time.Second, "English settings heading", func(nodes []autoNode) bool {
		return hasNodeText(nodes, "Appearance & Behavior")
	})
	h.pressLabel("Dark")
	deadline = time.Now().Add(5 * time.Second)
	for {
		s = h.readSettings()
		if s.Language == model.LangEN && s.ThemeMode == model.ThemeDark {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected language=en and theme_mode=dark persisted, got lang=%q theme=%q", s.Language, s.ThemeMode)
		}
		time.Sleep(50 * time.Millisecond)
	}

	// 6. Detach external core from header button.
	h.pressLabel("Detach")
	h.waitUntil(5*time.Second, "header action reverts to Start", func(nodes []autoNode) bool {
		_, ok := findPressableByLabel(nodes, "Start")
		return ok
	})
	h.pressKey("nav-overview")
	h.waitUntil(5*time.Second, "overview displays Stopped after detach", func(nodes []autoNode) bool {
		return hasNodeText(nodes, "Stopped")
	})
}
