package profile

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/bihua-university/livery/internal/model"
)

func convertURIList(text string) ([]byte, error) {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	var nodeOutbounds []map[string]any
	var nodeTags []string
	usedTags := map[string]bool{"direct": true, "proxy": true, "auto": true}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		ob, ok := parseShareURI(line)
		if !ok {
			continue
		}
		tag, _ := ob["tag"].(string)
		tag = uniqueTag(tag, usedTags)
		ob["tag"] = tag
		usedTags[tag] = true
		nodeOutbounds = append(nodeOutbounds, ob)
		nodeTags = append(nodeTags, tag)
	}
	if len(nodeOutbounds) == 0 {
		return nil, errors.New("no valid proxy share URIs found")
	}

	allOutbounds := make([]any, 0, len(nodeOutbounds)+3)
	allOutbounds = append(allOutbounds,
		map[string]any{
			"type":      "selector",
			"tag":       "proxy",
			"outbounds": append([]string{"auto"}, append(nodeTags, "direct")...),
		},
		map[string]any{
			"type":      "urltest",
			"tag":       "auto",
			"outbounds": nodeTags,
			"url":       model.DefaultDelayTestURL,
			"interval":  "3m",
		},
	)
	for _, n := range nodeOutbounds {
		allOutbounds = append(allOutbounds, n)
	}
	allOutbounds = append(allOutbounds, map[string]any{"type": "direct", "tag": "direct"})

	rules := []any{
		map[string]any{"protocol": "dns", "action": "hijack-dns"},
		map[string]any{"domain_suffix": []string{"ts.net"}, "outbound": "direct"},
		map[string]any{"ip_cidr": []string{"100.64.0.0/10", "fd7a:115c:a1e0::/48"}, "outbound": "direct"},
		map[string]any{"ip_is_private": true, "outbound": "direct"},
	}
	cfg := buildBaseSingBoxConfig(allOutbounds, rules, "proxy")
	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}

func convertURIListToMihomo(text string) ([]byte, error) {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	var proxies []map[string]any
	var proxyNames []string
	used := map[string]bool{"DIRECT": true, "REJECT": true, "Proxy": true, "Auto": true}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		ob, ok := parseShareURI(line)
		if !ok {
			continue
		}
		tag := uniqueTag(strVal(ob["tag"]), used)
		ob["tag"] = tag
		cp, ok := singBoxOutboundToClashProxy(ob)
		if !ok {
			continue
		}
		used[tag] = true
		proxies = append(proxies, cp)
		proxyNames = append(proxyNames, tag)
	}
	if len(proxies) == 0 {
		return nil, errors.New("no valid proxy share URIs found")
	}

	proxyGroupMembers := append([]string{"Auto"}, append(proxyNames, "DIRECT")...)
	doc := map[string]any{
		"mixed-port": model.DefaultMixedPort,
		"allow-lan":  false,
		"mode":       "rule",
		"log-level":  "info",
		"dns":        defaultMihomoDNSMap(),
		"proxies":    proxies,
		"proxy-groups": []map[string]any{
			{
				"name":    "Proxy",
				"type":    "select",
				"proxies": proxyGroupMembers,
			},
			{
				"name":     "Auto",
				"type":     "url-test",
				"proxies":  proxyNames,
				"url":      model.DefaultDelayTestURL,
				"interval": 180,
			},
		},
		"rules": []string{
			"DOMAIN-SUFFIX,ts.net,DIRECT",
			"IP-CIDR,100.64.0.0/10,DIRECT,no-resolve",
			"IP-CIDR6,fd7a:115c:a1e0::/48,DIRECT,no-resolve",
			"IP-CIDR,127.0.0.0/8,DIRECT,no-resolve",
			"IP-CIDR,172.16.0.0/12,DIRECT,no-resolve",
			"IP-CIDR,192.168.0.0/16,DIRECT,no-resolve",
			"IP-CIDR,10.0.0.0/8,DIRECT,no-resolve",
			"MATCH,Proxy",
		},
	}
	return yaml.Marshal(doc)
}

