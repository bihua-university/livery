package model

import (
	"time"

	"github.com/bihua-university/livery/internal/sysproxy"
)

// ThemeMode selects the active appearance mode.
type ThemeMode string

const (
	ThemeSystem ThemeMode = "system"
	ThemeLight  ThemeMode = "light"
	ThemeDark   ThemeMode = "dark"
)

// LanguageMode selects the UI language.
type LanguageMode string

const (
	LangZH LanguageMode = "zh"
	LangEN LanguageMode = "en"
)

// CloseAction controls what happens when the main window is closed.
type CloseAction string

const (
	CloseToTray CloseAction = "tray"
	CloseQuit   CloseAction = "quit"
)

// CoreType selects the proxy core engine backend (mihomo or sing-box).
type CoreType string

const (
	CoreMihomo  CoreType = "mihomo"
	CoreSingBox CoreType = "sing-box"
)

// NormalizeCoreType normalizes persisted or user-supplied core identifiers, defaulting to CoreMihomo.
func NormalizeCoreType(ct CoreType) CoreType {
	switch ct {
	case CoreSingBox, "singbox":
		return CoreSingBox
	default:
		return CoreMihomo
	}
}

// CoreDisplayName returns the human-readable name for a CoreType.
func CoreDisplayName(ct CoreType) string {
	if NormalizeCoreType(ct) == CoreSingBox {
		return "sing-box"
	}
	return "Mihomo"
}

// ProxyMode mirrors sing-box / Clash routing modes.
type ProxyMode string

const (
	ModeRule   ProxyMode = "rule"
	ModeGlobal ProxyMode = "global"
	ModeDirect ProxyMode = "direct"
)

// CustomRule defines a user-managed routing rule injected into the compiled sing-box config.
type CustomRule struct {
	ID             string `json:"id"`
	Enabled        bool   `json:"enabled"`
	MatchType      string `json:"match_type"` // domain_suffix, domain_keyword, domain, ip_cidr, process_name
	Pattern        string `json:"pattern"`
	Outbound       string `json:"outbound"` // proxy, direct, block, or custom tag
	Prepend        bool   `json:"prepend"`
	FromConnection bool   `json:"from_connection,omitempty"`
}

// ProfileOverride holds Clash-Verge-style per-profile override options.
type ProfileOverride struct {
	DNSMode          string `json:"dns_mode,omitempty"` // Empty inherits the profile's Mihomo DNS configuration.
	OverrideInbounds bool   `json:"override_inbounds"`
	InjectClashAPI   bool   `json:"inject_clash_api"`
	OverrideLog      bool   `json:"override_log"`
	PatchJSON        string `json:"patch_json,omitempty"`
}

// SubscriptionInfo stores Subscription-Userinfo quota metadata from remote subscriptions.
type SubscriptionInfo struct {
	Upload   int64 `json:"upload"`
	Download int64 `json:"download"`
	Total    int64 `json:"total"`
	Expire   int64 `json:"expire"` // unix seconds, 0 if unknown
}

// Profile represents a local or remote sing-box configuration profile.
type Profile struct {
	ID              string           `json:"id"`
	Name            string           `json:"name"`
	Kind            string           `json:"kind"` // "remote" or "local"
	URL             string           `json:"url,omitempty"`
	UserAgent       string           `json:"user_agent,omitempty"`
	AutoUpdate      bool             `json:"auto_update"`
	UpdateIntervalH int              `json:"update_interval_h"`
	UseProxyUpdate  bool             `json:"use_proxy_update"`
	FileName        string           `json:"file_name"`
	Format          string           `json:"format,omitempty"` // "mihomo" (yaml) or "sing-box" (json)
	UpdatedAt       time.Time        `json:"updated_at"`
	Usage           SubscriptionInfo `json:"usage"`
	Override        ProfileOverride  `json:"override"`
	LastError       string           `json:"last_error,omitempty"`
}

