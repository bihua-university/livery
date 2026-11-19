package profile

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"runtime"
	"strconv"
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

	// 2. Log level override.
	if prof.Override.OverrideLog || cfg["log-level"] == nil {
		if lvl := mapMihomoLogLevel(settings.LogLevel); lvl != "" {
			cfg["log-level"] = lvl
		}
	}

	// 3. Clash API controller injection (required for telemetry, proxies, connections, rules).
	host := settings.ClashAPIHost
	if host == "" {
		host = "127.0.0.1"
	}
	port := settings.ClashAPIPort
	if port <= 0 {
		port = 9090
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
			mPort = 7890
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
			stack = "mixed"
		}
		mtu := settings.TUNMTU
		if mtu <= 0 {
			mtu = 9000
		}
		iface := settings.TUNInterfaceName
		if iface == "" {
			iface = "sing-tun"
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

		dnsObj, _ := cfg["dns"].(map[string]any)
		if dnsObj == nil {
			dnsObj = defaultMihomoDNSMap()
		} else {
			dnsObj["enable"] = true
		}
		ensureMihomoTailscaleDNS(dnsObj)
		cfg["dns"] = dnsObj
	} else {
		if tunObj, ok := cfg["tun"].(map[string]any); ok && tunObj != nil {
			tunObj["enable"] = false
			cfg["tun"] = tunObj
		}
		if dnsObj, ok := cfg["dns"].(map[string]any); ok && dnsObj != nil && boolVal(dnsObj["enable"]) {
			ensureMihomoTailscaleDNS(dnsObj)
			cfg["dns"] = dnsObj
		}
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
	filters := strSlice(dnsObj["fake-ip-filter"])
	filters = appendUniqueStrings(filters, append([]string{"+.ts.net"}, microsoftNCSIFakeIPFilters...)...)
	dnsObj["fake-ip-filter"] = filters

	policy, _ := dnsObj["nameserver-policy"].(map[string]any)
	if policy == nil {
		policy = map[string]any{}
	}
	if _, has := policy["+.ts.net"]; !has {
		policy["+.ts.net"] = "system"
	}
	dnsObj["nameserver-policy"] = policy
}

