package profile

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bihua-university/livery/internal/model"
)

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

	// 3. Experimental Clash API injection (unconditional; required for telemetry and core control).
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
		host = model.DefaultClashAPIHost
	}
	port := settings.ClashAPIPort
	if port <= 0 {
		port = model.DefaultClashAPIPort
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

	// 4. Inbounds (mixed + optional TUN).
	inbounds := toSlice(cfg["inbounds"])
	if prof.Override.OverrideInbounds || len(inbounds) == 0 {
		listen := model.DefaultClashAPIHost
		if settings.AllowLAN {
			listen = "0.0.0.0"
		}
		port := settings.MixedPort
		if port <= 0 {
			port = model.DefaultMixedPort
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
		if r.FromConnection {
			switch r.Outbound {
			case "direct":
				obj["outbound"] = ensureSingBoxDirectTag(cfg)
			case "block": // The reject action is independent of outbound tags.
			default:
				if !hasNamedEntry(cfg["outbounds"], "tag", r.Outbound) {
					continue
				}
			}
		}
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

func ensureSingBoxDirectTag(cfg map[string]any) string {
	if existing := findSingBoxDirectTag(cfg); existing != "" {
		return existing
	}
	tag := "direct"
	for hasNamedEntry(cfg["outbounds"], "tag", tag) {
		tag += "-direct"
	}
	cfg["outbounds"] = append(toSlice(cfg["outbounds"]), map[string]any{"type": "direct", "tag": tag})
	return tag
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
