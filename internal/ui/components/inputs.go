package components

import (
	"github.com/egoist/mygo/ui"

	"github.com/bihua-university/livery/internal/theme"
)

// Toggle renders a capsule switch matching the blue active / white circular thumb style.
func Toggle(c *ui.Context, p theme.Palette, val *bool) ui.Element {
	sw := ui.SwitchBase(c, val).
		Size(38, 22).
		Radius(theme.RadiusFull).
		Cursor(ui.CursorPointer)
	target := float32(0)
	if val != nil && *val {
		target = 1
	}
	pos := sw.AnimateTo("knob", target, ui.Snappy)
	sw.Background(p.ToggleOff.Mix(p.Accent, pos))
	sw.Draw(func(pt *ui.Painter, r ui.Rect) {
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
	return sw
}

// LabeledInput renders a label above a full-width single-line text input.
func LabeledInput(c *ui.Context, p theme.Palette, s theme.TypeScale, key, label, placeholder string, val *string) ui.Element {
	var input ui.Element
	ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
		ui.Text(c, label).FontSize(s.XS).FontWeight(500).TextColor(p.ForegroundSubtle).SingleLine()
		input = ui.TextInput(c.Key(key), val).Placeholder(placeholder).FillWidth()
	})
	return input
}

// LabeledSelect renders a label above a full-width select drop-down.
func LabeledSelect(c *ui.Context, p theme.Palette, s theme.TypeScale, key, label string, val *string, options []string) ui.Element {
	var sel ui.Element
	ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
		ui.Text(c, label).FontSize(s.XS).FontWeight(500).TextColor(p.ForegroundSubtle).SingleLine()
		sel = ui.Select(c.Key(key), val, options).FillWidth()
	})
	return sel
}

// SwitchRow renders a titled setting row with an optional subtitle and right-aligned switch.
func SwitchRow(c *ui.Context, p theme.Palette, s theme.TypeScale, key, title, subtitle string, val *bool, badge func()) ui.Element {
	var sw ui.Element
	ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Text(c, title).FontSize(s.Base).FontWeight(500).TextColor(p.Foreground)
				if badge != nil {
					badge()
				}
			})
			if subtitle != "" {
				ui.Text(c, subtitle).FontSize(s.XS).TextColor(p.ForegroundSubtle)
			}
		})
		sw = Toggle(c.Key(key), p, val).Label(title)
	})
	return sw
}

// SettingRow renders a horizontal setting row with a left-aligned title and right-aligned trailing controls.
func SettingRow(c *ui.Context, p theme.Palette, s theme.TypeScale, title string, trailing func()) {
	ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
		ui.Text(c, title).FontSize(s.Base).TextColor(p.Foreground).Grow(1)
		if trailing != nil {
			trailing()
		}
	})
}

// ModalShell renders a dialog overlay container following the dialog radius hierarchy.
func ModalShell(c *ui.Context, p theme.Palette, open *bool, width float32, fn func()) ui.Element {
	return ui.DialogBase(c, open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.48))
		panel.Width(width).
			MaxWidthPercent(92).
			MaxHeightPercent(88).
			Background(p.Popover).
			Border(1, p.PopoverBdr).
			Radius(theme.RadiusDialog).
			Padding(20).
			Gap(14).
			Shadow(0, 16, 40, 0, ui.RGBA(0, 0, 0, 0.25))
		fn()
	})
}

// ModalActions renders a right-aligned action footer row for modal dialogs.
func ModalActions(c *ui.Context, fn func()) ui.Element {
	return ui.Row(c).Gap(8).Justify(ui.End).Children(fn)
}
