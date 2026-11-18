package pages

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/bihua-university/livery/internal/app"
	"github.com/bihua-university/livery/internal/icons"
	"github.com/bihua-university/livery/internal/theme"
	"github.com/bihua-university/livery/internal/ui/components"
)

// Logs renders the dense terminal-style core log viewer backed by on-demand disk tailing.
func Logs(c *ui.Context, a *app.App, p theme.Palette, s theme.TypeScale) {
	a.SyncLogsForView()

	ui.Column(c).Fill().Padding(16).Gap(12).MinHeight(0).Children(func() {
		ui.Row(c).Gap(10).AlignItems(ui.Center).Wrap().Children(func() {
			levels := []string{
				a.Tr("全部", "All"),
				"Info",
				"Warn",
				"Error",
				"Debug",
			}
			components.Segmented(c.Key("log-lvl-seg"), p, s, &a.LogLevelIdx, levels)

			ui.SearchField(c.Key("log-search"), &a.LogSearch).
				Placeholder(a.Tr("过滤日志关键字...", "Filter logs...")).
				Width(220)

			ui.Spacer(c)

			autoBtn := components.ActionButton(c, p, s, nil, a.Tr("自动滚动", "Auto-scroll"))
			if a.LogAutoScroll {
				autoBtn.Background(p.Selected).BorderColor(p.Primary)
			}
			if autoBtn.Clicked() {
				a.LogAutoScroll = !a.LogAutoScroll
			}

			pauseLbl := a.Tr("暂停", "Pause")
			pauseIcon := icons.Pause
			if a.LogPaused {
				pauseLbl = a.Tr("继续", "Resume")
				pauseIcon = icons.Play
			}
			if components.ActionButton(c, p, s, pauseIcon, pauseLbl).Clicked() {
				a.LogPaused = !a.LogPaused
			}
			if a.LogStore != nil {
				if components.IconActionButton(c, p, icons.Folder, a.Tr("打开日志目录", "Open Logs Folder")).Clicked() {
					app.OpenFolderInOS(a.LogStore.Dir())
				}
			}
			if components.IconActionButton(c, p, icons.Copy, a.Tr("复制日志", "Copy Logs")).Clicked() {
				copyVisibleLogs(a)
			}
			if components.IconActionButton(c, p, icons.Trash, a.Tr("清空日志", "Clear Logs")).Clicked() {
				a.ClearLogs()
			}
		})

		entries := a.Logs

		ui.Column(c).Grow(1).MinHeight(0).Background(p.Card).Border(1, p.CardBorder).Radius(theme.RadiusXL).Clip().Children(func() {
			if len(entries) == 0 {
				components.EmptyState(c, p, s, icons.Logs, a.Tr("暂无日志输出", "No log entries"), "", nil)
				return
			}

			a.LogListState.FollowEnd = a.LogAutoScroll
			a.LogListState.Key = func(i int) any {
				return fmt.Sprintf("%d-%d", i, entries[i].Time.UnixNano())
			}

			ui.List(c.Key("logs-list"), &a.LogListState, len(entries), func(i int) {
				entry := entries[i]
				row := ui.Row(c).Padding(4, 12).Gap(10).AlignItems(ui.Start)
				lvlColor := p.ForegroundSubtle
				switch entry.Level {
				case "warning", "warn":
					row.Background(p.WarningSurface)
					lvlColor = p.WarningSurfaceForeground
				case "error", "fatal":
					row.Background(p.DestructiveSurface)
					lvlColor = p.DestructiveSurfaceFg
				case "info":
					lvlColor = p.IconBlue
				}
				row.Children(func() {
					ui.Text(c, entry.Time.Format("15:04:05")).
						FontSize(s.SM).Font(theme.MonoFont()).FontFeatures("tnum").TextColor(p.ForegroundSubtlest).NoWrap()
					ui.Text(c, fmt.Sprintf("%-5s", strings.ToUpper(shortLevel(entry.Level)))).
						FontSize(s.SM).Font(theme.MonoFont()).FontWeight(600).TextColor(lvlColor).NoWrap()
					ui.Text(c, entry.Message).
						FontSize(s.SM).Font(theme.MonoFont()).TextColor(p.Foreground).Grow(1).Selectable()
				})
			}).Grow(1)
		})
	})
}

func copyVisibleLogs(a *app.App) {
	entries := a.Logs
	var sb strings.Builder
	for _, e := range entries {
		sb.WriteString(fmt.Sprintf("%s [%s] %s\n", e.Time.Format("15:04:05"), strings.ToUpper(e.Level), e.Message))
	}
	mygo.Clipboard.WriteText(sb.String())
	a.Notify(fmt.Sprintf(a.Tr("已复制 %d 条日志", "Copied %d log lines"), len(entries)))
}

func shortLevel(lvl string) string {
	if strings.EqualFold(lvl, "warning") {
		return "warn"
	}
	return lvl
}
