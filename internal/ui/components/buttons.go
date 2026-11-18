package components

import (
	"github.com/egoist/mygo/ui"

	"github.com/bihua-university/livery/internal/theme"
)

// ActionButton renders a compact neutral outline button.
func ActionButton(c *ui.Context, p theme.Palette, s theme.TypeScale, icon *ui.SVG, label string) ui.Element {
	btn := ui.ButtonBase(c).
		Height(28).
		Padding(0, 10).
		Gap(6).
		Radius(theme.RadiusLG).
		Background(p.Card).
		Border(1, p.Border).
		AlignItems(ui.Center).
		Cursor(ui.CursorPointer)
	if btn.Hovered() {
		btn.Background(p.SurfaceHover).BorderColor(p.BorderHover)
	}
	btn.Children(func() {
		if icon != nil {
			ui.Icon(c, icon).FontSize(14).TextColor(p.ForegroundSubtle)
		}
		if label != "" {
			ui.Text(c, label).FontSize(s.SM).FontWeight(500).TextColor(p.Foreground).SingleLine()
		}
	})
	return btn
}

// PrimaryActionButton renders a compact high-emphasis primary button.
func PrimaryActionButton(c *ui.Context, p theme.Palette, s theme.TypeScale, icon *ui.SVG, label string) ui.Element {
	btn := ui.ButtonBase(c).
		Height(28).
		Padding(0, 12).
		Gap(6).
		Radius(theme.RadiusLG).
		Background(p.Primary).
		AlignItems(ui.Center).
		Cursor(ui.CursorPointer)
	if btn.Hovered() {
		btn.Opacity(0.9)
	}
	btn.Children(func() {
		if icon != nil {
			ui.Icon(c, icon).FontSize(14).TextColor(p.PrimaryForeground)
		}
		if label != "" {
			ui.Text(c, label).FontSize(s.SM).FontWeight(500).TextColor(p.PrimaryForeground).SingleLine()
		}
	})
	return btn
}

// IconActionButton renders a square 28x28 icon-only button with an accessible label and tooltip.
func IconActionButton(c *ui.Context, p theme.Palette, icon *ui.SVG, tooltip string) ui.Element {
	btn := ui.ButtonBase(c).
		Size(28, 28).
		Center().
		Radius(theme.RadiusLG).
		Background(p.Card).
		Border(1, p.Border).
		Label(tooltip).
		Tooltip(tooltip).
		Cursor(ui.CursorPointer)
	if btn.Hovered() {
		btn.Background(p.SurfaceHover).BorderColor(p.BorderHover)
	}
	btn.Children(func() {
		ui.Icon(c, icon).FontSize(14).TextColor(p.ForegroundSubtle)
	})
	return btn
}

// Segmented renders a compact segmented control with a sliding active pill animation.
func Segmented(c *ui.Context, p theme.Palette, s theme.TypeScale, selected *int, labels []string) ui.Element {
	seg := ui.SegmentedBase(c, selected, len(labels))
	st := UseSliding(seg.Track, len(labels))
	seg.Track.
		Background(p.Tab).
		Border(1, p.TabBorder).
		Radius(theme.RadiusLG).
		Padding(2).
		Gap(2).
		Children(func() {
			hasPill := SlidingPill(c, st, *selected, 1, func(pill ui.Element) {
				pill.Radius(theme.RadiusMD).
					Background(p.TabActive).
					Border(1, p.TabBorder).
					Shadow(0, 1, 2, 0, ui.RGBA(0, 0, 0, 0.10))
			})

			for i, lbl := range labels {
				item := seg.Segment(i).
					Bind(&st.Items[i]).
					Height(24).
					Padding(0, 10).
					Radius(theme.RadiusMD).
					Center().
					Label(lbl).
					Cursor(ui.CursorPointer)
				fg := p.ForegroundSubtle
				if i == *selected {
					fg = p.Foreground
					if !hasPill {
						item.Background(p.TabActive).Border(1, p.TabBorder)
					}
				} else if item.Hovered() {
					item.Background(p.Hover)
				}
				item.Children(func() {
					txt := ui.Text(c, lbl).FontSize(s.SM).FontWeight(500).TextColor(fg).SingleLine()
					if i == *selected {
						txt.FontWeight(600)
					}
				})
			}
		})
	return seg.Track
}
