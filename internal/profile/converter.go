package profile

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/bihua-university/livery/internal/model"
)

// DetectAndNormalizeProfile detects Mihomo/Clash YAML, sing-box JSON, or Base64/URI share links.
// Native Clash/Mihomo YAML and sing-box JSON are preserved in their native format; URI lists are
// converted to the preferredCore's native format.
func DetectAndNormalizeProfile(raw []byte, preferredCore model.CoreType) ([]byte, string, string, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, "", "", errors.New("empty subscription content")
	}

	// 1. Native sing-box JSON
	if trimmed[0] == '{' {
		var obj map[string]any
		if err := json.Unmarshal(trimmed, &obj); err == nil && obj != nil {
			out, err := json.MarshalIndent(obj, "", "  ")
			if err != nil {
				return nil, "", "", err
			}
			return append(out, '\n'), "sing-box", ".json", nil
		}
	}

	// 2. Native Clash / Mihomo YAML
	if isClashYAML(trimmed) {
		return append(trimmed, '\n'), "mihomo", ".yaml", nil
	}

	// 3. Plain URI list or Base64-encoded payload
	text := string(trimmed)
	if !looksLikeURIList(text) {
		if decoded, ok := decodeBase64Flexible(text); ok {
			if looksLikeURIList(decoded) {
				text = decoded
			} else {
				dt := bytes.TrimSpace([]byte(decoded))
				if len(dt) > 0 && dt[0] == '{' {
					var obj map[string]any
					if json.Unmarshal(dt, &obj) == nil && obj != nil {
						out, _ := json.MarshalIndent(obj, "", "  ")
						return append(out, '\n'), "sing-box", ".json", nil
					}
				}
				if isClashYAML(dt) {
					return append(dt, '\n'), "mihomo", ".yaml", nil
				}
			}
		}
	}

	if looksLikeURIList(text) {
		if model.NormalizeCoreType(preferredCore) == model.CoreSingBox {
			out, err := convertURIList(text)
			if err != nil {
				return nil, "", "", err
			}
			return out, "sing-box", ".json", nil
		}
		out, err := convertURIListToMihomo(text)
		if err != nil {
			return nil, "", "", err
		}
		return out, "mihomo", ".yaml", nil
	}

	return nil, "", "", errors.New("unrecognized subscription format (expected Mihomo/Clash YAML, sing-box JSON, or Base64/URI links)")
}

// ConvertToMihomo detects Clash/Mihomo YAML, sing-box JSON, or Base64/URI share links
// and converts the input into Mihomo YAML.
func ConvertToMihomo(raw []byte) ([]byte, string, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, "", errors.New("empty subscription content")
	}

	if trimmed[0] == '{' {
		out, err := ConvertSingBoxToMihomo(trimmed)
		if err == nil {
			return out, "sing-box", nil
		}
	}

	if isClashYAML(trimmed) {
		return append(trimmed, '\n'), "clash", nil
	}

	text := string(trimmed)
	if !looksLikeURIList(text) {
		if decoded, ok := decodeBase64Flexible(text); ok {
			if looksLikeURIList(decoded) {
				text = decoded
			} else {
				dt := bytes.TrimSpace([]byte(decoded))
				if len(dt) > 0 && dt[0] == '{' {
					if out, err := ConvertSingBoxToMihomo(dt); err == nil {
						return out, "sing-box", nil
					}
				}
				if isClashYAML(dt) {
					return append(dt, '\n'), "clash", nil
				}
			}
		}
	}

	if looksLikeURIList(text) {
		out, err := convertURIListToMihomo(text)
		if err != nil {
			return nil, "", err
		}
		return out, "uri", nil
	}

	return nil, "", errors.New("unrecognized subscription format (expected Mihomo/Clash YAML, sing-box JSON, or Base64/URI links)")
}

func isClashYAML(data []byte) bool {
	if len(data) == 0 || data[0] == '{' || data[0] == '[' {
		return false
	}
	var m map[string]any
	if err := yaml.Unmarshal(data, &m); err != nil {
		return false
	}
	return isClashYAMLMap(m)
}

