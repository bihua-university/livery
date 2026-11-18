package pages

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/bihua-university/livery/internal/app"
	"github.com/bihua-university/livery/internal/icons"
	"github.com/bihua-university/livery/internal/model"
	"github.com/bihua-university/livery/internal/theme"
	"github.com/bihua-university/livery/internal/ui/components"
)

// Proxies renders the proxy toolbar and two-column accordion proxy group cards.
func Proxies(c *ui.Context, a *app.App, p theme.Palette, s theme.TypeScale) {
	ui.Column(c).Fill().Padding(16).Gap(12).MinHeight(0).Children(func() {
		// Top toolbar
		ui.Row(c).Gap(10).AlignItems(ui.Center).Wrap().Children(func() {
			modeIdx := 0
			switch a.SettingsData.ProxyMode {
			case model.ModeGlobal:
				modeIdx = 1
			case model.ModeDirect:
				modeIdx = 2
			}
			modes := []model.ProxyMode{model.ModeRule, model.ModeGlobal, model.ModeDirect}
			modeLabels := []string{a.Tr("规则模式", "Rule"), a.Tr("全局模式", "Global"), a.Tr("直连模式", "Direct")}
			if components.Segmented(c.Key("px-mode-seg"), p, s, &modeIdx, modeLabels).Changed() {
				a.SetProxyMode(modes[modeIdx])
			}

			ui.SearchField(c.Key("proxy-search"), &a.ProxySearch).
				Placeholder(a.Tr("搜索分组、节点或协议...", "Search groups or nodes...")).
				Width(220)

			sortLabels := []string{a.Tr("默认", "Default"), a.Tr("延迟", "Latency"), a.Tr("名称", "Name")}
			components.Segmented(c.Key("px-sort-seg"), p, s, &a.ProxySort, sortLabels)

			ui.Spacer(c)

			if a.CoreInfo.Status == model.CoreRunning && len(a.Groups) > 0 {
				expandedCount := 0
				for _, g := range a.Groups {
					if a.IsGroupExpanded(g.Name) {
						expandedCount++
					}
				}
				allExpanded := expandedCount == len(a.Groups)
				expandLbl := a.Tr("全部展开", "Expand All")
				if allExpanded {
					expandLbl = a.Tr("全部折叠", "Collapse All")
				}
				if components.ActionButton(c, p, s, icons.Layers, expandLbl).Clicked() {
					a.SetAllGroupsExpanded(!allExpanded)
				}

				if components.IconActionButton(c, p, icons.Refresh, a.Tr("刷新列表", "Refresh")).Clicked() {
					go func() {
						if groups, nodes, err := a.APIClient().GetProxies(context.Background()); err == nil {
							a.UpdateUI(func() {
								a.SetGroups(groups)
								a.Nodes = nodes
							})
						}
					}()
				}
			}
		})

		if a.CoreInfo.Status != model.CoreRunning {
			coreName := a.CoreDisplayName()
			startLbl := a.Tr("立即启动核心", "Start Core")
			if a.CoreRestarting {
				startLbl = a.Tr("重启中...", "Restarting...")
			} else if a.CoreStarting || a.CoreInfo.Status == model.CoreStarting {
				startLbl = a.Tr("启动中...", "Starting...")
			}
			components.EmptyState(c, p, s, icons.Proxies,
				coreName+a.Tr(" 核心未运行", " core is not running"),
				a.Tr("启动核心后可在此切换策略组与测试节点延迟。", "Start the core to switch outbound groups and measure node latency."),
				func() {
					if components.PrimaryActionButton(c, p, s, icons.Play, startLbl).Disabled(a.IsCoreBusy()).Clicked() {
						go a.StartCore()
					}
				},
			)
			return
		}

		if len(a.Groups) == 0 {
			ui.Column(c).Grow(1).Center().Gap(8).Children(func() {
				ui.Spinner(c)
				ui.Text(c, a.Tr("正在读取代理策略组...", "Loading proxy groups...")).FontSize(s.SM).TextColor(p.ForegroundSubtle)
			})
			return
		}

		groups := a.OrderedGroups()
		if len(a.ExpandedGroups) == 0 && len(groups) > 0 {
			if a.ExpandedGroups == nil {
				a.ExpandedGroups = make(map[string]bool)
			}
			a.ExpandedGroups[groups[0].Name] = true
		}

		q := strings.ToLower(strings.TrimSpace(a.ProxySearch))
		type groupEntry struct {
			group        model.ProxyGroup
			nodes        []string
			groupMatches bool
		}
		visible := make([]groupEntry, 0, len(groups))
		for _, g := range groups {
			groupMatches := q != "" && strings.Contains(strings.ToLower(g.Name), q)
			expanded := a.IsGroupExpanded(g.Name) || q != ""
			var nodes []string
			if expanded || q != "" {
				nodes = filteredSortedNodes(a, g, q, groupMatches)
			}
			if q != "" && !groupMatches && len(nodes) == 0 {
				continue
			}
			visible = append(visible, groupEntry{group: g, nodes: nodes, groupMatches: groupMatches})
		}

		ui.Scroll(c.Key("proxy-groups-scroll")).TrackScroll(&a.ProxiesScrolled).Grow(1).MinHeight(0).Children(func() {
			if len(visible) == 0 {
				ui.Box(c).Padding(24).Center().Children(func() {
					ui.Text(c, a.Tr("没有匹配的代理分组或节点", "No matching proxy groups or nodes")).
						FontSize(s.SM).TextColor(p.ForegroundSubtle)
				})
				return
			}

			if len(visible) == 1 {
				entry := visible[0]
				expanded := a.IsGroupExpanded(entry.group.Name) || q != ""
				proxyGroupAccordionCard(c, a, p, s, entry.group, entry.nodes, expanded, q, entry.groupMatches)
				return
			}

			// Split visible groups across two independent vertical columns so expanding
			// a group in one column never leaves whitespace beside it in the same row.
			leftCol := make([]groupEntry, 0, (len(visible)+1)/2)
			rightCol := make([]groupEntry, 0, len(visible)/2)
			for i, entry := range visible {
				if i%2 == 0 {
					leftCol = append(leftCol, entry)
				} else {
					rightCol = append(rightCol, entry)
				}
			}

			ui.Row(c).Gap(12).AlignItems(ui.Start).Children(func() {
				ui.Column(c).Grow(1).MinWidth(0).Gap(10).Children(func() {
					for _, entry := range leftCol {
						expanded := a.IsGroupExpanded(entry.group.Name) || q != ""
						proxyGroupAccordionCard(c, a, p, s, entry.group, entry.nodes, expanded, q, entry.groupMatches)
					}
				})
				ui.Column(c).Grow(1).MinWidth(0).Gap(10).Children(func() {
					for _, entry := range rightCol {
						expanded := a.IsGroupExpanded(entry.group.Name) || q != ""
						proxyGroupAccordionCard(c, a, p, s, entry.group, entry.nodes, expanded, q, entry.groupMatches)
					}
				})
			})
		})
	})
}

