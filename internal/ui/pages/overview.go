package pages

import (
	"fmt"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/chart"

	"github.com/bihua-university/livery/internal/app"
	"github.com/bihua-university/livery/internal/core"
	"github.com/bihua-university/livery/internal/icons"
	"github.com/bihua-university/livery/internal/model"
	"github.com/bihua-university/livery/internal/theme"
	"github.com/bihua-university/livery/internal/ui/components"
)

// Overview renders the minimalist Bento dashboard filling the viewport without vertical scrolling.
func Overview(c *ui.Context, a *app.App, p theme.Palette, s theme.TypeScale) {
	ui.Column(c).Fill().Padding(14).Gap(10).MinHeight(0).Children(func() {
		overviewAlerts(c, a, p, s)

		// Row 1: Hero core status + Upload + Download (2fr : 1fr : 1fr)
		ui.Grid(c).Shrink(0).ColumnTracks(ui.Fr(2).Min(280), ui.Fr(1).Min(150), ui.Fr(1).Min(150)).Gap(10).Children(func() {
			overviewHeroCard(c, a, p, s)
			trafficMetricCard(c, a, p, s,
				a.Tr("上传", "Upload"), icons.ArrowUp, p.ChartUp,
				a.CurrentUp, a.PeakUp, a.TotalUp)
			trafficMetricCard(c, a, p, s,
				a.Tr("下载", "Download"), icons.ArrowDown, p.ChartDown,
				a.CurrentDown, a.PeakDown, a.TotalDown)
		})

		// Row 2: Proxy Mode + System Proxy + TUN Mode (aligned 2fr : 1fr : 1fr)
		overviewQuickControlsRow(c, a, p, s)

		// Row 3: Local IP & LAN + Active Profile + Memory Info
		overviewInfoTrio(c, a, p, s)

		// Row 4: Live Traffic Chart + Proxy Groups Summary (stretches to fill remaining height)
		overviewBottomRow(c, a, p, s)
	})
}

func overviewAlerts(c *ui.Context, a *app.App, p theme.Palette, s theme.TypeScale) {
	coreName := a.CoreDisplayName()
	if !a.CoreInfo.Found {
		msg := fmt.Sprintf(a.Tr("您可以一键从 GitHub 下载最新版 %s 核心，或在设置中指定本地路径。", "Download the latest %s release automatically or set a custom binary path in Settings."), coreName)
		if a.InstallStatus != "" {
			msg = a.InstallStatus
		}
		components.AlertBanner(c, p, s,
			fmt.Sprintf(a.Tr("未找到 %s 核心二进制文件", "%s core binary not found"), coreName),
			msg, p.WarningSurface, p.Warning.Alpha(0.35), p.WarningSurfaceForeground, false,
			func() {
				if components.PrimaryActionButton(c, p, s, icons.Download, a.Tr("下载核心", "Install Core")).Disabled(a.InstallingCore).Clicked() {
					a.StartInstallCore()
				}
				if components.ActionButton(c, p, s, icons.Settings, a.Tr("前往设置", "Settings")).Clicked() {
					a.Page = app.PageSettings
				}
			},
		)
	} else if a.CoreInfo.Status == model.CoreError && a.CoreInfo.LastError != "" {
		components.AlertBanner(c, p, s,
			fmt.Sprintf(a.Tr("%s 启动或运行出错", "%s core error"), coreName),
			a.CoreInfo.LastError, p.DestructiveSurface, p.Destructive.Alpha(0.35), p.DestructiveSurfaceFg, true,
			func() {
				if components.ActionButton(c, p, s, icons.Play, a.Tr("重试启动", "Retry Start")).Disabled(a.IsCoreBusy()).Clicked() {
					go a.StartCore()
				}
				if components.ActionButton(c, p, s, icons.Logs, a.Tr("查看日志", "Logs")).Clicked() {
					a.Page = app.PageLogs
				}
			},
		)
	}

	if !a.CoreInfo.IsElevated && a.SettingsData.TUNEnabled {
		components.AlertBanner(c, p, s,
			a.Tr("TUN 虚拟网卡需要更高权限", "TUN mode requires elevated privileges"),
			a.Tr("请以管理员身份重启应用以正常创建虚拟网卡。", "Relaunch as Administrator to create the TUN interface."),
			p.WarningSurface, p.Warning.Alpha(0.35), p.WarningSurfaceForeground, false,
			func() {
				if components.ActionButton(c, p, s, icons.Shield, a.Tr("以管理员身份重启", "Relaunch as Admin")).Clicked() {
					if err := core.RelaunchElevated(); err != nil {
						a.Notify(err.Error())
					}
				}
			},
		)
	}
}

