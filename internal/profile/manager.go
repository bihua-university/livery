package profile

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/bihua-university/livery/internal/model"
)

// Manager handles persistent storage for AppSettings, Profiles, and compiled runtime config.
type Manager struct {
	mu       sync.RWMutex
	dataDir  string
	settings model.AppSettings
	profiles []model.Profile
}

// NewManager initializes the data directories and loads saved settings and profiles.
func NewManager(dataDir string) (*Manager, error) {
	if err := os.MkdirAll(filepath.Join(dataDir, "profiles"), 0o755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(dataDir, "bin"), 0o755); err != nil {
		return nil, err
	}
	m := &Manager{
		dataDir:  dataDir,
		settings: model.DefaultSettings(),
	}
	_ = m.loadSettings()
	_ = m.loadProfiles()
	if len(m.profiles) == 0 {
		p, err := m.CreateBlank("Default Profile")
		if err == nil {
			m.settings.ActiveProfileID = p.ID
			_ = m.SaveSettings(m.settings)
		}
	} else if m.settings.ActiveProfileID == "" {
		m.settings.ActiveProfileID = m.profiles[0].ID
		_ = m.SaveSettings(m.settings)
	}
	return m, nil
}

// DataDir returns the root directory for Livery data.
func (m *Manager) DataDir() string { return m.dataDir }

// BinDir returns the directory where downloaded core binaries live.
func (m *Manager) BinDir() string { return filepath.Join(m.dataDir, "bin") }

// RuntimeConfigPath returns the path to the compiled runtime config (runtime.yaml or runtime.json).
func (m *Manager) RuntimeConfigPath() string {
	if model.NormalizeCoreType(m.Settings().CoreType) == model.CoreSingBox {
		return filepath.Join(m.dataDir, "runtime.json")
	}
	return filepath.Join(m.dataDir, "runtime.yaml")
}

// Settings returns a copy of current AppSettings.
func (m *Manager) Settings() model.AppSettings {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cp := m.settings
	cp.CustomRules = slices.Clone(m.settings.CustomRules)
	return cp
}

// SaveSettings persists updated AppSettings to disk.
func (m *Manager) SaveSettings(s model.AppSettings) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.saveSettingsLocked(s)
}

// UpdateSettings merges an edit into the latest preferences under the write lock.
func (m *Manager) UpdateSettings(edit func(*model.AppSettings)) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.settings
	s.CustomRules = slices.Clone(s.CustomRules)
	edit(&s)
	return m.saveSettingsLocked(s)
}

func (m *Manager) saveSettingsLocked(s model.AppSettings) error {
	if err := writeJSONAtomic(filepath.Join(m.dataDir, "settings.json"), s); err != nil {
		return err
	}
	m.settings = s
	m.settings.CustomRules = slices.Clone(s.CustomRules)
	return nil
}

// SaveInboundSetting merges a confirmed asynchronous listener change with
// any other preferences saved while the API request was in flight.
func (m *Manager) SaveInboundSetting(field string, want model.AppSettings) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.settings
	switch field {
	case "mixed-port":
		s.MixedPort = want.MixedPort
	case "allow-lan":
		s.AllowLAN = want.AllowLAN
	default:
		return fmt.Errorf("unknown inbound setting %s", field)
	}
	return m.saveSettingsLocked(s)
}

// Profiles returns a snapshot slice of all profiles.
func (m *Manager) Profiles() []model.Profile {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return slices.Clone(m.profiles)
}

// ActiveProfile returns the currently active profile if found.
func (m *Manager) ActiveProfile() (model.Profile, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, p := range m.profiles {
		if p.ID == m.settings.ActiveProfileID {
			return p, true
		}
	}
	if len(m.profiles) > 0 {
		return m.profiles[0], true
	}
	return model.Profile{}, false
}

