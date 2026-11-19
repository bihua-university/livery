package app

import (
	"sync"
	"testing"
	"time"

	"github.com/bihua-university/livery/internal/core"
	"github.com/bihua-university/livery/internal/profile"
)

func TestRulesAndLogsStrictOnDemandLoading(t *testing.T) {
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

	// 1. Both Rules and Logs must start completely unloaded.
	if a.IsRulesLoaded() || len(a.Rules) != 0 || a.RulesLoading {
		t.Fatalf("expected rules unloaded initially, got loaded=%v len=%d loading=%v", a.IsRulesLoaded(), len(a.Rules), a.RulesLoading)
	}
	if a.IsLogsLoaded() || len(a.Logs) != 0 || a.LogsLoading {
		t.Fatalf("expected logs unloaded initially, got loaded=%v len=%d loading=%v", a.IsLogsLoaded(), len(a.Logs), a.LogsLoading)
	}

	// 2. While on Overview, SyncRulesForView and SyncLogsForView must be no-ops.
	uiMu.Lock()
	a.Page = PageOverview
	a.SyncRulesForView()
	a.SyncLogsForView()
	rulesLoadingOnOverview := a.RulesLoading || a.IsRulesLoaded()
	logsLoadingOnOverview := a.LogsLoading || a.IsLogsLoaded()
	uiMu.Unlock()
	if rulesLoadingOnOverview {
		t.Fatal("expected SyncRulesForView to be a no-op when not on PageRules")
	}
	if logsLoadingOnOverview {
		t.Fatal("expected SyncLogsForView to be a no-op when not on PageLogs")
	}

	// 3. Switching to PageRules loads rules asynchronously and releases them on leave.
	uiMu.Lock()
	a.Page = PageRules
	a.SyncRulesForView()
	uiMu.Unlock()

	deadline := time.Now().Add(3 * time.Second)
	for {
		uiMu.Lock()
		done := a.IsRulesLoaded() && !a.RulesLoading
		count := len(a.Rules)
		uiMu.Unlock()
		if done {
			if count == 0 {
				t.Fatal("expected active profile rules to be loaded on PageRules")
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for async rules load")
		}
		time.Sleep(10 * time.Millisecond)
	}

	uiMu.Lock()
	a.Page = PageOverview
	a.ReleaseRulesView()
	releasedRules := !a.IsRulesLoaded() && !a.RulesLoading && a.Rules == nil
	uiMu.Unlock()
	if !releasedRules {
		t.Fatal("expected ReleaseRulesView to clear Rules and reset loaded/loading flags")
	}

	// 4. Switching to PageLogs loads logs asynchronously from LogStore and releases them on leave.
	uiMu.Lock()
	a.Page = PageLogs
	a.SyncLogsForView()
	uiMu.Unlock()

	deadline = time.Now().Add(3 * time.Second)
	for {
		uiMu.Lock()
		done := a.IsLogsLoaded() && !a.LogsLoading
		count := len(a.Logs)
		uiMu.Unlock()
		if done {
			if count == 0 {
				t.Fatal("expected persisted log entries to be loaded on PageLogs")
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for async logs load")
		}
		time.Sleep(10 * time.Millisecond)
	}

	uiMu.Lock()
	a.Page = PageOverview
	a.ReleaseLogsView()
	releasedLogs := !a.IsLogsLoaded() && !a.LogsLoading && a.Logs == nil
	uiMu.Unlock()
	if !releasedLogs {
		t.Fatal("expected ReleaseLogsView to clear Logs and reset loaded/loading flags")
	}
}

func TestStreamsPauseWhenHiddenAndResumeOnShow(t *testing.T) {
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
	a.SetMainRunner(func(fn func()) { fn() })

	// Core running while hidden must NOT start UI telemetry streams.
	a.SetWindowHidden(true)
	cm.AttachExternal("1.19.3", "127.0.0.1:19099")
	a.OnCoreState(cm.Info())
	a.bgMu.Lock()
	runningWhileHidden := a.streamCancel != nil
	a.bgMu.Unlock()
	if runningWhileHidden {
		t.Fatal("expected telemetry streams to stay stopped while window is hidden")
	}

	// Showing the window resumes streams.
	a.WindowHidden.Store(false)
	a.OnWindowShown()
	a.bgMu.Lock()
	runningWhenShown := a.streamCancel != nil
	a.bgMu.Unlock()
	if !runningWhenShown {
		t.Fatal("expected OnWindowShown to start telemetry streams when core is running")
	}

	// Closing the window stops streams immediately.
	a.OnWindowClosed()
	a.bgMu.Lock()
	runningAfterClose := a.streamCancel != nil
	a.bgMu.Unlock()
	if runningAfterClose {
		t.Fatal("expected OnWindowClosed to stop telemetry streams")
	}
}