func overviewHeroCard(c *ui.Context, a *app.App, p theme.Palette, s theme.TypeScale) {
	running := a.CoreInfo.Status == model.CoreRunning
	dot := p.ForegroundSubtlest
	bg, fg := p.Tag, p.ForegroundSubtle
	statusText := a.Tr("未运行", "Stopped")
	if a.CoreRestarting {
		dot, bg, fg = p.Warning, p.WarningSurface, p.WarningSurfaceForeground
		statusText = a.Tr("重启中", "Restarting")
	} else {
		switch a.CoreInfo.Status {
		case model.CoreRunning:
			dot, bg, fg = p.Success, p.SuccessSurface, p.SuccessSurfaceForeground
			if a.CoreInfo.External {
				statusText = a.Tr("已接管", "Attached")
			} else {
				statusText = a.Tr("运行中", "Running")
			}
		case model.CoreStarting:
			dot, bg, fg = p.Warning, p.WarningSurface, p.WarningSurfaceForeground
			statusText = a.Tr("启动中", "Starting")
		case model.CoreError:
			dot, bg, fg = p.Destructive, p.DestructiveSurface, p.DestructiveSurfaceFg
			statusText = a.Tr("异常", "Error")
		}
	}

	components.Card(c, p, func() {
		ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Text(c, a.Tr("状态", "Status")).FontSize(s.SM).TextColor(p.ForegroundSubtle)
			components.StatusChip(c, s, statusText, bg, fg, dot)
			ui.Spacer(c)
			coreLabel := a.CoreDisplayName()
			if a.CoreInfo.Version != "" {
				coreLabel += " v" + a.CoreInfo.Version
			}
			ui.Text(c, coreLabel).
				FontSize(s.XS).Font(theme.MonoFont()).TextColor(p.ForegroundSubtlest).SingleLine()
		})

		ui.Spacer(c)
		headline := a.ActivePrimaryNode()
		if !running || headline == "--" {
			if prof, ok := a.PM.ActiveProfile(); ok {
				headline = prof.Name
			} else {
				headline = "Livery"
			}
		}
		ui.Text(c, headline).FontSize(s.XL + 2).FontWeight(600).TextColor(p.Foreground).SingleLine()

		ui.Row(c).Gap(16).AlignItems(ui.Center).Children(func() {
			ui.Text(c, fmt.Sprintf("%s %s", a.Tr("运行时长", "Uptime"), components.FormatUptime(a.CoreStartedAt))).
				FontSize(s.XS).Font(theme.MonoFont()).FontFeatures("tnum").TextColor(p.ForegroundSubtle).SingleLine()
			ui.Text(c, fmt.Sprintf("%s %d", a.Tr("连接", "Conn"), len(a.Conns))).
				FontSize(s.XS).Font(theme.MonoFont()).FontFeatures("tnum").TextColor(p.ForegroundSubtle).SingleLine()
			ui.Text(c, fmt.Sprintf("127.0.0.1:%d", a.SettingsData.MixedPort)).
				FontSize(s.XS).Font(theme.MonoFont()).FontFeatures("tnum").TextColor(p.ForegroundSubtle).SingleLine()
		})
	}).Padding(12, 14).Gap(8)
}

