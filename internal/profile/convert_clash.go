package profile

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/bihua-university/livery/internal/model"
)

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
				u = model.DefaultDelayTestURL
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
				"url":       model.DefaultDelayTestURL,
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
	finalTag := defaultFinal
	var converted []any

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
		converted = append(converted, entry)
	}

	tailscaleDirect := missingTailscaleSingBoxRules(converted, "direct")
	rules := make([]any, 0, 2+len(tailscaleDirect)+len(converted))
	rules = append(rules, map[string]any{"protocol": "dns", "action": "hijack-dns"})
	rules = append(rules, tailscaleDirect...)
	rules = append(rules, map[string]any{"ip_is_private": true, "outbound": "direct"})
	rules = append(rules, converted...)
	return rules, finalTag
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
