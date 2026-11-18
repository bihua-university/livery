package theme

import (
	"runtime"
	"sync"

	"github.com/egoist/mygo/ui"
)

// Radius hierarchy:
// First rounded container: RadiusXL (12), nested: RadiusLG (8) -> RadiusMD (6) -> RadiusSM (4).
// Dialog shells: RadiusDialog (16). Pills/dots: RadiusFull (999).
const (
	RadiusDialog float32 = 16
	RadiusXL     float32 = 12
	RadiusLG     float32 = 8
	RadiusMD     float32 = 6
	RadiusSM     float32 = 4
	RadiusFull   float32 = 999
)

// Palette holds semantic design tokens for light and dark themes.
type Palette struct {
	Dark bool

	// Structural layout surfaces
	Background    ui.Color
	BackgroundAlt ui.Color
	Header        ui.Color
	Panel         ui.Color
	Sidebar       ui.Color

	// Content & interactive surfaces
	Surface      ui.Color
	SurfaceHover ui.Color
	Card         ui.Color
	CardSelected ui.Color
	CardBorder   ui.Color
	Popover      ui.Color
	PopoverHead  ui.Color
	PopoverBdr   ui.Color
	Menu         ui.Color
	MenuHover    ui.Color
	Input        ui.Color
	InputFocused ui.Color
	InputBorder  ui.Color
	Hover        ui.Color
	Selected     ui.Color
	NavActive    ui.Color
	Tab          ui.Color
	TabActive    ui.Color
	TabBorder    ui.Color
	Tag          ui.Color

	// Borders
	Border      ui.Color
	BorderHover ui.Color

	// Text hierarchy
	Foreground         ui.Color
	ForegroundSubtle   ui.Color
	ForegroundSubtlest ui.Color
	ForegroundInverse  ui.Color

	// Brand & action fills
	Brand             ui.Color
	Primary           ui.Color
	PrimaryForeground ui.Color
	Secondary         ui.Color
	AccentSurface     ui.Color
	IconBlue          ui.Color
	IconPurple        ui.Color

	// Semantic feedback colors & translucent surfaces
	Success                   ui.Color
	SuccessForeground         ui.Color
	SuccessSurface            ui.Color
	SuccessSurfaceForeground  ui.Color
	Warning                   ui.Color
	WarningForeground         ui.Color
	WarningSurface            ui.Color
	WarningSurfaceForeground  ui.Color
	Destructive               ui.Color
	DestructiveForeground     ui.Color
	DestructiveSurface        ui.Color
	DestructiveSurfaceFg      ui.Color
	ProgressSurface           ui.Color
	ProgressSurfaceForeground ui.Color

	// Telemetry chart series
	ChartDown ui.Color
	ChartUp   ui.Color
}

// Light returns the light theme palette.
func Light() Palette {
	bdr := ui.RGBA(13, 13, 13, 0.10)
	bdrHover := ui.RGBA(13, 13, 13, 0.15)
	return Palette{
		Dark:                      false,
		Background:                ui.Hex("#f8f8f8"),
		BackgroundAlt:             ui.Hex("#ececee"),
		Header:                    ui.Hex("#ffffff"),
		Panel:                     ui.Hex("#ffffff"),
		Sidebar:                   ui.Hex("#ececee"),
		Surface:                   ui.RGBA(13, 13, 13, 0.03),
		SurfaceHover:              ui.RGBA(13, 13, 13, 0.05),
		Card:                      ui.Hex("#ffffff"),
		CardSelected:              ui.Hex("#f5f5f5"),
		CardBorder:                bdr,
		Popover:                   ui.Hex("#ffffff"),
		PopoverHead:               ui.Hex("#f8f8f8"),
		PopoverBdr:                bdr,
		Menu:                      ui.Hex("#ffffff"),
		MenuHover:                 ui.Hex("#f0f0f0"),
		Input:                     ui.Hex("#ffffff"),
		InputFocused:              ui.Hex("#ffffff"),
		InputBorder:               bdr,
		Hover:                     ui.RGBA(13, 13, 13, 0.05),
		Selected:                  ui.RGBA(13, 13, 13, 0.06),
		NavActive:                 ui.Hex("#ffffff"),
		Tab:                       ui.Hex("#f0f0f0"),
		TabActive:                 ui.Hex("#ffffff"),
		TabBorder:                 bdr,
		Tag:                       ui.Hex("#e6e6e6"),
		Border:                    bdr,
		BorderHover:               bdrHover,
		Foreground:                ui.Hex("#171717"),
		ForegroundSubtle:          ui.RGBA(23, 23, 23, 0.70),
		ForegroundSubtlest:        ui.RGBA(23, 23, 23, 0.42),
		ForegroundInverse:         ui.Hex("#ffffff"),
		Brand:                     ui.Hex("#000000"),
		Primary:                   ui.Hex("#000000"),
		PrimaryForeground:         ui.Hex("#ffffff"),
		Secondary:                 ui.Hex("#e6e6e6"),
		AccentSurface:             ui.Hex("#ebf4ff"),
		IconBlue:                  ui.Hex("#0066dd"),
		IconPurple:                ui.Hex("#9e77ed"),
		Success:                   ui.Hex("#1e8a3e"),
		SuccessForeground:         ui.Hex("#ffffff"),
		SuccessSurface:            ui.Hex("#eaf7ee"),
		SuccessSurfaceForeground:  ui.Hex("#166b32"),
		Warning:                   ui.Hex("#e07b00"),
		WarningForeground:         ui.Hex("#ffffff"),
		WarningSurface:            ui.RGBA(224, 123, 0, 0.12),
		WarningSurfaceForeground:  ui.Hex("#914e00"),
		Destructive:               ui.Hex("#e03131"),
		DestructiveForeground:     ui.Hex("#ffffff"),
		DestructiveSurface:        ui.RGBA(224, 49, 49, 0.10),
		DestructiveSurfaceFg:      ui.Hex("#b42020"),
		ProgressSurface:           ui.Hex("#ebf4ff"),
		ProgressSurfaceForeground: ui.Hex("#0867ce"),
		ChartDown:                 ui.Hex("#0b7fff"),
		ChartUp:                   ui.Hex("#1e8a3e"),
	}
}

