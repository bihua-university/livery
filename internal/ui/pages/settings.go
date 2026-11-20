package pages

import (
	"fmt"
	"reflect"
	"runtime"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/bihua-university/livery/internal/app"
	"github.com/bihua-university/livery/internal/core"
	"github.com/bihua-university/livery/internal/icons"
	"github.com/bihua-university/livery/internal/model"
	"github.com/bihua-university/livery/internal/sysproxy"
	"github.com/bihua-university/livery/internal/theme"
	"github.com/bihua-university/livery/internal/ui/components"
)

// Settings renders core, network, appearance, and data directory settings.
func Settings(c *ui.Context, a *app.App, p theme.Palette, s theme.TypeScale) {
	ui.Scroll(c.Key("settings-scroll")).TrackScroll(&a.SettingsScroll).Grow(1).Padding(20).Gap(16).Children(func() {
		ui.Column(c).FillWidth().Gap(16).Children(func() {
			ui.Text(c, a.Tr("设置", "Settings")).FontSize(s.SettingsTitle).FontWeight(600).TextColor(p.Foreground)

			// 1. Proxy Core (Mihomo / sing-box)
			components.Card(c, p, func() {
				coreType := model.NormalizeCoreType(a.SettingsData.CoreType)
				coreName := model.CoreDisplayName(coreType)
				components.SectionHeader(c, p, s, coreName+a.Tr(" 核心", " Core"), func() {
					if a.CoreInfo.Found {
						components.StatusChip(c, s, "v"+strings.TrimSpace(a.CoreInfo.Version), p.SuccessSurface, p.SuccessSurfaceForeground, p.Success)
					} else {
						components.StatusChip(c, s, a.Tr("未找到", "Not Found"), p.WarningSurface, p.WarningSurfaceForeground, p.Warning)
					}
				})

				ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
					ui.Text(c, a.Tr("核心模式", "Core Engine")).FontSize(s.Base).TextColor(p.Foreground).Grow(1)
					coreIdx := 0
					if coreType == model.CoreSingBox {
						coreIdx = 1
					}
					cores := []model.CoreType{model.CoreMihomo, model.CoreSingBox}
					if components.Segmented(c.Key("set-core-type-seg"), p, s, &coreIdx, []string{"Mihomo", "sing-box"}).Changed() {
						a.SwitchCoreType(cores[coreIdx])
					}
				})

				ui.Column(c).Gap(6).Children(func() {
					ui.Text(c, fmt.Sprintf(a.Tr("%s 核心路径或目录 (留空探测默认 bin/ 或 PATH；下载最新版将保存至此处)", "%s Binary Path / Dir (blank = default bin/ or PATH; downloads save here)"), coreName)).FontSize(s.SM).TextColor(p.ForegroundSubtle)
					pathPtr := &a.SettingsData.MihomoPath
					if coreType == model.CoreSingBox {
						pathPtr = &a.SettingsData.CorePath
					}
					ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
						if ui.TextInput(c.Key("set-core-path-"+string(coreType)), pathPtr).Placeholder(a.CoreInfo.BinaryPath).Grow(1).Submitted() {
							a.ApplySettings(false)
							a.CoreInfo = a.CM.RefreshBinaryInfo(*pathPtr)
						}
						if components.ActionButton(c, p, s, icons.Profiles, a.Tr("选文件", "File")).Clicked() {
							go pickCoreBinary(a)
						}
						if components.ActionButton(c, p, s, icons.Folder, a.Tr("选目录", "Folder")).Clicked() {
							go pickCoreDir(a)
						}
						if components.ActionButton(c, p, s, icons.Download, a.Tr("下载最新版", "Install Latest")).Disabled(a.InstallingCore).Clicked() {
							a.StartInstallCore()
						}
					})
					if a.InstallStatus != "" {
						ui.Text(c, a.InstallStatus).FontSize(s.XS).TextColor(p.IconBlue)
					}
				})

				ui.Row(c).Gap(12).Children(func() {
					if components.LabeledInput(c, p, s, "set-mixed-port", a.Tr("Mixed 入站端口 (HTTP/SOCKS5)", "Mixed Inbound Port"), "7890", &a.MixedPortDraft).Disabled(a.ApplyingInbound).Submitted() {
						commitNumericSettings(a)
					}
					if components.LabeledInput(c, p, s, "set-api-port", a.Tr("Clash API 端口", "Clash API Port"), "9090", &a.APIPortDraft).Submitted() {
						commitNumericSettings(a)
					}
					if components.LabeledInput(c, p, s, "set-api-secret", "Clash API Secret", a.Tr("可选", "Optional"), &a.SettingsData.ClashAPISecret).Submitted() {
						a.ApplySettings(true)
					}
				})

				ui.Row(c).Gap(12).Children(func() {
					ui.Column(c).Grow(2).Gap(4).Children(func() {
						ui.Text(c, a.Tr("延迟测速 URL", "Latency Test URL")).FontSize(s.SM).TextColor(p.ForegroundSubtle)
						if ui.TextInput(c.Key("set-test-url"), &a.SettingsData.DelayTestURL).FillWidth().Submitted() {
							a.ApplySettings(false)
						}
					})
					ui.Column(c).Grow(1).Gap(4).Children(func() {
						ui.Text(c, a.Tr("测速超时 (ms)", "Test timeout (ms)")).FontSize(s.SM).TextColor(p.ForegroundSubtle)
						ui.Row(c).Gap(4).Children(func() {
							submit := ui.TextInput(c.Key("delay-timeout"), &a.DelayTimeoutDraft).Grow(1).MinWidth(54).Submitted()
							save := components.IconActionButton(c.Key("save-delay-timeout"), p, icons.Check, a.Tr("保存测速超时", "Save test timeout")).Disabled(a.SavingPreference)
							if submit || save.Clicked() {
								ms, err := strconv.Atoi(strings.TrimSpace(a.DelayTimeoutDraft))
								if err != nil || ms < 1 || ms > 65535 {
									a.Notify(a.Tr("测速超时须为 1–65535 毫秒", "Enter a timeout from 1 to 65535 ms"))
									return
								}
								a.SavePreference(func(s *model.AppSettings) { s.DelayTestTimeoutMs = ms }, nil)
							}
						})
					})
					if components.LabeledSelect(c, p, s, "set-log-lvl", a.Tr("日志级别", "Log Level"), &a.SettingsData.LogLevel, []string{"trace", "debug", "info", "warn", "error"}).Changed() {
						a.ApplySettings(true)
					}
				})

				if ui.Checkbox(c.Key("set-close-connections-on-switch"), &a.SettingsData.CloseConnectionsOnSwitch,
					a.Tr("切换节点成功后断开所有活动连接", "Close all active connections after switching nodes")).Changed() {
					a.ApplySettings(false)
				}

				if prof, ok := a.PM.ActiveProfile(); coreType == model.CoreMihomo && ok {
					ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
						ui.Column(c).Grow(1).MinWidth(0).Children(func() {
							ui.Text(c, a.Tr("DNS 增强模式", "DNS enhanced mode")).FontSize(s.Base).TextColor(p.Foreground)
							ui.Text(c, prof.Name).SingleLine().FontSize(s.XS).TextColor(p.ForegroundSubtle)
						})
						modes := []string{"", "fake-ip", "redir-host"}
						idx := max(0, slices.Index(modes, prof.Override.DNSMode))
						labels := []string{a.Tr("跟随配置", "Use profile"), "fake-ip", "redir-host"}
						if components.Segmented(c.Key("set-dns-mode-"+prof.ID), p, s, &idx, labels).Disabled(a.SavingDNSMode || a.IsCoreBusy()).Changed() {
							a.SetDNSMode(prof.ID, modes[idx])
						}
					})
				}

				ui.Row(c).Gap(16).AlignItems(ui.Center).Children(func() {
					if ui.Checkbox(c.Key("set-allow-lan"), &a.SettingsData.AllowLAN, a.Tr("允许局域网连接 (0.0.0.0)", "Allow LAN (0.0.0.0)")).Disabled(a.ApplyingInbound).Changed() {
						a.ApplyInbound("allow-lan")
					}
					if ui.Checkbox(c.Key("set-autostart"), &a.SettingsData.AutoStartCore, a.Tr("启动应用时自动运行核心", "Auto-start core on launch")).Changed() {
						a.ApplySettings(false)
					}
					ui.Spacer(c)
					if components.PrimaryActionButton(c, p, s, icons.Check, a.Tr("应用端口与核心设置", "Apply Core Settings")).Disabled(a.ApplyingInbound).Clicked() {
						commitNumericSettings(a)
					}
				})
			})

			if model.NormalizeCoreType(a.SettingsData.CoreType) == model.CoreMihomo {
				components.Card(c, p, func() {
					components.SectionHeader(c, p, s, a.Tr("Mihomo 基础选项", "Mihomo options"), nil)
					boolLabels := []string{a.Tr("跟随配置", "Use profile"), a.Tr("开启", "On"), a.Tr("关闭", "Off")}
					boolRow := func(key, label string, value *bool, set func(*model.AppSettings, *bool)) {
						idx := 0
						if value != nil {
							idx = 2
							if *value {
								idx = 1
							}
						}
						ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
							ui.Text(c, label).FontSize(s.Base).TextColor(p.Foreground).Grow(1)
							if components.Segmented(c.Key(key), p, s, &idx, boolLabels).Disabled(a.SavingPreference || a.IsCoreBusy()).Changed() {
								var override *bool
								if idx > 0 {
									on := idx == 1
									override = &on
								}
								a.SavePreference(func(s *model.AppSettings) { set(s, override) }, a.ApplyProfileChanges)
							}
						})
					}
					boolRow("set-tcp-concurrent", a.Tr("TCP 并发连接", "Concurrent TCP connections"), a.SettingsData.TCPConcurrent, func(s *model.AppSettings, v *bool) { s.TCPConcurrent = v })
					boolRow("set-ipv6", "IPv6", a.SettingsData.IPv6, func(s *model.AppSettings, v *bool) { s.IPv6 = v })
					ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
						ui.Text(c, a.Tr("Geo 数据加载器", "Geo data loader")).FontSize(s.Base).TextColor(p.Foreground).Grow(1)
						values := []string{"", "standard", "memconservative"}
						idx := max(0, slices.Index(values, a.SettingsData.GeoDataLoader))
						labels := []string{boolLabels[0], a.Tr("标准", "Standard"), a.Tr("低内存", "Low memory")}
						if components.Segmented(c.Key("set-geodata-loader"), p, s, &idx, labels).Disabled(a.SavingPreference || a.IsCoreBusy()).Changed() {
							loader := values[idx]
							a.SavePreference(func(s *model.AppSettings) { s.GeoDataLoader = loader }, a.ApplyProfileChanges)
						}
					})
				})
				components.Card(c, p, func() {
					components.SectionHeader(c, p, s, a.Tr("DNS 缓存", "DNS cache"), func() {
						if components.ActionButton(c.Key("dns-cache-flush"), p, s, icons.Refresh, a.Tr("清理缓存", "Clear cache")).Disabled(a.CoreInfo.Status != model.CoreRunning || a.FlushingDNS).Clicked() {
							a.FlushDNSCaches()
						}
					})
					if ui.Checkbox(c.Key("dns-cache-auto"), &a.SettingsData.FlushDNSOnNetworkChange, a.Tr("切换网络后自动清理 DNS 和 Fake-IP 缓存", "Clear DNS and Fake-IP caches on network changes")).Changed() {
						a.ApplySettings(false)
					}
				})
			}

			// 2. System Proxy & TUN
			components.Card(c, p, func() {
				ui.Text(c, a.Tr("系统代理与 TUN 接管", "System Proxy & TUN")).FontSize(s.Base).FontWeight(600).TextColor(p.Foreground)

				if components.SwitchRow(c, p, s, "set-guard",
					a.Tr("系统代理守卫 (Proxy Guard)", "System Proxy Guard"),
					a.Tr("定期检查并防止系统代理被其他程序覆盖", "Periodically restore system proxy if altered by another app"),
					&a.SettingsData.ProxyGuard, nil).Changed() {
					a.ApplySettings(false)
					a.WakeProxyGuard()
				}

				if a.SettingsData.ProxyGuard {
					ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
						ui.Text(c, a.Tr("守卫检查间隔", "Guard Check Interval")).FontSize(s.SM).TextColor(p.ForegroundSubtle).Grow(1)
						intervals := []int{5, 10, 30, 60}
						labels := []string{"5s", "10s", "30s", "60s"}
						idx := 1
						for i, v := range intervals {
							if a.SettingsData.ProxyGuardInterval == v {
								idx = i
								break
							}
						}
						if components.Segmented(c.Key("set-guard-interval"), p, s, &idx, labels).Changed() {
							a.SettingsData.ProxyGuardInterval = intervals[idx]
							a.ApplySettings(false)
							a.WakeProxyGuard()
						}
					})
				}

				applyBypass := func() {
					a.SettingsData.SystemProxyBypass = sysproxy.NormalizeBypass(a.SettingsData.SystemProxyBypass)
					a.ApplySettings(false)
					if a.SettingsData.SystemProxyEnabled {
						go a.SetSystemProxy(true)
					} else {
						a.Notify(a.Tr("绕过列表已保存", "Bypass list saved"))
					}
				}

				ui.Column(c).Gap(6).Children(func() {
					ui.Text(c, a.Tr("系统代理绕过列表 (Bypass)", "System Proxy Bypass List")).FontSize(s.SM).TextColor(p.ForegroundSubtle)
					ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
						if ui.TextInput(c.Key("set-bypass"), &a.SettingsData.SystemProxyBypass).Grow(1).Submitted() {
							applyBypass()
						}
						if components.ActionButton(c, p, s, nil, a.Tr("恢复默认", "Reset")).Clicked() {
							a.SettingsData.SystemProxyBypass = sysproxy.DefaultBypass()
							applyBypass()
						}
						if components.ActionButton(c, p, s, icons.Check, a.Tr("应用", "Apply")).Clicked() {
							applyBypass()
						}
					})
				})

				ui.Divider(c)

				ui.Row(c).Gap(12).Children(func() {
					if components.LabeledSelect(c, p, s, "set-tun-stack", a.Tr("TUN 协议栈", "TUN Stack"), &a.SettingsData.TUNStack, []string{"mixed", "system", "gvisor"}).Changed() {
						a.ApplySettings(a.SettingsData.TUNEnabled)
					}
					if components.LabeledInput(c, p, s, "set-tun-iface", a.Tr("TUN 网卡名", "TUN Interface"), "sing-tun", &a.SettingsData.TUNInterfaceName).Submitted() {
						a.ApplySettings(a.SettingsData.TUNEnabled)
					}
					if components.LabeledInput(c, p, s, "set-tun-mtu", "TUN MTU", "9000", &a.TUNMTUDraft).Submitted() {
						commitNumericSettings(a)
					}
				})

				ui.Row(c).Gap(16).AlignItems(ui.Center).Children(func() {
					if ui.Checkbox(c.Key("set-tun-auto"), &a.SettingsData.TUNAutoRoute, "Auto Route").Changed() {
						a.ApplySettings(a.SettingsData.TUNEnabled)
					}
					if ui.Checkbox(c.Key("set-tun-strict"), &a.SettingsData.TUNStrictRoute, "Strict Route").Changed() {
						a.ApplySettings(a.SettingsData.TUNEnabled)
					}
					if ui.Checkbox(c.Key("set-tun-bypass-private"), &a.SettingsData.TUNBypassPrivateRoute, a.Tr("绕过私有路由", "Bypass Private Route")).Changed() {
						a.ApplySettings(a.SettingsData.TUNEnabled)
					}
					ui.Spacer(c)
					if runtime.GOOS == "windows" && !a.CoreInfo.IsElevated {
						if components.ActionButton(c, p, s, icons.Shield, a.Tr("以管理员运行", "Elevate (Admin)")).Clicked() {
							if err := core.RelaunchElevated(); err != nil {
								a.Notify(err.Error())
							}
						}
					}
				})
			})

			// 3. GeoIP & GeoSite Databases
			components.Card(c, p, func() {
				readyCount := 0
				for _, f := range a.GeoFiles {
					if f.Exists {
						readyCount++
					}
				}
				components.SectionHeader(c, p, s, a.Tr("GeoIP 与规则数据库", "GeoIP & GeoData"), func() {
					chipLabel := fmt.Sprintf("%d/%d %s", readyCount, len(a.GeoFiles), a.Tr("就绪", "Ready"))
					if readyCount > 0 {
						components.StatusChip(c, s, chipLabel, p.SuccessSurface, p.SuccessSurfaceForeground, p.Success)
					} else {
						components.StatusChip(c, s, a.Tr("未下载", "Missing"), p.WarningSurface, p.WarningSurfaceForeground, p.Warning)
					}
					btnLbl := a.Tr("立即更新 GeoIP", "Update GeoIP")
					if a.UpdatingGeo {
						btnLbl = a.Tr("更新中...", "Updating...")
					}
					if components.PrimaryActionButton(c, p, s, icons.Refresh, btnLbl).Disabled(a.UpdatingGeo).Clicked() {
						a.StartGeoUpdate()
					}
				})

				if a.GeoStatus != "" {
					ui.Text(c, a.GeoStatus).FontSize(s.XS).TextColor(p.IconBlue)
				}

				if len(a.GeoFiles) > 0 {
					ui.Grid(c).Columns(2).Gap(8).Children(func() {
						for _, f := range a.GeoFiles {
							ui.Row(c.Key("geo-file-"+f.Name)).
								Background(p.Surface).
								Border(1, p.Border).
								Radius(theme.RadiusLG).
								Padding(8, 10).
								Gap(8).
								AlignItems(ui.Center).
								Children(func() {
									ui.Text(c, f.Name).FontSize(s.SM).Font(theme.MonoFont()).FontWeight(500).TextColor(p.Foreground).Grow(1).SingleLine()
									if f.Exists {
										ui.Text(c, components.FormatBytes(f.Size)).FontSize(s.XS).Font(theme.MonoFont()).FontFeatures("tnum").TextColor(p.ForegroundSubtle).SingleLine()
										ui.Text(c, components.FormatRelativeTime(f.UpdatedAt, a.SettingsData.Language)).FontSize(s.XS).TextColor(p.ForegroundSubtlest).SingleLine()
									} else {
										ui.Text(c, a.Tr("未下载", "Not found")).FontSize(s.XS).TextColor(p.ForegroundSubtlest).SingleLine()
									}
								})
						}
					})
				}

				coreType := model.NormalizeCoreType(a.SettingsData.CoreType)
				ui.Column(c).Gap(6).Children(func() {
					ui.Text(c, a.Tr("Geo 数据目录 (留空使用默认配置目录，并同时兼容默认目录)", "GeoData Directory (blank = default data dir; compatible with default)")).FontSize(s.SM).TextColor(p.ForegroundSubtle)
					ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
						if ui.TextInput(c.Key("set-geo-dir"), &a.SettingsData.GeoDataDir).Placeholder(core.GeoDataDir(coreType, a.PM.DataDir())).Grow(1).Submitted() {
							a.ApplyGeoDataDir()
						}
						if components.ActionButton(c, p, s, icons.Folder, a.Tr("选目录", "Folder")).Clicked() {
							go pickGeoDataDir(a)
						}
						if components.ActionButton(c, p, s, icons.Check, a.Tr("应用", "Apply")).Clicked() {
							a.ApplyGeoDataDir()
						}
						if strings.TrimSpace(a.SettingsData.GeoDataDir) != "" {
							if components.ActionButton(c, p, s, nil, a.Tr("恢复默认", "Default")).Clicked() {
								a.SettingsData.GeoDataDir = ""
								a.ApplyGeoDataDir()
							}
						}
					})
				})

				if coreType == model.CoreMihomo {
					ui.Row(c).Gap(16).AlignItems(ui.Center).Children(func() {
						if ui.Checkbox(c.Key("set-geo-dat"), &a.SettingsData.GeoDataMode, a.Tr("启用 Geodata 模式 (.dat)", "Use Geodata (.dat) mode")).Changed() {
							a.ApplySettings(true)
						}
						if ui.Checkbox(c.Key("set-geo-auto"), &a.SettingsData.GeoAutoUpdate, a.Tr("自动更新 Geo 数据库 (24h)", "Auto-update Geo databases (24h)")).Changed() {
							a.ApplySettings(true)
						}
					})

					ui.Grid(c).Columns(2).Gap(10).Children(func() {
						if components.LabeledInput(c, p, s, "set-geo-ip-url", "GeoIP URL (geoip.dat)", model.DefaultGeoIPURL, &a.SettingsData.GeoIPURL).Submitted() {
							a.ApplySettings(true)
						}
						if components.LabeledInput(c, p, s, "set-geo-site-url", "GeoSite URL (geosite.dat)", model.DefaultGeoSiteURL, &a.SettingsData.GeoSiteURL).Submitted() {
							a.ApplySettings(true)
						}
						if components.LabeledInput(c, p, s, "set-geo-mmdb-url", "MMDB URL (geoip.metadb)", model.DefaultMMDBURL, &a.SettingsData.MMDBURL).Submitted() {
							a.ApplySettings(true)
						}
						if components.LabeledInput(c, p, s, "set-geo-asn-url", "ASN URL (GeoLite2-ASN.mmdb)", model.DefaultASNURL, &a.SettingsData.ASNURL).Submitted() {
							a.ApplySettings(true)
						}
					})
				}

				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					if components.ActionButton(c, p, s, icons.Folder, a.Tr("打开 Geo 目录", "Open Geo Folder")).Clicked() {
						app.OpenFolderInOS(core.EffectiveGeoDataDir(a.SettingsData, a.PM.DataDir()))
					}
					if components.ActionButton(c, p, s, icons.Refresh, a.Tr("刷新状态", "Refresh Status")).Clicked() {
						a.RefreshGeoStatus()
					}
					if coreType == model.CoreMihomo {
						if components.ActionButton(c, p, s, nil, a.Tr("恢复默认源", "Reset URLs")).Clicked() {
							a.ResetGeoURLs()
						}
						ui.Spacer(c)
						if components.ActionButton(c, p, s, icons.Check, a.Tr("应用 Geo 设置", "Apply Geo Settings")).Clicked() {
							a.SettingsData.GeoDataDir = strings.TrimSpace(a.SettingsData.GeoDataDir)
							a.CM.SetGeoDataDir(a.SettingsData.GeoDataDir)
							a.RefreshGeoStatus()
							a.ApplySettings(true)
							a.Notify(a.Tr("Geo 设置已保存", "Geo settings saved"))
						}
					}
				})
			})

			// 4. Appearance & Desktop Behavior
			components.Card(c, p, func() {
				ui.Text(c, a.Tr("外观与桌面行为", "Appearance & Behavior")).FontSize(s.Base).FontWeight(600).TextColor(p.Foreground)

				ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
					ui.Text(c, a.Tr("主题模式", "Theme Mode")).FontSize(s.Base).TextColor(p.Foreground).Grow(1)
					themeIdx := 0
					switch a.SettingsData.ThemeMode {
					case model.ThemeLight:
						themeIdx = 1
					case model.ThemeDark:
						themeIdx = 2
					}
					modes := []model.ThemeMode{model.ThemeSystem, model.ThemeLight, model.ThemeDark}
					labels := []string{a.Tr("跟随系统", "System"), a.Tr("浅色", "Light"), a.Tr("深色", "Dark")}
					if components.Segmented(c.Key("set-theme-seg"), p, s, &themeIdx, labels).Changed() {
						a.SetThemeMode(modes[themeIdx])
					}
				})

				ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
					ui.Text(c, a.Tr("界面字号", "UI Font Size")).FontSize(s.Base).TextColor(p.Foreground).Grow(1)
					ui.Text(c, fmt.Sprintf("%.0f px", a.UIFontDraft)).FontSize(s.SM).Font(theme.MonoFont()).FontFeatures("tnum").TextColor(p.ForegroundSubtle)
					if ui.Slider(c.Key("set-font-size"), &a.UIFontDraft, 12, 16).Step(1).Width(160).Changed() {
						a.SettingsData.UIFontSize = float32(a.UIFontDraft)
						_ = a.PM.SaveSettings(a.SettingsData)
					}
				})

				ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
					ui.Text(c, a.Tr("界面语言", "Language")).FontSize(s.Base).TextColor(p.Foreground).Grow(1)
					langIdx := 0
					if a.SettingsData.Language == model.LangEN {
						langIdx = 1
					}
					if components.Segmented(c.Key("set-lang-seg"), p, s, &langIdx, []string{"简体中文", "English"}).Changed() {
						if langIdx == 1 {
							a.SettingsData.Language = model.LangEN
							mygo.App.SetLocale("en")
						} else {
							a.SettingsData.Language = model.LangZH
							mygo.App.SetLocale("zh-Hans")
						}
						_ = a.PM.SaveSettings(a.SettingsData)
						a.RefreshTray()
					}
				})

				ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
					ui.Text(c, a.Tr("关闭主窗口时", "On Window Close")).FontSize(s.Base).TextColor(p.Foreground).Grow(1)
					closeIdx := 0
					if a.SettingsData.CloseAction == model.CloseQuit {
						closeIdx = 1
					}
					if components.Segmented(c.Key("set-close-seg"), p, s, &closeIdx, []string{a.Tr("最小化到托盘", "Hide to Tray"), a.Tr("直接退出", "Quit App")}).Changed() {
						if closeIdx == 1 {
							a.SettingsData.CloseAction = model.CloseQuit
						} else {
							a.SettingsData.CloseAction = model.CloseToTray
						}
						_ = a.PM.SaveSettings(a.SettingsData)
					}
				})

				ui.Row(c).Gap(16).AlignItems(ui.Center).Children(func() {
					if ui.Checkbox(c.Key("set-login"), &a.SettingsData.LaunchAtLogin, a.Tr("开机自启动", "Launch at Login")).Changed() {
						if err := mygo.App.SetOpenAtLogin(a.SettingsData.LaunchAtLogin); err != nil {
							a.Notify(err.Error())
						}
						_ = a.PM.SaveSettings(a.SettingsData)
					}
					if ui.Checkbox(c.Key("set-silent"), &a.SettingsData.SilentStart, a.Tr("静默启动 (隐藏到托盘)", "Silent Start (start in tray)")).Changed() {
						_ = a.PM.SaveSettings(a.SettingsData)
					}
				})
			})

			// 5. Software Updates (OTA)
			components.Card(c, p, func() {
				appVer := strings.TrimSpace(mygo.App.Version())
				verChip := "dev"
				if appVer != "" {
					verChip = "v" + strings.TrimPrefix(appVer, "v")
				}
				availVer := strings.TrimPrefix(strings.TrimSpace(a.AppUpdate.AvailableVersion), "v")

				components.SectionHeader(c, p, s, a.Tr("应用更新", "Software Updates"), func() {
					if availVer != "" {
						components.StatusChip(c, s, fmt.Sprintf(a.Tr("发现新版本 v%s", "Update v%s"), availVer), p.ProgressSurface, p.ProgressSurfaceForeground, p.IconBlue)
					} else if a.AppUpdate.Supported {
						components.StatusChip(c, s, verChip, p.SuccessSurface, p.SuccessSurfaceForeground, p.Success)
					} else {
						components.StatusChip(c, s, verChip, p.Tag, p.ForegroundSubtle, p.ForegroundSubtlest)
					}
					btnLabel := a.Tr("检查更新", "Check for Updates")
					if availVer != "" {
						btnLabel = a.Tr("立即更新", "Update Now")
					}
					if components.PrimaryActionButton(c, p, s, icons.Refresh, btnLabel).Clicked() {
						a.CheckAppUpdates()
					}
				})

				if !a.AppUpdate.LastCheck.IsZero() {
					components.KeyValueRow(c, p, s, a.Tr("上次检查时间", "Last Checked"), components.FormatRelativeTime(a.AppUpdate.LastCheck, a.SettingsData.Language), false)
				}
				if !a.AppUpdate.Supported {
					ui.Text(c, a.Tr("当前为开发构建或只读安装目录，不支持原地自动更新；点击“检查更新”可查看说明。", "Development builds or read-only installs do not support in-place OTA updates.")).FontSize(s.XS).TextColor(p.ForegroundSubtle)
				}

				ui.Row(c).Gap(16).AlignItems(ui.Center).Children(func() {
					if ui.Checkbox(c.Key("set-ota-auto-check"), &a.AppUpdate.AutomaticChecks, a.Tr("自动检查更新 (24h)", "Check automatically (24h)")).Changed() {
						a.SetUpdateAutomaticChecks(a.AppUpdate.AutomaticChecks)
					}
					if ui.Checkbox(c.Key("set-ota-auto-dl"), &a.AppUpdate.AutomaticDownloads, a.Tr("自动下载并安装更新", "Download & install automatically")).Changed() {
						a.SetUpdateAutomaticDownloads(a.AppUpdate.AutomaticDownloads)
					}
					if ui.Checkbox(c.Key("set-ota-beta"), &a.SettingsData.UpdatePrereleases, a.Tr("接收预发布版 (Beta)", "Include pre-releases (Beta)")).Changed() {
						a.SetUpdatePrereleases(a.SettingsData.UpdatePrereleases)
					}
				})
			})

			// 6. Data & Diagnostics
			components.Card(c, p, func() {
				ui.Text(c, a.Tr("数据目录与诊断", "Data & Diagnostics")).FontSize(s.Base).FontWeight(600).TextColor(p.Foreground)
				components.KeyValueRow(c, p, s, a.Tr("配置数据目录", "Data Directory"), a.PM.DataDir(), true)
				components.KeyValueRow(c, p, s, a.Tr("运行时配置文件", "Runtime Config"), a.PM.RuntimeConfigPath(), true)
				if a.LogStore != nil {
					components.KeyValueRow(c, p, s, a.Tr("日志文件 (磁盘滚动)", "Log File (Disk Rotating)"), a.LogStore.Path(), true)
				}
				ui.Row(c).Gap(8).Children(func() {
					if components.ActionButton(c, p, s, icons.Folder, a.Tr("打开数据目录", "Open Data Folder")).Clicked() {
						app.OpenFolderInOS(a.PM.DataDir())
					}
					if a.LogStore != nil {
						if components.ActionButton(c, p, s, icons.Logs, a.Tr("打开日志目录", "Open Logs Folder")).Clicked() {
							app.OpenFolderInOS(a.LogStore.Dir())
						}
					}
					if components.ActionButton(c, p, s, icons.Code, a.Tr("预览编译配置", "Inspect Runtime Config")).Clicked() {
						a.OpenRuntimePreview()
					}
				})
			})
		})
	})
}

