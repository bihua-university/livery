package pages

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/bihua-university/livery/internal/app"
	"github.com/bihua-university/livery/internal/core"
	"github.com/bihua-university/livery/internal/profile"
	"github.com/bihua-university/livery/internal/theme"
)

func TestSettingsSoftwareUpdatesSection(t *testing.T) {
	dir := t.TempDir()
	pm, err := profile.NewManager(dir)
	if err != nil {
		t.Fatalf("profile.NewManager: %v", err)
	}
	cm := core.NewManager(dir, dir, nil, nil)
	a := app.New(pm, cm)
	t.Cleanup(a.Shutdown)

	var checkedCount int
	var autoChecks, autoDownloads, prereleases bool
	a.SetAppUpdateHooks(app.AppUpdateHooks{
		State: func() app.AppUpdateState {
			return app.AppUpdateState{
				Supported:          true,
				AutomaticChecks:    autoChecks,
				AutomaticDownloads: autoDownloads,
				AvailableVersion:   "0.2.0",
				LastCheck:          time.Now().Add(-5 * time.Minute),
			}
		},
		CheckForUpdates: func() { checkedCount++ },
		SetAutomaticChecks: func(on bool) {
			autoChecks = on
		},
		SetAutomaticDownloads: func(on bool) {
			autoDownloads = on
		},
		SetPrereleases: func(on bool) {
			prereleases = on
		},
	})

	tester := ui.NewTester(func(c *ui.Context) {
		p := theme.Light()
		c.SetTheme(theme.BuildUITheme(p, a.SettingsData.UIFontSize))
		Settings(c, a, p, theme.Scale(14))
	}, 960, 900)

	// Scroll down to reveal the Software Updates section.
	tester.Scroll(480, 450, 0, 1200)
	tester.Scroll(480, 450, 0, 1200)

	if !tester.HasText("应用更新") {
		t.Fatalf("expected 应用更新 heading, texts: %v", tester.Texts())
	}
	if !tester.HasText("发现新版本 v0.2.0") {
		t.Fatalf("expected available version chip, texts: %v", tester.Texts())
	}

	if err := tester.Click("立即更新"); err != nil {
		t.Fatalf("Click(立即更新): %v", err)
	}
	if checkedCount != 1 {
		t.Fatalf("expected CheckForUpdates called once, got %d", checkedCount)
	}

	if err := tester.Click("自动检查更新 (24h)"); err != nil {
		t.Fatalf("Click(自动检查更新): %v", err)
	}
	if !autoChecks {
		t.Fatal("expected AutomaticChecks hook enabled after toggle")
	}

	if err := tester.Click("自动下载并安装更新"); err != nil {
		t.Fatalf("Click(自动下载并安装更新): %v", err)
	}
	if !autoDownloads {
		t.Fatal("expected AutomaticDownloads hook enabled after toggle")
	}

	if err := tester.Click("接收预发布版 (Beta)"); err != nil {
		t.Fatalf("Click(接收预发布版): %v", err)
	}
	if !prereleases || !a.SettingsData.UpdatePrereleases {
		t.Fatal("expected UpdatePrereleases persisted and hook called")
	}
}

func TestSettingsTUNBypassPrivateRouteCheckbox(t *testing.T) {
	dir := t.TempDir()
	pm, err := profile.NewManager(dir)
	if err != nil {
		t.Fatalf("profile.NewManager: %v", err)
	}
	cm := core.NewManager(dir, dir, nil, nil)
	a := app.New(pm, cm)
	t.Cleanup(a.Shutdown)

	if !a.SettingsData.TUNBypassPrivateRoute {
		t.Fatal("expected TUNBypassPrivateRoute default true")
	}

	tester := ui.NewTester(func(c *ui.Context) {
		p := theme.Light()
		c.SetTheme(theme.BuildUITheme(p, a.SettingsData.UIFontSize))
		Settings(c, a, p, theme.Scale(14))
	}, 960, 900)

	if !tester.HasText("绕过私有路由") {
		t.Fatalf("expected 绕过私有路由 checkbox, texts: %v", tester.Texts())
	}
	if err := tester.Click("绕过私有路由"); err != nil {
		t.Fatalf("Click(绕过私有路由): %v", err)
	}
	if a.SettingsData.TUNBypassPrivateRoute {
		t.Fatal("expected TUNBypassPrivateRoute to toggle false after click")
	}
}