// ConvertSingBoxToMihomo converts a sing-box JSON configuration into a Mihomo YAML configuration.
func ConvertSingBoxToMihomo(rawJSON []byte) ([]byte, error) {
	var cfg map[string]any
	if err := json.Unmarshal(rawJSON, &cfg); err != nil {
		return nil, fmt.Errorf("invalid sing-box JSON: %w", err)
	}
	if cfg == nil {
		return nil, errors.New("empty sing-box config")
	}

	var proxies []map[string]any
	var proxyNames []string
	var groups []map[string]any

	for _, rawOb := range toSlice(cfg["outbounds"]) {
		ob, ok := rawOb.(map[string]any)
		if !ok {
			continue
		}
		typ := strings.ToLower(strVal(ob["type"]))
		tag := strVal(ob["tag"])
		switch typ {
		case "direct", "block", "dns":
			continue
		case "selector", "urltest":
			if tag == "" {
				continue
			}
			members := make([]string, 0)
			for _, m := range strSlice(ob["outbounds"]) {
				members = append(members, mapSingBoxTargetToClash(m))
			}
			if len(members) == 0 {
				members = []string{"DIRECT"}
			}
			g := map[string]any{
				"name":    tag,
				"proxies": members,
			}
			if typ == "urltest" {
				g["type"] = "url-test"
				u := strVal(ob["url"])
				if u == "" {
					u = model.DefaultDelayTestURL
				}
				g["url"] = u
				interval := 180
				if ivStr := strVal(ob["interval"]); ivStr != "" {
					if d, err := time.ParseDuration(ivStr); err == nil && d > 0 {
						interval = max(int(d.Seconds()), 10)
					}
				}
				g["interval"] = interval
				if tol := intVal(ob["tolerance"]); tol > 0 {
					g["tolerance"] = tol
				}
			} else {
				g["type"] = "select"
			}
			groups = append(groups, g)
		default:
			if cp, ok := singBoxOutboundToClashProxy(ob); ok {
				proxies = append(proxies, cp)
				proxyNames = append(proxyNames, strVal(cp["name"]))
			}
		}
	}

	if len(groups) == 0 {
		members := append([]string(nil), proxyNames...)
		members = append(members, "DIRECT")
		groups = append(groups, map[string]any{
			"name":    "Proxy",
			"type":    "select",
			"proxies": members,
		})
	}

	defaultFinal, _ := groups[0]["name"].(string)
	if defaultFinal == "" {
		defaultFinal = "DIRECT"
	}

	routeObj, _ := cfg["route"].(map[string]any)
	clashRules := singBoxRulesToClashRules(routeObj, defaultFinal)

	doc := map[string]any{
		"mixed-port":   model.DefaultMixedPort,
		"allow-lan":    false,
		"mode":         "rule",
		"log-level":    "info",
		"dns":          defaultMihomoDNSMap(),
		"proxies":      proxies,
		"proxy-groups": groups,
		"rules":        clashRules,
	}
	return yaml.Marshal(doc)
}

func mapSingBoxTargetToClash(tag string) string {
	switch strings.ToLower(strings.TrimSpace(tag)) {
	case "direct":
		return "DIRECT"
	case "block", "reject":
		return "REJECT"
	default:
		return tag
	}
}

