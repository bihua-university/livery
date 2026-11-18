package core

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bihua-university/livery/internal/model"
)

const (
	singGeoIPURL   = "https://github.com/SagerNet/sing-geoip/releases/latest/download/geoip.db"
	singGeoSiteURL = "https://github.com/SagerNet/sing-geosite/releases/latest/download/geosite.db"
)

// GeoDataDir returns the default directory where GeoIP/GeoSite databases are stored for coreType.
func GeoDataDir(coreType model.CoreType, dataDir string) string {
	if model.NormalizeCoreType(coreType) == model.CoreSingBox {
		return dataDir
	}
	return filepath.Join(dataDir, "mihomo")
}

// ResolveGeoDataDir returns customDir when non-empty, otherwise falling back to GeoDataDir(coreType, dataDir).
func ResolveGeoDataDir(coreType model.CoreType, dataDir, customDir string) string {
	trimmed := strings.TrimSpace(customDir)
	if trimmed != "" {
		return filepath.Clean(trimmed)
	}
	return GeoDataDir(coreType, dataDir)
}

// EffectiveGeoDataDir resolves the active GeoIP/GeoSite directory from settings and dataDir.
func EffectiveGeoDataDir(settings model.AppSettings, dataDir string) string {
	return ResolveGeoDataDir(settings.CoreType, dataDir, settings.GeoDataDir)
}

type geoTarget struct {
	name string
	url  string
}

func geoTargetsForSettings(settings model.AppSettings) []geoTarget {
	ct := model.NormalizeCoreType(settings.CoreType)
	if ct == model.CoreSingBox {
		return []geoTarget{
			{name: "geoip.db", url: singGeoIPURL},
			{name: "geosite.db", url: singGeoSiteURL},
		}
	}

	mmdbName := "geoip.metadb"
	mmdbURL := strings.TrimSpace(settings.MMDBURL)
	if mmdbURL == "" {
		mmdbURL = model.DefaultMMDBURL
	}
	if strings.HasSuffix(strings.ToLower(mmdbURL), "country.mmdb") {
		mmdbName = "Country.mmdb"
	}

	geoipURL := strings.TrimSpace(settings.GeoIPURL)
	if geoipURL == "" {
		geoipURL = model.DefaultGeoIPURL
	}
	geositeURL := strings.TrimSpace(settings.GeoSiteURL)
	if geositeURL == "" {
		geositeURL = model.DefaultGeoSiteURL
	}
	asnURL := strings.TrimSpace(settings.ASNURL)
	if asnURL == "" {
		asnURL = model.DefaultASNURL
	}

	return []geoTarget{
		{name: mmdbName, url: mmdbURL},
		{name: "geoip.dat", url: geoipURL},
		{name: "geosite.dat", url: geositeURL},
		{name: "GeoLite2-ASN.mmdb", url: asnURL},
	}
}

// InspectGeoFiles checks local GeoIP/GeoSite database files in the configured directory,
// falling back to the default core GeoData directory for compatibility when a custom directory is set.
func InspectGeoFiles(settings model.AppSettings, dataDir string) []model.GeoFileStatus {
	primaryDir := EffectiveGeoDataDir(settings, dataDir)
	defaultDir := GeoDataDir(settings.CoreType, dataDir)
	targets := geoTargetsForSettings(settings)
	out := make([]model.GeoFileStatus, 0, len(targets))
	for _, t := range targets {
		p := filepath.Join(primaryDir, t.name)
		st := model.GeoFileStatus{
			Name: t.name,
			Path: p,
		}
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			st.Exists = true
			st.Size = info.Size()
			st.UpdatedAt = info.ModTime()
		} else if primaryDir != defaultDir {
			fallbackPath := filepath.Join(defaultDir, t.name)
			if info, err := os.Stat(fallbackPath); err == nil && !info.IsDir() {
				st.Path = fallbackPath
				st.Exists = true
				st.Size = info.Size()
				st.UpdatedAt = info.ModTime()
			}
		}
		out = append(out, st)
	}
	return out
}

// DownloadGeoDatabases downloads GeoIP/GeoSite databases atomically into the effective GeoData directory.
func DownloadGeoDatabases(ctx context.Context, settings model.AppSettings, dataDir string, useProxy bool, onProgress func(string)) ([]model.GeoFileStatus, error) {
	if onProgress == nil {
		onProgress = func(string) {}
	}
	dir := EffectiveGeoDataDir(settings, dataDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}

	tr := &http.Transport{Proxy: nil}
	defer tr.CloseIdleConnections()
	if useProxy && settings.MixedPort > 0 {
		if pu, err := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", settings.MixedPort)); err == nil {
			tr.Proxy = http.ProxyURL(pu)
		}
	}
	client := &http.Client{Transport: tr, Timeout: 120 * time.Second}

	targets := geoTargetsForSettings(settings)
	var errs []string
	downloaded := 0
	for i, t := range targets {
		if t.url == "" {
			continue
		}
		onProgress(fmt.Sprintf("(%d/%d) Downloading %s...", i+1, len(targets), t.name))
		dest := filepath.Join(dir, t.name)
		if err := downloadFileAtomic(ctx, client, t.url, dest); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", t.name, err))
			continue
		}
		downloaded++
	}

	files := InspectGeoFiles(settings, dataDir)
	if downloaded == 0 && len(errs) > 0 {
		return files, errors.New(strings.Join(errs, "; "))
	}
	if len(errs) > 0 {
		return files, fmt.Errorf("partial update (%d/%d): %s", downloaded, len(targets), strings.Join(errs, "; "))
	}
	return files, nil
}

func downloadFileAtomic(ctx context.Context, client *http.Client, rawURL, destPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Livery")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	tmpPath := destPath + ".tmp"
	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(f, io.LimitReader(resp.Body, 256<<20))
	closeErr := f.Close()
	if copyErr != nil {
		_ = os.Remove(tmpPath)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmpPath)
		return closeErr
	}
	if err := os.Rename(tmpPath, destPath); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}