// SetActive selects the active profile by ID and persists settings.
func (m *Manager) SetActive(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	found := false
	for _, p := range m.profiles {
		if p.ID == id {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("profile %q not found", id)
	}
	s := m.settings
	s.ActiveProfileID = id
	return m.saveSettingsLocked(s)
}

// ProfilePath returns the file path of a profile's raw JSON.
func (m *Manager) ProfilePath(p model.Profile) string {
	return filepath.Join(m.dataDir, "profiles", p.FileName)
}

// ReadProfileRaw returns the raw JSON bytes of a profile.
func (m *Manager) ReadProfileRaw(id string) ([]byte, error) {
	p, ok := m.byID(id)
	if !ok {
		return nil, fmt.Errorf("profile %q not found", id)
	}
	return os.ReadFile(m.ProfilePath(p))
}

// WriteProfileRaw validates and saves raw JSON or YAML content for a profile.
func (m *Manager) WriteProfileRaw(id string, content []byte) error {
	trimmed := bytes.TrimSpace(content)
	if len(trimmed) == 0 {
		return errors.New("profile content cannot be empty")
	}
	format := "mihomo"
	if trimmed[0] == '{' {
		var check map[string]any
		if err := json.Unmarshal(trimmed, &check); err != nil || check == nil {
			if err != nil {
				return fmt.Errorf("invalid JSON: %w", err)
			}
			return errors.New("invalid JSON object")
		}
		format = "sing-box"
	} else {
		var check map[string]any
		if err := yaml.Unmarshal(trimmed, &check); err != nil || check == nil {
			if err != nil {
				return fmt.Errorf("invalid YAML: %w", err)
			}
			return errors.New("invalid YAML mapping")
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.profiles {
		if m.profiles[i].ID == id {
			path := m.ProfilePath(m.profiles[i])
			if err := os.WriteFile(path, content, 0o644); err != nil {
				return err
			}
			m.profiles[i].Format = format
			m.profiles[i].UpdatedAt = time.Now()
			m.profiles[i].LastError = ""
			return m.saveProfilesLocked()
		}
	}
	return fmt.Errorf("profile %q not found", id)
}

// UpdateProfileMeta updates a profile's name, URL, auto-update settings, and overrides.
func (m *Manager) UpdateProfileMeta(updated model.Profile) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.profiles {
		if m.profiles[i].ID == updated.ID {
			updated.FileName = m.profiles[i].FileName
			updated.Format = m.profiles[i].Format
			m.profiles[i] = updated
			return m.saveProfilesLocked()
		}
	}
	return fmt.Errorf("profile %q not found", updated.ID)
}

// SetDNSMode persists just the selected profile's Mihomo DNS override.
func (m *Manager) SetDNSMode(id, mode string) error {
	if mode != "" && mode != "fake-ip" && mode != "redir-host" {
		return fmt.Errorf("unknown DNS mode %q", mode)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.profiles {
		p := &m.profiles[i]
		if p.ID == id {
			old := p.Override.DNSMode
			p.Override.DNSMode = mode
			if err := m.saveProfilesLocked(); err != nil {
				p.Override.DNSMode = old
				return err
			}
			return nil
		}
	}
	return fmt.Errorf("profile %q not found", id)
}

// CreateBlank creates a new valid starter profile matching the active core mode.
func (m *Manager) CreateBlank(name string) (model.Profile, error) {
	if strings.TrimSpace(name) == "" {
		name = "New Profile"
	}
	ct := model.NormalizeCoreType(m.Settings().CoreType)
	ext := ".yaml"
	format := "mihomo"
	starter := defaultMihomoStarterConfig
	if ct == model.CoreSingBox {
		ext = ".json"
		format = "sing-box"
		starter = defaultStarterConfig
	}
	id := newID()
	p := model.Profile{
		ID:              id,
		Name:            name,
		Kind:            "local",
		FileName:        id + ext,
		Format:          format,
		UpdatedAt:       time.Now(),
		UpdateIntervalH: 24,
		Override:        model.DefaultProfileOverride(),
	}
	if err := os.WriteFile(m.ProfilePath(p), []byte(starter), 0o644); err != nil {
		return model.Profile{}, err
	}
	m.mu.Lock()
	m.profiles = append(m.profiles, p)
	err := m.saveProfilesLocked()
	m.mu.Unlock()
	return p, err
}

// ImportLocal imports a local Mihomo/Clash YAML, sing-box JSON, or URI subscription file into the profile store.
func (m *Manager) ImportLocal(name, srcPath string) (model.Profile, error) {
	raw, err := os.ReadFile(srcPath)
	if err != nil {
		return model.Profile{}, err
	}
	data, format, ext, err := DetectAndNormalizeProfile(raw, m.Settings().CoreType)
	if err != nil {
		return model.Profile{}, err
	}
	if strings.TrimSpace(name) == "" {
		base := filepath.Base(srcPath)
		name = strings.TrimSuffix(base, filepath.Ext(base))
	}
	id := newID()
	p := model.Profile{
		ID:              id,
		Name:            name,
		Kind:            "local",
		FileName:        id + ext,
		Format:          format,
		UpdatedAt:       time.Now(),
		UpdateIntervalH: 24,
		Override:        model.DefaultProfileOverride(),
	}
	if err := os.WriteFile(m.ProfilePath(p), data, 0o644); err != nil {
		return model.Profile{}, err
	}
	m.mu.Lock()
	m.profiles = append(m.profiles, p)
	err = m.saveProfilesLocked()
	m.mu.Unlock()
	return p, err
}

// ImportRemote downloads a remote subscription (Mihomo/Clash YAML, sing-box JSON, or Base64/URI) and saves it as a profile.
func (m *Manager) ImportRemote(ctx context.Context, name, rawURL, userAgent string, useProxy bool) (model.Profile, error) {
	cleaned := unwrapSubDeepLink(strings.TrimSpace(rawURL))
	u, err := url.Parse(cleaned)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return model.Profile{}, errors.New("subscription URL must start with http:// or https://")
	}
	if strings.TrimSpace(name) == "" {
		name = u.Host
	}
	id := newID()
	p := model.Profile{
		ID:              id,
		Name:            name,
		Kind:            "remote",
		URL:             u.String(),
		UserAgent:       userAgent,
		AutoUpdate:      true,
		UpdateIntervalH: 24,
		UseProxyUpdate:  useProxy,
		FileName:        id + ".yaml",
		Override:        model.DefaultProfileOverride(),
	}
	res, err := m.fetchRemote(ctx, p)
	if err != nil {
		return model.Profile{}, err
	}
	if name == u.Host && res.name != "" {
		p.Name = res.name
	}
	p.Format = res.format
	p.FileName = id + res.ext
	p.Usage = res.usage
	p.UpdatedAt = time.Now()
	if err := os.WriteFile(m.ProfilePath(p), res.body, 0o644); err != nil {
		return model.Profile{}, err
	}
	m.mu.Lock()
	m.profiles = append(m.profiles, p)
	err = m.saveProfilesLocked()
	m.mu.Unlock()
	return p, err
}

// RefreshRemote updates an existing remote profile from its subscription URL.
func (m *Manager) RefreshRemote(ctx context.Context, id string) (model.Profile, error) {
	p, ok := m.byID(id)
	if !ok {
		return model.Profile{}, fmt.Errorf("profile %q not found", id)
	}
	if p.Kind != "remote" || p.URL == "" {
		return model.Profile{}, errors.New("profile is not a remote subscription")
	}
	res, err := m.fetchRemote(ctx, p)
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.profiles {
		if m.profiles[i].ID == id {
			if err != nil {
				m.profiles[i].LastError = err.Error()
				_ = m.saveProfilesLocked()
				return m.profiles[i], err
			}
			oldPath := m.ProfilePath(m.profiles[i])
			newFileName := m.profiles[i].ID + res.ext
			m.profiles[i].FileName = newFileName
			m.profiles[i].Format = res.format
			newPath := m.ProfilePath(m.profiles[i])
			if wErr := os.WriteFile(newPath, res.body, 0o644); wErr != nil {
				return m.profiles[i], wErr
			}
			if oldPath != newPath {
				_ = os.Remove(oldPath)
			}
			m.profiles[i].Usage = res.usage
			m.profiles[i].UpdatedAt = time.Now()
			m.profiles[i].LastError = ""
			_ = m.saveProfilesLocked()
			return m.profiles[i], nil
		}
	}
	return p, nil
}

// Duplicate clones an existing profile.
func (m *Manager) Duplicate(id string) (model.Profile, error) {
	src, ok := m.byID(id)
	if !ok {
		return model.Profile{}, fmt.Errorf("profile %q not found", id)
	}
	raw, err := os.ReadFile(m.ProfilePath(src))
	if err != nil {
		return model.Profile{}, err
	}
	ext := filepath.Ext(src.FileName)
	if ext == "" {
		ext = ".json"
	}
	cp := src
	cp.ID = newID()
	cp.Name = src.Name + " (Copy)"
	cp.FileName = cp.ID + ext
	cp.UpdatedAt = time.Now()
	if err := os.WriteFile(m.ProfilePath(cp), raw, 0o644); err != nil {
		return model.Profile{}, err
	}
	m.mu.Lock()
	m.profiles = append(m.profiles, cp)
	err = m.saveProfilesLocked()
	m.mu.Unlock()
	return cp, err
}

// Delete removes a profile and its file (keeps at least one profile).
func (m *Manager) Delete(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.profiles) <= 1 {
		return errors.New("at least one profile must remain")
	}
	idx := -1
	for i, p := range m.profiles {
		if p.ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("profile %q not found", id)
	}
	target := m.profiles[idx]
	_ = os.Remove(m.ProfilePath(target))
	m.profiles = append(m.profiles[:idx], m.profiles[idx+1:]...)
	if m.settings.ActiveProfileID == id {
		m.settings.ActiveProfileID = m.profiles[0].ID
		_ = writeJSONAtomic(filepath.Join(m.dataDir, "settings.json"), m.settings)
	}
	return m.saveProfilesLocked()
}

// CompileActive compiles the active profile with current settings and writes runtime config.
func (m *Manager) CompileActive() (string, []byte, error) {
	compiled, err := m.RenderActive(m.Settings())
	if err != nil {
		return "", nil, err
	}
	outPath, err := m.SaveRuntimeConfig(compiled)
	if err != nil {
		return "", nil, err
	}
	return outPath, compiled, nil
}

// SaveRuntimeConfig replaces the runtime file only after a complete write.
func (m *Manager) SaveRuntimeConfig(compiled []byte) (string, error) {
	path := m.RuntimeConfigPath()
	return path, writeAtomic(path, compiled)
}

// RenderActive compiles proposed settings without replacing the running core's file.
func (m *Manager) RenderActive(settings model.AppSettings) ([]byte, error) {
	prof, ok := m.ActiveProfile()
	if !ok {
		return nil, errors.New("no active profile")
	}
	raw, err := os.ReadFile(m.ProfilePath(prof))
	if err != nil {
		return nil, err
	}
	return CompileForCore(raw, prof, settings)
}

// ActiveRules parses routing rules from the active profile without writing runtime config to disk.
func (m *Manager) ActiveRules() []model.RuleEntry {
	prof, ok := m.ActiveProfile()
	if !ok {
		return nil
	}
	raw, err := os.ReadFile(m.ProfilePath(prof))
	if err != nil {
		return nil
	}
	compiled, err := CompileForCore(raw, prof, m.Settings())
	if err != nil {
		return nil
	}
	return ParseRulesFromConfig(compiled)
}

type remoteFetchResult struct {
	body   []byte
	usage  model.SubscriptionInfo
	name   string
	format string
	ext    string
}

func (m *Manager) fetchRemote(ctx context.Context, p model.Profile) (remoteFetchResult, error) {
	s := m.Settings()
	tr := &http.Transport{Proxy: nil}
	defer tr.CloseIdleConnections()
	if p.UseProxyUpdate {
		if proxyURL, err := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", s.MixedPort)); err == nil {
			tr.Proxy = http.ProxyURL(proxyURL)
		}
	}
	client := &http.Client{Transport: tr, Timeout: 25 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.URL, nil)
	if err != nil {
		return remoteFetchResult{}, err
	}
	ua := strings.TrimSpace(p.UserAgent)
	if ua == "" {
		if model.NormalizeCoreType(s.CoreType) == model.CoreSingBox {
			ua = "sing-box 1.11.0 (Livery)"
		} else {
			ua = "mihomo/1.19.0 (clash.meta; Livery)"
		}
	}
	req.Header.Set("User-Agent", ua)

	resp, err := client.Do(req)
	if err != nil {
		return remoteFetchResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return remoteFetchResult{}, fmt.Errorf("HTTP %d fetching subscription", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return remoteFetchResult{}, err
	}
	formatted, format, ext, err := DetectAndNormalizeProfile(body, s.CoreType)
	if err != nil {
		return remoteFetchResult{}, err
	}
	return remoteFetchResult{
		body:   formatted,
		usage:  parseUserInfo(resp.Header.Get("Subscription-Userinfo")),
		name:   parseContentDispositionName(resp.Header.Get("Content-Disposition")),
		format: format,
		ext:    ext,
	}, nil
}

func parseUserInfo(hdr string) model.SubscriptionInfo {
	var info model.SubscriptionInfo
	for _, part := range strings.Split(hdr, ";") {
		k, v, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		if err != nil {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(k)) {
		case "upload":
			info.Upload = n
		case "download":
			info.Download = n
		case "total":
			info.Total = n
		case "expire":
			info.Expire = n
		}
	}
	return info
}

func parseContentDispositionName(hdr string) string {
	for _, part := range strings.Split(hdr, ";") {
		k, v, ok := strings.Cut(strings.TrimSpace(part), "=")
		if ok && strings.EqualFold(k, "filename") {
			v = strings.Trim(v, `" '`)
			return strings.TrimSuffix(v, filepath.Ext(v))
		}
	}
	return ""
}

func (m *Manager) byID(id string) (model.Profile, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, p := range m.profiles {
		if p.ID == id {
			return p, true
		}
	}
	return model.Profile{}, false
}

func (m *Manager) loadSettings() error {
	b, err := os.ReadFile(filepath.Join(m.dataDir, "settings.json"))
	if err != nil {
		return err
	}
	s := model.DefaultSettings()
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	s = s.WithDefaults()
	m.mu.Lock()
	m.settings = s
	m.mu.Unlock()
	return nil
}

func (m *Manager) loadProfiles() error {
	b, err := os.ReadFile(filepath.Join(m.dataDir, "profiles.json"))
	if err != nil {
		return err
	}
	var list []model.Profile
	if err := json.Unmarshal(b, &list); err != nil {
		return err
	}
	m.mu.Lock()
	m.profiles = list
	m.mu.Unlock()
	return nil
}

func (m *Manager) saveProfilesLocked() error {
	return writeJSONAtomic(filepath.Join(m.dataDir, "profiles.json"), m.profiles)
}

func writeJSONAtomic(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(path, append(b, '\n'))
}

func writeAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".livery-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func newID() string {
	var b [6]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func unwrapSubDeepLink(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	if u.Scheme == "clash" || u.Scheme == "sing-box" {
		if target := u.Query().Get("url"); target != "" {
			return target
		}
	}
	return raw
}

// ParseClashInstallURL recognizes the subscription installation action from the OS.
func ParseClashInstallURL(raw string) (target, name string, ok bool) {
	link, err := url.Parse(raw)
	if err != nil || link.Scheme != "clash" || !strings.EqualFold(link.Host, "install-config") || link.User != nil || (link.Path != "" && link.Path != "/") {
		return "", "", false
	}
	query, err := url.ParseQuery(link.RawQuery)
	if err != nil || len(query["url"]) != 1 {
		return "", "", false
	}
	sub, err := url.Parse(query.Get("url"))
	if err != nil || (sub.Scheme != "https" && sub.Scheme != "http") || sub.Hostname() == "" {
		return "", "", false
	}
	return sub.String(), strings.TrimSpace(query.Get("name")), true
}
