package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/bihua-university/livery/internal/app"
	"github.com/bihua-university/livery/internal/core"
	"github.com/bihua-university/livery/internal/model"
	"github.com/bihua-university/livery/internal/profile"
	appui "github.com/bihua-university/livery/internal/ui"
)

func newTestApp(t *testing.T) (*app.App, func(*ui.Context)) {
	t.Helper()
	dir := t.TempDir()
	pm, err := profile.NewManager(dir)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	var a *app.App
	cm := core.NewManager(pm.BinDir(), pm.DataDir(), func(info model.CoreInfo) {
		if a != nil {
			a.OnCoreState(info)
		}
	}, nil)
	a = app.New(pm, cm)
	t.Cleanup(a.Shutdown)
	return a, func(c *ui.Context) {
		appui.View(c, a)
	}
}

func TestNavigationAndAllPagesRender(t *testing.T) {
	_, view := newTestApp(t)
	tt := ui.NewTester(view, 1120, 740)

	if !tt.HasText("Livery") {
		t.Fatalf("expected Livery brand in sidebar, texts: %v", tt.Texts())
	}
	if !tt.HasText("实时流量趋势 (60s)") {
		t.Fatalf("expected overview chart heading")
	}

	pages := []struct {
		navText    string
		expectText string
	}{
		{"代理", "sing-box 核心未运行"},
		{"配置", "Default Profile"},
		{"连接", "活跃连接"},
		{"规则", "生效路由规则"},
		{"日志", "Livery initialized"},
		{"设置", "外观与桌面行为"},
	}

	for _, tc := range pages {
		if err := tt.Click(tc.navText); err != nil {
			t.Fatalf("click nav %q: %v", tc.navText, err)
		}
		if !tt.HasText(tc.expectText) {
			t.Fatalf("after clicking %q expected %q, got texts: %v", tc.navText, tc.expectText, tt.Texts())
		}
	}
}

func TestThemeAndLanguageSwitching(t *testing.T) {
	a, view := newTestApp(t)
	tt := ui.NewTester(view, 1120, 740)

	if err := tt.Click("设置"); err != nil {
		t.Fatalf("click settings: %v", err)
	}
	tt.Scroll(500, 400, 0, 360)
	if err := tt.Click("English"); err != nil {
		t.Fatalf("switch language to English: %v", err)
	}
	if !tt.HasText("Appearance & Behavior") {
		t.Fatalf("expected English settings heading, got: %v", tt.Texts())
	}
	if a.Settings().Language != model.LangEN {
		t.Fatalf("expected LangEN persisted, got %q", a.Settings().Language)
	}

	if err := tt.Click("Dark"); err != nil {
		t.Fatalf("switch theme to Dark: %v", err)
	}
	if a.Settings().ThemeMode != model.ThemeDark {
		t.Fatalf("expected ThemeDark persisted, got %q", a.Settings().ThemeMode)
	}
}

func TestProxiesGroupAndNodeSelection(t *testing.T) {
	a, view := newTestApp(t)
	a.CoreInfo.Status = model.CoreRunning
	a.Groups = []model.ProxyGroup{
		{Name: "Proxy", Type: "Selector", Now: "HK-01", All: []string{"HK-01", "JP-01"}},
		{Name: "Streaming", Type: "Selector", Now: "US-01", All: []string{"US-01", "SG-01"}},
	}
	a.Nodes = map[string]model.ProxyNode{
		"HK-01": {Name: "HK-01", Type: "VLESS", Delay: 45, UDP: true},
		"JP-01": {Name: "JP-01", Type: "Trojan", Delay: 180, UDP: true},
		"US-01": {Name: "US-01", Type: "VLESS", Delay: 210, UDP: false},
		"SG-01": {Name: "SG-01", Type: "Hysteria2", Delay: 65, UDP: true},
	}
	a.SelectedGroup = "Proxy"

	tt := ui.NewTester(view, 1120, 740)
	if err := tt.Click("代理"); err != nil {
		t.Fatalf("click proxies: %v", err)
	}
	// Advance animation clock so the sliding sidebar indicator settles cleanly.
	tt.Advance(400 * 1e6)
	if !tt.HasText("HK-01") || !tt.HasText("45 ms") || !tt.HasText("180 ms") {
		t.Fatalf("expected proxy nodes and latencies, got: %v", tt.Texts())
	}
	if !tt.HasText("Streaming") {
		t.Fatalf("expected second proxy group card Streaming to be listed")
	}
	if tt.HasText("SG-01") {
		t.Fatalf("expected collapsed second group node SG-01 to be hidden initially")
	}

	if err := tt.Click("JP-01"); err != nil {
		t.Fatalf("select node JP-01: %v", err)
	}
	if a.Groups[0].Now != "JP-01" {
		t.Fatalf("expected active node updated to JP-01, got %q", a.Groups[0].Now)
	}

	// Expand the second accordion group and switch its active node.
	if err := tt.Click("Streaming"); err != nil {
		t.Fatalf("expand Streaming group: %v", err)
	}
	if !tt.HasText("SG-01") {
		t.Fatalf("expected SG-01 visible after expanding Streaming, got: %v", tt.Texts())
	}
	if err := tt.Click("SG-01"); err != nil {
		t.Fatalf("select node SG-01 in Streaming: %v", err)
	}
	if a.Groups[1].Now != "SG-01" {
		t.Fatalf("expected Streaming active node updated to SG-01, got %q", a.Groups[1].Now)
	}

	// All groups are now expanded, so toolbar offers Collapse All.
	if err := tt.Click("全部折叠"); err != nil {
		t.Fatalf("click collapse all: %v", err)
	}
	if a.IsGroupExpanded("Proxy") || a.IsGroupExpanded("Streaming") {
		t.Fatalf("expected all groups marked collapsed")
	}
	if tt.HasText("HK-01") || tt.HasText("US-01") {
		t.Fatalf("expected unselected body nodes hidden after Collapse All")
	}
	if !tt.HasText("JP-01") || !tt.HasText("SG-01") {
		t.Fatalf("expected active node names still shown in collapsed group headers")
	}
}

