package profile

import (
	"net/url"
	"strconv"
	"strings"
)

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
