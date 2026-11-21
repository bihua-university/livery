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

// Settings renders core, network, appearance, updates, and data directory settings
// in a centered, hairline-divided layout optimized for scannability.
func Settings(c *ui.Context, a *app.App, p theme.Palette, s theme.TypeScale) {
	ui.Scroll(c.Key("settings-scroll")).TrackScroll(&a.SettingsScroll).Grow(1).Padding(16, 20, 28, 20).Children(func() {
		ui.Column(c).FillWidth().AlignItems(ui.Center).Children(func() {
			ui.Column(c).FillWidth().MaxWidth(840).Gap(16).Children(func() {
				ui.Column(c).FillWidth().Gap(2).Children(func() {
					ui.Text(c, a.Tr("设置", "Settings")).FontSize(s.SettingsTitle).FontWeight(600).TextColor(p.Foreground)
					ui.Text(c, a.Tr("管理代理核心、系统网络接管、规则数据库与桌面偏好", "Manage proxy core, system networking, rule databases, and desktop preferences")).
						FontSize(s.XS).TextColor(p.ForegroundSubtle)
				})

				settingsCoreSection(c, a, p, s)
				if model.NormalizeCoreType(a.SettingsData.CoreType) == model.CoreMihomo {
					settingsMihomoSection(c, a, p, s)
				}
				settingsNetworkSection(c, a, p, s)
				settingsGeoSection(c, a, p, s)
				settingsAppearanceSection(c, a, p, s)
				settingsUpdatesSection(c, a, p, s)
				settingsDataSection(c, a, p, s)
			})
		})
	})
}

