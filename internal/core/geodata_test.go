package core

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/bihua-university/livery/internal/model"
)

func TestDownloadAndInspectGeoDatabases(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("geo-data-" + r.URL.Path))
	}))
	defer srv.Close()

	dataDir := t.TempDir()
	settings := model.DefaultSettings()
	settings.CoreType = model.CoreMihomo
	settings.GeoIPURL = srv.URL + "/geoip.dat"
	settings.GeoSiteURL = srv.URL + "/geosite.dat"
	settings.MMDBURL = srv.URL + "/geoip.metadb"
	settings.ASNURL = srv.URL + "/GeoLite2-ASN.mmdb"

	before := InspectGeoFiles(settings, dataDir)
	if len(before) != 4 {
		t.Fatalf("expected 4 geo file entries, got %d", len(before))
	}
	for _, f := range before {
		if f.Exists {
			t.Fatalf("expected %s to not exist initially", f.Name)
		}
	}

	files, err := DownloadGeoDatabases(context.Background(), settings, dataDir, false, nil)
	if err != nil {
		t.Fatalf("DownloadGeoDatabases: %v", err)
	}
	for _, f := range files {
		if !f.Exists || f.Size == 0 {
			t.Fatalf("expected %s to exist and be non-empty, got %+v", f.Name, f)
		}
	}
}

func TestCustomGeoDataDirAndDefaultCompatibility(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("geo-data-" + r.URL.Path))
	}))
	defer srv.Close()

	dataDir := t.TempDir()
	customDir := filepath.Join(t.TempDir(), "custom-geo")

	settings := model.DefaultSettings()
	settings.CoreType = model.CoreMihomo
	settings.GeoDataDir = customDir
	settings.GeoIPURL = srv.URL + "/geoip.dat"
	settings.GeoSiteURL = srv.URL + "/geosite.dat"
	settings.MMDBURL = srv.URL + "/geoip.metadb"
	settings.ASNURL = srv.URL + "/GeoLite2-ASN.mmdb"

	if got := EffectiveGeoDataDir(settings, dataDir); got != filepath.Clean(customDir) {
		t.Fatalf("expected EffectiveGeoDataDir %q, got %q", customDir, got)
	}

	// Default-dir compatibility: a file present only in the default dir should still be discovered.
	defaultMihomoDir := GeoDataDir(model.CoreMihomo, dataDir)
	if err := os.MkdirAll(defaultMihomoDir, 0o755); err != nil {
		t.Fatalf("mkdir default mihomo dir: %v", err)
	}
	fallbackGeoIP := filepath.Join(defaultMihomoDir, "geoip.dat")
	if err := os.WriteFile(fallbackGeoIP, []byte("fallback"), 0o644); err != nil {
		t.Fatalf("write fallback geoip: %v", err)
	}

	compatFiles := InspectGeoFiles(settings, dataDir)
	var foundFallback bool
	for _, f := range compatFiles {
		if f.Name == "geoip.dat" {
			foundFallback = f.Exists && f.Path == fallbackGeoIP
		}
	}
	if !foundFallback {
		t.Fatalf("expected geoip.dat to fall back to default dir %q, got %+v", fallbackGeoIP, compatFiles)
	}

	// Download should populate the configured custom directory and prefer it over fallback.
	files, err := DownloadGeoDatabases(context.Background(), settings, dataDir, false, nil)
	if err != nil {
		t.Fatalf("DownloadGeoDatabases to custom dir: %v", err)
	}
	for _, f := range files {
		wantPath := filepath.Join(customDir, f.Name)
		if !f.Exists || f.Path != wantPath {
			t.Fatalf("expected %s at %q, got %+v", f.Name, wantPath, f)
		}
	}
}
