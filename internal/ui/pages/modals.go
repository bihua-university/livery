package pages

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"github.com/bihua-university/livery/internal/app"
	"github.com/bihua-university/livery/internal/icons"
	"github.com/bihua-university/livery/internal/model"
	"github.com/bihua-university/livery/internal/profile"
	"github.com/bihua-university/livery/internal/theme"
	"github.com/bihua-university/livery/internal/ui/components"
)

// Modals renders all active modal dialogs and confirmation alerts.
func Modals(c *ui.Context, a *app.App, p theme.Palette, s theme.TypeScale) {
	importModal(c, a, p, s)
	newProfileModal(c, a, p, s)
	editorModal(c, a, p, s)
	metaModal(c, a, p, s)
	ruleModal(c, a, p, s)
	connDetailModal(c, a, p, s)
	qualityModal(c, a, p, s)

	btn := ui.AlertDialog(c, &a.ShowDeleteAlert,
		a.Tr("删除配置", "Delete Profile"),
		a.Tr("确定要删除此配置文件吗？此操作无法撤销。", "Are you sure you want to delete this profile? This cannot be undone."),
		a.Tr("取消", "Cancel"),
		a.Tr("删除", "Delete"),
	)
	if btn == 1 {
		if err := a.PM.Delete(a.DeleteTargetID); err != nil {
			a.Notify(err.Error())
		} else {
			a.Profiles = a.PM.Profiles()
			a.SettingsData = a.PM.Settings()
			a.Notify(a.Tr("已删除配置", "Profile deleted"))
		}
	}
}

func importModal(c *ui.Context, a *app.App, p theme.Palette, s theme.TypeScale) {
	if !a.ShowImportModal {
		return
	}
	components.ModalShell(c, p, &a.ShowImportModal, 460, func() {
		ui.Text(c, a.Tr("导入订阅链接", "Import Subscription URL")).FontSize(s.LG).FontWeight(600).TextColor(p.Foreground)
		ui.Column(c).Gap(8).Children(func() {
			ui.Text(c, a.Tr("订阅链接 (支持 sing-box JSON / Clash YAML / Base64)", "Subscription URL (sing-box JSON / Clash YAML / Base64)")).FontSize(s.SM).TextColor(p.ForegroundSubtle)
			ui.TextInput(c.Key("imp-url"), &a.ImportURL).Placeholder("https://example.com/sub").AutoFocus().FillWidth()

			ui.Text(c, a.Tr("配置名称 (可选)", "Profile Name (optional)")).FontSize(s.SM).TextColor(p.ForegroundSubtle)
			ui.TextInput(c.Key("imp-name"), &a.ImportName).Placeholder(a.Tr("留空则自动识别", "Auto-detect if blank")).FillWidth()

			ui.Text(c, "User-Agent").FontSize(s.SM).TextColor(p.ForegroundSubtle)
			uaPlaceholder := "mihomo/1.19.0 (clash.meta; Livery)"
			if model.NormalizeCoreType(a.SettingsData.CoreType) == model.CoreSingBox {
				uaPlaceholder = "sing-box 1.11.0 (Livery)"
			}
			ui.TextInput(c.Key("imp-ua"), &a.ImportUA).Placeholder(uaPlaceholder).FillWidth()

			ui.Checkbox(c.Key("imp-proxy"), &a.ImportUseProxy, a.Tr("通过当前本地代理端口下载", "Download via local proxy port"))
		})
		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			if components.ActionButton(c, p, s, nil, a.Tr("取消", "Cancel")).Clicked() {
				a.ShowImportModal = false
			}
			btnLabel := a.Tr("导入", "Import")
			if a.Importing {
				btnLabel = a.Tr("下载中...", "Importing...")
			}
			if components.PrimaryActionButton(c, p, s, icons.Download, btnLabel).Disabled(a.Importing).Clicked() {
				u, name, ua, useP := a.ImportURL, a.ImportName, a.ImportUA, a.ImportUseProxy
				a.Importing = true
				go func() {
					prof, err := a.PM.ImportRemote(context.Background(), name, u, ua, useP)
					a.UpdateUI(func() {
						a.Importing = false
						if err != nil {
							a.Notify(a.Tr("导入失败: ", "Import failed: ") + err.Error())
							return
						}
						a.Profiles = a.PM.Profiles()
						a.ShowImportModal = false
						a.ActivateProfile(prof.ID)
					})
				}()
			}
		})
	})
}