func isClashYAMLMap(m map[string]any) bool {
	if m == nil {
		return false
	}
	for _, k := range []string{"proxies", "proxy-groups", "proxy-providers"} {
		if _, ok := m[k]; ok {
			return true
		}
	}
	if _, hasRules := m["rules"]; hasRules {
		for _, k := range []string{"mixed-port", "port", "socks-port", "mode", "external-controller"} {
			if _, ok := m[k]; ok {
				return true
			}
		}
	}
	return false
}

// ConvertToSingBox detects native sing-box JSON, Clash/Mihomo YAML, or Base64/URI share links
// and converts the input into formatted sing-box JSON.
func ConvertToSingBox(raw []byte) ([]byte, string, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, "", errors.New("empty subscription content")
	}

	// 1. Native sing-box JSON
	if trimmed[0] == '{' {
		var obj map[string]any
		if err := json.Unmarshal(trimmed, &obj); err == nil && obj != nil {
			out, err := json.MarshalIndent(obj, "", "  ")
			if err != nil {
				return nil, "", err
			}
			return append(out, '\n'), "sing-box", nil
		}
	}

	// 2. Clash / Mihomo YAML
	if out, err := convertClashYAML(trimmed); err == nil {
		return out, "clash", nil
	}

	// 3. Plain URI list or Base64-encoded URI list
	text := string(trimmed)
	if !looksLikeURIList(text) {
		if decoded, ok := decodeBase64Flexible(text); ok && looksLikeURIList(decoded) {
			text = decoded
		} else if decoded, ok := decodeBase64Flexible(text); ok {
			// Some airports base64-encode a Clash YAML or sing-box JSON
			dt := bytes.TrimSpace([]byte(decoded))
			if len(dt) > 0 && dt[0] == '{' {
				var obj map[string]any
				if json.Unmarshal(dt, &obj) == nil && obj != nil {
					out, _ := json.MarshalIndent(obj, "", "  ")
					return append(out, '\n'), "sing-box", nil
				}
			}
			if out, err := convertClashYAML(dt); err == nil {
				return out, "clash", nil
			}
		}
	}

	if looksLikeURIList(text) {
		out, err := convertURIList(text)
		if err != nil {
			return nil, "", err
		}
		return out, "uri", nil
	}

	return nil, "", errors.New("unrecognized subscription format (expected sing-box JSON, Clash YAML, or Base64/URI links)")
}

type clashDoc struct {
	Proxies     []map[string]any `yaml:"proxies"`
	ProxyGroups []clashGroup     `yaml:"proxy-groups"`
	Rules       []string         `yaml:"rules"`
}

type clashGroup struct {
	Name      string   `yaml:"name"`
	Type      string   `yaml:"type"`
	Proxies   []string `yaml:"proxies"`
	URL       string   `yaml:"url"`
	Interval  int      `yaml:"interval"`
	Tolerance int      `yaml:"tolerance"`
}