func singBoxOutboundToClashProxy(ob map[string]any) (map[string]any, bool) {
	typ := strings.ToLower(strVal(ob["type"]))
	tag := strVal(ob["tag"])
	server := strVal(ob["server"])
	port := intVal(ob["server_port"])
	if tag == "" || server == "" || port <= 0 {
		return nil, false
	}

	cp := map[string]any{
		"name":   tag,
		"server": server,
		"port":   port,
	}

	applyTLS := func() {
		tlsObj, ok := ob["tls"].(map[string]any)
		if !ok || !boolVal(tlsObj["enabled"]) {
			return
		}
		cp["tls"] = true
		if sni := strVal(tlsObj["server_name"]); sni != "" {
			if typ == "trojan" || typ == "hysteria2" || typ == "tuic" || typ == "http" {
				cp["sni"] = sni
			} else {
				cp["servername"] = sni
			}
		}
		if boolVal(tlsObj["insecure"]) {
			cp["skip-cert-verify"] = true
		}
		if alpn := strSlice(tlsObj["alpn"]); len(alpn) > 0 {
			cp["alpn"] = alpn
		}
		if utls, ok := tlsObj["utls"].(map[string]any); ok && boolVal(utls["enabled"]) {
			if fp := strVal(utls["fingerprint"]); fp != "" {
				cp["client-fingerprint"] = fp
			}
		}
		if reality, ok := tlsObj["reality"].(map[string]any); ok && boolVal(reality["enabled"]) {
			rOpts := map[string]any{}
			if pub := strVal(reality["public_key"]); pub != "" {
				rOpts["public-key"] = pub
			}
			if sid := strVal(reality["short_id"]); sid != "" {
				rOpts["short-id"] = sid
			}
			if len(rOpts) > 0 {
				cp["reality-opts"] = rOpts
			}
		}
	}

	applyTransport := func() {
		tr, ok := ob["transport"].(map[string]any)
		if !ok {
			return
		}
		netType := strings.ToLower(strVal(tr["type"]))
		switch netType {
		case "ws":
			cp["network"] = "ws"
			wsOpts := map[string]any{}
			if p := strVal(tr["path"]); p != "" {
				wsOpts["path"] = p
			}
			if hdrs, ok := tr["headers"].(map[string]any); ok && len(hdrs) > 0 {
				wsOpts["headers"] = hdrs
			}
			if len(wsOpts) > 0 {
				cp["ws-opts"] = wsOpts
			}
		case "grpc":
			cp["network"] = "grpc"
			if sn := strVal(tr["service_name"]); sn != "" {
				cp["grpc-opts"] = map[string]any{"grpc-service-name": sn}
			}
		case "http":
			cp["network"] = "h2"
			h2Opts := map[string]any{}
			if p := strVal(tr["path"]); p != "" {
				h2Opts["path"] = p
			}
			if hosts := strSlice(tr["host"]); len(hosts) > 0 {
				h2Opts["host"] = hosts
			}
			if len(h2Opts) > 0 {
				cp["h2-opts"] = h2Opts
			}
		}
	}

	switch typ {
	case "shadowsocks":
		cp["type"] = "ss"
		cp["cipher"] = strVal(ob["method"])
		cp["password"] = strVal(ob["password"])
		return cp, true
	case "vmess":
		cp["type"] = "vmess"
		cp["uuid"] = strVal(ob["uuid"])
		cp["alterId"] = intVal(ob["alter_id"])
		sec := strVal(ob["security"])
		if sec == "" {
			sec = "auto"
		}
		cp["cipher"] = sec
		applyTLS()
		applyTransport()
		return cp, true
	case "vless":
		cp["type"] = "vless"
		cp["uuid"] = strVal(ob["uuid"])
		if flow := strVal(ob["flow"]); flow != "" {
			cp["flow"] = flow
		}
		applyTLS()
		applyTransport()
		return cp, true
	case "trojan":
		cp["type"] = "trojan"
		cp["password"] = strVal(ob["password"])
		applyTLS()
		applyTransport()
		return cp, true
	case "hysteria2":
		cp["type"] = "hysteria2"
		cp["password"] = strVal(ob["password"])
		if obfs, ok := ob["obfs"].(map[string]any); ok {
			if ot := strVal(obfs["type"]); ot != "" {
				cp["obfs"] = ot
				cp["obfs-password"] = strVal(obfs["password"])
			}
		}
		applyTLS()
		return cp, true
	case "tuic":
		cp["type"] = "tuic"
		cp["uuid"] = strVal(ob["uuid"])
		cp["password"] = strVal(ob["password"])
		if cc := strVal(ob["congestion_control"]); cc != "" {
			cp["congestion-controller"] = cc
		}
		if urm := strVal(ob["udp_relay_mode"]); urm != "" {
			cp["udp-relay-mode"] = urm
		}
		applyTLS()
		return cp, true
	case "socks":
		cp["type"] = "socks5"
		if u := strVal(ob["username"]); u != "" {
			cp["username"] = u
			cp["password"] = strVal(ob["password"])
		}
		return cp, true
	case "http":
		cp["type"] = "http"
		if u := strVal(ob["username"]); u != "" {
			cp["username"] = u
			cp["password"] = strVal(ob["password"])
		}
		applyTLS()
		return cp, true
	}
	return nil, false
}

