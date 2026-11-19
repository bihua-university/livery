package sysproxy

import (
	"fmt"
	"net"
	"net/netip"
	"path"
	"runtime"
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
	return EqualBypass(s.Bypass, expectedOSBypass(bypass))
}

func expectedOSBypass(bypass string) string {
	items := ParseBypassList(NormalizeBypass(bypass))
	if runtime.GOOS == "windows" {
		return SanitizeWinINetBypass(items)
	}
	return strings.Join(WithoutWinINetLocal(items), bypassSeparator)
}

// DefaultBypass returns the platform-appropriate default proxy bypass list.
func DefaultBypass() string {
	return defaultBypass
}

// IsLegacyDefaultBypass reports whether raw matches the pre-Tailscale default bypass list.
func IsLegacyDefaultBypass(raw string) bool {
	return EqualBypass(raw, legacyDefaultBypass)
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

// SanitizeWinINetBypass prepares bypass entries for WinINet:
// trims trailing slashes (which cause WinINet to reject the entire list),
// brackets bare IPv6 literals (including zone suffixes like fe80::1%eth0),
// and deduplicates case-insensitively while preserving order.
func SanitizeWinINetBypass(items []string) string {
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, raw := range items {
		entry := strings.TrimRight(strings.TrimSpace(raw), "/")
		if entry == "" {
			continue
		}
		if isBareIPv6(entry) {
			entry = "[" + entry + "]"
		}
		key := strings.ToLower(entry)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, entry)
	}
	return strings.Join(out, ";")
}

func isBareIPv6(entry string) bool {
	addr := entry
	if host, _, ok := strings.Cut(entry, "%"); ok {
		addr = host
	}
	parsed, err := netip.ParseAddr(addr)
	return err == nil && parsed.Is6()
}

// WithoutWinINetLocal strips Windows-only "<local>" tokens so macOS and Linux
// do not interpret it as a literal hostname.
func WithoutWinINetLocal(items []string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" || strings.EqualFold(trimmed, "<local>") {
			continue
		}
		out = append(out, trimmed)
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

type sysCommand struct {
	Program  string
	Args     []string
	Optional bool
}

func parseDarwinNetworkServices(stdout string) []string {
	var services []string
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "*") || strings.HasPrefix(line, "An asterisk ") {
			continue
		}
		services = append(services, line)
	}
	return services
}

func darwinBypassArgs(bypass string) []string {
	domains := WithoutWinINetLocal(ParseBypassList(bypass))
	if len(domains) == 0 {
		return []string{"Empty"}
	}
	return domains
}

func darwinManualCommands(service, host string, port int, bypass string) []sysCommand {
	p := strconv.Itoa(port)
	bypassCmd := append([]string{"-setproxybypassdomains", service}, darwinBypassArgs(bypass)...)
	return []sysCommand{
		{Program: "/usr/sbin/networksetup", Args: []string{"-setautoproxystate", service, "off"}},
		{Program: "/usr/sbin/networksetup", Args: []string{"-setwebproxy", service, host, p}},
		{Program: "/usr/sbin/networksetup", Args: []string{"-setsecurewebproxy", service, host, p}},
		{Program: "/usr/sbin/networksetup", Args: []string{"-setsocksfirewallproxy", service, host, p}},
		{Program: "/usr/sbin/networksetup", Args: bypassCmd},
		{Program: "/usr/sbin/networksetup", Args: []string{"-setwebproxystate", service, "on"}},
		{Program: "/usr/sbin/networksetup", Args: []string{"-setsecurewebproxystate", service, "on"}},
		{Program: "/usr/sbin/networksetup", Args: []string{"-setsocksfirewallproxystate", service, "on"}},
	}
}

func darwinStopCommands(service string) []sysCommand {
	return []sysCommand{
		{Program: "/usr/sbin/networksetup", Args: []string{"-setautoproxystate", service, "off"}},
		{Program: "/usr/sbin/networksetup", Args: []string{"-setwebproxystate", service, "off"}},
		{Program: "/usr/sbin/networksetup", Args: []string{"-setsecurewebproxystate", service, "off"}},
		{Program: "/usr/sbin/networksetup", Args: []string{"-setsocksfirewallproxystate", service, "off"}},
		{Program: "/usr/sbin/networksetup", Args: []string{"-setproxybypassdomains", service, "Empty"}},
	}
}

type linuxBackend int

const (
	backendGNOME linuxBackend = iota
	backendMATE
	backendKDE
)

var kdeConfigWriters = []string{"kwriteconfig6", "kwriteconfig5"}

func preferredLinuxBackend(desktop string) (linuxBackend, bool) {
	var hasGNOME bool
	for _, part := range strings.Split(desktop, ":") {
		d := strings.ToUpper(strings.TrimSpace(part))
		switch d {
		case "KDE":
			return backendKDE, true
		case "MATE":
			return backendMATE, true
		case "GNOME", "CINNAMON", "BUDGIE", "UNITY":
			hasGNOME = true
		}
	}
	if hasGNOME {
		return backendGNOME, true
	}
	return backendGNOME, false
}