func convertClashYAML(data []byte) ([]byte, error) {
	var doc clashDoc
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	if len(doc.Proxies) == 0 && len(doc.ProxyGroups) == 0 {
		return nil, errors.New("not a Clash YAML config")
	}

	var nodeOutbounds []map[string]any
	var nodeTags []string
	validTags := map[string]bool{"direct": true}

	for _, rawProxy := range doc.Proxies {
		ob, ok := clashProxyToSingBox(rawProxy)
		if !ok {
			continue
		}
		tag, _ := ob["tag"].(string)
		tag = uniqueTag(tag, validTags)
		ob["tag"] = tag
		validTags[tag] = true
		nodeOutbounds = append(nodeOutbounds, ob)
		nodeTags = append(nodeTags, tag)
	}

	if len(nodeOutbounds) == 0 {
		return nil, errors.New("no supported proxy nodes found in Clash YAML")
	}

	// Register group names upfront so groups can reference each other regardless of ordering.
	groupTagSet := make(map[string]bool, len(doc.ProxyGroups))
	for _, g := range doc.ProxyGroups {
		if g.Name != "" {
			groupTagSet[g.Name] = true
			validTags[g.Name] = true
		}
	}

	var groupOutbounds []map[string]any
	for _, g := range doc.ProxyGroups {
		if g.Name == "" {
			continue
		}
		members := make([]string, 0, len(g.Proxies))
		seen := make(map[string]bool)
		for _, m := range g.Proxies {
			mapped := mapClashTarget(m)
			if mapped == "" || strings.EqualFold(mapped, "reject") {
				continue
			}
			if !validTags[mapped] && !groupTagSet[mapped] {
				continue
			}
			if !seen[mapped] {
				seen[mapped] = true
				members = append(members, mapped)
			}
		}
		if len(members) == 0 {
			members = append(members, nodeTags...)
		}

		sbType := "selector"
		switch strings.ToLower(g.Type) {
		case "url-test", "fallback", "load-balance":
			sbType = "urltest"
		}
		gob := map[string]any{
			"type":      sbType,
			"tag":       g.Name,
			"outbounds": members,
		}
		if sbType == "urltest" {
			u := g.URL
			if u == "" {
				u = "http://www.gstatic.com/generate_204"
			}
			interval := "3m"
			if g.Interval > 0 {
				interval = fmt.Sprintf("%ds", g.Interval)
			}
			gob["url"] = u
			gob["interval"] = interval
			if g.Tolerance > 0 {
				gob["tolerance"] = g.Tolerance
			}
		}
		groupOutbounds = append(groupOutbounds, gob)
	}

	// Fallback if Clash YAML had proxies without proxy-groups.
	defaultFinal := ""
	if len(groupOutbounds) == 0 {
		autoTag := uniqueTag("auto", validTags)
		validTags[autoTag] = true
		proxyTag := uniqueTag("proxy", validTags)
		validTags[proxyTag] = true
		groupOutbounds = append(groupOutbounds,
			map[string]any{
				"type":      "selector",
				"tag":       proxyTag,
				"outbounds": append([]string{autoTag}, append(nodeTags, "direct")...),
			},
			map[string]any{
				"type":      "urltest",
				"tag":       autoTag,
				"outbounds": nodeTags,
				"url":       "http://www.gstatic.com/generate_204",
				"interval":  "3m",
			},
		)
		defaultFinal = proxyTag
	} else {
		defaultFinal, _ = groupOutbounds[0]["tag"].(string)
	}

	allOutbounds := make([]any, 0, len(groupOutbounds)+len(nodeOutbounds)+1)
	for _, g := range groupOutbounds {
		allOutbounds = append(allOutbounds, g)
	}
	for _, n := range nodeOutbounds {
		allOutbounds = append(allOutbounds, n)
	}
	allOutbounds = append(allOutbounds, map[string]any{
		"type": "direct",
		"tag":  "direct",
	})

	routeRules, finalTag := convertClashRules(doc.Rules, validTags, defaultFinal)
	cfg := buildBaseSingBoxConfig(allOutbounds, routeRules, finalTag)
	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}

