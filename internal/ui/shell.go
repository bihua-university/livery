package appui

import (
	"fmt"
	"runtime"

	"github.com/egoist/mygo/ui"

	"github.com/bihua-university/livery/internal/app"
	"github.com/bihua-university/livery/internal/icons"
	"github.com/bihua-university/livery/internal/model"
	"github.com/bihua-university/livery/internal/theme"
	"github.com/bihua-university/livery/internal/ui/components"
	"github.com/bihua-university/livery/internal/ui/pages"
)

// View renders the main window shell (sidebar, inset rounded workspace, header with window controls, modals, toasts).
func View(c *ui.Context, a *app.App) {
	a.SyncActivePage()
	hidden := c.WindowHidden()
	wasHidden := a.WindowHidden.Swap(hidden)
	a.SetOverviewVisible(!hidden && a.Page == app.PageOverview)
	if hidden {
		a.ReleaseLogsView()
		a.ReleaseRulesView()
		a.ReleaseEditorState()
		if !wasHidden {
			a.ReleaseHiddenData()
		}
	} else {
		if wasHidden {
			a.OnWindowShown()
		}
		if a.Page != a.PrevPage {
			if a.PrevPage == app.PageConnections {
				a.ReleaseConnectionView()
			}
			a.PrevPage = a.Page
			if a.Page == app.PageOverview || a.Page == app.PageProxies || a.Page == app.PageConnections {
				a.RequestSnapshotSync()
			}
		}
		if a.Page != app.PageLogs {
			a.ReleaseLogsView()
		}
		if a.Page != app.PageRules {
			a.ReleaseRulesView()
		}
	}

	p := a.ActivePalette(c.Theme().Dark)
	c.SetTheme(theme.BuildUITheme(p, a.SettingsData.UIFontSize))
	s := theme.Scale(a.SettingsData.UIFontSize)

	if len(a.PendingToasts) > 0 {
		for _, msg := range a.PendingToasts {
			c.Toast(msg)
		}
		a.PendingToasts = nil
	}

	bar := c.TitleBar()
	c.Root().Background(p.Sidebar)

	maximized := false
	if win := a.Window(); win != nil {
		maximized = win.IsMaximized()
	}

	ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
		buildSidebar(c, a, p, s, bar)

		panel := ui.Column(c).
			Grow(1).
			MinWidth(0).
			MinHeight(0).
			Background(p.Background).
			Clip()
		if maximized {
			panel.Margin(0).Radius(0)
		} else {
			panel.Margin(4, 4, 4, 0).
				Radius(theme.RadiusXL).
				Border(1, p.Border)
		}
		panel.Children(func() {
			buildHeader(c, a, p, s, bar)
			ui.Box(c).Height(1).FillWidth().Background(p.Border)
			ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Children(func() {
				switch a.Page {
				case app.PageOverview:
					pages.Overview(c, a, p, s)
				case app.PageProxies:
					pages.Proxies(c, a, p, s)
				case app.PageProfiles:
					pages.Profiles(c, a, p, s)
				case app.PageConnections:
					pages.Connections(c, a, p, s)
				case app.PageRules:
					pages.Rules(c, a, p, s)
				case app.PageLogs:
					pages.Logs(c, a, p, s)
				case app.PageSettings:
					pages.Settings(c, a, p, s)
				}
			})
		})
	})

	pages.Modals(c, a, p, s)
	if !a.ShowEditorModal {
		a.ReleaseEditorState()
	}
}

func buildSidebar(c *ui.Context, a *app.App, p theme.Palette, s theme.TypeScale, bar ui.TitleBar) {
	topPad := max(bar.Bottom, 10)
	ui.Column(c).Width(124).Shrink(0).Background(p.Sidebar).Padding(topPad, 8, 10, 8).Gap(10).Children(func() {
		ui.Row(c).Padding(4, 4).Gap(8).AlignItems(ui.Center).DragWindow().Children(func() {
			ui.Box(c).Size(22, 22).Radius(theme.RadiusMD).Background(p.Primary).Center().Children(func() {
				ui.Icon(c, icons.Livery).FontSize(13).TextColor(p.PrimaryForeground)
			})
			ui.Text(c, "Livery").FontSize(s.Base).FontWeight(600).TextColor(p.Foreground).Grow(1).SingleLine()
		})

		navItems := []components.NavItem{
			{ID: app.PageOverview, Icon: icons.Overview, Label: a.Tr("概览", "Overview")},
			{ID: app.PageProxies, Icon: icons.Proxies, Label: a.Tr("代理", "Proxies")},
			{ID: app.PageProfiles, Icon: icons.Profiles, Label: a.Tr("配置", "Profiles"), Badge: fmt.Sprintf("%d", len(a.Profiles))},
			{ID: app.PageConnections, Icon: icons.Connections, Label: a.Tr("连接", "Connections"), Badge: fmt.Sprintf("%d", len(a.Conns))},
			{ID: app.PageRules, Icon: icons.Rules, Label: a.Tr("规则", "Rules")},
			{ID: app.PageLogs, Icon: icons.Logs, Label: a.Tr("日志", "Logs")},
			{ID: app.PageSettings, Icon: icons.Settings, Label: a.Tr("设置", "Settings")},
		}

		components.SlidingNav(c, p, s, navItems, &a.Page)
		a.SyncActivePage()
	})
}

