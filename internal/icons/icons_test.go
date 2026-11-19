package icons

import (
	"bytes"
	"image/png"
	"testing"
)

func TestIconsAndPNGGeneration(t *testing.T) {
	if Livery == nil {
		t.Fatal("expected Livery SVG icon to be initialized")
	}

	tray := TrayPNG(true, true, false, true)
	cfg, err := png.DecodeConfig(bytes.NewReader(tray))
	if err != nil {
		t.Fatalf("decode TrayPNG: %v", err)
	}
	if cfg.Width != 32 || cfg.Height != 32 {
		t.Fatalf("expected 32x32 TrayPNG, got %dx%d", cfg.Width, cfg.Height)
	}

	tray2 := TrayPNG(true, true, false, true)
	if len(tray) == 0 || &tray[0] != &tray2[0] {
		t.Fatal("expected TrayPNG to return cached slice for identical visual state")
	}
	trayStopped := TrayPNG(false, false, false, true)
	if bytes.Equal(tray, trayStopped) {
		t.Fatal("expected distinct TrayPNG bytes for stopped vs running state")
	}

	appIcon := AppIconPNG()
	appCfg, err := png.DecodeConfig(bytes.NewReader(appIcon))
	if err != nil {
		t.Fatalf("decode AppIconPNG: %v", err)
	}
	if appCfg.Width != 256 || appCfg.Height != 256 {
		t.Fatalf("expected 256x256 AppIconPNG, got %dx%d", appCfg.Width, appCfg.Height)
	}
}