func settingsCoreSection(c *ui.Context, a *app.App, p theme.Palette, s theme.TypeScale) {
	coreType := model.NormalizeCoreType(a.SettingsData.CoreType)
	coreName := model.CoreDisplayName(coreType)

	components.SettingsGroup(c, p, s, icons.Zap,
		coreName+a.Tr(" 核心", " Core"),
		a.Tr("配置底层代理内核、监听端口与节点测速行为", "Configure the proxy core binary, inbound ports, and latency testing"),
		func() {
			if a.CoreInfo.Found {
				components.StatusChip(c, s, "v"+strings.TrimSpace(a.CoreInfo.Version), p.SuccessSurface, p.SuccessSurfaceForeground, p.Success)
			} else {
				components.StatusChip(c, s, a.Tr("未找到", "Not Found"), p.WarningSurface, p.WarningSurfaceForeground, p.Warning)
			}
		},
		func() {
			components.SettingItem(c, p, s,
				a.Tr("核心引擎", "Core Engine"),
				a.Tr("切换底层代理内核，核心运行时将自动重启生效", "Switch the underlying proxy engine; restarts automatically if running"),
				nil,
				func() {
					components.SegmentedValue(c.Key("set-core-type-seg"), p, s, coreType, []components.SegmentOption[model.CoreType]{
						{Value: model.CoreMihomo, Label: "Mihomo"},
						{Value: model.CoreSingBox, Label: "sing-box"},
					}, a.SwitchCoreType)
				},
			)

			components.SettingDivider(c, p)

			pathPtr := &a.SettingsData.MihomoPath
			if coreType == model.CoreSingBox {
				pathPtr = &a.SettingsData.CorePath
			}
			components.SettingBlock(c, p, s,
				fmt.Sprintf(a.Tr("%s 核心路径", "%s Binary Path"), coreName),
				a.Tr("留空自动探测默认 bin/ 目录或系统 PATH；点击下载将保存至此处", "Leave blank to auto-detect in default bin/ or PATH; downloads save here"),
				nil,
				func() {
					pathPlaceholder := a.CoreInfo.BinaryPath
					if pathPlaceholder == "" {
						pathPlaceholder = a.Tr("自动探测默认 bin/ 目录或系统 PATH", "Auto-detect default bin/ or PATH")
					}
					ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
						if ui.TextInput(c.Key("set-core-path-"+string(coreType)), pathPtr).Placeholder(pathPlaceholder).Grow(1).Submitted() {
							a.ApplySettings(false)
							a.CoreInfo = a.CM.RefreshBinaryInfo(*pathPtr)
						}
						if components.ActionButton(c, p, s, icons.Profiles, a.Tr("选文件", "File")).Clicked() {
							go pickCorePath(a, false)
						}
						if components.ActionButton(c, p, s, icons.Folder, a.Tr("选目录", "Folder")).Clicked() {
							go pickCorePath(a, true)
						}
						if components.ActionButton(c, p, s, icons.Download, a.Tr("下载最新版", "Install Latest")).Disabled(a.InstallingCore).Clicked() {
							a.StartInstallCore()
						}
					})
					if a.InstallStatus != "" {
						ui.Text(c, a.InstallStatus).FontSize(s.XS).TextColor(p.IconBlue)
					}
				},
			)

			components.SettingDivider(c, p)

			components.SettingSubheader(c, p, s, a.Tr("监听端口与控制接口", "Ports & Controller"), func() {
				if components.PrimaryActionButton(c, p, s, icons.Check, a.Tr("应用端口与核心设置", "Apply Core Settings")).Disabled(a.ApplyingInbound).Clicked() {
					commitNumericSettings(a)
				}
			})

			components.SettingBlock(c, p, s, "", "", nil, func() {
				ui.Grid(c).Columns(3).Gap(10).Children(func() {
					if components.LabeledInput(c, p, s, "set-mixed-port", a.Tr("Mixed 入站端口 (HTTP/SOCKS5)", "Mixed Inbound Port"), "7890", &a.MixedPortDraft).Disabled(a.ApplyingInbound).Submitted() {
						commitNumericSettings(a)
					}
					if components.LabeledInput(c, p, s, "set-api-port", a.Tr("Clash API 端口", "Clash API Port"), "9090", &a.APIPortDraft).Submitted() {
						commitNumericSettings(a)
					}
					if components.LabeledInput(c, p, s, "set-api-secret", "Clash API Secret", a.Tr("可选，留空不鉴权", "Optional"), &a.SettingsData.ClashAPISecret).Submitted() {
						a.ApplySettings(true)
					}
				})

				ui.Grid(c).Columns(2).Gap(10).Children(func() {
					if components.SettingToggleTile(c, p, s, "set-allow-lan",
						a.Tr("允许局域网连接 (0.0.0.0)", "Allow LAN (0.0.0.0)"),
						a.Tr("允许局域网内其他设备通过本机代理端口连接", "Allow LAN devices to use your mixed port"),
						&a.SettingsData.AllowLAN).Disabled(a.ApplyingInbound).Changed() {
						a.ApplyInbound("allow-lan")
					}
					if components.SettingToggleTile(c, p, s, "set-autostart",
						a.Tr("启动应用时自动运行核心", "Auto-start core on launch"),
						a.Tr("打开 Livery 后自动启动当前激活配置", "Start active profile automatically on launch"),
						&a.SettingsData.AutoStartCore).Changed() {
						a.ApplySettings(false)
					}
				})
			})

			components.SettingDivider(c, p)

			components.SettingSubheader(c, p, s, a.Tr("测速、日志与切换行为", "Latency, Logging & Switching"), nil)

			components.SettingBlock(c, p, s, "", "", nil, func() {
				ui.Grid(c).ColumnTracks(ui.Fr(2).Min(180), ui.Fr(1).Min(110), ui.Fr(1).Min(110)).Gap(10).Children(func() {
					ui.Column(c).MinWidth(0).Gap(4).Children(func() {
						ui.Text(c, a.Tr("延迟测速 URL", "Latency Test URL")).FontSize(s.XS).FontWeight(500).TextColor(p.ForegroundSubtle).SingleLine()
						if ui.TextInput(c.Key("set-test-url"), &a.SettingsData.DelayTestURL).FillWidth().Submitted() {
							a.ApplySettings(false)
						}
					})
					ui.Column(c).MinWidth(0).Gap(4).Children(func() {
						ui.Text(c, a.Tr("测速超时 (ms)", "Test timeout (ms)")).FontSize(s.XS).FontWeight(500).TextColor(p.ForegroundSubtle).SingleLine()
						ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
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
			})

			components.SettingDivider(c, p)

			if components.SettingToggleItem(c, p, s, "set-close-connections-on-switch",
				a.Tr("切换节点成功后断开所有活动连接", "Close all active connections after switching nodes"),
				a.Tr("切换节点后主动中断旧连接，让正在进行的流量立即切到新节点", "Interrupt existing connections on node switch so traffic immediately uses the new node"),
				&a.SettingsData.CloseConnectionsOnSwitch, nil).Changed() {
				a.ApplySettings(false)
			}

			if prof, ok := a.PM.ActiveProfile(); coreType == model.CoreMihomo && ok {
				components.SettingDivider(c, p)
				profID := prof.ID
				profName := prof.Name
				components.SettingItem(c, p, s,
					a.Tr("DNS 增强模式", "DNS enhanced mode"),
					a.Tr("fake-ip 响应更快；redir-host 返回真实 IP；跟随配置保留订阅原设置", "fake-ip resolves faster; redir-host returns real IPs; Use profile keeps subscription defaults"),
					func() { components.TagChip(c, p, s, profName) },
					func() {
						components.SegmentedValue(c.Key("set-dns-mode-"+profID), p, s, prof.Override.DNSMode, []components.SegmentOption[string]{
							{Value: "", Label: a.Tr("跟随配置", "Use profile")},
							{Value: "fake-ip", Label: "fake-ip"},
							{Value: "redir-host", Label: "redir-host"},
						}, func(mode string) {
							a.SetDNSMode(profID, mode)
						}).Disabled(a.SavingDNSMode || a.IsCoreBusy())
					},
				)
			}
		},
	)
}

func settingsMihomoSection(c *ui.Context, a *app.App, p theme.Palette, s theme.TypeScale) {
	useProfileLabel := a.Tr("跟随配置", "Use profile")
	boolOpts := []components.SegmentOption[boolOverride]{
		{Value: boolOverrideDefault, Label: useProfileLabel},
		{Value: boolOverrideOn, Label: a.Tr("开启", "On")},
		{Value: boolOverrideOff, Label: a.Tr("关闭", "Off")},
	}
	boolItem := func(key, title, desc string, value *bool, set func(*model.AppSettings, *bool)) {
		cur := boolOverrideDefault
		if value != nil {
			if *value {
				cur = boolOverrideOn
			} else {
				cur = boolOverrideOff
			}
		}
		components.SettingItem(c, p, s, title, desc, nil, func() {
			components.SegmentedValue(c.Key(key), p, s, cur, boolOpts, func(choice boolOverride) {
				var override *bool
				if choice != boolOverrideDefault {
					on := choice == boolOverrideOn
					override = &on
				}
				a.SavePreference(func(s *model.AppSettings) { set(s, override) }, a.ApplyProfileChanges)
			}).Disabled(a.SavingPreference || a.IsCoreBusy())
		})
	}

	components.SettingsGroup(c, p, s, icons.Sliders,
		a.Tr("Mihomo 进阶选项与 DNS 缓存", "Mihomo Options & DNS Cache"),
		a.Tr("覆写内核运行参数并管理本地 DNS 与 Fake-IP 缓存", "Override Mihomo runtime behavior and manage local DNS / Fake-IP caches"),
		nil,
		func() {
			boolItem("set-tcp-concurrent",
				a.Tr("TCP 并发连接", "Concurrent TCP connections"),
				a.Tr("并发连接同一域名解析出的多个 IP，加快建连速度", "Dial multiple resolved IPs concurrently for faster connection setup"),
				a.SettingsData.TCPConcurrent, func(s *model.AppSettings, v *bool) { s.TCPConcurrent = v })

			components.SettingDivider(c, p)

			boolItem("set-ipv6",
				"IPv6",
				a.Tr("允许内核处理 IPv6 流量与解析；纯 IPv4 环境建议关闭", "Allow IPv6 routing and AAAA lookups; turn off on IPv4-only networks"),
				a.SettingsData.IPv6, func(s *model.AppSettings, v *bool) { s.IPv6 = v })

			components.SettingDivider(c, p)

			components.SettingItem(c, p, s,
				a.Tr("Geo 数据加载器", "Geo data loader"),
				a.Tr("低内存模式可显著降低 Mihomo 常驻内存占用", "Low memory mode reduces Mihomo resident RAM usage"),
				nil,
				func() {
					components.SegmentedValue(c.Key("set-geodata-loader"), p, s, a.SettingsData.GeoDataLoader, []components.SegmentOption[string]{
						{Value: "", Label: useProfileLabel},
						{Value: "standard", Label: a.Tr("标准", "Standard")},
						{Value: "memconservative", Label: a.Tr("低内存", "Low memory")},
					}, func(loader string) {
						a.SavePreference(func(s *model.AppSettings) { s.GeoDataLoader = loader }, a.ApplyProfileChanges)
					}).Disabled(a.SavingPreference || a.IsCoreBusy())
				},
			)

			components.SettingDivider(c, p)

			components.SettingSubheader(c, p, s, a.Tr("DNS 缓存", "DNS Cache"), func() {
				if components.ActionButton(c.Key("dns-cache-flush"), p, s, icons.Refresh, a.Tr("清理缓存", "Clear cache")).Disabled(a.CoreInfo.Status != model.CoreRunning || a.FlushingDNS).Clicked() {
					a.FlushDNSCaches()
				}
			})

			if components.SettingToggleItem(c, p, s, "dns-cache-auto",
				a.Tr("切换网络后自动清理 DNS 和 Fake-IP 缓存", "Clear DNS and Fake-IP caches on network changes"),
				a.Tr("检测到 Wi-Fi 或网络切换时自动刷新缓存，避免旧解析失效", "Automatically flush stale DNS and Fake-IP records after network handoffs"),
				&a.SettingsData.FlushDNSOnNetworkChange, nil).Changed() {
				a.ApplySettings(false)
			}
		},
	)
}

func settingsNetworkSection(c *ui.Context, a *app.App, p theme.Palette, s theme.TypeScale) {
	components.SettingsGroup(c, p, s, icons.Shield,
		a.Tr("系统代理与 TUN 接管", "System Proxy & TUN"),
		a.Tr("配置操作系统代理守卫、绕过名单与 TUN 虚拟网卡参数", "Configure OS proxy guard, bypass rules, and TUN virtual interface options"),
		nil,
		func() {
			components.SettingSubheader(c, p, s, a.Tr("系统代理", "System Proxy"), nil)

			if components.SettingToggleItem(c, p, s, "set-guard",
				a.Tr("系统代理守卫 (Proxy Guard)", "System Proxy Guard"),
				a.Tr("定期检查并防止系统代理设置被其他程序意外覆盖", "Periodically restore system proxy if altered by another app"),
				&a.SettingsData.ProxyGuard, nil).Changed() {
				a.ApplySettings(false)
				a.WakeProxyGuard()
			}

			if a.SettingsData.ProxyGuard {
				components.SettingDivider(c, p)
				cur := a.SettingsData.ProxyGuardInterval
				if !slices.Contains([]int{5, 10, 30, 60}, cur) {
					cur = 10
				}
				components.SettingItem(c, p, s,
					a.Tr("守卫检查间隔", "Guard Check Interval"),
					a.Tr("间隔越短恢复越快，日常推荐 10s", "Shorter intervals recover faster; 10s is recommended"),
					nil,
					func() {
						components.SegmentedValue(c.Key("set-guard-interval"), p, s, cur, []components.SegmentOption[int]{
							{Value: 5, Label: "5s"},
							{Value: 10, Label: "10s"},
							{Value: 30, Label: "30s"},
							{Value: 60, Label: "60s"},
						}, func(interval int) {
							a.SettingsData.ProxyGuardInterval = interval
							a.ApplySettings(false)
							a.WakeProxyGuard()
						})
					},
				)
			}

			components.SettingDivider(c, p)

			applyBypass := func() {
				a.SettingsData.SystemProxyBypass = sysproxy.NormalizeBypass(a.SettingsData.SystemProxyBypass)
				a.ApplySettings(false)
				if a.SettingsData.SystemProxyEnabled {
					go a.SetSystemProxy(true)
				} else {
					a.Notify(a.Tr("绕过列表已保存", "Bypass list saved"))
				}
			}
			components.SettingBlock(c, p, s,
				a.Tr("系统代理绕过列表 (Bypass)", "System Proxy Bypass List"),
				a.Tr("匹配这些地址或网段的流量将直接连接，支持逗号或分号分隔", "Hosts or subnets bypassing the OS proxy, separated by commas or semicolons"),
				nil,
				func() {
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
				},
			)

			components.SettingDivider(c, p)

			components.SettingSubheader(c, p, s, a.Tr("TUN 虚拟网卡", "TUN Virtual Interface"), func() {
				if runtime.GOOS == "windows" && !a.CoreInfo.IsElevated {
					if components.ActionButton(c, p, s, icons.Shield, a.Tr("以管理员运行", "Elevate (Admin)")).Clicked() {
						if err := core.RelaunchElevated(); err != nil {
							a.Notify(err.Error())
						}
					}
				}
			})

			components.SettingBlock(c, p, s, "", "", nil, func() {
				ui.Grid(c).Columns(3).Gap(10).Children(func() {
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

				ui.Grid(c).Columns(3).Gap(10).Children(func() {
					if components.SettingToggleTile(c, p, s, "set-tun-auto",
						"Auto Route",
						a.Tr("自动配置默认路由接管流量", "Auto-configure system routes"),
						&a.SettingsData.TUNAutoRoute).Changed() {
						a.ApplySettings(a.SettingsData.TUNEnabled)
					}
					if components.SettingToggleTile(c, p, s, "set-tun-strict",
						"Strict Route",
						a.Tr("严格路由防止直连泄露", "Prevent non-TUN traffic leaks"),
						&a.SettingsData.TUNStrictRoute).Changed() {
						a.ApplySettings(a.SettingsData.TUNEnabled)
					}
					if components.SettingToggleTile(c, p, s, "set-tun-bypass-private",
						a.Tr("绕过私有路由", "Bypass Private Route"),
						a.Tr("直连局域网与 Tailscale 网段", "Keep LAN & Tailscale direct"),
						&a.SettingsData.TUNBypassPrivateRoute).Changed() {
						a.ApplySettings(a.SettingsData.TUNEnabled)
					}
				})
			})
		},
	)
}

func settingsGeoSection(c *ui.Context, a *app.App, p theme.Palette, s theme.TypeScale) {
	coreType := model.NormalizeCoreType(a.SettingsData.CoreType)
	readyCount := 0
	for _, f := range a.GeoFiles {
		if f.Exists {
			readyCount++
		}
	}

	components.SettingsGroup(c, p, s, icons.Proxies,
		a.Tr("GeoIP 与规则数据库", "GeoIP & GeoData"),
		a.Tr("管理按国家/地区与域名集合进行分流匹配的本地数据库", "Manage local databases used for region and domain-set routing rules"),
		func() {
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
		},
		func() {
			if len(a.GeoFiles) > 0 || a.GeoStatus != "" {
				components.SettingBlock(c, p, s, "", "", nil, func() {
					if a.GeoStatus != "" {
						ui.Text(c, a.GeoStatus).FontSize(s.XS).TextColor(p.IconBlue)
					}
					if len(a.GeoFiles) > 0 {
						ui.Grid(c).Columns(2).Gap(8).Children(func() {
							for _, f := range a.GeoFiles {
								dotColor := p.ForegroundSubtlest
								if f.Exists {
									dotColor = p.Success
								}
								ui.Row(c.Key("geo-file-"+f.Name)).
									Background(p.Surface).
									Border(1, p.Border).
									Radius(theme.RadiusLG).
									Padding(8, 10).
									Gap(8).
									AlignItems(ui.Center).
									Children(func() {
										ui.Box(c).Size(6, 6).Shrink(0).Radius(theme.RadiusFull).Background(dotColor)
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
				})
				components.SettingDivider(c, p)
			}

			components.SettingBlock(c, p, s,
				a.Tr("Geo 数据目录", "GeoData Directory"),
				a.Tr("留空使用默认配置目录，修改后仍兼容读取默认目录中的已有文件", "Blank uses the default data directory while staying compatible with existing files"),
				func() {
					if components.ActionButton(c, p, s, icons.Folder, a.Tr("打开 Geo 目录", "Open Geo Folder")).Clicked() {
						app.OpenFolderInOS(core.EffectiveGeoDataDir(a.SettingsData, a.PM.DataDir()))
					}
					if components.IconActionButton(c, p, icons.Refresh, a.Tr("刷新状态", "Refresh Status")).Clicked() {
						a.RefreshGeoStatus()
					}
				},
				func() {
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
				},
			)

			if coreType == model.CoreMihomo {
				components.SettingDivider(c, p)

				components.SettingBlock(c, p, s, "", "", nil, func() {
					ui.Grid(c).Columns(2).Gap(10).Children(func() {
						if components.SettingToggleTile(c, p, s, "set-geo-dat",
							a.Tr("启用 Geodata 模式 (.dat)", "Use Geodata (.dat) mode"),
							a.Tr("使用 .dat 替代 MMDB 兼容更多规则集", "Use .dat files for broader ruleset support"),
							&a.SettingsData.GeoDataMode).Changed() {
							a.ApplySettings(true)
						}
						if components.SettingToggleTile(c, p, s, "set-geo-auto",
							a.Tr("自动更新 Geo 数据库 (24h)", "Auto-update Geo databases (24h)"),
							a.Tr("每 24 小时后台自动刷新规则库", "Auto-refresh Geo databases every 24h"),
							&a.SettingsData.GeoAutoUpdate).Changed() {
							a.ApplySettings(true)
						}
					})
				})

				components.SettingDivider(c, p)

				components.SettingSubheader(c, p, s, a.Tr("自定义数据库下载源", "Custom Database Sources"), func() {
					if components.ActionButton(c, p, s, nil, a.Tr("恢复默认源", "Reset URLs")).Clicked() {
						a.ResetGeoURLs()
					}
					if components.ActionButton(c, p, s, icons.Check, a.Tr("应用 Geo 设置", "Apply Geo Settings")).Clicked() {
						a.SettingsData.GeoDataDir = strings.TrimSpace(a.SettingsData.GeoDataDir)
						a.CM.SetGeoDataDir(a.SettingsData.GeoDataDir)
						a.RefreshGeoStatus()
						a.ApplySettings(true)
						a.Notify(a.Tr("Geo 设置已保存", "Geo settings saved"))
					}
				})

				components.SettingBlock(c, p, s, "", "", nil, func() {
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
				})
			}
		},
	)
}

func settingsAppearanceSection(c *ui.Context, a *app.App, p theme.Palette, s theme.TypeScale) {
	components.SettingsGroup(c, p, s, icons.Settings,
		a.Tr("外观与桌面行为", "Appearance & Behavior"),
		a.Tr("个性化主题配色、界面缩放、语言与窗口启动/关闭行为", "Customize theme, typography scaling, language, and window lifecycle"),
		nil,
		func() {
			components.SettingItem(c, p, s,
				a.Tr("主题模式", "Theme Mode"),
				a.Tr("选择浅色、深色或自动跟随操作系统外观", "Use light, dark, or match your system appearance"),
				nil,
				func() {
					components.SegmentedValue(c.Key("set-theme-seg"), p, s, a.SettingsData.ThemeMode, []components.SegmentOption[model.ThemeMode]{
						{Value: model.ThemeSystem, Label: a.Tr("跟随系统", "System")},
						{Value: model.ThemeLight, Label: a.Tr("浅色", "Light")},
						{Value: model.ThemeDark, Label: a.Tr("深色", "Dark")},
					}, a.SetThemeMode)
				},
			)

			components.SettingDivider(c, p)

			components.SettingItem(c, p, s,
				a.Tr("界面字号", "UI Font Size"),
				a.Tr("调整界面全局文字缩放比例（12–16 px）", "Adjust overall UI text size (12–16 px)"),
				nil,
				func() {
					components.TagChip(c, p, s, fmt.Sprintf("%.0f px", a.UIFontDraft))
					if ui.Slider(c.Key("set-font-size"), &a.UIFontDraft, 12, 16).Step(1).Width(150).Changed() {
						a.SettingsData.UIFontSize = float32(a.UIFontDraft)
						_ = a.PM.SaveSettings(a.SettingsData)
					}
				},
			)

			components.SettingDivider(c, p)

			components.SettingItem(c, p, s,
				a.Tr("界面语言", "Language"),
				a.Tr("切换主窗口与系统托盘菜单的显示语言", "Switch display language for the window and tray menu"),
				nil,
				func() {
					components.SegmentedValue(c.Key("set-lang-seg"), p, s, a.SettingsData.Language, []components.SegmentOption[model.LanguageMode]{
						{Value: model.LangZH, Label: "简体中文"},
						{Value: model.LangEN, Label: "English"},
					}, func(lang model.LanguageMode) {
						a.SettingsData.Language = lang
						if lang == model.LangEN {
							mygo.App.SetLocale("en")
						} else {
							mygo.App.SetLocale("zh-Hans")
						}
						_ = a.PM.SaveSettings(a.SettingsData)
						a.RefreshTray()
					})
				},
			)

			components.SettingDivider(c, p)

			components.SettingItem(c, p, s,
				a.Tr("关闭主窗口时", "On Window Close"),
				a.Tr("决定点击主窗口关闭按钮后隐藏到托盘或直接退出应用", "Choose whether closing the main window minimizes to tray or quits"),
				nil,
				func() {
					components.SegmentedValue(c.Key("set-close-seg"), p, s, a.SettingsData.CloseAction, []components.SegmentOption[model.CloseAction]{
						{Value: model.CloseToTray, Label: a.Tr("最小化到托盘", "Hide to Tray")},
						{Value: model.CloseQuit, Label: a.Tr("直接退出", "Quit App")},
					}, func(action model.CloseAction) {
						a.SettingsData.CloseAction = action
						_ = a.PM.SaveSettings(a.SettingsData)
					})
				},
			)

			components.SettingDivider(c, p)

			components.SettingBlock(c, p, s, "", "", nil, func() {
				ui.Grid(c).Columns(2).Gap(10).Children(func() {
					if components.SettingToggleTile(c, p, s, "set-login",
						a.Tr("开机自启动", "Launch at Login"),
						a.Tr("登录系统后自动启动 Livery", "Launch Livery when you sign in"),
						&a.SettingsData.LaunchAtLogin).Changed() {
						if err := mygo.App.SetOpenAtLogin(a.SettingsData.LaunchAtLogin); err != nil {
							a.Notify(err.Error())
						}
						_ = a.PM.SaveSettings(a.SettingsData)
					}
					if components.SettingToggleTile(c, p, s, "set-silent",
						a.Tr("静默启动 (隐藏到托盘)", "Silent Start (start in tray)"),
						a.Tr("启动时不弹出主窗口，驻留托盘运行", "Start minimized in the system tray"),
						&a.SettingsData.SilentStart).Changed() {
						_ = a.PM.SaveSettings(a.SettingsData)
					}
				})
			})
		},
	)
}

func settingsUpdatesSection(c *ui.Context, a *app.App, p theme.Palette, s theme.TypeScale) {
	appVer := strings.TrimSpace(mygo.App.Version())
	verChip := "dev"
	if appVer != "" {
		verChip = "v" + strings.TrimPrefix(appVer, "v")
	}
	availVer := strings.TrimPrefix(strings.TrimSpace(a.AppUpdate.AvailableVersion), "v")

	components.SettingsGroup(c, p, s, icons.Download,
		a.Tr("应用更新", "Software Updates"),
		a.Tr("检查 Livery 新版本并配置自动更新策略", "Check for Livery releases and configure automatic update behavior"),
		func() {
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
		},
		func() {
			if !a.AppUpdate.LastCheck.IsZero() {
				components.SettingItem(c, p, s,
					a.Tr("上次检查时间", "Last Checked"), "", nil,
					func() {
						ui.Text(c, components.FormatRelativeTime(a.AppUpdate.LastCheck, a.SettingsData.Language)).
							FontSize(s.SM).TextColor(p.ForegroundSubtle).SingleLine()
					},
				)
				components.SettingDivider(c, p)
			}
			if !a.AppUpdate.Supported {
				ui.Row(c).FillWidth().Background(p.Surface).Padding(8, 14).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Icon(c, icons.Alert).FontSize(14).TextColor(p.ForegroundSubtle)
					ui.Text(c, a.Tr("当前为开发构建或只读安装目录，不支持原地自动更新；点击「检查更新」可查看说明。", "Development builds or read-only installs do not support in-place OTA updates.")).
						FontSize(s.XS).TextColor(p.ForegroundSubtle).Grow(1)
				})
				components.SettingDivider(c, p)
			}

			if components.SettingToggleItem(c, p, s, "set-ota-auto-check",
				a.Tr("自动检查更新 (24h)", "Check automatically (24h)"),
				a.Tr("每天在后台自动检查最新发行版本", "Check for new releases daily in the background"),
				&a.AppUpdate.AutomaticChecks, nil).Changed() {
				a.SetUpdateAutomaticChecks(a.AppUpdate.AutomaticChecks)
			}

			components.SettingDivider(c, p)

			if components.SettingToggleItem(c, p, s, "set-ota-auto-dl",
				a.Tr("自动下载并安装更新", "Download & install automatically"),
				a.Tr("检测到新版本后在后台静默准备更新包", "Automatically prepare updates in the background when available"),
				&a.AppUpdate.AutomaticDownloads, nil).Changed() {
				a.SetUpdateAutomaticDownloads(a.AppUpdate.AutomaticDownloads)
			}

			components.SettingDivider(c, p)

			if components.SettingToggleItem(c, p, s, "set-ota-beta",
				a.Tr("接收预发布版 (Beta)", "Include pre-releases (Beta)"),
				a.Tr("抢先体验测试版新功能，稳定性可能低于正式版本", "Opt into preview builds ahead of stable releases"),
				&a.SettingsData.UpdatePrereleases, nil).Changed() {
				a.SetUpdatePrereleases(a.SettingsData.UpdatePrereleases)
			}
		},
	)
}

func settingsDataSection(c *ui.Context, a *app.App, p theme.Palette, s theme.TypeScale) {
	components.SettingsGroup(c, p, s, icons.Folder,
		a.Tr("数据目录与诊断", "Data & Diagnostics"),
		a.Tr("查看本地配置与日志存储路径，或预览当前编译后的运行时配置", "Inspect local storage paths or preview the compiled runtime configuration"),
		nil,
		func() {
			pathRow := func(label, path string) {
				ui.Row(c).FillWidth().Padding(9, 14).Gap(12).AlignItems(ui.Center).Children(func() {
					ui.Text(c, label).FontSize(s.SM).FontWeight(500).TextColor(p.ForegroundSubtle).Width(136).Shrink(0).SingleLine()
					ui.Text(c, path).FontSize(s.XS).Font(theme.MonoFont()).TextColor(p.Foreground).Grow(1).MinWidth(0).SingleLine()
				})
			}

			pathRow(a.Tr("配置数据目录", "Data Directory"), a.PM.DataDir())
			components.SettingDivider(c, p)
			pathRow(a.Tr("运行时配置文件", "Runtime Config"), a.PM.RuntimeConfigPath())
			if a.LogStore != nil {
				components.SettingDivider(c, p)
				pathRow(a.Tr("日志文件 (磁盘滚动)", "Log File (Disk Rotating)"), a.LogStore.Path())
			}

			components.SettingDivider(c, p)

			ui.Row(c).FillWidth().Padding(10, 14).Gap(8).AlignItems(ui.Center).Children(func() {
				if components.ActionButton(c, p, s, icons.Folder, a.Tr("打开数据目录", "Open Data Folder")).Clicked() {
					app.OpenFolderInOS(a.PM.DataDir())
				}
				if a.LogStore != nil {
					if components.ActionButton(c, p, s, icons.Logs, a.Tr("打开日志目录", "Open Logs Folder")).Clicked() {
						app.OpenFolderInOS(a.LogStore.Dir())
					}
				}
				ui.Spacer(c)
				if components.ActionButton(c, p, s, icons.Code, a.Tr("预览编译配置", "Inspect Runtime Config")).Clicked() {
					a.OpenRuntimePreview()
				}
			})
		},
	)
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

type boolOverride uint8

const (
	boolOverrideDefault boolOverride = iota
	boolOverrideOn
	boolOverrideOff
)

func pickCorePath(a *app.App, dir bool) {
	targetCore := model.NormalizeCoreType(a.SettingsData.CoreType)
	opts := mygo.OpenDialogOptions{
		Parent: a.Window(),
		Title:  fmt.Sprintf(a.Tr("选择 %s 可执行文件", "Select %s executable"), model.CoreDisplayName(targetCore)),
	}
	notifyMsg := a.Tr("已更新核心路径", "Core path updated")
	if dir {
		opts.Title = fmt.Sprintf(a.Tr("选择 %s 核心存放目录", "Select %s core directory"), model.CoreDisplayName(targetCore))
		opts.Directory = true
		opts.CreateDirectories = true
		notifyMsg = a.Tr("已更新核心目录", "Core directory updated")
	}
	paths, err := mygo.Dialog.Open(opts)
	if err != nil || len(paths) == 0 {
		return
	}
	a.UpdateUI(func() {
		a.SettingsData.SetCorePathFor(targetCore, paths[0])
		_ = a.PM.SaveSettings(a.SettingsData)
		if a.SettingsData.CoreType == targetCore {
			a.CoreInfo = a.CM.RefreshBinaryInfo(paths[0])
		}
		a.Notify(notifyMsg)
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
