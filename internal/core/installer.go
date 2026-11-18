package core

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/bihua-university/livery/internal/model"
)

type releaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

func defaultCoreBinaryName(coreType model.CoreType) string {
	targetName := "mihomo"
	if model.NormalizeCoreType(coreType) == model.CoreSingBox {
		targetName = "sing-box"
	}
	if runtime.GOOS == "windows" {
		targetName += ".exe"
	}
	return targetName
}

// ResolveCoreDownloadTarget determines the destination directory and binary filename for downloading coreType.
// When configured is empty, it uses defaultBinDir and the default platform binary name.
// When configured points to a directory (existing dir, trailing separator, or extensionless Windows path),
// it places the default binary name inside that directory; otherwise it treats configured as a file path.
func ResolveCoreDownloadTarget(coreType model.CoreType, defaultBinDir, configured string) (string, string) {
	defaultName := defaultCoreBinaryName(coreType)
	raw := strings.TrimSpace(configured)
	if raw == "" {
		return defaultBinDir, defaultName
	}

	hasTrailingSep := strings.HasSuffix(raw, "/") || strings.HasSuffix(raw, "\\")
	cleaned := filepath.Clean(raw)
	if hasTrailingSep {
		return cleaned, defaultName
	}
	if st, err := os.Stat(cleaned); err == nil {
		if st.IsDir() {
			return cleaned, defaultName
		}
		return filepath.Dir(cleaned), filepath.Base(cleaned)
	}
	if runtime.GOOS == "windows" && filepath.Ext(cleaned) == "" {
		return cleaned, defaultName
	}
	return filepath.Dir(cleaned), filepath.Base(cleaned)
}

// DownloadLatest downloads and extracts the latest release binary for coreType into binDir.
func DownloadLatest(ctx context.Context, coreType model.CoreType, binDir string, onProgress func(string)) (string, error) {
	return DownloadLatestTo(ctx, coreType, binDir, "", onProgress)
}

