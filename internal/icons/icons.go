package icons

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"

	"github.com/egoist/mygo/ui"
)

func lucide(paths string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">` + paths + `</svg>`))
}

var (
	Livery      = lucide(`<path d="M5 19 14.5 4H20l-4.5 15Z"/><path d="M9 19 16.5 7"/><path d="M12.5 19 17.5 11"/>`)
	Overview    = lucide(`<rect width="7" height="9" x="3" y="3" rx="1"/><rect width="7" height="5" x="14" y="3" rx="1"/><rect width="7" height="9" x="14" y="12" rx="1"/><rect width="7" height="5" x="3" y="16" rx="1"/>`)
	Proxies     = lucide(`<circle cx="12" cy="12" r="10"/><path d="M12 2a14.5 14.5 0 0 0 0 20 14.5 14.5 0 0 0 0-20"/><path d="M2 12h20"/>`)
	Profiles    = lucide(`<path d="M15 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7Z"/><path d="M14 2v4a2 2 0 0 0 2 2h4"/><path d="M10 9H8"/><path d="M16 13H8"/><path d="M16 17H8"/>`)
	Connections = lucide(`<path d="M8 3v3a2 2 0 0 1-2 2H3"/><path d="M21 8h-3a2 2 0 0 1-2-2V3"/><path d="M3 16h3a2 2 0 0 1 2 2v3"/><path d="M16 21v-3a2 2 0 0 1 2-2h3"/>`)
	Rules       = lucide(`<path d="M3 6h18"/><path d="M7 12h10"/><path d="M10 18h4"/>`)
	Logs        = lucide(`<polyline points="4 17 10 11 4 5"/><line x1="12" x2="20" y1="19" y2="19"/>`)
	Settings    = lucide(`<path d="M12.22 2h-.44a2 2 0 0 0-2 2v.18a2 2 0 0 1-1 1.73l-.43.25a2 2 0 0 1-2 0l-.15-.08a2 2 0 0 0-2.73.73l-.22.38a2 2 0 0 0 .73 2.73l.15.1a2 2 0 0 1 1 1.72v.51a2 2 0 0 1-1 1.74l-.15.09a2 2 0 0 0-.73 2.73l.22.38a2 2 0 0 0 2.73.73l.15-.08a2 2 0 0 1 2 0l.43.25a2 2 0 0 1 1 1.73V20a2 2 0 0 0 2 2h.44a2 2 0 0 0 2-2v-.18a2 2 0 0 1 1-1.73l.43-.25a2 2 0 0 1 2 0l.15.08a2 2 0 0 0 2.73-.73l.22-.39a2 2 0 0 0-.73-2.73l-.15-.08a2 2 0 0 1-1-1.74v-.5a2 2 0 0 1 1-1.74l.15-.09a2 2 0 0 0 .73-2.73l-.22-.38a2 2 0 0 0-2.73-.73l-.15.08a2 2 0 0 1-2 0l-.43-.25a2 2 0 0 1-1-1.73V4a2 2 0 0 0-2-2z"/><circle cx="12" cy="12" r="3"/>`)

	Play        = lucide(`<polygon points="6 3 20 12 6 21 6 3"/>`)
	Stop        = lucide(`<rect width="14" height="14" x="5" y="5" rx="2"/>`)
	Refresh     = lucide(`<path d="M3 12a9 9 0 0 1 9-9 9.75 9.75 0 0 1 6.74 2.74L21 8"/><path d="M21 3v5h-5"/><path d="M21 12a9 9 0 0 1-9 9 9.75 9.75 0 0 1-6.74-2.74L3 16"/><path d="M8 16H3v5"/>`)
	Zap         = lucide(`<path d="M4 14a1 1 0 0 1-.78-1.63l9.9-10.2a.5.5 0 0 1 .86.46l-1.92 6.02A1 1 0 0 0 13 10h7a1 1 0 0 1 .78 1.63l-9.9 10.2a.5.5 0 0 1-.86-.46l1.92-6.02A1 1 0 0 0 11 14z"/>`)
	Plus        = lucide(`<path d="M5 12h14"/><path d="M12 5v14"/>`)
	Trash       = lucide(`<path d="M3 6h18"/><path d="M19 6v14c0 1-1 2-2 2H7c-1 0-2-1-2-2V6"/><path d="M8 6V4c0-1 1-2 2-2h4c1 0 2 1 2 2v2"/>`)
	Edit        = lucide(`<path d="M12 3H5a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"/><path d="M18.375 2.625a1 1 0 0 1 3 3l-9.013 9.014a2 2 0 0 1-.853.505l-2.873.84a.5.5 0 0 1-.62-.62l.84-2.873a2 2 0 0 1 .506-.852z"/>`)
	Code        = lucide(`<polyline points="16 18 22 12 16 6"/><polyline points="8 6 2 12 8 18"/>`)
	Folder      = lucide(`<path d="M20 20a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.9a2 2 0 0 1-1.69-.9L9.6 3.9A2 2 0 0 0 7.93 3H4a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2Z"/>`)
	Copy        = lucide(`<rect width="14" height="14" x="8" y="8" rx="2" ry="2"/><path d="M4 16c-1.1 0-2-.9-2-2V4c0-1.1.9-2 2-2h10c1.1 0 2 .9 2 2"/>`)
	Download    = lucide(`<path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" x2="12" y1="15" y2="3"/>`)
	Upload      = lucide(`<path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="17 8 12 3 7 8"/><line x1="12" x2="12" y1="3" y2="15"/>`)
	ArrowUp     = lucide(`<path d="m5 12 7-7 7 7"/><path d="M12 19V5"/>`)
	ArrowDown   = lucide(`<path d="M12 5v14"/><path d="m19 12-7 7-7-7"/>`)
	Shield      = lucide(`<path d="M20 13c0 5-3.5 7.5-7.66 8.95a1 1 0 0 1-.67-.01C7.5 20.5 4 18 4 13V6a1 1 0 0 1 1-1c2 0 4.5-1.2 6.24-2.72a1.17 1.17 0 0 1 1.52 0C14.51 3.81 17 5 19 5a1 1 0 0 1 1 1z"/>`)
	Check       = lucide(`<path d="M20 6 9 17l-5-5"/>`)
	X           = lucide(`<path d="M18 6 6 18"/><path d="m6 6 12 12"/>`)
	Pause       = lucide(`<rect x="14" y="4" width="4" height="16" rx="1"/><rect x="6" y="4" width="4" height="16" rx="1"/>`)
	Alert       = lucide(`<circle cx="12" cy="12" r="10"/><line x1="12" x2="12" y1="8" y2="12"/><line x1="12" x2="12.01" y1="16" y2="16"/>`)
	Sliders     = lucide(`<line x1="4" x2="4" y1="21" y2="14"/><line x1="4" x2="4" y1="10" y2="3"/><line x1="12" x2="12" y1="21" y2="12"/><line x1="12" x2="12" y1="8" y2="3"/><line x1="20" x2="20" y1="21" y2="16"/><line x1="20" x2="20" y1="12" y2="3"/><line x1="2" x2="6" y1="14" y2="14"/><line x1="10" x2="14" y1="8" y2="8"/><line x1="18" x2="22" y1="16" y2="16"/>`)
	WinMin      = lucide(`<path d="M5 12h14"/>`)
	WinMax      = lucide(`<rect width="12" height="12" x="6" y="6" rx="1.5"/>`)
	WinClose    = lucide(`<path d="M18 6 6 18"/><path d="m6 6 12 12"/>`)
	ChevronDown = lucide(`<path d="m6 9 6 6 6-6"/>`)
	ChevronUp   = lucide(`<path d="m18 15-6-6-6 6"/>`)
	Layers      = lucide(`<path d="m12.83 2.18a2 2 0 0 0-1.66 0L2.6 6.08a1 1 0 0 0 0 1.83l8.58 3.91a2 2 0 0 0 1.66 0l8.58-3.9a1 1 0 0 0 0-1.83Z"/><path d="m22 17.65-9.17 4.16a2 2 0 0 1-1.66 0L2 17.65"/><path d="m22 12.65-9.17 4.16a2 2 0 0 1-1.66 0L2 12.65"/>`)
)

