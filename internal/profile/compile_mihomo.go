package profile

import (
	"bytes"
	"fmt"
	"runtime"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/bihua-university/livery/internal/model"
)

// CompileForCore compiles a profile (Mihomo/Clash YAML, sing-box JSON, or URI list)
// into a ready-to-run configuration for settings.CoreType without mutating the source profile.
func CompileForCore(raw []byte, prof model.Profile, settings model.AppSettings) ([]byte, error) {
	if model.NormalizeCoreType(settings.CoreType) == model.CoreSingBox {
		trimmed := bytes.TrimSpace(raw)
		if len(trimmed) == 0 || trimmed[0] != '{' {
			converted, _, err := ConvertToSingBox(raw)
			if err != nil {
				return nil, err
			}
			raw = converted
		}
		return Compile(raw, prof, settings)
	}
	return CompileMihomo(raw, prof, settings)
}

// CompileMihomo merges a raw Clash/Mihomo YAML (or sing-box JSON / URI) profile with
// AppSettings, ProfileOverride, and CustomRules into a ready-to-run Mihomo YAML config.
func CompileMihomo(raw []byte, prof model.Profile, settings model.AppSettings) ([]byte, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("empty profile")
	}

	var cfg map[string]any
	if trimmed[0] != '{' && trimmed[0] != '[' {
		if err := yaml.Unmarshal(trimmed, &cfg); err != nil || !isClashYAMLMap(cfg) {
			cfg = nil
		}
	}
	if cfg == nil {
		converted, _, err := ConvertToMihomo(trimmed)
		if err != nil {
			return nil, err
		}
		if err := yaml.Unmarshal(converted, &cfg); err != nil {
			return nil, fmt.Errorf("invalid profile YAML: %w", err)
		}
		if cfg == nil {
			cfg = map[string]any{}
		}
	}

	// 1. Optional profile-level JSON/YAML merge patch.
	if strings.TrimSpace(prof.Override.PatchJSON) != "" {
		var patch map[string]any
		if err := yaml.Unmarshal([]byte(prof.Override.PatchJSON), &patch); err != nil {
			return nil, fmt.Errorf("invalid override patch: %w", err)
		}
		if patch != nil {
			cfg = deepMerge(cfg, patch)
		}
	}

	// Preserve profile values until a core option is explicitly set.
	if settings.TCPConcurrent != nil {
		cfg["tcp-concurrent"] = *settings.TCPConcurrent
	}
	if settings.IPv6 != nil {
		cfg["ipv6"] = *settings.IPv6
	}
	if settings.GeoDataLoader == "standard" || settings.GeoDataLoader == "memconservative" {
		cfg["geodata-loader"] = settings.GeoDataLoader
	}

	// 2. Log level override.
	if prof.Override.OverrideLog || cfg["log-level"] == nil {
		if lvl := mapMihomoLogLevel(settings.LogLevel); lvl != "" {
			cfg["log-level"] = lvl
		}
	}

	// 3. Clash API controller injection (required for telemetry, proxies, connections, rules).
	host := settings.ClashAPIHost
	if host == "" {
		host = model.DefaultClashAPIHost
	}
	port := settings.ClashAPIPort
	if port <= 0 {
		port = model.DefaultClashAPIPort
	}
	cfg["external-controller"] = fmt.Sprintf("%s:%d", host, port)
	if settings.ClashAPISecret != "" {
		cfg["secret"] = settings.ClashAPISecret
	} else {
		delete(cfg, "secret")
	}
	if settings.ProxyMode != "" {
		cfg["mode"] = string(settings.ProxyMode)
	}

	// 4. Mixed inbound & LAN access.
	_, hasPort := cfg["port"]
	_, hasSocks := cfg["socks-port"]
	_, hasMixed := cfg["mixed-port"]
	if prof.Override.OverrideInbounds || (!hasPort && !hasSocks && !hasMixed) {
		mPort := settings.MixedPort
		if mPort <= 0 {
			mPort = model.DefaultMixedPort
		}
		cfg["mixed-port"] = mPort
		cfg["allow-lan"] = settings.AllowLAN
		if prof.Override.OverrideInbounds {
			delete(cfg, "port")
			delete(cfg, "socks-port")
		}
	}

	// 5. TUN mode.
	if settings.TUNEnabled {
		stack := settings.TUNStack
		if stack == "" {
			stack = model.DefaultTUNStack
		}
		mtu := settings.TUNMTU
		if mtu <= 0 {
			mtu = model.DefaultTUNMTU
		}
		iface := settings.TUNInterfaceName
		if iface == "" {
			iface = model.DefaultTUNInterfaceName
		}
		tunObj, _ := cfg["tun"].(map[string]any)
		if tunObj == nil {
			tunObj = map[string]any{}
		}
		tunObj["enable"] = true
		tunObj["stack"] = stack
		tunObj["mtu"] = mtu
		tunObj["auto-route"] = settings.TUNAutoRoute
		tunObj["strict-route"] = settings.TUNStrictRoute
		tunObj["auto-detect-interface"] = true
		tunObj["device"] = iface
		if len(strSlice(tunObj["dns-hijack"])) == 0 {
			tunObj["dns-hijack"] = []string{"any:53"}
		}
		if runtime.GOOS == "darwin" && strings.TrimSpace(strVal(tunObj["dns-mode"])) == "" {
			tunObj["dns-mode"] = "native"
		}
		excludes := strSlice(tunObj["route-exclude-address"])
		if settings.TUNBypassPrivateRoute {
			excludes = appendUniqueStrings(excludes, privateRouteAddresses...)
		}
		excludes = appendUniqueStrings(excludes, tailscaleRouteExcludeAddresses...)
		tunObj["route-exclude-address"] = excludes
		cfg["tun"] = tunObj
	} else {
		if tunObj, ok := cfg["tun"].(map[string]any); ok && tunObj != nil {
			tunObj["enable"] = false
			cfg["tun"] = tunObj
		}
	}

	mode := prof.Override.DNSMode
	overrideDNS := mode == "fake-ip" || mode == "redir-host"
	dns, _ := cfg["dns"].(map[string]any)
	if settings.TUNEnabled || overrideDNS {
		if dns == nil {
			dns = defaultMihomoDNSMap()
			cfg["dns"] = dns
		}
		dns["enable"] = true
	}
	if overrideDNS {
		dns["enhanced-mode"] = mode
		if len(strSlice(dns["nameserver"])) == 0 {
			dns["nameserver"] = defaultMihomoDNSMap()["nameserver"]
		}
	}
	if boolVal(dns["enable"]) {
		ensureMihomoTailscaleDNS(dns)
	}

	// 5b. GeoIP / GeoSite database settings.
	if settings.GeoDataMode {
		cfg["geodata-mode"] = true
	}
	if settings.GeoAutoUpdate {
		cfg["geo-auto-update"] = true
		if settings.GeoUpdateIntervalH > 0 {
			cfg["geo-update-interval"] = settings.GeoUpdateIntervalH
		}
	}
	geox, _ := cfg["geox-url"].(map[string]any)
	if geox == nil {
		geox = map[string]any{}
	}
	if u := strings.TrimSpace(settings.GeoIPURL); u != "" {
		geox["geoip"] = u
	}
	if u := strings.TrimSpace(settings.GeoSiteURL); u != "" {
		geox["geosite"] = u
	}
	if u := strings.TrimSpace(settings.MMDBURL); u != "" {
		geox["mmdb"] = u
	}
	if u := strings.TrimSpace(settings.ASNURL); u != "" {
		geox["asn"] = u
	}
	if len(geox) > 0 {
		cfg["geox-url"] = geox
	}

	// 6. Custom routing rules & Tailscale direct bypass rules injection.
	existing := strSlice(cfg["rules"])
	var prepended, appended []string
	for _, r := range settings.CustomRules {
		if !r.Enabled || strings.TrimSpace(r.Pattern) == "" || strings.TrimSpace(r.Outbound) == "" {
			continue
		}
		if r.FromConnection && r.Outbound != "direct" && r.Outbound != "block" && r.Outbound != "GLOBAL" && !hasNamedEntry(cfg["proxy-groups"], "name", r.Outbound) {
			continue // This profile does not provide the selected group.
		}
		lines := customRuleToClashLines(r, cfg)
		if r.Prepend {
			prepended = append(prepended, lines...)
		} else {
			appended = append(appended, lines...)
		}
	}
	tailscaleDirect := missingTailscaleClashRules(existing)
	var beforeMatch, afterOrMatch []string
	for _, line := range existing {
		upper := strings.ToUpper(strings.TrimSpace(line))
		if strings.HasPrefix(upper, "MATCH,") || strings.HasPrefix(upper, "FINAL,") || len(afterOrMatch) > 0 {
			afterOrMatch = append(afterOrMatch, line)
		} else {
			beforeMatch = append(beforeMatch, line)
		}
	}
	merged := make([]string, 0, len(prepended)+len(tailscaleDirect)+len(existing)+len(appended))
	merged = append(merged, prepended...)
	merged = append(merged, tailscaleDirect...)
	merged = append(merged, beforeMatch...)
	merged = append(merged, appended...)
	merged = append(merged, afterOrMatch...)
	if len(merged) > 0 || cfg["rules"] != nil {
		cfg["rules"] = merged
	}

	out, err := yaml.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	return out, nil
}