func newProfileModal(c *ui.Context, a *app.App, p theme.Palette, s theme.TypeScale) {
	if !a.ShowNewProfileModal {
		return
	}
	components.ModalShell(c, p, &a.ShowNewProfileModal, 380, func() {
		ui.Text(c, a.Tr("新建空白配置", "New Blank Profile")).FontSize(s.LG).FontWeight(600).TextColor(p.Foreground)
		ui.TextInput(c.Key("new-prof-name"), &a.NewProfileName).Placeholder(a.Tr("配置名称", "Profile name")).AutoFocus().FillWidth()
		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			if components.ActionButton(c, p, s, nil, a.Tr("取消", "Cancel")).Clicked() {
				a.ShowNewProfileModal = false
			}
			if components.PrimaryActionButton(c, p, s, icons.Plus, a.Tr("创建", "Create")).Clicked() {
				prof, err := a.PM.CreateBlank(a.NewProfileName)
				if err != nil {
					a.Notify(err.Error())
				} else {
					a.Profiles = a.PM.Profiles()
					a.ShowNewProfileModal = false
					a.OpenProfileEditor(prof.ID)
				}
			}
		})
	})
}

func editorModal(c *ui.Context, a *app.App, p theme.Palette, s theme.TypeScale) {
	if !a.ShowEditorModal || a.EditorState == nil {
		return
	}
	components.ModalShell(c, p, &a.ShowEditorModal, 780, func() {
		ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Text(c, a.EditorTitle).FontSize(s.LG).FontWeight(600).TextColor(p.Foreground).Grow(1)
			if a.EditorError != "" {
				components.StatusChip(c, s, a.EditorError, p.DestructiveSurface, p.DestructiveSurfaceFg, p.Destructive)
			}
		})
		ui.Box(c).Height(420).FillWidth().Radius(theme.RadiusXL).Border(1, p.Border).Clip().Children(func() {
			ed := editor.View(c, a.EditorState).Fill()
			if a.EditorReadOnly {
				ed.ReadOnly(true)
			}
		})
		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			if components.ActionButton(c, p, s, nil, a.Tr("关闭", "Close")).Clicked() {
				a.ShowEditorModal = false
			}
			if !a.EditorReadOnly {
				if components.PrimaryActionButton(c, p, s, icons.Check, a.Tr("校验并保存", "Validate & Save")).Clicked() {
					content := []byte(a.EditorState.Buffer.String())
					if err := a.PM.WriteProfileRaw(a.EditingProfileID, content); err != nil {
						a.EditorError = err.Error()
						return
					}
					if a.CoreInfo.Found {
						pattern := "livery-check-*.yaml"
						if model.NormalizeCoreType(a.SettingsData.CoreType) == model.CoreSingBox {
							pattern = "livery-check-*.json"
						}
						if tmp, err := os.CreateTemp("", pattern); err == nil {
							tmpPath := tmp.Name()
							_, _ = tmp.Write(content)
							_ = tmp.Close()
							defer os.Remove(tmpPath)
							if prof, ok := a.FindProfile(a.EditingProfileID); ok {
								compiled, cErr := profile.CompileForCore(content, prof, a.SettingsData)
								if cErr != nil {
									a.EditorError = cErr.Error()
									return
								}
								_ = os.WriteFile(tmpPath, compiled, 0o644)
							}
							if checkErr := a.CM.CheckConfig(tmpPath); checkErr != nil {
								a.EditorError = checkErr.Error()
								return
							}
						}
					}
					a.Profiles = a.PM.Profiles()
					a.ShowEditorModal = false
					a.Notify(a.Tr("配置已保存", "Profile saved"))
					if a.EditingProfileID == a.SettingsData.ActiveProfileID && a.CoreInfo.Status == model.CoreRunning {
						a.ApplyProfileChanges()
					}
				}
			}
		})
	})
}

