package sysproxy

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
)

var opMu sync.Mutex

// Status describes the current OS system proxy configuration.
type Status struct {
	Enabled       bool   `json:"enabled"`
	Server        string `json:"server"`
	Bypass        string `json:"bypass"`
	AutoConfigURL string `json:"auto_config_url,omitempty"`
}

// MatchesTarget reports whether the OS system proxy is currently enabled and matches
// the requested host, port, and bypass list without an active PAC override.
func (s Status) MatchesTarget(host string, port int, bypass string) bool {
	if !s.Enabled || strings.TrimSpace(s.AutoConfigURL) != "" {
		return false
	}
	expectedServer := net.JoinHostPort(NormalizeHost(host), strconv.Itoa(port))
	if !strings.EqualFold(strings.TrimSpace(s.Server), expectedServer) {
		return false
	}
	return EqualBypass(s.Bypass, NormalizeBypass(bypass))
}

// DefaultBypass returns the platform-appropriate default proxy bypass list.
func DefaultBypass() string {
	return defaultBypass
}

// BypassSeparator returns the platform-native separator used in bypass strings.
func BypassSeparator() string {
	return bypassSeparator
}

// NormalizeHost resolves wildcard/empty bind addresses to loopback 127.0.0.1.
func NormalizeHost(host string) string {
	host = strings.TrimSpace(host)
	if host == "" || host == "0.0.0.0" || host == "::" {
		return "127.0.0.1"
	}
	return host
}

// ParseBypassList splits a bypass list separated by semicolons, commas, or newlines,
// trims whitespace, and removes empty or duplicate entries preserving order.
func ParseBypassList(raw string) []string {
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ';' || r == ',' || r == '\r' || r == '\n'
	})
	seen := make(map[string]struct{}, len(parts))
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		key := strings.ToLower(part)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, part)
	}
	return out
}

// NormalizeBypass formats a bypass string using the OS-native separator,
// falling back to DefaultBypass when empty.
func NormalizeBypass(raw string) string {
	items := ParseBypassList(raw)
	if len(items) == 0 {
		return defaultBypass
	}
	return strings.Join(items, bypassSeparator)
}

// FormatBypass builds a platform-formatted bypass string, optionally prepending
// the platform defaults before custom entries.
func FormatBypass(useDefault bool, custom string) string {
	customItems := ParseBypassList(custom)
	if len(customItems) == 0 {
		return defaultBypass
	}
	if !useDefault {
		return strings.Join(customItems, bypassSeparator)
	}
	return NormalizeBypass(defaultBypass + bypassSeparator + strings.Join(customItems, bypassSeparator))
}

// EqualBypass compares two bypass lists ignoring separator differences, duplicates, and case.
func EqualBypass(a, b string) bool {
	la := ParseBypassList(a)
	lb := ParseBypassList(b)
	if len(la) != len(lb) {
		return false
	}
	for i := range la {
		if !strings.EqualFold(la[i], lb[i]) {
			return false
		}
	}
	return true
}

// Enable configures the OS system proxy to point to host:port with the given bypass list.
// Writes are serialized and skipped when the OS state already matches the requested target.
// If a partial write fails mid-way, Disable rollback is attempted automatically.
func Enable(host string, port int, bypass string) error {
	if port <= 0 || port > 65535 {
		return fmt.Errorf("invalid proxy port: %d", port)
	}
	host = NormalizeHost(host)
	normBypass := NormalizeBypass(bypass)
	server := net.JoinHostPort(host, strconv.Itoa(port))

	opMu.Lock()
	defer opMu.Unlock()

	if cur, err := getSystemProxy(); err == nil && cur.MatchesTarget(host, port, normBypass) {
		return nil
	}

	partial, err := setSystemProxy(true, server, host, port, normBypass)
	if err != nil {
		if partial {
			_, _ = setSystemProxy(false, "", "", 0, "")
		}
		return err
	}
	return nil
}

// Disable turns off the OS system proxy.
func Disable() error {
	opMu.Lock()
	defer opMu.Unlock()

	if cur, err := getSystemProxy(); err == nil && !cur.Enabled {
		return nil
	}
	_, err := setSystemProxy(false, "", "", 0, "")
	return err
}

// Current reads the current OS system proxy state.
func Current() (Status, error) {
	opMu.Lock()
	defer opMu.Unlock()
	return getSystemProxy()
}