// DownloadLatestTo downloads and extracts the latest release binary for coreType into targetDir/targetName.
// If targetName is empty, the default platform binary filename is used.
func DownloadLatestTo(ctx context.Context, coreType model.CoreType, targetDir, targetName string, onProgress func(string)) (string, error) {
	if onProgress == nil {
		onProgress = func(string) {}
	}
	ct := model.NormalizeCoreType(coreType)
	coreName := model.CoreDisplayName(ct)
	onProgress(fmt.Sprintf("Querying latest %s release...", coreName))

	apiURL := "https://api.github.com/repos/MetaCubeX/mihomo/releases/latest"
	if ct == model.CoreSingBox {
		apiURL = "https://api.github.com/repos/SagerNet/sing-box/releases/latest"
	}

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	client := &http.Client{Transport: tr, Timeout: 90 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "Livery")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API returned HTTP %d", resp.StatusCode)
	}

	var rel struct {
		TagName string         `json:"tag_name"`
		Assets  []releaseAsset `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return "", err
	}

	var downloadURL, assetName string
	if ct == model.CoreSingBox {
		suffix := platformAssetSuffix()
		if suffix == "" {
			return "", fmt.Errorf("unsupported platform %s/%s for auto-download", runtime.GOOS, runtime.GOARCH)
		}
		for _, a := range rel.Assets {
			if strings.HasSuffix(a.Name, suffix) && !strings.Contains(a.Name, "with-core") {
				downloadURL = a.BrowserDownloadURL
				assetName = a.Name
				break
			}
		}
		if downloadURL == "" {
			return "", fmt.Errorf("no release asset matching *%s found in %s", suffix, rel.TagName)
		}
	} else {
		downloadURL, assetName = selectMihomoAsset(rel.Assets, rel.TagName)
		if downloadURL == "" {
			return "", fmt.Errorf("no mihomo release asset matching %s/%s found in %s", runtime.GOOS, runtime.GOARCH, rel.TagName)
		}
	}

	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return "", err
	}

	onProgress(fmt.Sprintf("Downloading %s...", assetName))
	dlReq, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return "", err
	}
	dlReq.Header.Set("User-Agent", "Livery")
	dlResp, err := client.Do(dlReq)
	if err != nil {
		return "", err
	}
	defer dlResp.Body.Close()
	if dlResp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download failed with HTTP %d", dlResp.StatusCode)
	}

	archiveFile, err := os.CreateTemp(targetDir, "livery-archive-*")
	if err != nil {
		return "", err
	}
	archivePath := archiveFile.Name()
	archiveSize, copyErr := io.Copy(archiveFile, io.LimitReader(dlResp.Body, 128<<20))
	closeErr := archiveFile.Close()
	if copyErr != nil {
		_ = os.Remove(archivePath)
		return "", copyErr
	}
	if closeErr != nil {
		_ = os.Remove(archivePath)
		return "", closeErr
	}
	defer os.Remove(archivePath)

	onProgress(fmt.Sprintf("Extracting %s binary...", coreName))
	archiveTargetName := defaultCoreBinaryName(ct)
	destName := strings.TrimSpace(targetName)
	if destName == "" {
		destName = archiveTargetName
	}
	destPath := filepath.Join(targetDir, destName)
	tmpPath := destPath + ".tmp"

	switch {
	case strings.HasSuffix(assetName, ".zip"):
		err = extractFromZipFile(archivePath, archiveSize, archiveTargetName, ct, tmpPath)
	case strings.HasSuffix(assetName, ".tar.gz"):
		err = extractFromTarGzFile(archivePath, archiveTargetName, tmpPath)
	case strings.HasSuffix(assetName, ".gz"):
		err = extractFromGzipFile(archivePath, tmpPath)
	default:
		return "", fmt.Errorf("unsupported archive format: %s", assetName)
	}
	if err != nil {
		_ = os.Remove(tmpPath)
		return "", err
	}

	if err := os.Rename(tmpPath, destPath); err != nil {
		_ = os.Remove(tmpPath)
		return "", err
	}
	return destPath, nil
}

func selectMihomoAsset(assets []releaseAsset, tagName string) (string, string) {
	prefix := fmt.Sprintf("mihomo-%s-%s-", runtime.GOOS, runtime.GOARCH)
	bestScore := -1
	var bestURL, bestName string

	for _, a := range assets {
		if !strings.HasPrefix(a.Name, prefix) {
			continue
		}
		if !strings.HasSuffix(a.Name, ".zip") && !strings.HasSuffix(a.Name, ".gz") {
			continue
		}
		rest := strings.TrimPrefix(a.Name, prefix)
		if strings.Contains(strings.ToLower(rest), "alpha") {
			continue
		}
		score := 20
		switch {
		case tagName != "" && strings.HasPrefix(rest, tagName+"."):
			score = 100
		case strings.HasPrefix(rest, "compatible-"):
			score = 80
		case !strings.Contains(rest, "go120") && !strings.Contains(rest, "go122") &&
			!strings.HasPrefix(rest, "v1-") && !strings.HasPrefix(rest, "v2-") && !strings.HasPrefix(rest, "v3-"):
			score = 60
		}
		if runtime.GOOS == "windows" && strings.HasSuffix(a.Name, ".zip") {
			score += 10
		}
		if score > bestScore {
			bestScore = score
			bestURL = a.BrowserDownloadURL
			bestName = a.Name
		}
	}
	return bestURL, bestName
}

func platformAssetSuffix() string {
	osPart := runtime.GOOS
	archPart := runtime.GOARCH
	switch osPart {
	case "windows":
		ext := ".zip"
		if archPart == "amd64" || archPart == "arm64" || archPart == "386" {
			return fmt.Sprintf("windows-%s%s", archPart, ext)
		}
	case "darwin":
		if archPart == "amd64" || archPart == "arm64" {
			return fmt.Sprintf("darwin-%s.tar.gz", archPart)
		}
	case "linux":
		if archPart == "amd64" || archPart == "arm64" {
			return fmt.Sprintf("linux-%s.tar.gz", archPart)
		}
	}
	return ""
}

func writeStreamToFile(r io.Reader, destPath string, limit int64) error {
	out, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, io.LimitReader(r, limit))
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func extractFromZipFile(archivePath string, size int64, target string, coreType model.CoreType, destPath string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	zr, err := zip.NewReader(f, size)
	if err != nil {
		return err
	}
	for _, entry := range zr.File {
		if filepath.Base(entry.Name) == target {
			rc, err := entry.Open()
			if err != nil {
				return err
			}
			defer rc.Close()
			return writeStreamToFile(rc, destPath, 256<<20)
		}
	}
	if model.NormalizeCoreType(coreType) == model.CoreMihomo {
		for _, entry := range zr.File {
			if entry.FileInfo().IsDir() {
				continue
			}
			base := strings.ToLower(filepath.Base(entry.Name))
			if strings.HasPrefix(base, "mihomo") && (runtime.GOOS != "windows" || strings.HasSuffix(base, ".exe")) {
				rc, err := entry.Open()
				if err != nil {
					return err
				}
				defer rc.Close()
				return writeStreamToFile(rc, destPath, 256<<20)
			}
		}
	}
	return fmt.Errorf("%s binary not found inside zip archive", model.CoreDisplayName(coreType))
}

func extractFromGzipFile(archivePath, destPath string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	return writeStreamToFile(gz, destPath, 256<<20)
}

func extractFromTarGzFile(archivePath, target, destPath string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if filepath.Base(hdr.Name) == target && hdr.Typeflag == tar.TypeReg {
			return writeStreamToFile(tr, destPath, 256<<20)
		}
	}
	return errors.New("sing-box binary not found inside tar.gz archive")
}
