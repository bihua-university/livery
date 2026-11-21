package components

import (
	"github.com/egoist/mygo/ui"

	"github.com/bihua-university/livery/internal/theme"
)

// SettingsGroup renders a titled settings section header above a hairline-divided card container.
func SettingsGroup(c *ui.Context, p theme.Palette, s theme.TypeScale, icon *ui.SVG, title, subtitle string, trailing, content func()) {
	ui.Column(c).FillWidth().Gap(6).Children(func() {
		ui.Row(c).FillWidth().Padding(0, 2).Gap(8).AlignItems(ui.Center).Children(func() {
			if icon != nil {
				ui.Box(c).
					Size(24, 24).
					Shrink(0).
					Radius(theme.RadiusMD).
					Background(p.Surface).
					Border(1, p.Border).
					Center().
					Children(func() {
						ui.Icon(c, icon).FontSize(13).TextColor(p.ForegroundSubtle)
					})
			}
			ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
				ui.Text(c, title).FontSize(s.Base).FontWeight(600).TextColor(p.Foreground).SingleLine()
				if subtitle != "" {
					ui.Text(c, subtitle).FontSize(s.XS).TextColor(p.ForegroundSubtle).SingleLine()
				}
			})
			if trailing != nil {
				ui.Row(c).Shrink(0).Gap(8).AlignItems(ui.Center).Children(trailing)
			}
		})

		ui.Column(c).
			FillWidth().
			Background(p.Card).
			Border(1, p.CardBorder).
			Radius(theme.RadiusXL).
			Gap(0).
			Clip().
			Children(content)
	})
}

// SettingDivider renders a hairline separator between rows inside a SettingsGroup card.
func SettingDivider(c *ui.Context, p theme.Palette) {
	ui.Box(c).Height(1).FillWidth().Background(p.Border)
}

// SettingSubheader renders a subtle tinted band inside a card to separate sub-groups of settings.
func SettingSubheader(c *ui.Context, p theme.Palette, s theme.TypeScale, title string, trailing func()) {
	ui.Row(c).
		FillWidth().
		Background(p.Surface).
		BorderWidth(0, 0, 1, 0).
		BorderColor(p.Border).
		Padding(6, 14).
		Gap(8).
		AlignItems(ui.Center).
		Children(func() {
			ui.Text(c, title).FontSize(s.XS).FontWeight(600).TextColor(p.ForegroundSubtle).Grow(1).SingleLine()
			if trailing != nil {
				ui.Row(c).Shrink(0).Gap(6).AlignItems(ui.Center).Children(trailing)
			}
		})
}

// SettingItem renders a two-column configuration row with a title, plain-language description,
// optional inline badge, and right-aligned control.
func SettingItem(c *ui.Context, p theme.Palette, s theme.TypeScale, title, desc string, badge, trailing func()) {
	ui.Row(c).FillWidth().Padding(10, 14).Gap(14).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Text(c, title).FontSize(s.Base).FontWeight(500).TextColor(p.Foreground).SingleLine()
				if badge != nil {
					badge()
				}
			})
			if desc != "" {
				ui.Text(c, desc).FontSize(s.XS).TextColor(p.ForegroundSubtle)
			}
		})
		if trailing != nil {
			ui.Row(c).Shrink(0).Gap(8).AlignItems(ui.Center).Children(trailing)
		}
	})
}

// SettingToggleItem renders a full-row interactive switch setting with title, description,
// subtle hover feedback, and a right-aligned animated toggle pill.
func SettingToggleItem(c *ui.Context, p theme.Palette, s theme.TypeScale, key, title, desc string, val *bool, badge func()) ui.Element {
	sw := ui.SwitchBase(c.Key(key), val).
		FillWidth().
		Padding(10, 14).
		Gap(14).
		AlignItems(ui.Center).
		Label(title).
		Cursor(ui.CursorPointer)
	if sw.Hovered() {
		sw.Background(p.SurfaceHover)
	}
	pos := toggleKnobPos(sw, val)
	sw.Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Text(c, title).FontSize(s.Base).FontWeight(500).TextColor(p.Foreground).SingleLine()
				if badge != nil {
					badge()
				}
			})
			if desc != "" {
				ui.Text(c, desc).FontSize(s.XS).TextColor(p.ForegroundSubtle)
			}
		})
		renderToggleTrack(c, p, pos)
	})
	return sw
}

// SettingToggleTile renders a compact Bento-style interactive toggle tile for multi-column
// grids of closely related boolean options.
func SettingToggleTile(c *ui.Context, p theme.Palette, s theme.TypeScale, key, title, desc string, val *bool) ui.Element {
	sw := ui.SwitchBase(c.Key(key), val).
		FillWidth().
		Padding(9, 12).
		Gap(10).
		AlignItems(ui.Center).
		Radius(theme.RadiusLG).
		Background(p.Surface).
		Border(1, p.Border).
		Label(title).
		Cursor(ui.CursorPointer)
	if sw.Hovered() {
		sw.Background(p.SurfaceHover).BorderColor(p.BorderHover)
	}
	pos := toggleKnobPos(sw, val)
	sw.Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			ui.Text(c, title).FontSize(s.SM).FontWeight(500).TextColor(p.Foreground).SingleLine()
			if desc != "" {
				ui.Text(c, desc).FontSize(s.XS).TextColor(p.ForegroundSubtle)
			}
		})
		renderToggleTrack(c, p, pos)
	})
	return sw
}

// SettingBlock renders a vertical configuration block inside a SettingsGroup card (title/description
// + optional right-aligned actions above full-width inputs or grids).
func SettingBlock(c *ui.Context, p theme.Palette, s theme.TypeScale, title, desc string, trailing, content func()) {
	ui.Column(c).FillWidth().Padding(10, 14).Gap(8).Children(func() {
		if title != "" || desc != "" || trailing != nil {
			ui.Row(c).FillWidth().Gap(12).AlignItems(ui.Center).Children(func() {
				ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
					if title != "" {
						ui.Text(c, title).FontSize(s.Base).FontWeight(500).TextColor(p.Foreground).SingleLine()
					}
					if desc != "" {
						ui.Text(c, desc).FontSize(s.XS).TextColor(p.ForegroundSubtle)
					}
				})
				if trailing != nil {
					ui.Row(c).Shrink(0).Gap(6).AlignItems(ui.Center).Children(trailing)
				}
			})
		}
		if content != nil {
			content()
		}
	})
}

func toggleKnobPos(sw ui.Element, val *bool) float32 {
	target := float32(0)
	if val != nil && *val {
		target = 1
	}
	return sw.AnimateTo("knob", target, ui.Snappy)
}

func renderToggleTrack(c *ui.Context, p theme.Palette, pos float32) {
	track := ui.Box(c).
		Size(36, 20).
		Shrink(0).
		Radius(theme.RadiusFull).
		Background(p.ToggleOff.Mix(p.Accent, pos))
	track.Draw(func(pt *ui.Painter, r ui.Rect) {
		const inset float32 = 2.5
		d := r.H - 2*inset
		knob := ui.Rect{
			X: r.X + inset + pos*(r.W-r.H),
			Y: r.Y + inset,
			W: d,
			H: d,
		}
		pt.Shadow(knob, d/2, 0, 1, 2.5, 0, ui.RGBA(0, 0, 0, 0.22))
		pt.Fill(knob, p.AccentForeground, d/2)
	})
}