func metaModal(c *ui.Context, a *app.App, p theme.Palette, s theme.TypeScale) {
	if !a.ShowMetaModal {
		return
	}
	components.ModalShell(c, p, &a.ShowMetaModal, 480, func() {
		ui.Text(c, a.Tr("配置属性与覆写", "Profile Properties & Overrides")).FontSize(s.LG).FontWeight(600).TextColor(p.Foreground)
		ui.Column(c).Gap(10).Children(func() {
			ui.Text(c, a.Tr("名称", "Name")).FontSize(s.SM).TextColor(p.ForegroundSubtle)
			ui.TextInput(c.Key("meta-name"), &a.MetaDraft.Name).FillWidth()

			if a.MetaDraft.Kind == "remote" {
				ui.Text(c, a.Tr("订阅链接", "Subscription URL")).FontSize(s.SM).TextColor(p.ForegroundSubtle)
				ui.TextInput(c.Key("meta-url"), &a.MetaDraft.URL).FillWidth()

				ui.Checkbox(c.Key("meta-autoup"), &a.MetaDraft.AutoUpdate, a.Tr("定期自动更新订阅", "Auto-update subscription"))
				ui.Checkbox(c.Key("meta-proxyup"), &a.MetaDraft.UseProxyUpdate, a.Tr("通过本地代理端口更新", "Update via local proxy"))
			}

			ui.Divider(c)
			ui.Text(c, a.Tr("运行时覆写 (Mixin)", "Runtime Overrides (Mixin)")).FontSize(s.SM).FontWeight(600).TextColor(p.Foreground)
			ui.Checkbox(c.Key("meta-ov-in"), &a.MetaDraft.Override.OverrideInbounds, a.Tr("接管 Mixed 入站端口与局域网设置", "Override mixed inbound & LAN settings"))
			ui.Checkbox(c.Key("meta-ov-api"), &a.MetaDraft.Override.InjectClashAPI, a.Tr("注入 Clash API 控制器 (用于测速与监控)", "Inject Clash API controller (telemetry & delay test)"))
			ui.Checkbox(c.Key("meta-ov-log"), &a.MetaDraft.Override.OverrideLog, a.Tr("应用全局日志级别设置", "Apply global log level"))

			ui.Text(c, a.Tr("自定义 JSON/YAML Merge Patch (可选)", "Custom JSON/YAML Merge Patch (optional)")).FontSize(s.SM).TextColor(p.ForegroundSubtle)
			ui.TextArea(c.Key("meta-patch"), &a.MetaDraft.Override.PatchJSON).Placeholder(`{"dns": {"strategy": "prefer_ipv4"}}`).Height(90).FillWidth()
		})
		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			if components.ActionButton(c, p, s, nil, a.Tr("取消", "Cancel")).Clicked() {
				a.ShowMetaModal = false
			}
			if components.PrimaryActionButton(c, p, s, icons.Check, a.Tr("保存", "Save")).Clicked() {
				if err := a.PM.UpdateProfileMeta(a.MetaDraft); err != nil {
					a.Notify(err.Error())
					return
				}
				a.Profiles = a.PM.Profiles()
				a.ShowMetaModal = false
				a.Notify(a.Tr("配置属性已保存", "Profile updated"))
				if a.MetaDraft.ID == a.SettingsData.ActiveProfileID && a.CoreInfo.Status == model.CoreRunning {
					a.ApplyProfileChanges()
				}
			}
		})
	})
}