// Dark returns the dark theme palette.
func Dark() Palette {
	bdr := ui.RGBA(255, 255, 255, 0.10)
	bdrHover := ui.RGBA(255, 255, 255, 0.15)
	return Palette{
		Dark:                      true,
		Background:                ui.Hex("#161616"),
		BackgroundAlt:             ui.Hex("#2b2b2b"),
		Header:                    ui.Hex("#161616"),
		Panel:                     ui.Hex("#202020"),
		Sidebar:                   ui.Hex("#2B2B2B"),
		Surface:                   ui.RGBA(255, 255, 255, 0.05),
		SurfaceHover:              ui.RGBA(255, 255, 255, 0.10),
		Card:                      ui.Hex("#2b2b2b"),
		CardSelected:              ui.Hex("#363636"),
		CardBorder:                bdr,
		Popover:                   ui.Hex("#2b2b2b"),
		PopoverHead:               ui.Hex("#202020"),
		PopoverBdr:                bdr,
		Menu:                      ui.Hex("#2b2b2b"),
		MenuHover:                 ui.Hex("#363636"),
		Input:                     ui.Hex("#2b2b2b"),
		InputFocused:              ui.Hex("#2b2b2b"),
		InputBorder:               bdr,
		Hover:                     ui.RGBA(255, 255, 255, 0.06),
		Selected:                  ui.RGBA(255, 255, 255, 0.10),
		NavActive:                 ui.Hex("#383838"),
		Tab:                       ui.Hex("#202020"),
		TabActive:                 ui.Hex("#161616"),
		TabBorder:                 bdr,
		Tag:                       ui.Hex("#363636"),
		Border:                    bdr,
		BorderHover:               bdrHover,
		Foreground:                ui.Hex("#f5f5f5"),
		ForegroundSubtle:          ui.RGBA(212, 212, 212, 0.65),
		ForegroundSubtlest:        ui.RGBA(212, 212, 212, 0.35),
		ForegroundInverse:         ui.Hex("#000000"),
		Brand:                     ui.Hex("#ffffff"),
		Primary:                   ui.Hex("#ffffff"),
		PrimaryForeground:         ui.Hex("#000000"),
		Secondary:                 ui.Hex("#363636"),
		AccentSurface:             ui.Hex("#001d3d"),
		IconBlue:                  ui.Hex("#4099ff"),
		IconPurple:                ui.Hex("#7b5ce5"),
		Success:                   ui.Hex("#46bf72"),
		SuccessForeground:         ui.Hex("#000000"),
		SuccessSurface:            ui.RGBA(70, 191, 114, 0.16),
		SuccessSurfaceForeground:  ui.Hex("#87d9a4"),
		Warning:                   ui.Hex("#ff8a30"),
		WarningForeground:         ui.Hex("#000000"),
		WarningSurface:            ui.RGBA(255, 138, 48, 0.16),
		WarningSurfaceForeground:  ui.Hex("#ffa159"),
		Destructive:               ui.Hex("#ff5c5c"),
		DestructiveForeground:     ui.Hex("#ffffff"),
		DestructiveSurface:        ui.RGBA(255, 92, 92, 0.16),
		DestructiveSurfaceFg:      ui.Hex("#ff8585"),
		ProgressSurface:           ui.RGBA(70, 130, 240, 0.16),
		ProgressSurfaceForeground: ui.Hex("#8fc0ff"),
		ChartDown:                 ui.Hex("#4099ff"),
		ChartUp:                   ui.Hex("#46bf72"),
	}
}