// AppSettings stores persistent client settings.
type AppSettings struct {
	// Proxy core & inbound settings
	CoreType           CoreType `json:"core_type"`
	CorePath           string   `json:"core_path"`   // sing-box binary path
	MihomoPath         string   `json:"mihomo_path"` // mihomo binary path
	AutoStartCore      bool     `json:"auto_start_core"`
	ClashAPIHost       string   `json:"clash_api_host"`
	ClashAPIPort       int      `json:"clash_api_port"`
	ClashAPISecret     string   `json:"clash_api_secret"`
	MixedPort          int      `json:"mixed_port"`
	AllowLAN           bool     `json:"allow_lan"`
	LogLevel           string   `json:"log_level"`
	DelayTestURL       string   `json:"delay_test_url"`
	DelayTestTimeoutMs int      `json:"delay_test_timeout_ms"`
	HideTimedOutNodes  bool     `json:"hide_timed_out_nodes,omitempty"`
	TCPConcurrent      *bool    `json:"tcp_concurrent,omitempty"`
	IPv6               *bool    `json:"ipv6,omitempty"`
	GeoDataLoader      string   `json:"geodata_loader,omitempty"`

	// Routing & system proxy
	CloseConnectionsOnSwitch bool `json:"close_connections_on_switch"`
	FlushDNSOnNetworkChange bool `json:"flush_dns_on_network_change"`
	ProxyMode          ProxyMode `json:"proxy_mode"`
	SystemProxyEnabled bool      `json:"system_proxy_enabled"`
	SystemProxyBypass  string    `json:"system_proxy_bypass"`
	ProxyGuard         bool      `json:"proxy_guard"`
	ProxyGuardInterval int       `json:"proxy_guard_interval_sec"`

	// TUN mode
	TUNEnabled            bool   `json:"tun_enabled"`
	TUNStack              string `json:"tun_stack"` // mixed, system, gvisor
	TUNMTU                int    `json:"tun_mtu"`
	TUNAutoRoute          bool   `json:"tun_auto_route"`
	TUNStrictRoute        bool   `json:"tun_strict_route"`
	TUNBypassPrivateRoute bool   `json:"tun_bypass_private_route"`
	TUNInterfaceName      string `json:"tun_interface_name"`

	// Appearance & desktop behavior
	ThemeMode       ThemeMode    `json:"theme_mode"`
	UIFontSize      float32      `json:"ui_font_size"`
	Language        LanguageMode `json:"language"`
	LaunchAtLogin   bool         `json:"launch_at_login"`
	SilentStart     bool         `json:"silent_start"`
	CloseAction     CloseAction  `json:"close_action"`
	ActiveProfileID string       `json:"active_profile_id"`

	// GeoIP / GeoSite database settings
	GeoDataMode        bool   `json:"geodata_mode"`
	GeoDataDir         string `json:"geo_data_dir"`
	GeoAutoUpdate      bool   `json:"geo_auto_update"`
	GeoUpdateIntervalH int    `json:"geo_update_interval_h"`
	GeoIPURL           string `json:"geoip_url"`
	GeoSiteURL         string `json:"geosite_url"`
	MMDBURL            string `json:"mmdb_url"`
	ASNURL             string `json:"asn_url"`

	// Custom user routing rules
	CustomRules []CustomRule `json:"custom_rules"`

	// App OTA updates
	UpdatePrereleases bool `json:"update_prereleases"`
}

// CorePathFor returns the configured custom binary path for the specified core type.
func (s AppSettings) CorePathFor(ct CoreType) string {
	if NormalizeCoreType(ct) == CoreSingBox {
		return s.CorePath
	}
	return s.MihomoPath
}

// ActiveCorePath returns the configured custom binary path for the active core type.
func (s AppSettings) ActiveCorePath() string {
	return s.CorePathFor(s.CoreType)
}

// SetCorePathFor updates the custom binary path for the specified core type.
func (s *AppSettings) SetCorePathFor(ct CoreType, path string) {
	if NormalizeCoreType(ct) == CoreSingBox {
		s.CorePath = path
		return
	}
	s.MihomoPath = path
}

const (
	DefaultGeoIPURL   = "https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/geoip.dat"
	DefaultGeoSiteURL = "https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/geosite.dat"
	DefaultMMDBURL    = "https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/geoip.metadb"
	DefaultASNURL     = "https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/GeoLite2-ASN.mmdb"
)

// DefaultSettings returns sane Clash-Verge-like defaults with mihomo as the default core.
func DefaultSettings() AppSettings {
	return AppSettings{
		FlushDNSOnNetworkChange: true,
		CoreType:           CoreMihomo,
		ClashAPIHost:       "127.0.0.1",
		ClashAPIPort:       9090,
		MixedPort:          7890,
		AllowLAN:           false,
		LogLevel:           "info",
		DelayTestURL:       "http://www.gstatic.com/generate_204",
		DelayTestTimeoutMs: 5000,
		ProxyMode:          ModeRule,
		SystemProxyBypass:  sysproxy.DefaultBypass(),
		ProxyGuard:         false,
		ProxyGuardInterval: 10,
		TUNEnabled:            false,
		TUNStack:              "mixed",
		TUNMTU:                9000,
		TUNAutoRoute:          true,
		TUNStrictRoute:        true,
		TUNBypassPrivateRoute: true,
		TUNInterfaceName:      "sing-tun",
		GeoDataMode:        false,
		GeoAutoUpdate:      false,
		GeoUpdateIntervalH: 24,
		GeoIPURL:           DefaultGeoIPURL,
		GeoSiteURL:         DefaultGeoSiteURL,
		MMDBURL:            DefaultMMDBURL,
		ASNURL:             DefaultASNURL,
		ThemeMode:          ThemeSystem,
		UIFontSize:         14,
		Language:           LangZH,
		CloseAction:        CloseToTray,
		AutoStartCore:      true,
	}
}