func ruleModal(c *ui.Context, a *app.App, p theme.Palette, s theme.TypeScale) {
	if !a.ShowRuleModal {
		return
	}
	components.ModalShell(c, p, &a.ShowRuleModal, 440, func() {
		ui.Text(c, a.Tr("添加自定义规则", "Add Custom Rule")).FontSize(s.LG).FontWeight(600).TextColor(p.Foreground)
		ui.Column(c).Gap(10).Children(func() {
			ui.Text(c, a.Tr("匹配类型", "Match Type")).FontSize(s.SM).TextColor(p.ForegroundSubtle)
			ui.Select(c.Key("rule-type-sel"), &a.DraftRuleType, []string{
				"domain_suffix", "domain", "domain_keyword", "ip_cidr", "process_name",
			}).FillWidth()

			ui.Text(c, a.Tr("匹配内容 (支持逗号分隔多个)", "Pattern (comma-separated allowed)")).FontSize(s.SM).TextColor(p.ForegroundSubtle)
			ui.TextInput(c.Key("rule-pat-in"), &a.DraftRulePat).Placeholder("example.com, openai.com").AutoFocus().FillWidth()

			ui.Text(c, a.Tr("目标出站 (proxy / direct / block / 组名)", "Target Outbound")).FontSize(s.SM).TextColor(p.ForegroundSubtle)
			ui.TextInput(c.Key("rule-out-in"), &a.DraftRuleOut).Placeholder("direct").FillWidth()

			ui.Checkbox(c.Key("rule-prep-cb"), &a.DraftRulePrep, a.Tr("优先于订阅规则匹配 (前置)", "Higher priority than profile rules (Prepend)"))
		})
		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			if components.ActionButton(c, p, s, nil, a.Tr("取消", "Cancel")).Clicked() {
				a.ShowRuleModal = false
			}
			if components.PrimaryActionButton(c, p, s, icons.Plus, a.Tr("添加并应用", "Add & Apply")).Disabled(a.SavingPreference).Clicked() {
				if strings.TrimSpace(a.DraftRulePat) == "" || strings.TrimSpace(a.DraftRuleOut) == "" {
					a.Notify(a.Tr("请填写匹配内容与目标出站", "Pattern and outbound are required"))
					return
				}
				a.AddCustomRule(model.CustomRule{
					Enabled:   true,
					MatchType: a.DraftRuleType,
					Pattern:   strings.TrimSpace(a.DraftRulePat),
					Outbound:  strings.TrimSpace(a.DraftRuleOut),
					Prepend:   a.DraftRulePrep,
				})
			}
		})
	})
}

func connDetailModal(c *ui.Context, a *app.App, p theme.Palette, s theme.TypeScale) {
	if !a.ShowConnModal {
		return
	}
	var target *model.Connection
	for i := range a.Conns {
		if a.Conns[i].ID == a.SelectedConnID {
			target = &a.Conns[i]
			break
		}
	}
	if target == nil {
		for i := range a.ClosedConns {
			if a.ClosedConns[i].ID == a.SelectedConnID {
				target = &a.ClosedConns[i]
				break
			}
		}
	}
	if target == nil {
		a.ShowConnModal = false
		return
	}
	conn := *target
	components.ModalShell(c, p, &a.ShowConnModal, 520, func() {
		ui.Text(c, a.Tr("连接详情", "Connection Details")).FontSize(s.LG).FontWeight(600).TextColor(p.Foreground)
		ui.Column(c).Gap(8).Children(func() {
			components.KeyValueRow(c, p, s, "ID", conn.ID, true)
			components.KeyValueRow(c, p, s, a.Tr("域名", "Host"), conn.Metadata.Host, true)
			components.KeyValueRow(c, p, s, a.Tr("目标 IP", "Destination"), conn.Metadata.DestinationIP+":"+conn.Metadata.DestinationPort, true)
			components.KeyValueRow(c, p, s, a.Tr("来源", "Source"), conn.Metadata.SourceIP+":"+conn.Metadata.SourcePort, true)
			components.KeyValueRow(c, p, s, a.Tr("协议", "Network/Type"), conn.Metadata.Network+" / "+conn.Metadata.Type, true)
			if conn.Metadata.ProcessPath != "" {
				components.KeyValueRow(c, p, s, a.Tr("进程", "Process"), filepath.Base(conn.Metadata.ProcessPath), true)
			}
			components.KeyValueRow(c, p, s, a.Tr("匹配规则", "Rule"), conn.Rule+" ("+conn.RulePayload+")", true)
			components.KeyValueRow(c, p, s, a.Tr("代理链", "Chains"), strings.Join(conn.Chains, " → "), true)
			components.KeyValueRow(c, p, s, a.Tr("流量", "Traffic"), fmt.Sprintf("↓ %s  ↑ %s", components.FormatBytes(conn.Download), components.FormatBytes(conn.Upload)), true)
		})
		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			if components.ActionButton(c, p, s, nil, a.Tr("关闭窗口", "Close")).Clicked() {
				a.ShowConnModal = false
			}
			if a.ConnFilterTab == 0 {
				id := conn.ID
				if components.PrimaryActionButton(c, p, s, icons.X, a.Tr("断开此连接", "Close Connection")).Clicked() {
					a.ShowConnModal = false
					go func() {
						_ = a.APIClient().CloseConnection(context.Background(), id)
					}()
				}
			}
		})
	})
}
