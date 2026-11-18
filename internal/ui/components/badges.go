package components

import (
	"fmt"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/bihua-university/livery/internal/model"
	"github.com/bihua-university/livery/internal/theme"
)

// StatusChip renders a semantic status badge with an optional indicator dot.
func StatusChip(c *ui.Context, s theme.TypeScale, label string, bg, fg, dot ui.Color) ui.Element {
	return ui.Row(c).
		Background(bg).
		Radius(theme.RadiusMD).
		Padding(3, 8).
		Gap(6).
		AlignItems(ui.Center).
		Children(func() {
			if dot != ui.Transparent {
				ui.Box(c).Size(6, 6).Radius(theme.RadiusFull).Background(dot)
			}
			ui.Text(c, label).FontSize(s.XS).FontWeight(500).TextColor(fg).SingleLine()
		})
}

// TagChip renders a neutral monospace micro badge.
func TagChip(c *ui.Context, p theme.Palette, s theme.TypeScale, label string) ui.Element {
	return ui.Box(c).
		Background(p.Tag).
		Radius(theme.RadiusSM).
		Padding(2, 6).
		Children(func() {
			ui.Text(c, label).FontSize(s.XS).Font(theme.MonoFont()).TextColor(p.ForegroundSubtle).SingleLine()
		})
}

// LatencyColors returns the badge background and foreground colors for a delay measurement.
func LatencyColors(p theme.Palette, delay int, testing bool) (ui.Color, ui.Color) {
	if testing || delay < 0 {
		return p.Tag, p.ForegroundSubtle
	}
	switch {
	case delay > 0 && delay < 150:
		return p.SuccessSurface, p.SuccessSurfaceForeground
	case delay >= 150 && delay < 400:
		return p.WarningSurface, p.WarningSurfaceForeground
	default:
		return p.DestructiveSurface, p.DestructiveSurfaceFg
	}
}

// LatencyChip renders a clickable color-coded latency pill for a proxy node.
func LatencyChip(c *ui.Context, p theme.Palette, s theme.TypeScale, delay int, testing bool, timeoutLabel, tooltip string) ui.Element {
	label := FormatLatency(delay, testing, timeoutLabel)
	bg, fg := LatencyColors(p, delay, testing)
	btn := ui.ButtonBase(c).
		Background(bg).
		Radius(theme.RadiusSM).
		Padding(2, 6).
		Cursor(ui.CursorPointer).
		Label(tooltip).
		Tooltip(tooltip)
	btn.Children(func() {
		ui.Text(c, label).FontSize(s.XS).Font(theme.MonoFont()).FontFeatures("tnum").TextColor(fg).SingleLine()
	})
	return btn
}

// KeyValueRow renders a single label/value metadata row.
func KeyValueRow(c *ui.Context, p theme.Palette, s theme.TypeScale, k, v string, mono bool) {
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, k).FontSize(s.SM).TextColor(p.ForegroundSubtle).Grow(1)
		txt := ui.Text(c, v).FontSize(s.SM).TextColor(p.Foreground).SingleLine()
		if mono {
			txt.Font(theme.MonoFont()).FontFeatures("tnum")
		}
	})
}

// QuotaBar renders subscription traffic usage progress and optional expiry date.
func QuotaBar(c *ui.Context, p theme.Palette, s theme.TypeScale, usage model.SubscriptionInfo, expirePrefix string) {
	if usage.Total <= 0 {
		return
	}
	used := usage.Upload + usage.Download
	ratio := float64(used) / float64(usage.Total)
	if ratio > 1 {
		ratio = 1
	}
	ui.Column(c).Gap(4).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			ui.Text(c, fmt.Sprintf("%s / %s", FormatBytes(used), FormatBytes(usage.Total))).
				FontSize(s.XS).Font(theme.MonoFont()).FontFeatures("tnum").TextColor(p.ForegroundSubtle).Grow(1)
			if usage.Expire > 0 {
				exp := time.Unix(usage.Expire, 0).Format("2006-01-02")
				ui.Text(c, expirePrefix+exp).FontSize(s.XS).Font(theme.MonoFont()).TextColor(p.ForegroundSubtle)
			}
		})
		ui.Progress(c, ratio).FillWidth().Height(5)
	})
}