func proxyGroupAccordionCard(c *ui.Context, a *app.App, p theme.Palette, s theme.TypeScale, g model.ProxyGroup, nodes []string, expanded bool, q string, groupMatches bool) {
	isSelector := strings.EqualFold(g.Type, "Selector")
	gName := g.Name

	card := ui.Column(c.Key("grp-card-"+gName)).
		FillWidth().
		Background(p.Card).
		Border(1, p.CardBorder).
		Radius(theme.RadiusXL).
		Gap(0).
		Clip()

	target := float32(0)
	if expanded {
		target = 1
	}
	progress := card.Animate("expand", target, 220*time.Millisecond)

	card.Children(func() {
		// Card header row: entire strip highlights on hover and toggles expand/collapse.
		header := ui.ButtonBase(c.Key("grp-toggle-"+gName)).
			Row().
			FillWidth().
			Padding(10, 12).
			Gap(10).
			AlignItems(ui.Center).
			Label(gName).
			Cursor(ui.CursorPointer)
		if header.Hovered() {
			header.Background(p.SurfaceHover)
		}
		header.OnClick(func() {
			a.SelectedGroup = gName
			a.ToggleGroupExpanded(gName)
		})
		header.Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
				ui.Text(c, gName).
					FontSize(s.Base).
					FontWeight(600).
					TextColor(p.Foreground).
					SingleLine()

				sub := g.Type
				if g.Now != "" {
					sub = g.Now
				}
				ui.Text(c, sub).
					FontSize(s.XS).
					TextColor(p.ForegroundSubtle).
					SingleLine()
			})

			if expanded {
				testBtn := ui.ButtonBase(c.Key("grp-test-"+gName)).
					Size(26, 26).
					Center().
					Radius(theme.RadiusFull).
					Background(p.Surface).
					Label(a.Tr("组测速", "Test Group") + " " + gName).
					Tooltip(a.Tr("组测速", "Test Group")).
					Cursor(ui.CursorPointer)
				if testBtn.Hovered() {
					testBtn.Background(p.Hover)
				}
				testBtn.OnClick(func() { a.TestGroupDelay(gName) })
				testBtn.Children(func() {
					ui.Icon(c, icons.Zap).FontSize(13).TextColor(p.ForegroundSubtle)
				})
			} else {
				ui.Text(c, g.Type).
					FontSize(s.XS).
					Font(theme.MonoFont()).
					TextColor(p.ForegroundSubtlest).
					Padding(0, 4).
					SingleLine()
			}

			ui.Box(c).
				Size(26, 26).
				Center().
				Radius(theme.RadiusFull).
				Background(p.Surface).
				Children(func() {
					ui.Icon(c, icons.ChevronDown).
						FontSize(14).
						TextColor(p.ForegroundSubtle).
						Rotate(180 * progress)
				})
		})

		if !expanded && progress == 0 {
			return
		}
		if nodes == nil {
			nodes = filteredSortedNodes(a, g, q, groupMatches)
		}

		clip := ui.Column(c.Key("grp-clip-" + gName)).Shrink(0).FillWidth()
		clip.Children(func() {
			inner := ui.Column(c.Key("grp-body-"+gName)).
				Shrink(0).
				FillWidth().
				Padding(2, 12, 12, 12)
			inner.Children(func() {
				if len(nodes) == 0 {
					ui.Box(c).Padding(12).Center().Children(func() {
						ui.Text(c, a.Tr("该分组下没有匹配的节点", "No matching nodes in this group")).
							FontSize(s.SM).TextColor(p.ForegroundSubtle)
					})
					return
				}
				ui.Grid(c).Columns(2).Gap(8).Children(func() {
					for _, nodeName := range nodes {
						node, ok := a.Nodes[nodeName]
						if !ok {
							node = model.ProxyNode{Name: nodeName, Type: "Outbound", Delay: -1}
						}
						selected := g.Now == nodeName
						proxyNodeTile(c, a, p, s, gName, node, selected, isSelector)
					}
				})
			})
			if progress < 1 {
				innerH := inner.Bounds().H
				clip.ClipY().Height(progress * innerH)
				inner.Opacity(0.15 + 0.85*progress)
			}
		})
	})
}

