package components

import (
	"github.com/egoist/mygo/ui"

	"github.com/bihua-university/livery/internal/theme"
)

// SlidingState holds persistent layout handles for a container and its selectable children.
type SlidingState struct {
	Track ui.Handle
	Items []ui.Handle
}

// UseSliding allocates or resizes persistent SlidingState bound to container.
func UseSliding(container ui.Element, count int) *SlidingState {
	st := ui.Local(container, "sliding-state", func() SlidingState {
		return SlidingState{}
	})
	if st == nil {
		return &SlidingState{Items: make([]ui.Handle, count)}
	}
	if len(st.Items) != count {
		items := make([]ui.Handle, count)
		copy(items, st.Items)
		st.Items = items
	}
	container.Bind(&st.Track)
	return st
}

// SlidingPill renders an animated sliding indicator inside the bound Track toward the active item's bounds.
// It returns true once geometry has committed so the active item can leave its own background transparent.
func SlidingPill(c *ui.Context, st *SlidingState, activeIdx int, borderWidth float32, style func(pill ui.Element)) bool {
	if st == nil || activeIdx < 0 || activeIdx >= len(st.Items) {
		return false
	}
	trackRect := st.Track.Bounds(c)
	activeRect := st.Items[activeIdx].Bounds(c)
	if trackRect.W <= 0 || activeRect.W <= 0 || activeRect.H <= 0 {
		c.Invalidate()
		return false
	}

	relX := activeRect.X - trackRect.X - borderWidth
	relY := activeRect.Y - trackRect.Y - borderWidth

	pill := ui.Box(c.Key("sliding-pill")).
		Absolute().
		ZIndex(-1).
		PassThrough()
	pill.Left(pill.AnimateTo("sx", relX, ui.Snappy))
	pill.Top(pill.AnimateTo("sy", relY, ui.Snappy))
	pill.Width(pill.AnimateTo("sw", activeRect.W, ui.Snappy))
	pill.Height(pill.AnimateTo("sh", activeRect.H, ui.Snappy))
	if style != nil {
		style(pill)
	}
	return true
}

// NavItem describes one entry in a vertical sliding navigation list.
type NavItem struct {
	ID    string
	Icon  *ui.SVG
	Label string
	Badge string
}

// SlidingNav renders a vertical navigation list with a spring-animated active pill indicator.
func SlidingNav(c *ui.Context, p theme.Palette, s theme.TypeScale, items []NavItem, activeID *string) {
	activeIdx := 0
	for i, item := range items {
		if item.ID == *activeID {
			activeIdx = i
			break
		}
	}

	navList := ui.Column(c.Key("sliding-nav")).Grow(1).Gap(2)
	st := UseSliding(navList, len(items))
	navList.Children(func() {
		hasPill := SlidingPill(c, st, activeIdx, 0, func(pill ui.Element) {
			pill.Radius(theme.RadiusLG).
				Background(p.NavActive).
				Border(1, p.Border)
		})

		for i, item := range items {
			active := item.ID == *activeID
			row := ui.ButtonBase(c.Key("nav-"+item.ID)).
				Bind(&st.Items[i]).
				Height(34).
				Padding(0, 8).
				Gap(8).
				Radius(theme.RadiusLG).
				AlignItems(ui.Center).
				Label(item.Label).
				Cursor(ui.CursorPointer)
			if active && !hasPill {
				row.Background(p.NavActive).Border(1, p.Border)
			} else if !active && row.Hovered() {
				row.Background(p.Hover)
			}
			id := item.ID
			row.OnClick(func() { *activeID = id })
			row.Children(func() {
				icColor := p.ForegroundSubtle
				txtColor := p.ForegroundSubtle
				if active {
					icColor = p.Foreground
					txtColor = p.Foreground
				}
				ui.Icon(c, item.Icon).FontSize(16).TextColor(icColor)
				txt := ui.Text(c, item.Label).FontSize(s.Base).TextColor(txtColor).Grow(1).SingleLine()
				if active {
					txt.FontWeight(600)
				}
				if item.Badge != "" && item.Badge != "0" {
					ui.Box(c).Background(p.Tag).Radius(theme.RadiusSM).Padding(1, 5).Children(func() {
						ui.Text(c, item.Badge).FontSize(s.XS).Font(theme.MonoFont()).FontFeatures("tnum").TextColor(p.ForegroundSubtle)
					})
				}
			})
		}
	})
}