func TestProxiesPrimarySelectGroupAlwaysTopAndTwoColumns(t *testing.T) {
	a, view := newTestApp(t)
	a.CoreInfo.Status = model.CoreRunning
	a.SettingsData.ProxyMode = model.ModeGlobal
	a.Groups = []model.ProxyGroup{
		{Name: "GLOBAL", Type: "Selector", Now: "Auto", All: []string{"Auto", "选择节点"}},
		{Name: "Auto", Type: "URLTest", Now: "HK-01", All: []string{"HK-01", "JP-01"}},
		{Name: "选择节点", Type: "Selector", Now: "HK-01", All: []string{"HK-01", "JP-01", "SG-01", "US-01"}},
	}
	a.Nodes = map[string]model.ProxyNode{
		"HK-01": {Name: "HK-01", Type: "VLESS", Delay: 40},
		"JP-01": {Name: "JP-01", Type: "VLESS", Delay: 80},
		"SG-01": {Name: "SG-01", Type: "VLESS", Delay: 95},
		"US-01": {Name: "US-01", Type: "VLESS", Delay: 190},
	}

	tt := ui.NewTester(view, 1120, 740)
	if err := tt.Click("代理"); err != nil {
		t.Fatalf("click proxies: %v", err)
	}
	ordered := a.OrderedGroups()
	if len(ordered) == 0 || ordered[0].Name != "选择节点" {
		t.Fatalf("expected 选择节点 ordered first, got %+v", ordered)
	}
	bSelect, ok1 := tt.Find("选择节点")
	bGlobal, ok2 := tt.Find("GLOBAL")
	bAuto, ok3 := tt.Find("Auto")
	if !ok1 || !ok2 || !ok3 {
		t.Fatalf("expected all 3 groups visible")
	}
	if bSelect.Y > bGlobal.Y || bSelect.Y >= bAuto.Y {
		t.Fatalf("expected 选择节点 at top-left above Auto, got Y: select=%v global=%v auto=%v", bSelect.Y, bGlobal.Y, bAuto.Y)
	}
	// Verify 2-column layout inside expanded 选择节点:
	// Row 1: HK-01 (col 1), JP-01 (col 2); Row 2: SG-01 (col 1), US-01 (col 2).
	bJP, _ := tt.Find("JP-01")
	bSG, _ := tt.Find("SG-01")
	bUS, _ := tt.Find("US-01")
	if bSG.Y <= bJP.Y || bUS.X <= bSG.X {
		t.Fatalf("expected 2-column node grid (SG below JP, US right of SG), got JP=%v SG=%v US=%v", bJP, bSG, bUS)
	}
}

func TestOverviewSegmentedFillsEqually(t *testing.T) {
	_, view := newTestApp(t)
	tt := ui.NewTester(view, 1120, 740)

	bRule, ok1 := tt.Find("规则")
	bGlobal, ok2 := tt.Find("全局")
	bDirect, ok3 := tt.Find("直连")
	if !ok1 || !ok2 || !ok3 {
		t.Fatalf("expected all 3 proxy mode segments visible")
	}
	if bDirect.X-bRule.X < 120 {
		t.Fatalf("expected FillEqually segments to span card width, got rule=%v global=%v direct=%v", bRule, bGlobal, bDirect)
	}
	if err := tt.Click("全局"); err != nil {
		t.Fatalf("click global segment: %v", err)
	}
	tt.Advance(400 * 1e6)
}