func filteredSortedNodes(a *app.App, g model.ProxyGroup, q string, includeAllOnGroupMatch bool) []string {
	if q == "" && a.ProxySort == 0 {
		return g.All
	}
	out := make([]string, 0, len(g.All))
	if q == "" {
		out = append(out, g.All...)
	} else {
		for _, name := range g.All {
			n := a.Nodes[name]
			if !strings.Contains(strings.ToLower(name), q) && !strings.Contains(strings.ToLower(n.Type), q) {
				continue
			}
			out = append(out, name)
		}
		if len(out) == 0 && includeAllOnGroupMatch {
			if a.ProxySort == 0 {
				return g.All
			}
			out = append(out, g.All...)
		}
	}
	switch a.ProxySort {
	case 1:
		sort.SliceStable(out, func(i, j int) bool {
			di := a.Nodes[out[i]].Delay
			dj := a.Nodes[out[j]].Delay
			if di <= 0 {
				di = 999999
			}
			if dj <= 0 {
				dj = 999999
			}
			if di != dj {
				return di < dj
			}
			return out[i] < out[j]
		})
	case 2:
		sort.Strings(out)
	}
	return out
}

func proxyNodeTile(c *ui.Context, a *app.App, p theme.Palette, s theme.TypeScale, groupName string, node model.ProxyNode, selected, selectable bool) {
	tile := ui.ButtonBase(c.Key("node-"+groupName+"-"+node.Name)).
		Column().
		Padding(10, 12).
		Gap(10).
		Radius(theme.RadiusLG).
		Border(1, p.Border).
		Label(node.Name)
	if selectable {
		tile.Cursor(ui.CursorPointer)
	}
	if selected {
		tile.Background(p.CardSelected).BorderColor(p.BorderHover)
	} else {
		tile.Background(p.Surface)
		if tile.Hovered() && selectable {
			tile.Background(p.SurfaceHover).BorderColor(p.BorderHover)
		}
	}
	if selectable {
		gName, nName := groupName, node.Name
		tile.OnClick(func() { a.SelectProxy(gName, nName) })
	}

	tile.Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Text(c, node.Name).
				FontSize(s.SM).
				FontWeight(500).
				TextColor(p.Foreground).
				Grow(1).
				SingleLine()
			if selected {
				ui.Box(c).
					Size(18, 18).
					Radius(theme.RadiusFull).
					Background(ui.Hex("#3B82F6")).
					Center().
					Children(func() {
						ui.Icon(c, icons.Check).FontSize(11).TextColor(ui.Hex("#FFFFFF"))
					})
			}
		})

		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			proto := node.Type
			if node.UDP {
				proto += " · UDP"
			}
			ui.Text(c, proto).
				FontSize(s.XS).
				TextColor(p.ForegroundSubtle).
				Grow(1).
				SingleLine()

			nName := node.Name
			if node.Testing || node.Delay >= 0 {
				delayText := components.FormatLatency(node.Delay, node.Testing, a.Tr("超时", "Timeout"))
				_, dFg := components.LatencyColors(p, node.Delay, node.Testing)
				if node.Delay > 0 && node.Delay < 150 && !node.Testing {
					dFg = p.Success
				}
				delayBtn := ui.ButtonBase(c.Key("node-lat-"+groupName+"-"+nName)).
					Padding(1, 4).
					Radius(theme.RadiusSM).
					Cursor(ui.CursorPointer).
					Label(a.Tr("点击测速 ", "Test ") + nName).
					Tooltip(a.Tr("点击测速", "Click to test latency"))
				if delayBtn.Hovered() {
					delayBtn.Background(p.Hover)
				}
				delayBtn.OnClick(func() { a.TestNodeDelay(nName) })
				delayBtn.Children(func() {
					ui.Text(c, delayText).
						FontSize(s.XS).
						Font(theme.MonoFont()).
						FontFeatures("tnum").
						TextColor(dFg).
						SingleLine()
				})
			} else {
				zapBtn := ui.ButtonBase(c.Key("node-zap-"+groupName+"-"+nName)).
					Size(20, 20).
					Center().
					Radius(theme.RadiusSM).
					Cursor(ui.CursorPointer).
					Label(a.Tr("点击测速 ", "Test ") + nName).
					Tooltip(a.Tr("点击测速", "Click to test latency"))
				if zapBtn.Hovered() {
					zapBtn.Background(p.Hover)
				}
				zapBtn.OnClick(func() { a.TestNodeDelay(nName) })
				zapBtn.Children(func() {
					ui.Icon(c, icons.Zap).FontSize(12).TextColor(p.IconBlue)
				})
			}
		})
	})
}