func appendUniqueStrings(base []string, extras ...string) []string {
	seen := make(map[string]struct{}, len(base)+len(extras))
	out := make([]string, 0, len(base)+len(extras))
	for _, s := range base {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		k := strings.ToLower(s)
		if _, ok := seen[k]; !ok {
			seen[k] = struct{}{}
			out = append(out, s)
		}
	}
	for _, s := range extras {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		k := strings.ToLower(s)
		if _, ok := seen[k]; !ok {
			seen[k] = struct{}{}
			out = append(out, s)
		}
	}
	return out
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

// Compile merges a raw sing-box JSON profile with AppSettings, ProfileOverride, and CustomRules
// into a ready-to-run sing-box configuration JSON (without mutating the original profile).
func Compile(rawJSON []byte, prof model.Profile, settings model.AppSettings) ([]byte, error) {
	var cfg map[string]any
	if err := json.Unmarshal(rawJSON, &cfg); err != nil {
		return nil, fmt.Errorf("invalid profile JSON: %w", err)
	}
	if cfg == nil {
		cfg = map[string]any{}
	}

	// 1. Optional profile-level JSON merge patch.
	if strings.TrimSpace(prof.Override.PatchJSON) != "" {
		var patch map[string]any
		if err := json.Unmarshal([]byte(prof.Override.PatchJSON), &patch); err != nil {
			return nil, fmt.Errorf("invalid override patch JSON: %w", err)
		}
		cfg = deepMerge(cfg, patch)
	}

	// 1b. Auto-migrate legacy pre-1.12 DNS format for sing-box 1.12+/1.14+ compatibility.
	migrateLegacyDNS(cfg)

	// 2. Log level override.
	if prof.Override.OverrideLog || cfg["log"] == nil {
		logObj, _ := cfg["log"].(map[string]any)
		if logObj == nil {
			logObj = map[string]any{}
		}
		if settings.LogLevel != "" {
			logObj["level"] = settings.LogLevel
		}
		logObj["timestamp"] = true
		cfg["log"] = logObj
	}

	// 3. Experimental Clash API injection (required for telemetry, proxies, connections, rules).
	if prof.Override.InjectClashAPI || true {
		exp, _ := cfg["experimental"].(map[string]any)
		if exp == nil {
			exp = map[string]any{}
		}
		clashAPI, _ := exp["clash_api"].(map[string]any)
		if clashAPI == nil {
			clashAPI = map[string]any{}
		}
		host := settings.ClashAPIHost
		if host == "" {
			host = "127.0.0.1"
		}
		port := settings.ClashAPIPort
		if port <= 0 {
			port = 9090
		}
		clashAPI["external_controller"] = fmt.Sprintf("%s:%d", host, port)
		if settings.ClashAPISecret != "" {
			clashAPI["secret"] = settings.ClashAPISecret
		} else {
			delete(clashAPI, "secret")
		}
		if settings.ProxyMode != "" {
			clashAPI["default_mode"] = string(settings.ProxyMode)
		}
		exp["clash_api"] = clashAPI
		cfg["experimental"] = exp
	}

	// 4. Inbounds (mixed + optional TUN).
	inbounds := toSlice(cfg["inbounds"])
	if prof.Override.OverrideInbounds || len(inbounds) == 0 {
		listen := "127.0.0.1"
		if settings.AllowLAN {
			listen = "0.0.0.0"
		}
		port := settings.MixedPort
		if port <= 0 {
			port = 7890
		}
		mixed := map[string]any{
			"type":        "mixed",
			"tag":         "mixed-in",
			"listen":      listen,
			"listen_port": port,
		}
		inbounds = upsertInbound(inbounds, "mixed", mixed)
	}
	if settings.TUNEnabled {
		stack := settings.TUNStack
		if stack == "" {
			stack = "mixed"
		}
		mtu := settings.TUNMTU
		if mtu <= 0 {
			mtu = 9000
		}
		iface := settings.TUNInterfaceName
		if iface == "" {
			iface = "sing-tun"
		}
		var excludes []string
		if existingTUN := findInboundByType(inbounds, "tun"); existingTUN != nil {
			excludes = strSlice(existingTUN["route_exclude_address"])
		}
		if settings.TUNBypassPrivateRoute {
			excludes = appendUniqueStrings(excludes, privateRouteAddresses...)
		}
		excludes = appendUniqueStrings(excludes, tailscaleRouteExcludeAddresses...)
		tun := map[string]any{
			"type":                  "tun",
			"tag":                   "tun-in",
			"interface_name":        iface,
			"address":               []string{"172.19.0.1/30"},
			"mtu":                   mtu,
			"auto_route":            settings.TUNAutoRoute,
			"strict_route":          settings.TUNStrictRoute,
			"stack":                 stack,
			"route_exclude_address": excludes,
		}
		inbounds = upsertInbound(inbounds, "tun", tun)

		routeObj, _ := cfg["route"].(map[string]any)
		if routeObj == nil {
			routeObj = map[string]any{}
		}
		routeObj["auto_detect_interface"] = true
		cfg["route"] = routeObj
	} else {
		inbounds = removeInboundType(inbounds, "tun")
	}
	cfg["inbounds"] = inbounds

	// 4b. Tailscale MagicDNS resolution via local/system DNS in sing-box DNS config.
	if dnsObj, ok := cfg["dns"].(map[string]any); ok && dnsObj != nil {
		ensureSingBoxTailscaleDNS(dnsObj, settings.TUNEnabled)
		cfg["dns"] = dnsObj
	}

	// 5. Custom routing rules & Tailscale direct bypass rules injection.
	routeObj, _ := cfg["route"].(map[string]any)
	if routeObj == nil {
		routeObj = map[string]any{}
	}
	existing := toSlice(routeObj["rules"])
	var prepended, appended []any
	for _, r := range settings.CustomRules {
		if !r.Enabled || strings.TrimSpace(r.Pattern) == "" || strings.TrimSpace(r.Outbound) == "" {
			continue
		}
		obj := customRuleToSingBox(r)
		if r.Prepend {
			prepended = append(prepended, obj)
		} else {
			appended = append(appended, obj)
		}
	}
	var tailscaleDirect []any
	if directTag := findSingBoxDirectTag(cfg); directTag != "" {
		tailscaleDirect = missingTailscaleSingBoxRules(existing, directTag)
	}
	if len(prepended) > 0 || len(tailscaleDirect) > 0 || len(appended) > 0 {
		var leadingHijack, restExisting []any
		for i, raw := range existing {
			if len(restExisting) == 0 && isSingBoxHijackDNSRule(raw) {
				leadingHijack = append(leadingHijack, existing[i])
			} else {
				restExisting = append(restExisting, existing[i])
			}
		}
		merged := make([]any, 0, len(prepended)+len(leadingHijack)+len(tailscaleDirect)+len(restExisting)+len(appended))
		merged = append(merged, prepended...)
		merged = append(merged, leadingHijack...)
		merged = append(merged, tailscaleDirect...)
		merged = append(merged, restExisting...)
		merged = append(merged, appended...)
		routeObj["rules"] = merged
		cfg["route"] = routeObj
	}

	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}

func findInboundByType(inbounds []any, typ string) map[string]any {
	for _, raw := range inbounds {
		if m, ok := raw.(map[string]any); ok {
			if strings.EqualFold(strVal(m["type"]), typ) {
				return m
			}
		}
	}
	return nil
}

func ensureSingBoxTailscaleDNS(dnsObj map[string]any, tunEnabled bool) {
	if dnsObj == nil {
		return
	}
	servers := toSlice(dnsObj["servers"])
	usedTags := make(map[string]bool, len(servers))
	localTag := ""
	for _, raw := range servers {
		srv, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		tag := strings.TrimSpace(strVal(srv["tag"]))
		if tag != "" {
			usedTags[strings.ToLower(tag)] = true
		}
		if localTag == "" && strings.EqualFold(strVal(srv["type"]), "local") {
			localTag = tag
			if localTag == "" {
				localTag = "dns-local"
				if usedTags[localTag] {
					localTag = "dns-local-ts"
				}
				srv["tag"] = localTag
				usedTags[strings.ToLower(localTag)] = true
			}
		}
	}
	if localTag == "" {
		if !tunEnabled {
			return
		}
		localTag = "dns-local"
		for i := 2; usedTags[strings.ToLower(localTag)]; i++ {
			localTag = fmt.Sprintf("dns-local-%d", i)
		}
		servers = append([]any{map[string]any{"type": "local", "tag": localTag}}, servers...)
		dnsObj["servers"] = servers
	}

	rules := toSlice(dnsObj["rules"])
	for _, raw := range rules {
		r, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if ruleHasStringValue(r["domain_suffix"], "ts.net") || ruleHasStringValue(r["domain"], "ts.net") {
			return
		}
	}
	tsRule := map[string]any{
		"domain_suffix": []string{"ts.net"},
		"server":        localTag,
	}
	dnsObj["rules"] = append([]any{tsRule}, rules...)
}

func findSingBoxDirectTag(cfg map[string]any) string {
	for _, raw := range toSlice(cfg["outbounds"]) {
		ob, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if strings.EqualFold(strVal(ob["type"]), "direct") {
			if tag := strings.TrimSpace(strVal(ob["tag"])); tag != "" {
				return tag
			}
		}
	}
	return ""
}

func missingTailscaleSingBoxRules(existing []any, directTag string) []any {
	hasDomain := false
	hasV4 := false
	hasV6 := false
	for _, raw := range existing {
		r, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if ruleHasStringValue(r["domain_suffix"], "ts.net") {
			hasDomain = true
		}
		if ruleHasStringValue(r["ip_cidr"], "100.64.0.0/10") {
			hasV4 = true
		}
		if ruleHasStringValue(r["ip_cidr"], "fd7a:115c:a1e0::/48") {
			hasV6 = true
		}
	}
	var out []any
	if !hasDomain {
		out = append(out, map[string]any{
			"domain_suffix": []string{"ts.net"},
			"outbound":      directTag,
		})
	}
	var cidrs []string
	if !hasV4 {
		cidrs = append(cidrs, "100.64.0.0/10")
	}
	if !hasV6 {
		cidrs = append(cidrs, "fd7a:115c:a1e0::/48")
	}
	if len(cidrs) > 0 {
		out = append(out, map[string]any{
			"ip_cidr":  cidrs,
			"outbound": directTag,
		})
	}
	return out
}

func ruleHasStringValue(v any, target string) bool {
	switch t := v.(type) {
	case string:
		return strings.EqualFold(strings.TrimSpace(t), target)
	case []string:
		for _, item := range t {
			if strings.EqualFold(strings.TrimSpace(item), target) {
				return true
			}
		}
	case []any:
		for _, raw := range t {
			if strings.EqualFold(strings.TrimSpace(strVal(raw)), target) {
				return true
			}
		}
	}
	return false
}

func isSingBoxHijackDNSRule(raw any) bool {
	m, ok := raw.(map[string]any)
	if !ok {
		return false
	}
	if strings.EqualFold(strVal(m["action"]), "hijack-dns") {
		return true
	}
	return strings.EqualFold(strVal(m["protocol"]), "dns") && strings.EqualFold(strVal(m["outbound"]), "dns-out")
}

// ParseRulesFromConfig extracts human-readable rule entries from a sing-box JSON or Mihomo YAML config.
func ParseRulesFromConfig(raw []byte) []model.RuleEntry {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil
	}
	if trimmed[0] != '{' {
		return parseClashRulesFromYAML(trimmed)
	}
	var cfg struct {
		Route struct {
			Final string           `json:"final"`
			Rules []map[string]any `json:"rules"`
		} `json:"route"`
	}
	if err := json.Unmarshal(trimmed, &cfg); err != nil {
		return nil
	}
	entries := make([]model.RuleEntry, 0, len(cfg.Route.Rules)+1)
	for _, r := range cfg.Route.Rules {
		outbound, _ := r["outbound"].(string)
		if act, ok := r["action"].(string); ok && outbound == "" {
			outbound = act
		}
		typ, payload := summarizeRule(r)
		if typ == "" {
			continue
		}
		entries = append(entries, model.RuleEntry{
			Type:    typ,
			Payload: payload,
			Proxy:   outbound,
		})
	}
	if cfg.Route.Final != "" {
		entries = append(entries, model.RuleEntry{
			Type:    "final",
			Payload: "*",
			Proxy:   cfg.Route.Final,
		})
	}
	return entries
}