func commitNumericSettings(a *app.App) {
	prev := a.PM.Settings()
	if port, err := strconv.Atoi(strings.TrimSpace(a.MixedPortDraft)); err == nil && port > 0 && port <= 65535 {
		a.SettingsData.MixedPort = port
	}
	if port, err := strconv.Atoi(strings.TrimSpace(a.APIPortDraft)); err == nil && port > 0 && port <= 65535 {
		a.SettingsData.ClashAPIPort = port
	}
	if mtu, err := strconv.Atoi(strings.TrimSpace(a.TUNMTUDraft)); err == nil && mtu >= 576 && mtu <= 65535 {
		a.SettingsData.TUNMTU = mtu
	}
	otherSettings := a.SettingsData
	otherSettings.MixedPort = prev.MixedPort
	if reflect.DeepEqual(otherSettings, prev) {
		if a.SettingsData.MixedPort != prev.MixedPort {
			a.ApplyInbound("mixed-port")
		} else if !a.CoreInfo.External {
			a.ApplyProfileChanges()
		}
		return
	}
	if err := a.PM.SaveSettings(a.SettingsData); err != nil {
		a.Notify(err.Error())
		return
	}
	a.Notify(a.Tr("设置已保存", "Settings saved"))
	if a.SettingsData.SystemProxyEnabled && a.SettingsData.MixedPort != prev.MixedPort {
		go a.SetSystemProxy(true)
	}
	if a.CoreInfo.Status == model.CoreRunning {
		go a.RestartCore()
	}
}

