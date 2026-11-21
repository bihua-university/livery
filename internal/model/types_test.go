package model

import (
	"testing"

	"github.com/bihua-university/livery/internal/sysproxy"
)

func TestDefaultProfileOverride(t *testing.T) {
	got := DefaultProfileOverride()
	if !got.OverrideInbounds || !got.InjectClashAPI || !got.OverrideLog || got.DNSMode != "" || got.PatchJSON != "" {
		t.Fatalf("unexpected DefaultProfileOverride: %+v", got)
	}
}

func TestAppSettingsWithDefaults(t *testing.T) {
	def := DefaultSettings()

	got := (AppSettings{}).WithDefaults()
	if got.CoreType != CoreMihomo ||
		got.ClashAPIHost != DefaultClashAPIHost ||
		got.ClashAPIPort != DefaultClashAPIPort ||
		got.MixedPort != DefaultMixedPort ||
		got.DelayTestURL != DefaultDelayTestURL ||
		got.DelayTestTimeoutMs != DefaultDelayTestTimeoutMs ||
		got.TUNStack != DefaultTUNStack ||
		got.TUNMTU != DefaultTUNMTU ||
		got.TUNInterfaceName != DefaultTUNInterfaceName ||
		got.UIFontSize != def.UIFontSize ||
		got.ThemeMode != ThemeSystem ||
		got.Language != LangZH ||
		got.CloseAction != CloseToTray ||
		got.ProxyMode != ModeRule ||
		got.GeoUpdateIntervalH != def.GeoUpdateIntervalH ||
		got.GeoIPURL != DefaultGeoIPURL ||
		got.GeoSiteURL != DefaultGeoSiteURL ||
		got.MMDBURL != DefaultMMDBURL ||
		got.ASNURL != DefaultASNURL ||
		got.SystemProxyBypass != sysproxy.DefaultBypass() {
		t.Fatalf("WithDefaults did not populate zero fields: %+v", got)
	}

	custom := AppSettings{
		CoreType:           "singbox",
		ClashAPIHost:       "0.0.0.0",
		ClashAPIPort:       19090,
		MixedPort:          17890,
		DelayTestURL:       "https://example.com/204",
		DelayTestTimeoutMs: 3000,
		TUNStack:           "gvisor",
		TUNMTU:             1500,
		TUNInterfaceName:   "utun9",
		UIFontSize:         16,
		ThemeMode:          ThemeDark,
		Language:           LangEN,
		CloseAction:        CloseQuit,
		ProxyMode:          ModeGlobal,
		GeoUpdateIntervalH: 48,
		GeoIPURL:           "https://mirror.example/geoip.dat",
		GeoSiteURL:         "https://mirror.example/geosite.dat",
		MMDBURL:            "https://mirror.example/mmdb",
		ASNURL:             "https://mirror.example/asn",
		SystemProxyBypass:  "localhost;*.internal",
	}.WithDefaults()
	if custom.CoreType != CoreSingBox || custom.UIFontSize != 16 || custom.SystemProxyBypass != "localhost;*.internal" || custom.ClashAPIPort != 19090 {
		t.Fatalf("WithDefaults overwrote valid custom values: %+v", custom)
	}

	outOfRange := AppSettings{
		ClashAPIPort:       -1,
		MixedPort:          -5,
		DelayTestTimeoutMs: -10,
		TUNMTU:             -100,
		UIFontSize:         24,
		GeoUpdateIntervalH: -2,
	}.WithDefaults()
	if outOfRange.ClashAPIPort != DefaultClashAPIPort ||
		outOfRange.MixedPort != DefaultMixedPort ||
		outOfRange.DelayTestTimeoutMs != DefaultDelayTestTimeoutMs ||
		outOfRange.TUNMTU != DefaultTUNMTU ||
		outOfRange.UIFontSize != def.UIFontSize ||
		outOfRange.GeoUpdateIntervalH != def.GeoUpdateIntervalH {
		t.Fatalf("WithDefaults did not reset invalid values: %+v", outOfRange)
	}
}