func singBoxRulesToClashRules(routeObj map[string]any, defaultFinal string) []string {
	if routeObj == nil {
		return []string{"MATCH," + mapSingBoxTargetToClash(defaultFinal)}
	}
	var out []string
	for _, raw := range toSlice(routeObj["rules"]) {
		r, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		target := mapSingBoxTargetToClash(strVal(r["outbound"]))
		if act := strVal(r["action"]); target == "" && strings.EqualFold(act, "reject") {
			target = "REJECT"
		}
		if target == "" {
			continue
		}
		appendList := func(clashType string, v any, extra string) bool {
			var vals []string
			switch t := v.(type) {
			case string:
				if t != "" {
					vals = []string{t}
				}
			case []any:
				vals = strSlice(t)
			case []string:
				vals = t
			case float64:
				vals = []string{fmt.Sprintf("%.0f", t)}
			case int:
				vals = []string{strconv.Itoa(t)}
			}
			if len(vals) == 0 {
				return false
			}
			for _, item := range vals {
				line := clashType + "," + item + "," + target
				if extra != "" {
					line += "," + extra
				}
				out = append(out, line)
			}
			return true
		}

		switch {
		case appendList("DOMAIN", r["domain"], ""):
		case appendList("DOMAIN-SUFFIX", r["domain_suffix"], ""):
		case appendList("DOMAIN-KEYWORD", r["domain_keyword"], ""):
		case appendList("DOMAIN-REGEX", r["domain_regex"], ""):
		case appendList("IP-CIDR", r["ip_cidr"], "no-resolve"):
		case appendList("SRC-IP-CIDR", r["source_ip_cidr"], "no-resolve"):
		case appendList("DST-PORT", r["port"], ""):
		case appendList("SRC-PORT", r["source_port"], ""):
		case appendList("PROCESS-NAME", r["process_name"], ""):
		case appendList("NETWORK", r["network"], ""):
		case appendList("GEOSITE", r["geosite"], ""):
		case appendList("GEOIP", r["geoip"], "no-resolve"):
		case r["rule_set"] != nil:
			var sets []string
			switch t := r["rule_set"].(type) {
			case string:
				if t != "" {
					sets = []string{t}
				}
			case []any:
				sets = strSlice(t)
			case []string:
				sets = t
			}
			for _, rs := range sets {
				lowerRS := strings.ToLower(strings.TrimSpace(rs))
				switch {
				case strings.HasPrefix(lowerRS, "geosite-"):
					out = append(out, "GEOSITE,"+strings.TrimPrefix(lowerRS, "geosite-")+","+target)
				case strings.HasPrefix(lowerRS, "geoip-"):
					code := strings.TrimPrefix(lowerRS, "geoip-")
					if code == "private" {
						out = append(out, "GEOIP,LAN,"+target+",no-resolve")
					} else {
						out = append(out, "GEOIP,"+strings.ToUpper(code)+","+target+",no-resolve")
					}
				}
			}
		case boolVal(r["ip_is_private"]):
			out = append(out, "GEOIP,LAN,"+target+",no-resolve")
		}
	}
	finalTarget := mapSingBoxTargetToClash(strVal(routeObj["final"]))
	if finalTarget == "" {
		finalTarget = mapSingBoxTargetToClash(defaultFinal)
	}
	out = append(out, "MATCH,"+finalTarget)
	return out
}

func buildBaseSingBoxConfig(outbounds []any, routeRules []any, finalTag string) map[string]any {
	return map[string]any{
		"log": map[string]any{
			"level":     "info",
			"timestamp": true,
		},
		"dns": map[string]any{
			"servers": []any{
				map[string]any{"type": "local", "tag": "dns-local"},
				map[string]any{"type": "udp", "tag": "dns-direct", "server": "223.5.5.5"},
				map[string]any{"type": "udp", "tag": "dns-remote", "server": "1.1.1.1", "detour": finalTag},
			},
			"rules": []any{
				map[string]any{"domain_suffix": []string{"ts.net"}, "server": "dns-local"},
			},
			"final": "dns-remote",
		},
		"inbounds": []any{
			map[string]any{
				"type":        "mixed",
				"tag":         "mixed-in",
				"listen":      model.DefaultClashAPIHost,
				"listen_port": model.DefaultMixedPort,
			},
		},
		"outbounds": outbounds,
		"route": map[string]any{
			"default_domain_resolver": "dns-direct",
			"rules":                   routeRules,
			"final":                   finalTag,
			"auto_detect_interface":   true,
		},
	}
}