func pickCoreBinary(a *app.App) {
	targetCore := model.NormalizeCoreType(a.SettingsData.CoreType)
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Parent: a.Window(),
		Title:  fmt.Sprintf(a.Tr("选择 %s 可执行文件", "Select %s executable"), model.CoreDisplayName(targetCore)),
	})
	if err != nil || len(paths) == 0 {
		return
	}
	a.UpdateUI(func() {
		a.SettingsData.SetCorePathFor(targetCore, paths[0])
		_ = a.PM.SaveSettings(a.SettingsData)
		if a.SettingsData.CoreType == targetCore {
			a.CoreInfo = a.CM.RefreshBinaryInfo(paths[0])
		}
		a.Notify(a.Tr("已更新核心路径", "Core path updated"))
	})
}

func pickCoreDir(a *app.App) {
	targetCore := model.NormalizeCoreType(a.SettingsData.CoreType)
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Parent:            a.Window(),
		Title:             fmt.Sprintf(a.Tr("选择 %s 核心存放目录", "Select %s core directory"), model.CoreDisplayName(targetCore)),
		Directory:         true,
		CreateDirectories: true,
	})
	if err != nil || len(paths) == 0 {
		return
	}
	a.UpdateUI(func() {
		a.SettingsData.SetCorePathFor(targetCore, paths[0])
		_ = a.PM.SaveSettings(a.SettingsData)
		if a.SettingsData.CoreType == targetCore {
			a.CoreInfo = a.CM.RefreshBinaryInfo(paths[0])
		}
		a.Notify(a.Tr("已更新核心目录", "Core directory updated"))
	})
}

func pickGeoDataDir(a *app.App) {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Parent:            a.Window(),
		Title:             a.Tr("选择 GeoIP / GeoData 存放目录", "Select GeoIP / GeoData directory"),
		Directory:         true,
		CreateDirectories: true,
	})
	if err != nil || len(paths) == 0 {
		return
	}
	a.UpdateUI(func() {
		a.SettingsData.GeoDataDir = paths[0]
		a.ApplyGeoDataDir()
	})
}