func trafficMetricCard(c *ui.Context, a *app.App, p theme.Palette, s theme.TypeScale, title string, icon *ui.SVG, accent ui.Color, current, peak, total int64) {
	components.Card(c, p, func() {
		ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
			ui.Icon(c, icon).FontSize(13).TextColor(accent)
			ui.Text(c, title).FontSize(s.SM).TextColor(p.ForegroundSubtle).Grow(1).SingleLine()
			ui.Text(c, a.Tr("峰值 ", "Peak ")+components.FormatRate(peak)).
				FontSize(s.XS).Font(theme.MonoFont()).FontFeatures("tnum").TextColor(p.ForegroundSubtlest).SingleLine()
		})
		ui.Spacer(c)
		ui.Text(c, components.FormatRate(current)).
			FontSize(s.XL + 2).FontWeight(600).Font(theme.MonoFont()).FontFeatures("tnum").TextColor(p.Foreground).SingleLine()
		ui.Text(c, a.Tr("累计 ", "Total ")+components.FormatBytes(total)).
			FontSize(s.XS).Font(theme.MonoFont()).FontFeatures("tnum").TextColor(p.ForegroundSubtlest).SingleLine()
	}).Padding(12, 14).Gap(8)
}

func overviewQuickControlsRow(c *ui.Context, a *app.App, p theme.Palette, s theme.TypeScale) {
	ui.Grid(c).Shrink(0).ColumnTracks(ui.Fr(2).Min(280), ui.Fr(1).Min(150), ui.Fr(1).Min(150)).Gap(10).Children(func() {
		components.Card(c, p, func() {
			ui.Row(c).AlignItems(ui.Center).Gap(12).Children(func() {
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Icon(c, icons.Proxies).FontSize(14).TextColor(p.ForegroundSubtle)
					ui.Text(c, a.Tr("出站模式", "Proxy Mode")).FontSize(s.SM).FontWeight(500).TextColor(p.ForegroundSubtle).SingleLine()
				})

				modeIdx := 0
				switch a.SettingsData.ProxyMode {
				case model.ModeGlobal:
					modeIdx = 1
				case model.ModeDirect:
					modeIdx = 2
				}
				modes := []model.ProxyMode{model.ModeRule, model.ModeGlobal, model.ModeDirect}
				labels := []string{a.Tr("规则", "Rule"), a.Tr("全局", "Global"), a.Tr("直连", "Direct")}
				if components.Segmented(c.Key("ov-mode-seg"), p, s, &modeIdx, labels).FillEqually().Grow(1).Changed() {
					a.SetProxyMode(modes[modeIdx])
				}
			})
		}).Padding(12, 16)

		components.Card(c, p, func() {
			sysOn := a.SettingsData.SystemProxyEnabled
			sub := a.Tr("已关闭", "Disabled")
			if sysOn {
				sub = fmt.Sprintf(a.Tr("端口 %d", "Port %d"), a.SettingsData.MixedPort)
			}
			ui.Row(c).AlignItems(ui.Center).Gap(10).Children(func() {
				ui.Column(c).Grow(1).Gap(2).Children(func() {
					ui.Text(c, a.Tr("系统代理", "System Proxy")).FontSize(s.Base).FontWeight(600).TextColor(p.Foreground).SingleLine()
					ui.Text(c, sub).FontSize(s.XS).Font(theme.MonoFont()).TextColor(p.ForegroundSubtle).SingleLine()
				})
				if components.Toggle(c.Key("ov-sysproxy"), p, &sysOn).Label(a.Tr("系统代理", "System Proxy")).Changed() {
					go a.SetSystemProxy(sysOn)
				}
			})
		}).Padding(12, 16)

		components.Card(c, p, func() {
			tunOn := a.SettingsData.TUNEnabled
			sub := a.Tr("已关闭", "Disabled")
			if tunOn {
				sub = fmt.Sprintf("%s (%s)", a.SettingsData.TUNInterfaceName, a.SettingsData.TUNStack)
			}
			ui.Row(c).AlignItems(ui.Center).Gap(10).Children(func() {
				ui.Column(c).Grow(1).Gap(2).Children(func() {
					ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
						ui.Text(c, a.Tr("虚拟网卡", "TUN Mode")).FontSize(s.Base).FontWeight(600).TextColor(p.Foreground).SingleLine()
						if a.CoreInfo.IsElevated {
							components.TagChip(c, p, s, a.Tr("已提权", "Admin"))
						}
					})
					ui.Text(c, sub).FontSize(s.XS).Font(theme.MonoFont()).TextColor(p.ForegroundSubtle).SingleLine()
				})
				if components.Toggle(c.Key("ov-tun"), p, &tunOn).Label(a.Tr("虚拟网卡", "TUN Mode")).Changed() {
					a.SetTUNMode(tunOn)
				}
			})
		}).Padding(12, 16)
	})
}