func resolveLinuxBackend(desktop string, available func(string) bool) (linuxBackend, string, bool) {
	toolFor := func(b linuxBackend) (string, bool) {
		switch b {
		case backendKDE:
			for _, w := range kdeConfigWriters {
				if available(w) {
					return w, true
				}
			}
			return "", false
		default:
			if available("gsettings") {
				return "gsettings", true
			}
			return "", false
		}
	}

	if b, ok := preferredLinuxBackend(desktop); ok {
		tool, found := toolFor(b)
		return b, tool, found
	}
	for _, b := range []linuxBackend{backendGNOME, backendKDE} {
		if tool, found := toolFor(b); found {
			return b, tool, true
		}
	}
	return backendGNOME, "", false
}

func linuxGSettingsSchema(b linuxBackend) string {
	if b == backendMATE {
		return "org.mate.system.proxy"
	}
	return "org.gnome.system.proxy"
}

func formatGSettingsStringList(items []string) string {
	escaped := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.ReplaceAll(item, `\`, `\\`)
		item = strings.ReplaceAll(item, `'`, `\'`)
		escaped = append(escaped, "'"+item+"'")
	}
	return "[" + strings.Join(escaped, ", ") + "]"
}

func kdeWriteCommand(tool, home, key, value string) sysCommand {
	file := path.Join(home, ".config", "kioslaverc")
	return sysCommand{
		Program: tool,
		Args:    []string{"--file", file, "--group", "Proxy Settings", "--key", key, value},
	}
}

func kdeReparseCommand() sysCommand {
	return sysCommand{
		Program: "dbus-send",
		Args: []string{
			"--session",
			"--type=signal",
			"/KIO/Scheduler",
			"org.kde.KIO.Scheduler.reparseSlaveConfiguration",
			"string:",
		},
		Optional: true,
	}
}

func linuxGSettingsManualCommands(schema, host string, port int, bypass string) []sysCommand {
	p := strconv.Itoa(port)
	ignores := formatGSettingsStringList(WithoutWinINetLocal(ParseBypassList(bypass)))
	cmds := []sysCommand{
		{Program: "gsettings", Args: []string{"set", schema, "ignore-hosts", ignores}},
	}
	for _, proto := range []string{"http", "https", "socks"} {
		section := schema + "." + proto
		cmds = append(cmds,
			sysCommand{Program: "gsettings", Args: []string{"set", section, "host", host}},
			sysCommand{Program: "gsettings", Args: []string{"set", section, "port", p}},
		)
	}
	cmds = append(cmds, sysCommand{Program: "gsettings", Args: []string{"set", schema, "mode", "manual"}})
	return cmds
}

func linuxManualCommands(b linuxBackend, tool, home, host string, port int, bypass string) []sysCommand {
	if b != backendKDE {
		return linuxGSettingsManualCommands(linuxGSettingsSchema(b), host, port, bypass)
	}
	p := strconv.Itoa(port)
	noProxy := strings.Join(WithoutWinINetLocal(ParseBypassList(bypass)), ",")
	cmds := []sysCommand{
		kdeWriteCommand(tool, home, "NoProxyFor", noProxy),
		kdeWriteCommand(tool, home, "httpProxy", "http://"+host+":"+p),
		kdeWriteCommand(tool, home, "httpsProxy", "http://"+host+":"+p),
		kdeWriteCommand(tool, home, "socksProxy", "socks://"+host+":"+p),
		kdeWriteCommand(tool, home, "ProxyType", "1"),
		kdeReparseCommand(),
	}
	for _, gCmd := range linuxGSettingsManualCommands("org.gnome.system.proxy", host, port, bypass) {
		gCmd.Optional = true
		cmds = append(cmds, gCmd)
	}
	return cmds
}

func linuxStopCommands(b linuxBackend, tool, home string) []sysCommand {
	if b != backendKDE {
		schema := linuxGSettingsSchema(b)
		return []sysCommand{
			{Program: "gsettings", Args: []string{"set", schema, "mode", "none"}},
		}
	}
	return []sysCommand{
		kdeWriteCommand(tool, home, "ProxyType", "0"),
		kdeReparseCommand(),
		{Program: "gsettings", Args: []string{"set", "org.gnome.system.proxy", "mode", "none"}, Optional: true},
	}
}

func parseKioslavercStatus(content string) Status {
	inProxySection := false
	var proxyType, httpProxy, autoURL, noProxy string
	for _, rawLine := range strings.Split(content, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			inProxySection = strings.EqualFold(strings.TrimSpace(line[1:len(line)-1]), "Proxy Settings")
			continue
		}
		if !inProxySection {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		switch k {
		case "ProxyType":
			proxyType = v
		case "httpProxy":
			httpProxy = v
		case "Proxy Config Script":
			autoURL = v
		case "NoProxyFor":
			noProxy = v
		}
	}
	switch proxyType {
	case "2":
		return Status{Enabled: false, AutoConfigURL: autoURL}
	case "1":
		srv := httpProxy
		for _, pfx := range []string{"http://", "https://", "socks://", "socks5://"} {
			srv = strings.TrimPrefix(srv, pfx)
		}
		return Status{
			Enabled: srv != "",
			Server:  srv,
			Bypass:  noProxy,
		}
	default:
		return Status{Enabled: false}
	}
}