func TestAddCustomRuleViaModal(t *testing.T) {
	a, view := newTestApp(t)
	tt := ui.NewTester(view, 1120, 740)

	if err := tt.Click("规则"); err != nil {
		t.Fatalf("click rules: %v", err)
	}
	if err := tt.Click("添加自定义规则"); err != nil {
		t.Fatalf("click add rule: %v", err)
	}
	tt.Type("openai.com")
	if err := tt.Click("添加并应用"); err != nil {
		t.Fatalf("submit rule: %v", err)
	}
	if !tt.HasText("openai.com") {
		t.Fatalf("expected openai.com rule rendered in list")
	}
	if len(a.Settings().CustomRules) != 1 {
		t.Fatalf("expected 1 custom rule saved, got %d", len(a.Settings().CustomRules))
	}
}

func TestConnectionsRowSelectionStyle(t *testing.T) {
	a, view := newTestApp(t)
	a.SettingsData.ThemeMode = model.ThemeDark
	a.Conns = []model.Connection{
		{
			ID: "c1",
			Metadata: model.ConnectionMetadata{
				Host:            "example.com",
				DestinationPort: "443",
				Network:         "tcp",
				Type:            "mixed",
			},
			Chains:   []string{"HK-01", "Proxy"},
			Rule:     "domain_suffix",
			Download: 2048,
			Upload:   512,
		},
	}

	tt := ui.NewTester(view, 1120, 740)
	if err := tt.Click("连接"); err != nil {
		t.Fatalf("click connections: %v", err)
	}
	if err := tt.Click("example.com:443"); err != nil {
		t.Fatalf("click connection row: %v", err)
	}
	if a.SelectedConnID != "c1" {
		t.Fatalf("expected SelectedConnID=c1, got %q", a.SelectedConnID)
	}
	pal := a.ActivePalette(true)
	wantRow := pal.Card.Mix(pal.Foreground, 0.11)
	if a.ConnTableState.CellsStyle.Row != wantRow {
		t.Fatalf("expected CellsStyle.Row=%+v, got %+v", wantRow, a.ConnTableState.CellsStyle.Row)
	}
}

func TestLogsReleasedFromMemoryOutsideLogsPage(t *testing.T) {
	a, view := newTestApp(t)
	defer a.Shutdown()

	tt := ui.NewTester(view, 1120, 740)
	if a.Logs != nil {
		t.Fatalf("expected a.Logs to be nil on overview page, got %d entries", len(a.Logs))
	}
	if err := tt.Click("日志"); err != nil {
		t.Fatalf("click logs: %v", err)
	}
	if len(a.Logs) == 0 {
		t.Fatalf("expected logs to load lazily on logs page")
	}
	if err := tt.Click("概览"); err != nil {
		t.Fatalf("click overview: %v", err)
	}
	if a.Logs != nil {
		t.Fatalf("expected a.Logs to be released (nil) after leaving logs page, got %d", len(a.Logs))
	}
}

func TestRulesReleasedFromMemoryOutsideRulesPage(t *testing.T) {
	a, view := newTestApp(t)

	tt := ui.NewTester(view, 1120, 740)
	if a.Rules != nil {
		t.Fatalf("expected a.Rules to be nil on overview page, got %d entries", len(a.Rules))
	}
	if err := tt.Click("规则"); err != nil {
		t.Fatalf("click rules: %v", err)
	}
	if len(a.Rules) == 0 {
		t.Fatalf("expected rules to load lazily on rules page")
	}
	if err := tt.Click("概览"); err != nil {
		t.Fatalf("click overview: %v", err)
	}
	if a.Rules != nil {
		t.Fatalf("expected a.Rules to be released (nil) after leaving rules page, got %d", len(a.Rules))
	}
}

func TestAttachExistingExternalCoreViaClashAPI(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/version":
			_ = json.NewEncoder(w).Encode(map[string]string{"version": "1.14.4"})
		case "/proxies":
			_ = json.NewEncoder(w).Encode(map[string]any{"proxies": map[string]any{}})
		case "/connections":
			_ = json.NewEncoder(w).Encode(map[string]any{"connections": []any{}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse server url: %v", err)
	}
	port, _ := strconv.Atoi(u.Port())

	a, view := newTestApp(t)
	a.SettingsData.ClashAPIHost = u.Hostname()
	a.SettingsData.ClashAPIPort = port

	if !a.TryAttachCore(context.Background()) {
		t.Fatalf("expected TryAttachCore to detect external Clash API")
	}
	info := a.Info()
	if info.Status != model.CoreRunning || !info.External || info.Version != "1.14.4" {
		t.Fatalf("expected external running core state, got %+v", info)
	}

	tt := ui.NewTester(view, 1120, 740)
	if !tt.HasText("已接管") || !tt.HasText("断开") {
		t.Fatalf("expected Attached status and Detach header action, texts: %v", tt.Texts())
	}

	a.StopCore()
	if a.Info().Status != model.CoreStopped || a.Info().External {
		t.Fatalf("expected StopCore to cleanly detach external core, got %+v", a.Info())
	}
}