func overviewInfoTrio(c *ui.Context, a *app.App, p theme.Palette, s theme.TypeScale) {
	ui.Grid(c).Shrink(0).Columns(3).Gap(10).Children(func() {
		// 1. Local IP & LAN Proxy
		components.Card(c, p, func() {
			components.SectionHeader(c, p, s, a.Tr("内网 IP", "Local IP"), func() {
				if components.ActionButton(c.Key("ov-check-quality"), p, s, icons.Zap, a.Tr("节点质量检测", "Check egress")).Disabled(a.CoreInfo.Status != model.CoreRunning || a.IsCoreBusy()).Clicked() {
					a.ShowQualityModal = true
					a.CheckQuality()
				}
				if components.IconActionButton(c, p, icons.Copy, a.Tr("复制内网 IP", "Copy Local IP")).Clicked() {
					mygo.Clipboard.WriteText(a.LocalIP)
					a.Notify(a.Tr("已复制 ", "Copied ") + a.LocalIP)
				}
			})
			ui.Text(c, a.LocalIP).FontSize(s.LG).FontWeight(600).Font(theme.MonoFont()).FontFeatures("tnum").TextColor(p.Foreground).SingleLine()
			ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
				lanOn := a.SettingsData.AllowLAN
				lanText := a.Tr("局域网代理 · 已关闭", "LAN Proxy · Off")
				if lanOn {
					lanText = a.Tr("局域网代理 · 0.0.0.0", "LAN Proxy · 0.0.0.0")
				}
				ui.Text(c, lanText).FontSize(s.XS).TextColor(p.ForegroundSubtle).Grow(1).SingleLine()
				if components.Toggle(c.Key("ov-allow-lan"), p, &lanOn).Label(a.Tr("局域网代理", "Allow LAN")).Changed() {
					a.SettingsData.AllowLAN = lanOn
					a.ApplySettings(true)
				}
			})
		}).Padding(12, 14).Gap(8)

		// 2. Active Profile
		components.Card(c, p, func() {
			prof, ok := a.PM.ActiveProfile()
			components.SectionHeader(c, p, s, a.Tr("当前配置", "Active Profile"), func() {
				ui.Row(c).Gap(6).Children(func() {
					if ok && prof.Kind == "remote" {
						if components.IconActionButton(c, p, icons.Refresh, a.Tr("更新订阅", "Update")).Clicked() {
							a.RefreshProfileAsync(prof.ID)
						}
					}
					if components.IconActionButton(c, p, icons.Profiles, a.Tr("管理配置", "Profiles")).Clicked() {
						a.Page = app.PageProfiles
					}
				})
			})
			if ok {
				ui.Text(c, prof.Name).FontSize(s.LG).FontWeight(600).TextColor(p.Foreground).SingleLine()
				if prof.Usage.Total > 0 {
					components.QuotaBar(c, p, s, prof.Usage, a.Tr("到期 ", "Expires "))
				} else {
					sub := a.Tr("本地配置文件", "Local profile")
					if prof.Kind == "remote" {
						sub = prof.URL
					}
					ui.Text(c, sub).FontSize(s.XS).TextColor(p.ForegroundSubtle).SingleLine()
				}
			} else {
				ui.Text(c, a.Tr("添加配置", "Add Profile")).FontSize(s.LG).FontWeight(600).TextColor(p.ForegroundSubtle)
			}
		}).Padding(12, 14).Gap(8)

		// 3. Memory Info
		components.Card(c, p, func() {
			components.SectionHeader(c, p, s, a.Tr("内存信息", "Memory"), func() {
				components.TagChip(c, p, s, fmt.Sprintf("API :%d", a.SettingsData.ClashAPIPort))
			})
			totalMem := a.AppMemoryBytes + a.MemoryBytes
			ui.Text(c, components.FormatBytes(totalMem)).
				FontSize(s.LG).FontWeight(600).Font(theme.MonoFont()).FontFeatures("tnum").TextColor(p.Foreground).SingleLine()

			ratio := 0.0
			if totalMem > 0 {
				ratio = float64(a.AppMemoryBytes) / float64(totalMem)
			}
			ui.Progress(c, ratio).FillWidth().Height(5)

			ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
				ui.Text(c, a.Tr("应用 ", "App ")+components.FormatBytes(a.AppMemoryBytes)).
					FontSize(s.XS).Font(theme.MonoFont()).FontFeatures("tnum").TextColor(p.ForegroundSubtle).SingleLine()
				ui.Text(c, a.Tr("核心 ", "Core ")+components.FormatBytes(a.MemoryBytes)).
					FontSize(s.XS).Font(theme.MonoFont()).FontFeatures("tnum").TextColor(p.ForegroundSubtle).SingleLine()
			})
		}).Padding(12, 14).Gap(8)
	})
}

