package components

import (
	"github.com/egoist/mygo/ui"

	"github.com/bihua-university/livery/internal/icons"
	"github.com/bihua-university/livery/internal/theme"
)

// Card renders a standard bordered content card.
func Card(c *ui.Context, p theme.Palette, fn func()) ui.Element {
	return ui.Column(c).
		Background(p.Card).
		Border(1, p.CardBorder).
		Radius(theme.RadiusXL).
		Padding(16).
		Gap(12).
		Children(fn)
}

// SurfacePanel renders a low-elevation surface container.
func SurfacePanel(c *ui.Context, p theme.Palette, fn func()) ui.Element {
	return ui.Column(c).
		Background(p.Surface).
		Border(1, p.Border).
		Radius(theme.RadiusLG).
		Padding(12).
		Gap(8).
		Children(fn)
}

// SectionHeader renders a card or section title with an optional trailing widget.
func SectionHeader(c *ui.Context, p theme.Palette, s theme.TypeScale, title string, trailing func()) {
	ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
		ui.Text(c, title).FontSize(s.Base).FontWeight(600).TextColor(p.Foreground).Grow(1).SingleLine()
		if trailing != nil {
			trailing()
		}
	})
}

// MetricCard renders a dense telemetry KPI card with title, monospace value, and subtitle.
func MetricCard(c *ui.Context, p theme.Palette, s theme.TypeScale, title, value, sub string, icon *ui.SVG, accent ui.Color) {
	Card(c, p, func() {
		ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Text(c, title).FontSize(s.SM).TextColor(p.ForegroundSubtle).Grow(1).SingleLine()
			ui.Icon(c, icon).FontSize(14).TextColor(accent)
		})
		ui.Text(c, value).FontSize(s.XL).FontWeight(600).Font(theme.MonoFont()).FontFeatures("tnum").TextColor(p.Foreground).SingleLine()
		ui.Text(c, sub).FontSize(s.XS).Font(theme.MonoFont()).FontFeatures("tnum").TextColor(p.ForegroundSubtlest).SingleLine()
	})
}

// AlertBanner renders a semantic warning or error banner with optional trailing action buttons.
func AlertBanner(c *ui.Context, p theme.Palette, s theme.TypeScale, title, body string, bg, border, fg ui.Color, monoBody bool, actions func()) {
	ui.Row(c).
		Background(bg).
		Border(1, border).
		Radius(theme.RadiusXL).
		Padding(12, 16).
		Gap(12).
		AlignItems(ui.Center).
		Children(func() {
			ui.Icon(c, icons.Alert).FontSize(18).TextColor(fg)
			ui.Column(c).Grow(1).Gap(2).Children(func() {
				ui.Text(c, title).FontSize(s.Base).FontWeight(600).TextColor(fg)
				txt := ui.Text(c, body).FontSize(s.SM).TextColor(fg).MaxLines(3)
				if monoBody {
					txt.Font(theme.MonoFont())
				}
			})
			if actions != nil {
				actions()
			}
		})
}

// EmptyState renders a centered empty placeholder with an icon, title, subtitle, and optional action.
func EmptyState(c *ui.Context, p theme.Palette, s theme.TypeScale, icon *ui.SVG, title, subtitle string, action func()) {
	ui.Column(c).Grow(1).Fill().Center().Gap(8).Children(func() {
		ui.Icon(c, icon).FontSize(28).TextColor(p.ForegroundSubtlest)
		ui.Text(c, title).FontSize(s.Base).FontWeight(600).TextColor(p.Foreground)
		if subtitle != "" {
			ui.Text(c, subtitle).FontSize(s.SM).TextColor(p.ForegroundSubtle)
		}
		if action != nil {
			action()
		}
	})
}

// LoadingState renders a centered animated spinner with loading copy.
func LoadingState(c *ui.Context, p theme.Palette, s theme.TypeScale, title, subtitle string) {
	ui.Column(c).Grow(1).Fill().Center().Gap(10).Children(func() {
		ui.Spinner(c).FontSize(22).TextColor(p.Primary).Label(title)
		ui.Text(c, title).FontSize(s.Base).FontWeight(600).TextColor(p.Foreground)
		if subtitle != "" {
			ui.Text(c, subtitle).FontSize(s.SM).TextColor(p.ForegroundSubtle)
		}
	})
}