func parseClashRulesFromYAML(raw []byte) []model.RuleEntry {
	var doc struct {
		Rules []string `yaml:"rules"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil
	}
	entries := make([]model.RuleEntry, 0, len(doc.Rules))
	for _, line := range doc.Rules {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, ",")
		if len(parts) < 2 {
			continue
		}
		typ := strings.TrimSpace(parts[0])
		upper := strings.ToUpper(typ)
		if upper == "MATCH" || upper == "FINAL" {
			entries = append(entries, model.RuleEntry{
				Type:    typ,
				Payload: "*",
				Proxy:   strings.TrimSpace(parts[1]),
			})
			continue
		}
		if len(parts) < 3 {
			continue
		}
		entries = append(entries, model.RuleEntry{
			Type:    typ,
			Payload: strings.TrimSpace(parts[1]),
			Proxy:   strings.TrimSpace(parts[2]),
		})
	}
	return entries
}

func summarizeRule(r map[string]any) (string, string) {
	keys := []string{
		"rule_set", "domain", "domain_suffix", "domain_keyword", "domain_regex",
		"geosite", "geoip", "ip_cidr", "source_ip_cidr", "port", "port_range",
		"process_name", "process_path", "protocol", "network", "clash_mode", "ip_is_private",
	}
	for _, k := range keys {
		if v, ok := r[k]; ok {
			return k, formatRuleValue(v)
		}
	}
	if _, ok := r["action"]; ok {
		return "action", formatRuleValue(r["action"])
	}
	return "", ""
}

func formatRuleValue(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case bool:
		if t {
			return "true"
		}
		return "false"
	case float64:
		return fmt.Sprintf("%.0f", t)
	case []any:
		parts := make([]string, 0, len(t))
		for _, item := range t {
			parts = append(parts, fmt.Sprint(item))
		}
		return strings.Join(parts, ", ")
	default:
		b, _ := json.Marshal(t)
		return string(b)
	}
}

func customRuleToSingBox(r model.CustomRule) map[string]any {
	obj := map[string]any{}
	val := strings.TrimSpace(r.Pattern)
	if strings.Contains(val, ",") {
		parts := strings.Split(val, ",")
		clean := make([]string, 0, len(parts))
		for _, p := range parts {
			if p = strings.TrimSpace(p); p != "" {
				clean = append(clean, p)
			}
		}
		obj[r.MatchType] = clean
	} else {
		obj[r.MatchType] = []string{val}
	}
	switch strings.ToLower(r.Outbound) {
	case "reject", "block":
		obj["action"] = "reject"
	default:
		obj["outbound"] = r.Outbound
	}
	return obj
}

func upsertInbound(inbounds []any, typ string, item map[string]any) []any {
	for i, raw := range inbounds {
		if m, ok := raw.(map[string]any); ok {
			if t, _ := m["type"].(string); t == typ {
				inbounds[i] = item
				return inbounds
			}
		}
	}
	return append(inbounds, item)
}

func removeInboundType(inbounds []any, typ string) []any {
	out := make([]any, 0, len(inbounds))
	for _, raw := range inbounds {
		if m, ok := raw.(map[string]any); ok {
			if t, _ := m["type"].(string); t == typ {
				continue
			}
		}
		out = append(out, raw)
	}
	return out
}

func toSlice(v any) []any {
	if s, ok := v.([]any); ok {
		return append([]any(nil), s...)
	}
	return nil
}

func deepMerge(dst, src map[string]any) map[string]any {
	out := make(map[string]any, len(dst))
	for k, v := range dst {
		out[k] = v
	}
	for k, v := range src {
		if sm, ok := v.(map[string]any); ok {
			if dm, ok2 := out[k].(map[string]any); ok2 {
				out[k] = deepMerge(dm, sm)
				continue
			}
		}
		out[k] = v
	}
	return out
}

// migrateLegacyDNS upgrades pre-1.12 dns.servers (address-based) and dns.rules (outbound-based)
// to sing-box 1.12+ / 1.14+ typed DNS servers and route.default_domain_resolver.
func migrateLegacyDNS(cfg map[string]any) {
	dnsObj, ok := cfg["dns"].(map[string]any)
	if !ok || dnsObj == nil {
		return
	}

	var defaultResolver string
	if servers, ok := dnsObj["servers"].([]any); ok {
		outServers := make([]any, 0, len(servers))
		for _, raw := range servers {
			srv, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			// Already migrated if "type" is present
			if _, hasType := srv["type"]; hasType {
				outServers = append(outServers, srv)
				continue
			}
			addr, _ := srv["address"].(string)
			tag, _ := srv["tag"].(string)
			detour, _ := srv["detour"].(string)
			if addr == "" {
				continue
			}
			migrated := convertLegacyDNSServer(tag, addr, detour)
			if migrated != nil {
				outServers = append(outServers, migrated)
			}
		}
		dnsObj["servers"] = outServers
	}

	if rules, ok := dnsObj["rules"].([]any); ok {
		kept := make([]any, 0, len(rules))
		for _, raw := range rules {
			r, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			if _, hasOutbound := r["outbound"]; hasOutbound {
				if srv, ok := r["server"].(string); ok && defaultResolver == "" {
					defaultResolver = srv
				}
				continue
			}
			kept = append(kept, r)
		}
		if len(kept) > 0 {
			dnsObj["rules"] = kept
		} else {
			delete(dnsObj, "rules")
		}
	}

	if defaultResolver != "" {
		routeObj, _ := cfg["route"].(map[string]any)
		if routeObj == nil {
			routeObj = map[string]any{}
		}
		if _, has := routeObj["default_domain_resolver"]; !has {
			routeObj["default_domain_resolver"] = defaultResolver
		}
		cfg["route"] = routeObj
	}
}

func convertLegacyDNSServer(tag, addr, detour string) map[string]any {
	lower := strings.ToLower(strings.TrimSpace(addr))
	switch {
	case lower == "local":
		m := map[string]any{"type": "local"}
		if tag != "" {
			m["tag"] = tag
		}
		return m
	case lower == "fakeip":
		m := map[string]any{"type": "fakeip"}
		if tag != "" {
			m["tag"] = tag
		}
		return m
	case strings.HasPrefix(lower, "rcode://"):
		return nil
	case !strings.Contains(addr, "://"):
		m := map[string]any{"type": "udp", "server": addr}
		if tag != "" {
			m["tag"] = tag
		}
		if detour != "" && !strings.EqualFold(detour, "direct") {
			m["detour"] = detour
		}
		return m
	}

	u, err := url.Parse(addr)
	if err != nil {
		return nil
	}
	typ := strings.ToLower(u.Scheme)
	m := map[string]any{
		"type":   typ,
		"server": u.Hostname(),
	}
	if tag != "" {
		m["tag"] = tag
	}
	if p := u.Port(); p != "" {
		if port, err := strconv.Atoi(p); err == nil && port > 0 {
			m["server_port"] = port
		}
	}
	if (typ == "https" || typ == "h3") && u.Path != "" && u.Path != "/" {
		m["path"] = u.Path
	}
	if detour != "" && !strings.EqualFold(detour, "direct") {
		m["detour"] = detour
	}
	return m
}