func clashProxyToSingBox(p map[string]any) (map[string]any, bool) {
	name := strVal(p["name"])
	typ := strings.ToLower(strVal(p["type"]))
	server := strVal(p["server"])
	port := intVal(p["port"])
	if name == "" || server == "" || port <= 0 {
		return nil, false
	}

	ob := map[string]any{
		"tag":         name,
		"server":      server,
		"server_port": port,
	}

	switch typ {
	case "ss":
		ob["type"] = "shadowsocks"
		ob["method"] = strVal(p["cipher"])
		ob["password"] = strVal(p["password"])
		if plugin := strVal(p["plugin"]); plugin != "" {
			if opts, ok := p["plugin-opts"].(map[string]any); ok {
				switch plugin {
				case "obfs":
					ob["plugin"] = "obfs-local"
					ob["plugin_opts"] = fmt.Sprintf("obfs=%s;obfs-host=%s", strVal(opts["mode"]), strVal(opts["host"]))
				case "v2ray-plugin":
					ob["plugin"] = "v2ray-plugin"
				}
			}
		}
		return ob, true

	case "vmess":
		ob["type"] = "vmess"
		ob["uuid"] = strVal(p["uuid"])
		if aid := intVal(p["alterId"]); aid > 0 {
			ob["alter_id"] = aid
		}
		cipher := strVal(p["cipher"])
		if cipher == "" {
			cipher = "auto"
		}
		ob["security"] = cipher
		if boolVal(p["tls"]) {
			ob["tls"] = buildSingBoxTLS(strVal(p["servername"]), strVal(p["sni"]), boolVal(p["skip-cert-verify"]), strSlice(p["alpn"]), strVal(p["client-fingerprint"]), nil)
		}
		if tr := buildClashTransport(p); tr != nil {
			ob["transport"] = tr
		}
		return ob, true

	case "vless":
		ob["type"] = "vless"
		ob["uuid"] = strVal(p["uuid"])
		if flow := strVal(p["flow"]); flow != "" {
			ob["flow"] = flow
		}
		realityMap, _ := p["reality-opts"].(map[string]any)
		if boolVal(p["tls"]) || realityMap != nil {
			ob["tls"] = buildSingBoxTLS(strVal(p["servername"]), strVal(p["sni"]), boolVal(p["skip-cert-verify"]), strSlice(p["alpn"]), strVal(p["client-fingerprint"]), realityMap)
		}
		if tr := buildClashTransport(p); tr != nil {
			ob["transport"] = tr
		}
		return ob, true

	case "trojan":
		ob["type"] = "trojan"
		ob["password"] = strVal(p["password"])
		ob["tls"] = buildSingBoxTLS(strVal(p["sni"]), "", boolVal(p["skip-cert-verify"]), strSlice(p["alpn"]), strVal(p["client-fingerprint"]), nil)
		if tr := buildClashTransport(p); tr != nil {
			ob["transport"] = tr
		}
		return ob, true

	case "hysteria2", "hy2":
		ob["type"] = "hysteria2"
		ob["password"] = strVal(p["password"])
		if obfs := strVal(p["obfs"]); obfs != "" {
			ob["obfs"] = map[string]any{
				"type":     obfs,
				"password": strVal(p["obfs-password"]),
			}
		}
		ob["tls"] = buildSingBoxTLS(strVal(p["sni"]), "", boolVal(p["skip-cert-verify"]), strSlice(p["alpn"]), "", nil)
		return ob, true

	case "tuic":
		ob["type"] = "tuic"
		ob["uuid"] = strVal(p["uuid"])
		ob["password"] = strVal(p["password"])
		if cc := strVal(p["congestion-controller"]); cc != "" {
			ob["congestion_control"] = cc
		}
		if urm := strVal(p["udp-relay-mode"]); urm != "" {
			ob["udp_relay_mode"] = urm
		}
		ob["tls"] = buildSingBoxTLS(strVal(p["sni"]), "", boolVal(p["skip-cert-verify"]), strSlice(p["alpn"]), "", nil)
		return ob, true

	case "socks5":
		ob["type"] = "socks"
		ob["version"] = "5"
		if u := strVal(p["username"]); u != "" {
			ob["username"] = u
			ob["password"] = strVal(p["password"])
		}
		return ob, true

	case "http":
		ob["type"] = "http"
		if u := strVal(p["username"]); u != "" {
			ob["username"] = u
			ob["password"] = strVal(p["password"])
		}
		if boolVal(p["tls"]) {
			ob["tls"] = buildSingBoxTLS(strVal(p["sni"]), "", boolVal(p["skip-cert-verify"]), nil, "", nil)
		}
		return ob, true
	}

	return nil, false
}

func buildSingBoxTLS(sni1, sni2 string, insecure bool, alpn []string, fp string, reality map[string]any) map[string]any {
	sni := sni1
	if sni == "" {
		sni = sni2
	}
	tls := map[string]any{
		"enabled": true,
	}
	if sni != "" {
		tls["server_name"] = sni
	}
	if insecure {
		tls["insecure"] = true
	}
	if len(alpn) > 0 {
		tls["alpn"] = alpn
	}
	if fp != "" {
		tls["utls"] = map[string]any{
			"enabled":     true,
			"fingerprint": fp,
		}
	}
	if reality != nil {
		if pub := strVal(reality["public-key"]); pub != "" {
			rObj := map[string]any{
				"enabled":    true,
				"public_key": pub,
			}
			if sid := strVal(reality["short-id"]); sid != "" {
				rObj["short_id"] = sid
			}
			tls["reality"] = rObj
			if _, ok := tls["utls"]; !ok {
				tls["utls"] = map[string]any{
					"enabled":     true,
					"fingerprint": "chrome",
				}
			}
		}
	}
	return tls
}