// TrayPNG generates a crisp 32x32 PNG tray icon featuring the Livery aircraft tailfin mark.
func TrayPNG(running, sysProxy, tun, darkTaskbar bool) []byte {
	const size = 32
	const samples = 4
	img := image.NewNRGBA(image.Rect(0, 0, size, size))

	finColor := color.NRGBA{R: 115, G: 115, B: 115, A: 255}
	if darkTaskbar {
		finColor = color.NRGBA{R: 220, G: 224, B: 230, A: 255}
	}
	if running {
		finColor = color.NRGBA{R: 30, G: 138, B: 62, A: 255} // active green
		if tun {
			finColor = color.NRGBA{R: 56, G: 189, B: 248, A: 255} // aviation sky-blue when TUN active
		} else if sysProxy {
			finColor = color.NRGBA{R: 70, G: 191, B: 114, A: 255}
		}
	}

	for y := range size {
		for x := range size {
			cov := 0.0
			for sy := range samples {
				for sx := range samples {
					nx := (float64(x) + (float64(sx)+0.5)/samples) / float64(size)
					ny := (float64(y) + (float64(sy)+0.5)/samples) / float64(size)
					if ny >= 0.14 && ny <= 0.86 {
						t := (ny - 0.14) / 0.72
						xLeft := 0.62*(1-t) + 0.12*t
						xRight := 0.88*(1-t) + 0.72*t
						if nx >= xLeft && nx <= xRight {
							u := (nx - xLeft)
							// Cut two crisp diagonal livery speed-stripes parallel to the leading edge
							inGap1 := u >= 0.13 && u <= 0.195
							inGap2 := u >= 0.29 && u <= 0.345
							if !inGap1 && !inGap2 {
								cov += 1.0
							}
						}
					}
				}
			}
			alpha := cov / float64(samples*samples)
			if alpha > 0 {
				c := finColor
				c.A = uint8(float64(c.A) * alpha)
				img.SetNRGBA(x, y, c)
			}
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

// AppIconPNG generates a 256x256 PNG application icon for resources/icon.png
// featuring a midnight-navy squircle and an aircraft tailfin painted with diagonal livery stripes.
func AppIconPNG() []byte {
	const size = 256
	const samples = 4
	img := image.NewNRGBA(image.Rect(0, 0, size, size))

	coral := color.NRGBA{R: 255, G: 94, B: 58, A: 255}
	sky := color.NRGBA{R: 56, G: 189, B: 248, A: 255}
	white := color.NRGBA{R: 248, G: 250, B: 252, A: 255}
	cobalt := color.NRGBA{R: 37, G: 99, B: 235, A: 255}

	for y := range size {
		for x := range size {
			var rSum, gSum, bSum, aSum float64
			for sy := range samples {
				for sx := range samples {
					fx := float64(x) + (float64(sx)+0.5)/samples
					fy := float64(y) + (float64(sy)+0.5)/samples
					// Rounded square SDF
					px := math.Abs(fx-127.5) - 86
					py := math.Abs(fy-127.5) - 86
					dist := math.Hypot(math.Max(px, 0), math.Max(py, 0)) + math.Min(math.Max(px, py), 0) - 28
					if dist > 0 {
						continue
					}

					nx := fx / float64(size)
					ny := fy / float64(size)
					// Deep midnight-navy vertical gradient
					bgR := 18.0 + 14.0*(1.0-ny)
					bgG := 24.0 + 18.0*(1.0-ny)
					bgB := 40.0 + 24.0*(1.0-ny)
					c := color.NRGBA{R: uint8(bgR), G: uint8(bgG), B: uint8(bgB), A: 255}

					if ny >= 0.24 && ny <= 0.74 {
						t := (ny - 0.24) / 0.50
						xLeft := 0.65*(1-t) + 0.21*t
						xRight := 0.79*(1-t) + 0.66*t
						if nx >= xLeft && nx <= xRight {
							u := nx - xLeft
							switch {
							case u < 0.105:
								c = coral
							case u >= 0.125 && u < 0.225:
								c = sky
							case u >= 0.245 && u < 0.340:
								c = white
							case u >= 0.360:
								c = cobalt
							}
						}
					}

					rSum += float64(c.R)
					gSum += float64(c.G)
					bSum += float64(c.B)
					aSum += 255.0
				}
			}
			denom := float64(samples * samples)
			if aSum > 0 {
				img.SetNRGBA(x, y, color.NRGBA{
					R: uint8(rSum / aSum * 255.0),
					G: uint8(gSum / aSum * 255.0),
					B: uint8(bSum / aSum * 255.0),
					A: uint8(aSum / denom),
				})
			}
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}
