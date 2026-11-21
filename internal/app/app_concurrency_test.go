package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui/editor"

	"github.com/bihua-university/livery/internal/core"
	"github.com/bihua-university/livery/internal/model"
	"github.com/bihua-university/livery/internal/profile"
)

func TestActivePageConcurrentSyncAndRead(t *testing.T) {
	dir := t.TempDir()
	pm, err := profile.NewManager(dir)
	if err != nil {
		t.Fatalf("profile.NewManager: %v", err)
	}
	cm := core.NewManager(dir, dir, nil, nil)
	a := New(pm, cm)
	t.Cleanup(func() {
		a.Shutdown()
	})

	if got := a.currentPage(); got != PageOverview {
		t.Fatalf("expected initial currentPage=%q, got %q", PageOverview, got)
	}

	a.SetActivePage(PageLogs)
	if a.Page != PageLogs || a.currentPage() != PageLogs {
		t.Fatalf("expected Page and currentPage to be %q, got Page=%q currentPage=%q", PageLogs, a.Page, a.currentPage())
	}

	a.Page = PageProxies
	a.SyncActivePage()
	if got := a.currentPage(); got != PageProxies {
		t.Fatalf("expected currentPage=%q after SyncActivePage, got %q", PageProxies, got)
	}

	pages := []string{
		PageOverview,
		PageProxies,
		PageProfiles,
		PageConnections,
		PageRules,
		PageLogs,
		PageSettings,
	}
	valid := make(map[string]bool, len(pages))
	for _, p := range pages {
		valid[p] = true
	}

	var wg sync.WaitGroup
	stop := make(chan struct{})

	// Simulated UI thread mutating Page and mirroring via SetActivePage / SyncActivePage.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := range 2000 {
			p := pages[i%len(pages)]
			if i%2 == 0 {
				a.SetActivePage(p)
			} else {
				a.Page = p
				a.SyncActivePage()
			}
		}
		close(stop)
	}()

	// Concurrent background readers invoking currentPage and OnLog.
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					got := a.currentPage()
					if !valid[got] {
						t.Errorf("unexpected page %q", got)
						return
					}
					a.OnLog(model.LogEntry{
						Time:    time.Now(),
						Level:   "info",
						Source:  "test",
						Message: "concurrent log",
					})
				}
			}
		}()
	}

	wg.Wait()
}