func buildClashTransport(p map[string]any) map[string]any {
	netType := strings.ToLower(strVal(p["network"]))
	switch netType {
	case "ws":
		tr := map[string]any{"type": "ws"}
		if wsOpts, ok := p["ws-opts"].(map[string]any); ok {
			if path := strVal(wsOpts["path"]); path != "" {
				tr["path"] = path
			}
			if hdrs, ok := wsOpts["headers"].(map[string]any); ok && len(hdrs) > 0 {
				cleanHdrs := make(map[string]any, len(hdrs))
				for k, v := range hdrs {
					cleanHdrs[k] = strVal(v)
				}
				tr["headers"] = cleanHdrs
			}
		}
		return tr
	case "grpc":
		tr := map[string]any{"type": "grpc"}
		if gOpts, ok := p["grpc-opts"].(map[string]any); ok {
			if svc := strVal(gOpts["grpc-service-name"]); svc != "" {
				tr["service_name"] = svc
			}
		}
		return tr
	case "h2", "http":
		tr := map[string]any{"type": "http"}
		if hOpts, ok := p["h2-opts"].(map[string]any); ok {
			if path := strVal(hOpts["path"]); path != "" {
				tr["path"] = path
			}
			if hosts := strSlice(hOpts["host"]); len(hosts) > 0 {
				tr["host"] = hosts
			}
		}
		return tr
	}
	return nil
}