func overviewBottomRow(c *ui.Context, a *app.App, p theme.Palette, s theme.TypeScale) {
	ui.Grid(c).Grow(1).MinHeight(0).ColumnTracks(ui.Fr(2).Min(320), ui.Fr(1).Min(240)).RowTracks(ui.Fr(1)).Gap(10).Children(func() {
		components.Card(c, p, func() {
			components.SectionHeader(c, p, s, a.Tr("实时流量趋势 (60s)", "Live Traffic (60s)"), func() {
				ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
					ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
						ui.Box(c).Size(8, 8).Radius(theme.RadiusFull).Background(p.ChartDown)
						ui.Text(c, a.Tr("下行", "Down")).FontSize(s.XS).TextColor(p.ForegroundSubtle)
					})
					ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
						ui.Box(c).Size(8, 8).Radius(theme.RadiusFull).Background(p.ChartUp)
						ui.Text(c, a.Tr("上行", "Up")).FontSize(s.XS).TextColor(p.ForegroundSubtle)
					})
				})
			})
			trafficChart(c, a, p, s)
		}).Fill().MinHeight(0).Padding(12, 14).Gap(8)

		components.Card(c, p, func() {
			components.SectionHeader(c, p, s, a.Tr("策略组", "Proxy Groups"), func() {
				if components.ActionButton(c, p, s, icons.Proxies, a.Tr("切换节点", "Proxies")).Clicked() {
					a.Page = app.PageProxies
				}
			})

			groups := a.OrderedGroups()
			if a.CoreInfo.Status != model.CoreRunning || len(groups) == 0 {
				components.EmptyState(c, p, s, icons.Layers,
					a.Tr("暂无策略组", "No proxy groups"),
					a.Tr("启动核心后显示当前策略组节点", "Start the core to view active groups"),
					nil)
				return
			}

			ui.Column(c).Grow(1).MinHeight(0).Gap(6).Clip().Children(func() {
				limit := min(len(groups), 6)
				for _, g := range groups[:limit] {
					grp := g
					row := ui.ButtonBase(c.Key("ov-grp-"+grp.Name)).
						Row().
						Padding(7, 10).
						Gap(8).
						Radius(theme.RadiusLG).
						Background(p.Surface).
						Border(1, p.Border).
						AlignItems(ui.Center).
						Label(grp.Name).
						Cursor(ui.CursorPointer)
					if row.Hovered() {
						row.Background(p.SurfaceHover).BorderColor(p.BorderHover)
					}
					row.OnClick(func() {
						a.SelectedGroup = grp.Name
						if a.ExpandedGroups == nil {
							a.ExpandedGroups = make(map[string]bool)
						}
						a.ExpandedGroups[grp.Name] = true
						a.Page = app.PageProxies
					})
					row.Children(func() {
						ui.Text(c, grp.Name).FontSize(s.SM).FontWeight(500).TextColor(p.Foreground).Grow(1).SingleLine()
						if grp.Now != "" {
							ui.Text(c, grp.Now).FontSize(s.XS).Font(theme.MonoFont()).TextColor(p.IconBlue).SingleLine()
						}
					})
				}
			})
		}).Fill().MinHeight(0).Padding(12, 14).Gap(8)
	})
}