type uiThemeCacheKey struct {
	dark     bool
	fontSize float32
	table    bool
}

var (
	uiThemeMu    sync.RWMutex
	uiThemeCache = make(map[uiThemeCacheKey]*ui.Theme, 4)
)

// BuildUITheme converts a Palette into a MyGo ui.Theme so native widgets match.
func BuildUITheme(p Palette, uiFontSize float32) *ui.Theme {
	if uiFontSize < 12 || uiFontSize > 18 {
		uiFontSize = 14
	}
	key := uiThemeCacheKey{dark: p.Dark, fontSize: uiFontSize}
	uiThemeMu.RLock()
	cached := uiThemeCache[key]
	uiThemeMu.RUnlock()
	if cached != nil {
		return cached
	}

	uiThemeMu.Lock()
	defer uiThemeMu.Unlock()
	if cached := uiThemeCache[key]; cached != nil {
		return cached
	}
	t := ui.LightTheme()
	if p.Dark {
		t = ui.DarkTheme()
	}
	copyTheme := *t
	copyTheme.Dark = p.Dark
	copyTheme.Background = p.Background
	copyTheme.Surface = p.Card
	copyTheme.SurfaceHover = p.SurfaceHover
	copyTheme.SurfacePressed = p.Selected
	copyTheme.Border = p.Border
	copyTheme.Text = p.Foreground
	copyTheme.TextMuted = p.ForegroundSubtle
	copyTheme.Accent = p.Primary
	copyTheme.AccentHover = p.Primary.Mix(p.ForegroundSubtle, 0.15)
	copyTheme.AccentPressed = p.Primary.Mix(p.ForegroundSubtle, 0.25)
	copyTheme.AccentText = p.PrimaryForeground
	copyTheme.InactiveAccent = p.Secondary
	copyTheme.Danger = p.Destructive
	copyTheme.Warning = p.Warning
	copyTheme.Success = p.Success
	copyTheme.Selection = p.IconBlue.Alpha(0.28)
	copyTheme.Focus = p.IconBlue.Alpha(0.55)
	copyTheme.Inverse = p.Popover
	copyTheme.InverseText = p.Foreground
	copyTheme.Radius = RadiusLG
	copyTheme.Spacing = 4
	copyTheme.FontSize = uiFontSize
	if runtime.GOOS == "windows" {
		copyTheme.Font = "Microsoft YaHei UI"
	}
	uiThemeCache[key] = &copyTheme
	return &copyTheme
}

// BuildTableTheme returns a cached table-styled ui.Theme variant matching p.
func BuildTableTheme(p Palette, base *ui.Theme) *ui.Theme {
	fontSize := float32(14)
	if base != nil && base.FontSize >= 12 && base.FontSize <= 18 {
		fontSize = base.FontSize
	}
	key := uiThemeCacheKey{dark: p.Dark, fontSize: fontSize, table: true}
	uiThemeMu.RLock()
	cached := uiThemeCache[key]
	uiThemeMu.RUnlock()
	if cached != nil {
		return cached
	}

	uiBase := BuildUITheme(p, fontSize)
	uiThemeMu.Lock()
	defer uiThemeMu.Unlock()
	if cached := uiThemeCache[key]; cached != nil {
		return cached
	}
	copyTheme := *uiBase
	copyTheme.Background = p.Card
	copyTheme.Accent = p.Card.Mix(p.Foreground, 0.11)
	copyTheme.InactiveAccent = copyTheme.Accent
	copyTheme.AccentText = p.Foreground
	uiThemeCache[key] = &copyTheme
	return &copyTheme
}

// MonoFont returns a readable monospace font stack for technical values.
func MonoFont() string {
	switch runtime.GOOS {
	case "darwin":
		return "SF Mono"
	case "windows":
		return "Consolas"
	default:
		return "monospace"
	}
}

// TypeScale derives application UI font token sizes from base UI font size.
type TypeScale struct {
	Base          float32
	XL            float32
	LG            float32
	Caption       float32
	SM            float32
	XS            float32
	SettingsTitle float32
}

// Scale returns the typography hierarchy anchored at base (default 14px).
func Scale(base float32) TypeScale {
	if base < 12 || base > 18 {
		base = 14
	}
	return TypeScale{
		Base:          base,
		XL:            base + 4,
		LG:            base + 2,
		Caption:       base - 1,
		SM:            base - 2,
		XS:            base - 4,
		SettingsTitle: base + 10,
	}
}