var privateRouteAddresses = []string{
	"0.0.0.0/8",
	"10.0.0.0/8",
	"127.0.0.0/8",
	"169.254.0.0/16",
	"172.16.0.0/12",
	"192.168.0.0/16",
	"224.0.0.0/4",
	"255.255.255.255/32",
	"fc00::/7",
	"fe80::/10",
	"ff00::/8",
}

var tailscaleRouteExcludeAddresses = []string{
	"100.64.0.0/10",
	"fd7a:115c:a1e0::/48",
}

var microsoftNCSIFakeIPFilters = []string{
	"dns.msftnsci.com",
	"www.msftnsci.com",
	"www.msftconnecttest.com",
}

func defaultMihomoDNSMap() map[string]any {
	return map[string]any{
		"enable":                  true,
		"enhanced-mode":           "fake-ip",
		"fake-ip-range":           "198.18.0.1/16",
		"fake-ip-filter":          append([]string{"+.lan", "+.local", "+.ts.net"}, microsoftNCSIFakeIPFilters...),
		"proxy-server-nameserver": []string{"223.5.5.5", "119.29.29.29"},
		"nameserver":              []string{"https://dns.alidns.com/dns-query", "https://doh.pub/dns-query", "223.5.5.5"},
		"fallback":                []string{"https://dns.cloudflare.com/dns-query", "https://dns.google/dns-query"},
		"fallback-filter": map[string]any{
			"geoip":      true,
			"geoip-code": "CN",
			"ipcidr":     []string{"240.0.0.0/4"},
		},
		"nameserver-policy": map[string]any{
			"+.ts.net": "system",
		},
	}
}