func trafficChart(c *ui.Context, a *app.App, p theme.Palette, s theme.TypeScale) {
	n := len(a.TrafficHistory)
	if n < 2 {
		return
	}
	maxVal := 1024.0
	hasDown := false
	hasUp := false
	for _, sample := range a.TrafficHistory {
		if sample.Down > 0 {
			hasDown = true
			if float64(sample.Down) > maxVal {
				maxVal = float64(sample.Down)
			}
		}
		if sample.Up > 0 {
			hasUp = true
			if float64(sample.Up) > maxVal {
				maxVal = float64(sample.Up)
			}
		}
	}

	x := chart.NewLinear(0, float64(n-1))
	y := chart.NewLinear(0, maxVal*1.15).Nice(4)
	y.Format = func(v float64) string {
		if v <= 0 {
			return "0"
		}
		return components.FormatRate(int64(v))
	}

	yt := y.Ticks(4)
	if cap(a.ChartXs) < n {
		a.ChartXs = make([]float32, n)
		a.ChartYsDown = make([]float32, n)
		a.ChartYsUp = make([]float32, n)
	}
	xs := a.ChartXs[:n]
	ysDown := a.ChartYsDown[:n]
	ysUp := a.ChartYsUp[:n]
	for i, sample := range a.TrafficHistory {
		xs[i] = x.At(float64(i))
		ysDown[i] = y.At(float64(sample.Down))
		ysUp[i] = y.At(float64(sample.Up))
	}

	axisLabel := func(_ chart.Side, tick chart.Tick) {
		ui.Text(c, tick.Label).NoWrap().FontSize(s.XS).Font(theme.MonoFont()).TextColor(p.ForegroundSubtlest)
	}

	chart.Plot(c, chart.Axes{Left: yt, Label: axisLabel}, func(area ui.Element) {
		area.Draw(func(painter *ui.Painter, r ui.Rect) {
			for _, tick := range yt {
				yPos := chart.Y(r, tick.At)
				painter.Line(r.X, yPos, r.X+r.W, yPos, 1, p.Border)
			}
			// Render curves with GPU-native StrokePolyline + FillGradient strips instead of
			// CPU-rasterized FillPath/StrokePath so MyGo's MaskAtlas never grows on chart frames.
			a.ChartPtsDown = sampleMonotoneSeries(a.ChartPtsDown, &a.ChartSlopes, r, xs, ysDown, hasDown)
			if hasDown {
				fillChartAreaStrips(painter, r, a.ChartPtsDown, p.ChartDown, 0.16)
			}
			painter.StrokePolyline(a.ChartPtsDown, 2, opaqueColor(p.ChartDown))

			a.ChartPtsUp = sampleMonotoneSeries(a.ChartPtsUp, &a.ChartSlopes, r, xs, ysUp, hasUp)
			if hasUp {
				fillChartAreaStrips(painter, r, a.ChartPtsUp, p.ChartUp, 0.12)
			}
			painter.StrokePolyline(a.ChartPtsUp, 1.8, opaqueColor(p.ChartUp))
		})
	}).Label(a.Tr("流量图表", "Traffic chart")).Grow(1).MinHeight(100).GapX(8).GapY(4)
}

func opaqueColor(c ui.Color) ui.Color {
	c.A = 255
	return c
}