// GeoFileStatus describes a locally cached GeoIP / GeoSite / MMDB / ASN database file.
type GeoFileStatus struct {
	Name      string    `json:"name"`
	Path      string    `json:"path"`
	Exists    bool      `json:"exists"`
	Size      int64     `json:"size"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CoreStatus describes the lifecycle state of the sing-box core process.
type CoreStatus string

const (
	CoreStopped  CoreStatus = "stopped"
	CoreStarting CoreStatus = "starting"
	CoreRunning  CoreStatus = "running"
	CoreError    CoreStatus = "error"
)

// CoreInfo holds runtime status & binary discovery info for the active core.
type CoreInfo struct {
	CoreType    CoreType   `json:"core_type"`
	Status      CoreStatus `json:"status"`
	Version     string     `json:"version"`
	BinaryPath  string     `json:"binary_path"`
	Found       bool       `json:"found"`
	PID         int        `json:"pid"`
	External    bool       `json:"external"`
	StartedAt   time.Time  `json:"started_at"`
	LastError   string     `json:"last_error"`
	IsElevated  bool       `json:"is_elevated"`
	RuntimePath string     `json:"runtime_path"`
}

// ProxyNode represents an individual outbound proxy or group member.
type ProxyNode struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Delay   int    `json:"delay"` // -1 = untested, 0 = timeout/error, >0 = ms
	Testing bool   `json:"testing"`
	UDP     bool   `json:"udp"`
}

// ProxyGroup represents a selectable or automatic outbound group.
type ProxyGroup struct {
	Name    string   `json:"name"`
	Type    string   `json:"type"` // Selector, URLTest, Fallback, LoadBalance
	Now     string   `json:"now"`
	All     []string `json:"all"`
	Testing bool     `json:"testing"`
}

// TrafficSample represents one second of upload/download telemetry.
type TrafficSample struct {
	Timestamp time.Time `json:"ts"`
	Up        int64     `json:"up"`   // bytes/sec
	Down      int64     `json:"down"` // bytes/sec
}

// ConnectionMetadata holds Clash API connection metadata.
type ConnectionMetadata struct {
	Network         string `json:"network"`
	Type            string `json:"type"`
	SourceIP        string `json:"sourceIP"`
	DestinationIP   string `json:"destinationIP"`
	SourcePort      string `json:"sourcePort"`
	DestinationPort string `json:"destinationPort"`
	Host            string `json:"host"`
	DNSMode         string `json:"dnsMode"`
	ProcessPath     string `json:"processPath"`
	RemoteDest      string `json:"remoteDestination"`
}

// Connection represents an active or closed connection from /connections.
type Connection struct {
	ID          string             `json:"id"`
	Metadata    ConnectionMetadata `json:"metadata"`
	Upload      int64              `json:"upload"`
	Download    int64              `json:"download"`
	Start       time.Time          `json:"start"`
	Chains      []string           `json:"chains"`
	Rule        string             `json:"rule"`
	RulePayload string             `json:"rulePayload"`
}

// ConnectionsSnapshot is returned by Clash API /connections.
type ConnectionsSnapshot struct {
	DownloadTotal int64        `json:"downloadTotal"`
	UploadTotal   int64        `json:"uploadTotal"`
	Connections   []Connection `json:"connections"`
	Memory        int64        `json:"memory"`
}

// RuleEntry represents a parsed rule from Clash API /rules or compiled config.
type RuleEntry struct {
	Type    string `json:"type"`
	Payload string `json:"payload"`
	Proxy   string `json:"proxy"`
	Size    int    `json:"size,omitempty"`
}

// LogEntry represents a single sing-box or app log line.
type LogEntry struct {
	Time    time.Time `json:"time"`
	Level   string    `json:"level"` // info, warning, error, debug, trace
	Source  string    `json:"source"`
	Message string    `json:"message"`
}
