package profile

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/bihua-university/livery/internal/model"
)

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