func convertClashRules(rawRules []string, validTags map[string]bool, defaultFinal string) ([]any, string) {
	rules := []any{
		map[string]any{"protocol": "dns", "action": "hijack-dns"},
		map[string]any{"ip_is_private": true, "outbound": "direct"},
	}
	finalTag := defaultFinal

	for _, line := range rawRules {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, ",")
		if len(parts) < 2 {
			continue
		}
		ruleType := strings.ToUpper(strings.TrimSpace(parts[0]))
		if ruleType == "MATCH" || ruleType == "FINAL" {
			target := mapClashTarget(parts[1])
			if validTags[target] {
				finalTag = target
			}
			continue
		}
		if len(parts) < 3 {
			continue
		}
		payload := strings.TrimSpace(parts[1])
		target := mapClashTarget(parts[2])
		isReject := strings.EqualFold(target, "reject")
		if !isReject && !validTags[target] {
			target = defaultFinal
		}

		var entry map[string]any
		switch ruleType {
		case "DOMAIN":
			entry = map[string]any{"domain": []string{payload}}
		case "DOMAIN-SUFFIX":
			entry = map[string]any{"domain_suffix": []string{payload}}
		case "DOMAIN-KEYWORD":
			entry = map[string]any{"domain_keyword": []string{payload}}
		case "DOMAIN-REGEX":
			entry = map[string]any{"domain_regex": []string{payload}}
		case "IP-CIDR", "IP-CIDR6":
			entry = map[string]any{"ip_cidr": []string{payload}}
		case "SRC-IP-CIDR":
			entry = map[string]any{"source_ip_cidr": []string{payload}}
		case "DST-PORT":
			if p, err := strconv.Atoi(payload); err == nil {
				entry = map[string]any{"port": []int{p}}
			}
		case "SRC-PORT":
			if p, err := strconv.Atoi(payload); err == nil {
				entry = map[string]any{"source_port": []int{p}}
			}
		case "PROCESS-NAME":
			entry = map[string]any{"process_name": []string{payload}}
		case "GEOIP":
			if strings.EqualFold(payload, "LAN") {
				entry = map[string]any{"ip_is_private": true}
			}
		}
		if entry == nil {
			continue
		}
		if isReject {
			entry["action"] = "reject"
		} else {
			entry["outbound"] = target
		}
		rules = append(rules, entry)
	}
	return rules, finalTag
}

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
			"url":       "http://www.gstatic.com/generate_204",
			"interval":  "3m",
		},
	)
	for _, n := range nodeOutbounds {
		allOutbounds = append(allOutbounds, n)
	}
	allOutbounds = append(allOutbounds, map[string]any{"type": "direct", "tag": "direct"})

	rules := []any{
		map[string]any{"protocol": "dns", "action": "hijack-dns"},
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
		"mixed-port": 7890,
		"allow-lan":  false,
		"mode":       "rule",
		"log-level":  "info",
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
				"url":      "http://www.gstatic.com/generate_204",
				"interval": 180,
			},
		},
		"rules": []string{
			"IP-CIDR,127.0.0.0/8,DIRECT,no-resolve",
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
					u = "http://www.gstatic.com/generate_204"
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
		"mixed-port":   7890,
		"allow-lan":    false,
		"mode":         "rule",
		"log-level":    "info",
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

func parseShareURI(raw string) (map[string]any, bool) {
	lower := strings.ToLower(raw)
	switch {
	case strings.HasPrefix(lower, "vmess://"):
		return parseVMessURI(raw[len("vmess://"):])
	case strings.HasPrefix(lower, "vless://"):
		return parseVLESSURI(raw)
	case strings.HasPrefix(lower, "trojan://"):
		return parseTrojanURI(raw)
	case strings.HasPrefix(lower, "ss://"):
		return parseSSURI(raw)
	case strings.HasPrefix(lower, "hysteria2://"), strings.HasPrefix(lower, "hy2://"):
		return parseHysteria2URI(raw)
	case strings.HasPrefix(lower, "tuic://"):
		return parseTUICURI(raw)
	}
	return nil, false
}

func parseVMessURI(payload string) (map[string]any, bool) {
	decoded, ok := decodeBase64Flexible(payload)
	if !ok {
		return nil, false
	}
	var v struct {
		PS   string `json:"ps"`
		Add  string `json:"add"`
		Port any    `json:"port"`
		ID   string `json:"id"`
		Aid  any    `json:"aid"`
		Scy  string `json:"scy"`
		Net  string `json:"net"`
		Type string `json:"type"`
		Host string `json:"host"`
		Path string `json:"path"`
		TLS  string `json:"tls"`
		SNI  string `json:"sni"`
		Fp   string `json:"fp"`
	}
	if err := json.Unmarshal([]byte(decoded), &v); err != nil || v.Add == "" || v.ID == "" {
		return nil, false
	}
	port := intVal(v.Port)
	if port <= 0 {
		return nil, false
	}
	tag := v.PS
	if tag == "" {
		tag = v.Add
	}
	scy := v.Scy
	if scy == "" {
		scy = "auto"
	}
	ob := map[string]any{
		"type":        "vmess",
		"tag":         tag,
		"server":      v.Add,
		"server_port": port,
		"uuid":        v.ID,
		"security":    scy,
	}
	if aid := intVal(v.Aid); aid > 0 {
		ob["alter_id"] = aid
	}
	if strings.EqualFold(v.TLS, "tls") {
		ob["tls"] = buildSingBoxTLS(v.SNI, v.Host, false, nil, v.Fp, nil)
	}
	if strings.EqualFold(v.Net, "ws") {
		tr := map[string]any{"type": "ws"}
		if v.Path != "" {
			tr["path"] = v.Path
		}
		if v.Host != "" {
			tr["headers"] = map[string]any{"Host": v.Host}
		}
		ob["transport"] = tr
	} else if strings.EqualFold(v.Net, "grpc") {
		tr := map[string]any{"type": "grpc"}
		if v.Path != "" {
			tr["service_name"] = v.Path
		}
		ob["transport"] = tr
	}
	return ob, true
}

func parseVLESSURI(raw string) (map[string]any, bool) {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return nil, false
	}
	port, _ := strconv.Atoi(u.Port())
	if port <= 0 {
		port = 443
	}
	q := u.Query()
	tag := u.Fragment
	if tag == "" {
		tag = u.Hostname()
	}
	ob := map[string]any{
		"type":        "vless",
		"tag":         tag,
		"server":      u.Hostname(),
		"server_port": port,
		"uuid":        u.User.Username(),
	}
	if flow := q.Get("flow"); flow != "" {
		ob["flow"] = flow
	}
	sec := strings.ToLower(q.Get("security"))
	if sec == "tls" || sec == "reality" {
		var reality map[string]any
		if sec == "reality" {
			reality = map[string]any{
				"public-key": q.Get("pbk"),
				"short-id":   q.Get("sid"),
			}
		}
		var alpn []string
		if a := q.Get("alpn"); a != "" {
			alpn = strings.Split(a, ",")
		}
		ob["tls"] = buildSingBoxTLS(q.Get("sni"), "", q.Get("allowInsecure") == "1", alpn, q.Get("fp"), reality)
	}
	switch strings.ToLower(q.Get("type")) {
	case "ws":
		tr := map[string]any{"type": "ws"}
		if p := q.Get("path"); p != "" {
			tr["path"] = p
		}
		if h := q.Get("host"); h != "" {
			tr["headers"] = map[string]any{"Host": h}
		}
		ob["transport"] = tr
	case "grpc":
		tr := map[string]any{"type": "grpc"}
		if sn := q.Get("serviceName"); sn != "" {
			tr["service_name"] = sn
		}
		ob["transport"] = tr
	}
	return ob, true
}