func buildHeader(c *ui.Context, a *app.App, p theme.Palette, s theme.TypeScale, bar ui.TitleBar) {
	h := max(bar.Height-4, 42)
	padRight := max(bar.Right, 10)
	if runtime.GOOS != "darwin" {
		padRight = 8
	}
	ui.Row(c).
		Height(h).
		Padding(0, padRight, 0, 16).
		Gap(10).
		AlignItems(ui.Center).
		Background(p.Header).
		DragWindow().
		Children(func() {
			ui.Text(c, pageTitle(a)).FontSize(s.Base).FontWeight(600).TextColor(p.Foreground).SingleLine()
			ui.Spacer(c)

			ui.Row(c).
				Background(p.Surface).
				Border(1, p.Border).
				Radius(theme.RadiusLG).
				Padding(4, 10).
				Gap(10).
				AlignItems(ui.Center).
				Children(func() {
					ui.Icon(c, icons.ArrowDown).FontSize(12).TextColor(p.ChartDown)
					ui.Text(c, components.FormatRate(a.CurrentDown)).FontSize(s.SM).Font(theme.MonoFont()).FontFeatures("tnum").TextColor(p.Foreground).SingleLine()
					ui.Box(c).Width(1).Height(12).Background(p.Border)
					ui.Icon(c, icons.ArrowUp).FontSize(12).TextColor(p.ChartUp)
					ui.Text(c, components.FormatRate(a.CurrentUp)).FontSize(s.SM).Font(theme.MonoFont()).FontFeatures("tnum").TextColor(p.Foreground).SingleLine()
				})

			if a.SettingsData.SystemProxyEnabled {
				components.StatusChip(c, s, a.Tr("系统代理", "SysProxy"), p.SuccessSurface, p.SuccessSurfaceForeground, p.Success)
			}
			if a.SettingsData.TUNEnabled {
				components.StatusChip(c, s, "TUN", p.ProgressSurface, p.ProgressSurfaceForeground, p.IconBlue)
			}

			if a.CoreRestarting {
				components.ActionButton(c, p, s, icons.Refresh, a.Tr("重启中...", "Restarting...")).Disabled(true)
			} else if a.CoreStarting || a.CoreInfo.Status == model.CoreStarting {
				components.PrimaryActionButton(c, p, s, icons.Play, a.Tr("启动中...", "Starting...")).Disabled(true)
			} else if a.CoreInfo.Status == model.CoreRunning {
				if a.CoreInfo.External {
					if components.ActionButton(c, p, s, icons.Stop, a.Tr("断开", "Detach")).Clicked() {
						go a.StopCore()
					}
				} else {
					if components.ActionButton(c, p, s, icons.Refresh, a.Tr("重启", "Restart")).Clicked() {
						go a.RestartCore()
					}
					if components.ActionButton(c, p, s, icons.Stop, a.Tr("停止", "Stop")).Clicked() {
						go a.StopCore()
					}
				}
			} else {
				if components.PrimaryActionButton(c, p, s, icons.Play, a.Tr("启动", "Start")).Clicked() {
					go a.StartCore()
				}
			}

			if runtime.GOOS != "darwin" {
				ui.Box(c).Width(1).Height(14).Margin(0, 2).Background(p.Border)
				windowControls(c, a, p)
			}
		})
}

func windowControls(c *ui.Context, a *app.App, p theme.Palette) {
	ui.Row(c).Gap(2).AlignItems(ui.Center).Children(func() {
		minBtn := ui.ButtonBase(c.Key("win-min")).
			Size(28, 28).
			Center().
			Radius(theme.RadiusMD).
			Label(a.Tr("最小化", "Minimize")).
			Cursor(ui.CursorPointer)
		if minBtn.Hovered() {
			minBtn.Background(p.Hover)
		}
		minBtn.Children(func() {
			ui.Icon(c, icons.WinMin).FontSize(14).TextColor(p.ForegroundSubtle)
		})
		if minBtn.Clicked() {
			if win := a.Window(); win != nil {
				win.Minimize()
			}
		}

		maxBtn := ui.ButtonBase(c.Key("win-max")).
			Size(28, 28).
			Center().
			Radius(theme.RadiusMD).
			Label(a.Tr("最大化", "Maximize")).
			Cursor(ui.CursorPointer).
			MaximizeButton()
		if maxBtn.Hovered() {
			maxBtn.Background(p.Hover)
		}
		maxBtn.Children(func() {
			ui.Icon(c, icons.WinMax).FontSize(13).TextColor(p.ForegroundSubtle)
		})
		if maxBtn.Clicked() {
			if win := a.Window(); win != nil {
				if win.IsMaximized() {
					win.Unmaximize()
				} else {
					win.Maximize()
				}
			}
		}

		closeBtn := ui.ButtonBase(c.Key("win-close")).
			Size(28, 28).
			Center().
			Radius(theme.RadiusMD).
			Label(a.Tr("关闭", "Close")).
			Cursor(ui.CursorPointer)
		closeIconColor := p.ForegroundSubtle
		if closeBtn.Hovered() {
			closeBtn.Background(p.Destructive)
			closeIconColor = p.DestructiveForeground
		}
		closeBtn.Children(func() {
			ui.Icon(c, icons.WinClose).FontSize(14).TextColor(closeIconColor)
		})
		if closeBtn.Clicked() {
			if win := a.Window(); win != nil {
				win.Close()
			}
		}
	})
}

func pageTitle(a *app.App) string {
	switch a.Page {
	case app.PageOverview:
		return a.Tr("概览仪表盘", "Overview")
	case app.PageProxies:
		return a.Tr("代理节点", "Proxies")
	case app.PageProfiles:
		return a.Tr("配置订阅", "Profiles")
	case app.PageConnections:
		return a.Tr("实时连接", "Connections")
	case app.PageRules:
		return a.Tr("分流规则", "Rules")
	case app.PageLogs:
		return a.Tr("运行日志", "Logs")
	case app.PageSettings:
		return a.Tr("系统设置", "Settings")
	default:
		return "Livery"
	}
}
