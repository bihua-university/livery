package pages

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/bihua-university/livery/internal/app"
	"github.com/bihua-university/livery/internal/icons"
	"github.com/bihua-university/livery/internal/model"
	"github.com/bihua-university/livery/internal/theme"
	"github.com/bihua-university/livery/internal/ui/components"
)

// Connections renders the active and closed network connections table.
func Connections(c *ui.Context, a *app.App, p theme.Palette, s theme.TypeScale) {
	ui.Column(c).Fill().Padding(16).Gap(12).MinHeight(0).Children(func() {
		ui.Row(c).Gap(10).AlignItems(ui.Center).Wrap().Children(func() {
			tabLabels := []string{
				fmt.Sprintf("%s (%d)", a.Tr("活跃连接", "Active"), len(a.Conns)),
				fmt.Sprintf("%s (%d)", a.Tr("已关闭", "Closed"), len(a.ClosedConns)),
			}
			components.Segmented(c.Key("conn-tab-seg"), p, s, &a.ConnFilterTab, tabLabels)

			ui.SearchField(c.Key("conn-search"), &a.ConnSearch).
				Placeholder(a.Tr("搜索域名、IP、规则或链...", "Filter host, IP, rule, chain...")).
				Width(260)

			ui.Spacer(c)

			ui.Text(c, fmt.Sprintf("↓ %s   ↑ %s", components.FormatBytes(a.TotalDown), components.FormatBytes(a.TotalUp))).
				FontSize(s.SM).Font(theme.MonoFont()).FontFeatures("tnum").TextColor(p.ForegroundSubtle)

			pauseLbl := a.Tr("暂停", "Pause")
			pauseIcon := icons.Pause
			if a.ConnPaused {
				pauseLbl = a.Tr("继续", "Resume")
				pauseIcon = icons.Play
			}
			if components.ActionButton(c, p, s, pauseIcon, pauseLbl).Clicked() {
				a.ConnPaused = !a.ConnPaused
			}
			if components.ActionButton(c, p, s, icons.Trash, a.Tr("断开全部", "Close All")).Clicked() {
				go func() {
					if err := a.APIClient().CloseAllConnections(context.Background()); err != nil {
						a.UpdateUI(func() { a.Notify(err.Error()) })
						return
					}
					a.UpdateUI(func() {
						a.Conns = nil
						a.Notify(a.Tr("已断开全部活跃连接", "Closed all active connections"))
					})
				}()
			}
		})

		list := filteredConnections(a)

		tableTheme := theme.BuildTableTheme(p, c.Theme())

		ui.Themed(c, tableTheme).Grow(1).MinHeight(0).Background(p.Card).Border(1, p.CardBorder).Radius(theme.RadiusXL).Clip().Children(func() {
			if len(list) == 0 {
				msg := a.Tr("暂无匹配连接", "No matching connections")
				if a.CoreInfo.Status != model.CoreRunning {
					msg = a.Tr("核心未运行", "Core is not running")
				}
				components.EmptyState(c, p, s, icons.Connections, msg, "", nil)
				return
			}

			cols := []ui.TableColumn{
				{Title: a.Tr("目标地址", "Host"), MinWidth: 150},
				{Title: a.Tr("类型", "Type"), Width: 96},
				{Title: a.Tr("规则", "Rule"), Width: 150},
				{Title: a.Tr("节点链", "Chains"), Width: 170},
				{Title: a.Tr("下行", "Down"), Width: 88, Align: ui.End},
				{Title: a.Tr("上行", "Up"), Width: 88, Align: ui.End},
				{Title: a.Tr("时长", "Time"), Width: 90, Align: ui.End},
			}
			selIdx := -1
			for i, item := range list {
				if item.ID == a.SelectedConnID {
					selIdx = i
					break
				}
			}
			a.ConnTableState.Selected = &selIdx
			a.ConnTableState.RowHeight = 32
			a.ConnTableState.CellsStyle = ui.CellsStyle{
				Row: tableTheme.Accent,
			}
			a.ConnTableState.Key = func(i int) any { return list[i].ID }

			tbl := ui.Table(c.Key("conn-table"), &a.ConnTableState, cols, len(list), func(row, col int) {
				conn := list[row]
				switch col {
				case 0:
					host := conn.Metadata.Host
					if host == "" {
						host = conn.Metadata.DestinationIP
					}
					if conn.Metadata.DestinationPort != "" {
						host = host + ":" + conn.Metadata.DestinationPort
					}
					ui.Text(c, host).FontSize(s.SM).Font(theme.MonoFont()).TextColor(p.Foreground).SingleLine()
				case 1:
					typ := strings.ToUpper(conn.Metadata.Network)
					if conn.Metadata.Type != "" {
						typ = typ + "/" + conn.Metadata.Type
					}
					ui.Text(c, typ).FontSize(s.XS).Font(theme.MonoFont()).TextColor(p.ForegroundSubtle).SingleLine()
				case 2:
					rule := conn.Rule
					if conn.RulePayload != "" {
						rule = rule + "(" + conn.RulePayload + ")"
					}
					ui.Text(c, rule).FontSize(s.XS).TextColor(p.ForegroundSubtle).SingleLine()
				case 3:
					chains := strings.Join(conn.Chains, " → ")
					ui.Text(c, chains).FontSize(s.XS).TextColor(p.IconBlue).SingleLine()
				case 4:
					ui.Text(c, components.FormatBytes(conn.Download)).FontSize(s.XS).Font(theme.MonoFont()).FontFeatures("tnum").TextColor(p.Foreground)
				case 5:
					ui.Text(c, components.FormatBytes(conn.Upload)).FontSize(s.XS).Font(theme.MonoFont()).FontFeatures("tnum").TextColor(p.ForegroundSubtle)
				case 6:
					dur := time.Since(conn.Start).Truncate(time.Second)
					ui.Text(c, dur.String()).Padding(0, 10, 0, 0).FontSize(s.XS).Font(theme.MonoFont()).FontFeatures("tnum").TextColor(p.ForegroundSubtle)
				}
			}).Fill()

			if tbl.Changed() && selIdx >= 0 && selIdx < len(list) {
				a.SelectedConnID = list[selIdx].ID
			}
			if tbl.Submitted() && selIdx >= 0 && selIdx < len(list) {
				a.SelectedConnID = list[selIdx].ID
				a.ShowConnModal = true
			}
		})
	})
}

func filteredConnections(a *app.App) []model.Connection {
	src := a.Conns
	if a.ConnFilterTab == 1 {
		src = a.ClosedConns
	}
	q := strings.ToLower(strings.TrimSpace(a.ConnSearch))
	if q == "" {
		return src
	}
	var out []model.Connection
	for _, item := range src {
		hay := strings.ToLower(strings.Join([]string{
			item.Metadata.Host,
			item.Metadata.DestinationIP,
			item.Metadata.ProcessPath,
			item.Rule,
			item.RulePayload,
			strings.Join(item.Chains, " "),
		}, " "))
		if strings.Contains(hay, q) {
			out = append(out, item)
		}
	}
	return out
}