func TestSaveEditorProfileAsync_InvalidContent(t *testing.T) {
	dir := t.TempDir()
	pm, err := profile.NewManager(dir)
	if err != nil {
		t.Fatalf("profile.NewManager: %v", err)
	}
	cm := core.NewManager(dir, dir, nil, nil)
	a := New(pm, cm)
	t.Cleanup(func() {
		a.Shutdown()
	})

	var uiMu sync.Mutex
	a.SetMainRunner(func(fn func()) {
		uiMu.Lock()
		defer uiMu.Unlock()
		fn()
	})

	prof, err := pm.CreateBlank("InvalidTest")
	if err != nil {
		t.Fatalf("CreateBlank: %v", err)
	}

	uiMu.Lock()
	a.Profiles = pm.Profiles()
	a.CoreInfo.Found = false
	a.OpenProfileEditor(prof.ID)
	a.EditorState = &editor.State{Buffer: editor.NewBuffer("{invalid json")}
	a.SaveEditorProfileAsync()
	savingStarted := a.SavingEditor
	uiMu.Unlock()

	if !savingStarted {
		t.Fatal("expected SavingEditor to be true immediately after SaveEditorProfileAsync")
	}

	deadline := time.Now().Add(3 * time.Second)
	for {
		uiMu.Lock()
		done := !a.SavingEditor
		editorErr := a.EditorError
		modalOpen := a.ShowEditorModal
		toasts := len(a.PendingToasts)
		uiMu.Unlock()

		if done {
			if editorErr == "" {
				t.Fatal("expected EditorError to be populated for invalid JSON content")
			}
			if !modalOpen {
				t.Fatal("expected editor modal to stay open on save error")
			}
			if toasts != 0 {
				t.Fatalf("expected no success toast on save error, got %d", toasts)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for SaveEditorProfileAsync error result")
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Read-only editor should be ignored by SaveEditorProfileAsync.
	uiMu.Lock()
	a.EditorReadOnly = true
	a.SaveEditorProfileAsync()
	readOnlySaving := a.SavingEditor
	uiMu.Unlock()
	if readOnlySaving {
		t.Fatal("expected SaveEditorProfileAsync to be a no-op for read-only editor")
	}
}

func TestSaveEditorProfileAsync_ValidSave(t *testing.T) {
	dir := t.TempDir()
	pm, err := profile.NewManager(dir)
	if err != nil {
		t.Fatalf("profile.NewManager: %v", err)
	}
	cm := core.NewManager(dir, dir, nil, nil)
	a := New(pm, cm)
	t.Cleanup(func() {
		a.Shutdown()
	})

	var uiMu sync.Mutex
	a.SetMainRunner(func(fn func()) {
		uiMu.Lock()
		defer uiMu.Unlock()
		fn()
	})

	prof, err := pm.CreateBlank("ValidTest")
	if err != nil {
		t.Fatalf("CreateBlank: %v", err)
	}

	wantYAML := "mixed-port: 7890\nmode: rule\nproxies: []\nproxy-groups: []\nrules:\n  - MATCH,DIRECT\n"

	uiMu.Lock()
	a.Profiles = pm.Profiles()
	a.CoreInfo.Found = false
	a.OpenProfileEditor(prof.ID)
	a.EditorState = &editor.State{Buffer: editor.NewBuffer(wantYAML)}
	a.SaveEditorProfileAsync()
	savingStarted := a.SavingEditor
	uiMu.Unlock()

	if !savingStarted {
		t.Fatal("expected SavingEditor to be true immediately after SaveEditorProfileAsync")
	}

	deadline := time.Now().Add(3 * time.Second)
	for {
		uiMu.Lock()
		done := !a.SavingEditor
		editorErr := a.EditorError
		modalOpen := a.ShowEditorModal
		toasts := append([]string(nil), a.PendingToasts...)
		uiMu.Unlock()

		if done {
			if editorErr != "" {
				t.Fatalf("expected no EditorError on valid save, got %q", editorErr)
			}
			if modalOpen {
				t.Fatal("expected editor modal to close on valid save")
			}
			if len(toasts) == 0 {
				t.Fatal("expected success toast notification on valid save")
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for SaveEditorProfileAsync valid save result")
		}
		time.Sleep(10 * time.Millisecond)
	}

	raw, err := pm.ReadProfileRaw(prof.ID)
	if err != nil {
		t.Fatalf("ReadProfileRaw: %v", err)
	}
	if strings.TrimSpace(string(raw)) != strings.TrimSpace(wantYAML) {
		t.Fatalf("saved profile content mismatch:\ngot:\n%s\nwant:\n%s", string(raw), wantYAML)
	}
}

func TestTryAttachCoreUsesManagerSettings(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/version" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"version":"1.19.3"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("url.Parse: %v", err)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatalf("strconv.Atoi: %v", err)
	}

	dir := t.TempDir()
	pm, err := profile.NewManager(dir)
	if err != nil {
		t.Fatalf("profile.NewManager: %v", err)
	}
	s := pm.Settings()
	s.ClashAPIHost = u.Hostname()
	s.ClashAPIPort = port
	if err := pm.SaveSettings(s); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}

	cm := core.NewManager(dir, dir, nil, nil)
	a := New(pm, cm)
	t.Cleanup(func() {
		a.Shutdown()
	})

	// Deliberately zero UI-thread SettingsData to verify TryAttachCore reads thread-safe PM.Settings().
	a.SettingsData.ClashAPIPort = 0
	a.SettingsData.ClashAPIHost = ""

	if !a.TryAttachCore(context.Background()) {
		t.Fatal("expected TryAttachCore to attach using PM.Settings()")
	}
	info := cm.Info()
	if info.Status != model.CoreRunning || !info.External || info.Version != "1.19.3" {
		t.Fatalf("unexpected core info after attach: %+v", info)
	}
}

func TestSortGroupsOrdering(t *testing.T) {
	groups := []model.ProxyGroup{
		{Name: "GLOBAL", Type: "Selector"},
		{Name: "Streaming", Type: "URLTest"},
		{Name: "Proxy", Type: "Selector"},
		{Name: "节点选择", Type: "Selector"},
	}

	sortGroups(groups, model.ModeRule)
	gotRule := []string{groups[0].Name, groups[1].Name, groups[2].Name, groups[3].Name}
	wantRule := []string{"节点选择", "Proxy", "Streaming", "GLOBAL"}
	for i := range wantRule {
		if gotRule[i] != wantRule[i] {
			t.Fatalf("ModeRule order mismatch: got %v, want %v", gotRule, wantRule)
		}
	}

	sortGroups(groups, model.ModeGlobal)
	gotGlobal := []string{groups[0].Name, groups[1].Name, groups[2].Name, groups[3].Name}
	wantGlobal := []string{"节点选择", "Proxy", "GLOBAL", "Streaming"}
	for i := range wantGlobal {
		if gotGlobal[i] != wantGlobal[i] {
			t.Fatalf("ModeGlobal order mismatch: got %v, want %v", gotGlobal, wantGlobal)
		}
	}
}