func parseTrojanURI(raw string) (map[string]any, bool) {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return nil, false
	}
	port, _ := strconv.Atoi(u.Port())
	if port <= 0 {
		port = 443
	}
	q := u.Query()
	tag := u.Fragment
	if tag == "" {
		tag = u.Hostname()
	}
	var alpn []string
	if a := q.Get("alpn"); a != "" {
		alpn = strings.Split(a, ",")
	}
	ob := map[string]any{
		"type":        "trojan",
		"tag":         tag,
		"server":      u.Hostname(),
		"server_port": port,
		"password":    u.User.Username(),
		"tls":         buildSingBoxTLS(q.Get("sni"), "", q.Get("allowInsecure") == "1", alpn, q.Get("fp"), nil),
	}
	if strings.EqualFold(q.Get("type"), "ws") {
		tr := map[string]any{"type": "ws"}
		if p := q.Get("path"); p != "" {
			tr["path"] = p
		}
		if h := q.Get("host"); h != "" {
			tr["headers"] = map[string]any{"Host": h}
		}
		ob["transport"] = tr
	} else if strings.EqualFold(q.Get("type"), "grpc") {
		tr := map[string]any{"type": "grpc"}
		if sn := q.Get("serviceName"); sn != "" {
			tr["service_name"] = sn
		}
		ob["transport"] = tr
	}
	return ob, true
}

func parseSSURI(raw string) (map[string]any, bool) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, false
	}
	tag := u.Fragment
	host := u.Hostname()
	portStr := u.Port()
	var method, password string

	if u.User != nil && u.User.Username() != "" {
		if pw, hasPW := u.User.Password(); hasPW {
			method = u.User.Username()
			password = pw
		} else if dec, ok := decodeBase64Flexible(u.User.Username()); ok {
			if m, p, found := strings.Cut(dec, ":"); found {
				method, password = m, p
			}
		}
	} else {
		// SIP002 legacy base64(method:password@host:port)
		body := strings.TrimPrefix(raw, "ss://")
		if idx := strings.Index(body, "#"); idx >= 0 {
			tag, _ = url.QueryUnescape(body[idx+1:])
			body = body[:idx]
		}
		if dec, ok := decodeBase64Flexible(body); ok {
			if creds, hostPort, found := strings.Cut(dec, "@"); found {
				method, password, _ = strings.Cut(creds, ":")
				if h, p, ok2 := strings.Cut(hostPort, ":"); ok2 {
					host, portStr = h, p
				}
			}
		}
	}

	port, _ := strconv.Atoi(portStr)
	if host == "" || port <= 0 || method == "" {
		return nil, false
	}
	if tag == "" {
		tag = host
	}
	return map[string]any{
		"type":        "shadowsocks",
		"tag":         tag,
		"server":      host,
		"server_port": port,
		"method":      method,
		"password":    password,
	}, true
}

func parseHysteria2URI(raw string) (map[string]any, bool) {
	normalized := raw
	if strings.HasPrefix(strings.ToLower(raw), "hy2://") {
		normalized = "hysteria2://" + raw[len("hy2://"):]
	}
	u, err := url.Parse(normalized)
	if err != nil || u.Hostname() == "" {
		return nil, false
	}
	port, _ := strconv.Atoi(u.Port())
	if port <= 0 {
		port = 443
	}
	q := u.Query()
	tag := u.Fragment
	if tag == "" {
		tag = u.Hostname()
	}
	ob := map[string]any{
		"type":        "hysteria2",
		"tag":         tag,
		"server":      u.Hostname(),
		"server_port": port,
		"password":    u.User.Username(),
		"tls":         buildSingBoxTLS(q.Get("sni"), "", q.Get("insecure") == "1", nil, "", nil),
	}
	if obfs := q.Get("obfs"); obfs != "" {
		ob["obfs"] = map[string]any{
			"type":     obfs,
			"password": q.Get("obfs-password"),
		}
	}
	return ob, true
}

