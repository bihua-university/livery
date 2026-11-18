package main

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/updater"
	"github.com/egoist/mygo/plugins/updater/native"
	"github.com/egoist/mygo/ui"

	"github.com/bihua-university/livery/internal/app"
	"github.com/bihua-university/livery/internal/core"
	"github.com/bihua-university/livery/internal/icons"
	"github.com/bihua-university/livery/internal/model"
	"github.com/bihua-university/livery/internal/profile"
	appui "github.com/bihua-university/livery/internal/ui"
)

func main() {
	debug.SetGCPercent(25)

	automationMode := strings.TrimSpace(os.Getenv("MYGO_AUTOMATION")) != ""
	if !automationMode && !mygo.App.RequestSingleInstanceLock() {
		return
	}

	mygo.App.SetName("Livery")

	userDataDir := strings.TrimSpace(os.Getenv("LIVERY_DATA_DIR"))
	if userDataDir == "" {
		var err error
		userDataDir, err = mygo.App.Path(mygo.PathUserData)
		if err != nil {
			home, _ := os.UserHomeDir()
			userDataDir = filepath.Join(home, ".livery")
		}
	}

	pm, err := profile.NewManager(userDataDir)
	if err != nil {
		log.Fatalf("init profile manager: %v", err)
	}

	var controller *app.App
	cm := core.NewManager(pm.BinDir(), pm.DataDir(),
		func(info model.CoreInfo) {
			if controller != nil {
				controller.OnCoreState(info)
			}
		},
		func(entry model.LogEntry) {
			if controller != nil {
				controller.OnLog(entry)
			}
		},
	)
	controller = app.New(pm, cm)
	s0 := pm.Settings()
	cm.SetGeoDataDir(s0.GeoDataDir)
	cm.SetCoreType(s0.CoreType, s0.ActiveCorePath())

	if s0.Language == model.LangEN {
		mygo.App.SetLocale("en")
	} else {
		mygo.App.SetLocale("zh-Hans")
	}

	mygo.Use(native.Plugin)
	controller.SetAppUpdateHooks(app.AppUpdateHooks{
		State: func() app.AppUpdateState {
			return app.AppUpdateState{
				Supported:          mygo.Updater.Enabled(),
				AutomaticChecks:    updater.AutomaticChecks(),
				AutomaticDownloads: updater.AutomaticDownloads(),
				AvailableVersion:   updater.AvailableVersion(),
				LastCheck:          updater.LastCheck(),
			}
		},
		CheckForUpdates:       updater.CheckForUpdates,
		SetAutomaticChecks:    updater.SetAutomaticChecks,
		SetAutomaticDownloads: updater.SetAutomaticDownloads,
		SetPrereleases:        mygo.Updater.SetPrereleases,
	})
	updater.OnChange(func() {
		controller.RefreshAppUpdateState()
	})
	updater.OnInstalled(func() {
		controller.LogApp("info", "Application update installed; ready to relaunch")
	})

	switch controller.Settings().ThemeMode {
	case model.ThemeLight:
		mygo.Theme.SetSource(mygo.ThemeLight)
	case model.ThemeDark:
		mygo.Theme.SetSource(mygo.ThemeDark)
	default:
		mygo.Theme.SetSource(mygo.ThemeSystem)
	}

	renderView := func(c *ui.Context) {
		appui.View(c, controller)
	}

	var mainWin *mygo.Window
	var quitting bool

	showWindow := func() {
		if mainWin == nil || mainWin.IsDestroyed() {
			s := controller.Settings()
			mainWin = mygo.NewWindow(mygo.WindowOptions{
				Title:             "Livery",
				Width:             1120,
				Height:            740,
				MinWidth:          780,
				MinHeight:         540,
				BackgroundColor:   "light-dark(#ececee, #2b2b2b)",
				TitleBarStyle:     mygo.TitleBarHidden,
				TitleBarHeight:    44,
				HideWindowButtons: runtime.GOOS != "darwin",
				StateKey:          "main",
				Hidden:            s.SilentStart,
				Content:           ui.View(renderView),
			})
			controller.SetWindow(mainWin)
			mainWin.OnClose(func(e *mygo.CloseEvent) {
				if !quitting && controller.Settings().CloseAction == model.CloseToTray {
					e.PreventDefault()
					mainWin.Hide()
				}
			})
		}
		mainWin.Show()
		mainWin.Restore()
		mainWin.Focus()
	}

	mygo.App.OnSecondInstance(func(_ []string, _ string) {
		showWindow()
	})
	mygo.App.OnActivate(func(hasVisibleWindows bool) {
		if !hasVisibleWindows {
			showWindow()
		}
	})
	mygo.App.OnWindowAllClosed(func() {
		// Keep running in system tray when main window is hidden/closed.
		if automationMode {
			mygo.App.Quit()
		}
	})
	mygo.App.OnBeforeQuit(func(_ *mygo.QuitEvent) {
		quitting = true
		controller.Shutdown()
	})

	bgCtx, bgCancel := context.WithCancel(context.Background())
	defer bgCancel()

	mygo.App.WhenReady(func() {
		s := controller.Settings()
		if !s.SilentStart {
			showWindow()
		} else {
			mainWin = mygo.NewWindow(mygo.WindowOptions{
				Title:             "Livery",
				Width:             1120,
				Height:            740,
				MinWidth:          780,
				MinHeight:         540,
				BackgroundColor:   "light-dark(#ececee, #2b2b2b)",
				TitleBarStyle:     mygo.TitleBarHidden,
				TitleBarHeight:    44,
				HideWindowButtons: runtime.GOOS != "darwin",
				StateKey:          "main",
				Hidden:            true,
				Content:           ui.View(renderView),
			})
			controller.SetWindow(mainWin)
			mainWin.OnClose(func(e *mygo.CloseEvent) {
				if !quitting && controller.Settings().CloseAction == model.CloseToTray {
					e.PreventDefault()
					mainWin.Hide()
				}
			})
		}

		if !automationMode {
			tray, trayErr := setupTray(controller, showWindow)
			if trayErr != nil {
				log.Printf("tray setup: %v", trayErr)
			} else {
				controller.SetTrayRefresh(func() {
					refreshTrayMenu(tray, controller, showWindow)
				})
				tray.OnAppearanceChange(func() {
					refreshTrayMenu(tray, controller, showWindow)
				})
				tray.OnClick(func() {
					if mainWin != nil && mainWin.IsVisible() {
						mainWin.Focus()
					} else {
						showWindow()
					}
				})
			}
		}

		setAppMenu := func() {
			var items []*mygo.MenuItem
			if runtime.GOOS == "darwin" {
				items = append(items, &mygo.MenuItem{
					Label: "Livery",
					Submenu: []*mygo.MenuItem{
						{Role: mygo.RoleAbout},
						updater.MenuItem(),
						mygo.Separator(),
						{Role: mygo.RoleServices},
						mygo.Separator(),
						{Role: mygo.RoleHide},
						{Role: mygo.RoleHideOthers},
						{Role: mygo.RoleUnhide},
						mygo.Separator(),
						{Role: mygo.RoleQuit},
					},
				})
			} else {
				items = append(items, &mygo.MenuItem{Role: mygo.RoleAppMenu})
			}
			items = append(items,
				&mygo.MenuItem{Role: mygo.RoleEditMenu},
				&mygo.MenuItem{Role: mygo.RoleWindowMenu},
			)
			mygo.App.SetMenu(mygo.NewMenu(items))
		}
		setAppMenu()
		mygo.App.OnLocaleChanged(func(string) {
			setAppMenu()
			controller.RefreshTray()
		})

		if !automationMode {
			_ = mygo.GlobalShortcut.Register("CmdOrCtrl+Shift+M", func() {
				if mainWin != nil && mainWin.IsVisible() {
					mainWin.Hide()
				} else {
					showWindow()
				}
			})
		}

		controller.StartBackground(bgCtx)
	})

	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

func setupTray(controller *app.App, showWindow func()) (*mygo.Tray, error) {
	info := controller.Info()
	s := controller.Settings()
	tray, err := mygo.NewTray(mygo.TrayOptions{
		ID:             "livery-main",
		Icon:           icons.TrayPNG(info.Status == model.CoreRunning, s.SystemProxyEnabled, s.TUNEnabled, true),
		IconIsTemplate: false,
		ToolTip:        trayToolTip(s.CoreType),
		Menu:           buildTrayMenu(controller, showWindow),
	})
	if err != nil {
		return nil, err
	}
	refreshTrayMenu(tray, controller, showWindow)
	return tray, nil
}

func refreshTrayMenu(tray *mygo.Tray, controller *app.App, showWindow func()) {
	if tray == nil {
		return
	}
	info := controller.Info()
	s := controller.Settings()
	_ = tray.SetIcon(icons.TrayPNG(info.Status == model.CoreRunning, s.SystemProxyEnabled, s.TUNEnabled, tray.Dark()), false)
	tray.SetToolTip(trayToolTip(s.CoreType))
	tray.SetMenu(buildTrayMenu(controller, showWindow))
}

func trayToolTip(coreType model.CoreType) string {
	return "Livery - " + model.CoreDisplayName(coreType)
}

func buildTrayMenu(controller *app.App, showWindow func()) *mygo.Menu {
	s := controller.Settings()
	info := controller.Info()
	zh := s.Language != model.LangEN

	label := func(zhTxt, enTxt string) string {
		if zh {
			return zhTxt
		}
		return enTxt
	}

	coreName := model.CoreDisplayName(s.CoreType)
	statusLabel := label(coreName+": 已停止", coreName+": Stopped")
	switch {
	case controller.CoreRestarting:
		statusLabel = label(coreName+": 重启中...", coreName+": Restarting...")
	case controller.CoreStarting || info.Status == model.CoreStarting:
		statusLabel = label(coreName+": 启动中...", coreName+": Starting...")
	case info.Status == model.CoreRunning:
		statusLabel = label(coreName+": 运行中", coreName+": Running")
	}

	return mygo.NewMenu([]*mygo.MenuItem{
		{Label: label("打开 Livery", "Show Livery"), Click: func(*mygo.MenuItem, *mygo.Window) { showWindow() }},
		{Label: statusLabel, Disabled: true},
		mygo.Separator(),
		{
			Label:   label("Mihomo 核心", "Mihomo Core"),
			Type:    mygo.MenuItemRadio,
			Checked: model.NormalizeCoreType(s.CoreType) == model.CoreMihomo,
			Click:   func(*mygo.MenuItem, *mygo.Window) { controller.SwitchCoreType(model.CoreMihomo) },
		},
		{
			Label:   label("sing-box 核心", "sing-box Core"),
			Type:    mygo.MenuItemRadio,
			Checked: model.NormalizeCoreType(s.CoreType) == model.CoreSingBox,
			Click:   func(*mygo.MenuItem, *mygo.Window) { controller.SwitchCoreType(model.CoreSingBox) },
		},
		mygo.Separator(),
		{
			Label:   label("规则模式", "Rule Mode"),
			Type:    mygo.MenuItemRadio,
			Checked: s.ProxyMode == model.ModeRule,
			Click:   func(*mygo.MenuItem, *mygo.Window) { controller.SetProxyMode(model.ModeRule) },
		},
		{
			Label:   label("全局模式", "Global Mode"),
			Type:    mygo.MenuItemRadio,
			Checked: s.ProxyMode == model.ModeGlobal,
			Click:   func(*mygo.MenuItem, *mygo.Window) { controller.SetProxyMode(model.ModeGlobal) },
		},
		{
			Label:   label("直连模式", "Direct Mode"),
			Type:    mygo.MenuItemRadio,
			Checked: s.ProxyMode == model.ModeDirect,
			Click:   func(*mygo.MenuItem, *mygo.Window) { controller.SetProxyMode(model.ModeDirect) },
		},
		mygo.Separator(),
		{
			Label:   label("系统代理", "System Proxy"),
			Type:    mygo.MenuItemCheckbox,
			Checked: s.SystemProxyEnabled,
			Click:   func(it *mygo.MenuItem, _ *mygo.Window) { go controller.SetSystemProxy(it.IsChecked()) },
		},
		{
			Label:   label("TUN 模式", "TUN Mode"),
			Type:    mygo.MenuItemCheckbox,
			Checked: s.TUNEnabled,
			Click:   func(it *mygo.MenuItem, _ *mygo.Window) { controller.SetTUNMode(it.IsChecked()) },
		},
		mygo.Separator(),
		{
			Label:    label("重启核心", "Restart Core"),
			Disabled: controller.IsCoreBusy(),
			Click:    func(*mygo.MenuItem, *mygo.Window) { go controller.RestartCore() },
		},
		{
			Label: func() string {
				if v := strings.TrimPrefix(strings.TrimSpace(controller.AppUpdate.AvailableVersion), "v"); v != "" {
					return label("发现新版本 (v"+v+")", "Update Available (v"+v+")")
				}
				return label("检查更新...", "Check for Updates...")
			}(),
			Click: func(*mygo.MenuItem, *mygo.Window) { controller.CheckAppUpdates() },
		},
		{
			Label: label("退出", "Quit"),
			Role:  mygo.RoleQuit,
			Click: func(*mygo.MenuItem, *mygo.Window) {
				if runtime.GOOS != "darwin" {
					mygo.App.Quit()
				}
			},
		},
	})
}
