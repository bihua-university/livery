package pages

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/bihua-university/livery/internal/app"
	"github.com/bihua-university/livery/internal/icons"
	"github.com/bihua-university/livery/internal/model"
	"github.com/bihua-university/livery/internal/theme"
	"github.com/bihua-university/livery/internal/ui/components"
)

// Rules renders custom override rules and active sing-box routing rules.
func Rules(c *ui.Context, a *app.App, p theme.Palette, s theme.TypeScale) {
	a.SyncRulesForView()

	ui.Column(c).Fill().Padding(16).Gap(12).MinHeight(0).Children(func() {
		ui.Row(c).Gap(10).AlignItems(ui.Center).Wrap().Children(func() {
			ui.SearchField(c.Key("rule-search"), &a.RuleSearch).
				Placeholder(a.Tr("搜索规则类型、内容或出站...", "Search rule type, pattern, or outbound...")).
				Width(280)
			ui.Spacer(c)
			if components.IconActionButton(c, p, icons.Refresh, a.Tr("刷新规则", "Refresh Rules")).Disabled(a.RulesLoading).Clicked() {
				a.RefreshRulesNow()
			}
			if components.PrimaryActionButton(c, p, s, icons.Plus, a.Tr("添加自定义规则", "Add Custom Rule")).Clicked() {
				a.DraftRuleType = "domain_suffix"
				a.DraftRulePat = ""
				a.DraftRuleOut = "direct"
				a.DraftRulePrep = true
				a.ShowRuleModal = true
			}
		})

		if len(a.SettingsData.CustomRules) > 0 {
			components.Card(c, p, func() {
				components.SectionHeader(c, p, s, a.Tr("自定义覆写规则", "Custom Override Rules"), func() {
					ui.Text(c, fmt.Sprintf("%d", len(a.SettingsData.CustomRules))).FontSize(s.XS).Font(theme.MonoFont()).TextColor(p.ForegroundSubtle)
				})
				ui.Column(c).Gap(6).Children(func() {
					for i, cr := range a.SettingsData.CustomRules {
						idx := i
						r := cr
						ui.Row(c.Key("crule-"+r.ID)).
							Background(p.Surface).
							Border(1, p.Border).
							Radius(theme.RadiusLG).
							Padding(6, 10).
							Gap(10).
							AlignItems(ui.Center).
							Children(func() {
								en := r.Enabled
								if components.Toggle(c.Key("crule-sw-"+r.ID), p, &en).Label(r.Pattern).Changed() {
									a.SettingsData.CustomRules[idx].Enabled = en
									_ = a.PM.SaveSettings(a.SettingsData)
									a.ReloadRulesFromActiveProfile()
									a.ApplyProfileChanges()
								}
								prio := a.Tr("前置", "Prepend")
								if !r.Prepend {
									prio = a.Tr("追加", "Append")
								}
								components.TagChip(c, p, s, prio)
								components.TagChip(c, p, s, r.MatchType)
								ui.Text(c, r.Pattern).FontSize(s.SM).Font(theme.MonoFont()).TextColor(p.Foreground).Grow(1).SingleLine()
								ui.Text(c, "→ "+r.Outbound).FontSize(s.SM).Font(theme.MonoFont()).TextColor(p.IconBlue).SingleLine()
								components.IconActionButton(c, p, icons.Trash, a.Tr("删除", "Delete")).OnClick(func() {
									a.DeleteCustomRuleAt(idx)
								})
							})
					}
				})
			})
		}

		filtered := filteredRules(a)
		ui.Column(c).Grow(1).MinHeight(0).Background(p.Card).Border(1, p.CardBorder).Radius(theme.RadiusXL).Clip().Children(func() {
			ui.Row(c).Padding(10, 14).Gap(8).Background(p.PopoverHead).BorderWidth(0, 0, 1, 0).BorderColor(p.Border).AlignItems(ui.Center).Children(func() {
				ui.Text(c, a.Tr("生效路由规则", "Active Routing Rules")).FontSize(s.SM).FontWeight(600).TextColor(p.Foreground).Grow(1)
				if a.RulesLoading && a.IsRulesLoaded() {
					ui.Spinner(c).FontSize(13).TextColor(p.Primary).Label(a.Tr("正在刷新规则", "Refreshing rules"))
				}
				if a.IsRulesLoaded() {
					ui.Text(c, fmt.Sprintf("%d %s", len(filtered), a.Tr("条", "rules"))).
						FontSize(s.XS).Font(theme.MonoFont()).FontFeatures("tnum").TextColor(p.ForegroundSubtle)
				}
			})
			if a.RulesLoading && !a.IsRulesLoaded() {
				components.LoadingState(c, p, s,
					a.Tr("正在加载分流规则...", "Loading routing rules..."),
					a.Tr("按需读取当前配置与核心生效规则", "Loading active profile and core rules on demand"))
				return
			}
			if len(filtered) == 0 {
				components.EmptyState(c, p, s, icons.Rules, a.Tr("暂无匹配规则", "No matching rules"), "", nil)
				return
			}
			a.RuleListState.Key = func(i int) any { return fmt.Sprintf("%d-%s-%s", i, filtered[i].Type, filtered[i].Payload) }
			ui.List(c.Key("rules-list"), &a.RuleListState, len(filtered), func(i int) {
				r := filtered[i]
				ui.Row(c).Padding(7, 14).Gap(10).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(p.Border).Children(func() {
					components.TagChip(c, p, s, r.Type).Shrink(0)
					ui.Text(c, r.Payload).FontSize(s.SM).Font(theme.MonoFont()).TextColor(p.Foreground).Grow(1).MinWidth(0).SingleLine()
					outColor := p.IconBlue
					if strings.EqualFold(r.Proxy, "direct") {
						outColor = p.Success
					} else if strings.EqualFold(r.Proxy, "reject") || strings.EqualFold(r.Proxy, "block") {
						outColor = p.Destructive
					}
					ui.Text(c, r.Proxy).FontSize(s.SM).Font(theme.MonoFont()).FontWeight(500).TextColor(outColor).SingleLine()
				})
			}).Grow(1)
		})
	})
}

func filteredRules(a *app.App) []model.RuleEntry {
	q := strings.ToLower(strings.TrimSpace(a.RuleSearch))
	if q == "" {
		return a.Rules
	}
	var out []model.RuleEntry
	for _, r := range a.Rules {
		if strings.Contains(strings.ToLower(r.Type), q) ||
			strings.Contains(strings.ToLower(r.Payload), q) ||
			strings.Contains(strings.ToLower(r.Proxy), q) {
			out = append(out, r)
		}
	}
	return out
}