func parseTUICURI(raw string) (map[string]any, bool) {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return nil, false
	}
	port, _ := strconv.Atoi(u.Port())
	if port <= 0 {
		port = 443
	}
	pw, _ := u.User.Password()
	q := u.Query()
	tag := u.Fragment
	if tag == "" {
		tag = u.Hostname()
	}
	var alpn []string
	if a := q.Get("alpn"); a != "" {
		alpn = strings.Split(a, ",")
	}
	ob := map[string]any{
		"type":        "tuic",
		"tag":         tag,
		"server":      u.Hostname(),
		"server_port": port,
		"uuid":        u.User.Username(),
		"password":    pw,
		"tls":         buildSingBoxTLS(q.Get("sni"), "", q.Get("allow_insecure") == "1", alpn, "", nil),
	}
	if cc := q.Get("congestion_control"); cc != "" {
		ob["congestion_control"] = cc
	}
	return ob, true
}

func buildBaseSingBoxConfig(outbounds []any, routeRules []any, finalTag string) map[string]any {
	return map[string]any{
		"log": map[string]any{
			"level":     "info",
			"timestamp": true,
		},
		"dns": map[string]any{
			"servers": []any{
				map[string]any{"type": "udp", "tag": "dns-direct", "server": "223.5.5.5"},
				map[string]any{"type": "udp", "tag": "dns-remote", "server": "1.1.1.1", "detour": finalTag},
			},
			"final": "dns-remote",
		},
		"inbounds": []any{
			map[string]any{
				"type":        "mixed",
				"tag":         "mixed-in",
				"listen":      "127.0.0.1",
				"listen_port": 7890,
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

func mapClashTarget(name string) string {
	n := strings.TrimSpace(name)
	switch strings.ToUpper(n) {
	case "DIRECT":
		return "direct"
	case "REJECT", "REJECT-DROP", "PASS":
		return "reject"
	default:
		return n
	}
}

func uniqueTag(base string, used map[string]bool) string {
	base = strings.TrimSpace(base)
	if base == "" {
		base = "node"
	}
	if !used[base] {
		return base
	}
	for i := 2; ; i++ {
		cand := fmt.Sprintf("%s #%d", base, i)
		if !used[cand] {
			return cand
		}
	}
}

func looksLikeURIList(s string) bool {
	for _, prefix := range []string{"vmess://", "vless://", "trojan://", "ss://", "hysteria2://", "hy2://", "tuic://"} {
		if strings.Contains(strings.ToLower(s), prefix) {
			return true
		}
	}
	return false
}

func decodeBase64Flexible(s string) (string, bool) {
	clean := strings.TrimSpace(s)
	clean = strings.ReplaceAll(clean, "\r", "")
	clean = strings.ReplaceAll(clean, "\n", "")
	encodings := []*base64.Encoding{
		base64.StdEncoding,
		base64.RawStdEncoding,
		base64.URLEncoding,
		base64.RawURLEncoding,
	}
	for _, enc := range encodings {
		if b, err := enc.DecodeString(clean); err == nil && len(b) > 0 {
			return string(b), true
		}
	}
	return "", false
}

func strVal(v any) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	default:
		return fmt.Sprint(t)
	}
}

func intVal(v any) int {
	switch t := v.(type) {
	case int:
		return t
	case int64:
		return int(t)
	case float64:
		return int(t)
	case string:
		n, _ := strconv.Atoi(strings.TrimSpace(t))
		return n
	default:
		return 0
	}
}

func boolVal(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return strings.EqualFold(t, "true") || t == "1"
	default:
		return false
	}
}

func strSlice(v any) []string {
	arr, ok := v.([]any)
	if !ok {
		if ss, ok2 := v.([]string); ok2 {
			return ss
		}
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		if s := strVal(item); s != "" {
			out = append(out, s)
		}
	}
	return out
}