// sampleMonotoneSeries maps normalized (xs, ys) into pixel coordinates inside r using
// Steffen monotone cubic interpolation, reusing dst and slopesBuf to avoid per-frame allocations.
func sampleMonotoneSeries(dst [][2]float32, slopesBuf *[]float32, r ui.Rect, xs, ys []float32, nonZero bool) [][2]float32 {
	n := len(xs)
	if n < 2 || r.W <= 0 || r.H <= 0 {
		return dst[:0]
	}
	bottom := r.Y + r.H
	if !nonZero {
		dst = dst[:0]
		dst = append(dst,
			[2]float32{chart.X(r, xs[0]), bottom},
			[2]float32{chart.X(r, xs[n-1]), bottom},
		)
		return dst
	}

	if cap(*slopesBuf) < n {
		*slopesBuf = make([]float32, n)
	}
	m := (*slopesBuf)[:n]

	px := func(i int) float32 { return chart.X(r, xs[i]) }
	py := func(i int) float32 { return chart.Y(r, ys[i]) }

	if n >= 3 {
		for i := 1; i < n-1; i++ {
			m[i] = steffenSlope(px(i-1), py(i-1), px(i), py(i), px(i+1), py(i+1))
		}
		m[0] = steffenEndSlope(px(0), py(0), px(1), py(1), m[1])
		m[n-1] = steffenEndSlope(px(n-2), py(n-2), px(n-1), py(n-1), m[n-2])
	} else {
		for i := range m {
			m[i] = 0
		}
	}

	dst = dst[:0]
	dst = append(dst, [2]float32{px(0), py(0)})
	const subDivs = 3
	for i := 1; i < n; i++ {
		x0, y0 := px(i-1), py(i-1)
		x1, y1 := px(i), py(i)
		dx := x1 - x0
		if n < 3 || dx <= 2 || (y0 == y1 && m[i-1] == 0 && m[i] == 0) {
			dst = append(dst, [2]float32{x1, y1})
			continue
		}
		// Cubic Hermite evaluation equivalent to Steffen's Bezier control points.
		h := dx
		m0 := m[i-1] * h
		m1 := m[i] * h
		for s := 1; s < subDivs; s++ {
			t := float32(s) / float32(subDivs)
			t2 := t * t
			t3 := t2 * t
			yVal := (2*t3-3*t2+1)*y0 + (t3-2*t2+t)*m0 + (-2*t3+3*t2)*y1 + (t3-t2)*m1
			if yVal < r.Y {
				yVal = r.Y
			} else if yVal > bottom {
				yVal = bottom
			}
			dst = append(dst, [2]float32{x0 + dx*t, yVal})
		}
		dst = append(dst, [2]float32{x1, y1})
	}
	return dst
}

func steffenSlope(x0, y0, x1, y1, x2, y2 float32) float32 {
	h0, h1 := x1-x0, x2-x1
	if h0 == 0 || h1 == 0 {
		return 0
	}
	s0, s1 := (y1-y0)/h0, (y2-y1)/h1
	if (s0 > 0) != (s1 > 0) || s0 == 0 || s1 == 0 {
		return 0
	}
	p := (s0*h1 + s1*h0) / (h0 + h1)
	a0, a1, ap := abs32(s0), abs32(s1), 0.5*abs32(p)
	lim := a0
	if a1 < lim {
		lim = a1
	}
	if ap < lim {
		lim = ap
	}
	if s0 < 0 {
		return -2 * lim
	}
	return 2 * lim
}

func steffenEndSlope(x0, y0, x1, y1, t float32) float32 {
	h := x1 - x0
	if h == 0 {
		return t
	}
	return (3*(y1-y0)/h - t) / 2
}

func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

// fillChartAreaStrips renders a soft vertical gradient beneath non-zero curve segments using
// GPU-native FillGradient rectangles (zero MaskAtlas allocation).
func fillChartAreaStrips(painter *ui.Painter, r ui.Rect, pts [][2]float32, col ui.Color, topAlpha float32) {
	if len(pts) < 2 {
		return
	}
	bottom := r.Y + r.H
	grad := ui.LinearGradient{
		From:  col.Alpha(topAlpha),
		To:    col.Alpha(0),
		Angle: 180,
	}
	for i := 1; i < len(pts); i++ {
		x0, y0 := pts[i-1][0], pts[i-1][1]
		x1, y1 := pts[i][0], pts[i][1]
		w := x1 - x0
		if w <= 0 {
			continue
		}
		top := (y0 + y1) * 0.5
		h := bottom - top
		if h <= 0.5 {
			continue
		}
		painter.FillGradient(ui.Rect{X: x0, Y: top, W: w, H: h}, grad, 0)
	}
}