func ensureMihomoTailscaleDNS(dnsObj map[string]any) {
	if dnsObj == nil {
		return
	}
	// These bypass domains are blacklist entries. A whitelist reverses their
	// meaning, and a rule-mode filter requires full routing rules.
	switch strings.ToLower(strVal(dnsObj["fake-ip-filter-mode"])) {
	case "", "blacklist":
		filters := strSlice(dnsObj["fake-ip-filter"])
		dnsObj["fake-ip-filter"] = appendUniqueStrings(filters, append([]string{"+.ts.net"}, microsoftNCSIFakeIPFilters...)...)
	}

	policy, _ := dnsObj["nameserver-policy"].(map[string]any)
	if policy == nil {
		policy = map[string]any{}
	}
	if _, has := policy["+.ts.net"]; !has {
		policy["+.ts.net"] = "system"
	}
	dnsObj["nameserver-policy"] = policy
}

func missingTailscaleClashRules(existing []string) []string {
	hasRule := func(substr string) bool {
		upperSub := strings.ToUpper(substr)
		for _, line := range existing {
			compact := strings.ReplaceAll(strings.ToUpper(line), " ", "")
			if strings.Contains(compact, upperSub) {
				return true
			}
		}
		return false
	}
	var out []string
	if !hasRule(",TS.NET,") {
		out = append(out, "DOMAIN-SUFFIX,ts.net,DIRECT")
	}
	if !hasRule(",100.64.0.0/10,") {
		out = append(out, "IP-CIDR,100.64.0.0/10,DIRECT,no-resolve")
	}
	if !hasRule(",FD7A:115C:A1E0::/48,") {
		out = append(out, "IP-CIDR6,fd7a:115c:a1e0::/48,DIRECT,no-resolve")
	}
	return out
}

func mapMihomoLogLevel(lvl string) string {
	switch strings.ToLower(strings.TrimSpace(lvl)) {
	case "warn", "warning":
		return "warning"
	case "trace", "debug":
		return "debug"
	case "info", "error", "silent":
		return strings.ToLower(strings.TrimSpace(lvl))
	default:
		return "info"
	}
}

func customRuleToClashLines(r model.CustomRule, cfg map[string]any) []string {
	var clashType, extra string
	switch r.MatchType {
	case "domain":
		clashType = "DOMAIN"
	case "domain_suffix":
		clashType = "DOMAIN-SUFFIX"
	case "domain_keyword":
		clashType = "DOMAIN-KEYWORD"
	case "domain_regex":
		clashType = "DOMAIN-REGEX"
	case "ip_cidr":
		clashType = "IP-CIDR"
		extra = ",no-resolve"
	case "process_name":
		clashType = "PROCESS-NAME"
	default:
		return nil
	}
	target := resolveClashOutbound(r.Outbound, cfg)
	parts := strings.Split(r.Pattern, ",")
	var lines []string
	for _, pat := range parts {
		pat = strings.TrimSpace(pat)
		if pat == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("%s,%s,%s%s", clashType, pat, target, extra))
	}
	return lines
}

func resolveClashOutbound(outbound string, cfg map[string]any) string {
	trimmed := strings.TrimSpace(outbound)
	switch strings.ToLower(trimmed) {
	case "direct":
		return "DIRECT"
	case "reject", "block":
		return "REJECT"
	}
	if groups, ok := cfg["proxy-groups"].([]any); ok {
		for _, raw := range groups {
			if gm, ok := raw.(map[string]any); ok {
				if name, _ := gm["name"].(string); strings.EqualFold(name, trimmed) {
					return name
				}
			}
		}
	}
	return trimmed
}
