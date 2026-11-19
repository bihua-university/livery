package profile

import (
	"encoding/base64"
	"encoding/json"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/bihua-university/livery/internal/model"
)

func TestConvertClashYAML(t *testing.T) {
	clashYAML := `
proxies:
  - name: HK-SS
    type: ss
    server: 1.2.3.4
    port: 8388
    cipher: aes-256-gcm
    password: pass
  - name: JP-VLESS
    type: vless
    server: jp.example.com
    port: 443
    uuid: 11111111-1111-1111-1111-111111111111
    tls: true
    servername: jp.example.com
    network: ws
    ws-opts:
      path: /ws
      headers:
        Host: jp.example.com
    reality-opts:
      public-key: pub123
      short-id: abcd
proxy-groups:
  - name: Proxy
    type: select
    proxies:
      - Auto
      - HK-SS
      - JP-VLESS
      - DIRECT
  - name: Auto
    type: url-test
    proxies:
      - HK-SS
      - JP-VLESS
    url: http://www.gstatic.com/generate_204
    interval: 180
rules:
  - DOMAIN-SUFFIX,google.com,Proxy
  - IP-CIDR,10.0.0.0/8,DIRECT
  - MATCH,Proxy
`

	out, kind, err := ConvertToSingBox([]byte(clashYAML))
	if err != nil {
		t.Fatalf("ConvertToSingBox: %v", err)
	}
	if kind != "clash" {
		t.Fatalf("expected kind=clash, got %q", kind)
	}

	var cfg map[string]any
	if err := json.Unmarshal(out, &cfg); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	obs := cfg["outbounds"].([]any)
	if len(obs) != 5 { // Proxy, Auto, HK-SS, JP-VLESS, direct
		t.Fatalf("expected 5 outbounds, got %d", len(obs))
	}
	rules := ParseRulesFromConfig(out)
	if len(rules) < 4 {
		t.Fatalf("expected parsed rules >= 4, got %d", len(rules))
	}
	var hasTSDomain, hasTSCIDR bool
	for _, r := range rules {
		if r.Type == "domain_suffix" && r.Payload == "ts.net" && r.Proxy == "direct" {
			hasTSDomain = true
		}
		if r.Type == "ip_cidr" && r.Proxy == "direct" {
			hasTSCIDR = true
		}
	}
	if !hasTSDomain || !hasTSCIDR {
		t.Fatalf("expected Tailscale direct rules in converted sing-box config, got %+v", rules)
	}
	dnsObj, _ := cfg["dns"].(map[string]any)
	var hasLocal bool
	for _, srvRaw := range toSlice(dnsObj["servers"]) {
		if srv, ok := srvRaw.(map[string]any); ok && srv["type"] == "local" {
			hasLocal = true
		}
	}
	if !hasLocal {
		t.Fatalf("expected dns-local server in converted sing-box config, got %+v", dnsObj)
	}
}

func TestConvertBase64URIList(t *testing.T) {
	links := "vless://11111111-1111-1111-1111-111111111111@us.example.com:443?security=tls&type=ws&path=/v#US-01\n" +
		"trojan://secret@sg.example.com:443?sni=sg.example.com#SG-01\n" +
		"hysteria2://pass@uk.example.com:443?sni=uk.example.com#UK-HY2\n"
	encoded := base64.StdEncoding.EncodeToString([]byte(links))

	out, kind, err := ConvertToSingBox([]byte(encoded))
	if err != nil {
		t.Fatalf("ConvertToSingBox: %v", err)
	}
	if kind != "uri" {
		t.Fatalf("expected kind=uri, got %q", kind)
	}
	var cfg map[string]any
	if err := json.Unmarshal(out, &cfg); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	obs := cfg["outbounds"].([]any)
	if len(obs) != 6 { // proxy, auto, US-01, SG-01, UK-HY2, direct
		t.Fatalf("expected 6 outbounds, got %d", len(obs))
	}
}

func TestDetectAndNormalizeProfile(t *testing.T) {
	clashYAML := "mixed-port: 7890\nproxies:\n  - name: n1\n    type: ss\n    server: 1.1.1.1\n    port: 443\n    cipher: aes-128-gcm\n    password: p\n"
	out, format, ext, err := DetectAndNormalizeProfile([]byte(clashYAML), model.CoreMihomo)
	if err != nil {
		t.Fatalf("DetectAndNormalizeProfile yaml: %v", err)
	}
	if format != "mihomo" || ext != ".yaml" || len(out) == 0 {
		t.Fatalf("unexpected yaml normalization result: format=%q ext=%q", format, ext)
	}

	links := "trojan://secret@sg.example.com:443?sni=sg.example.com#SG-01\n"
	encoded := base64.StdEncoding.EncodeToString([]byte(links))
	mOut, mFmt, mExt, err := DetectAndNormalizeProfile([]byte(encoded), model.CoreMihomo)
	if err != nil {
		t.Fatalf("DetectAndNormalizeProfile uri->mihomo: %v", err)
	}
	if mFmt != "mihomo" || mExt != ".yaml" {
		t.Fatalf("expected mihomo/.yaml, got %q/%q", mFmt, mExt)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(mOut, &doc); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
	if len(toSlice(doc["proxies"])) != 1 {
		t.Fatalf("expected 1 proxy in mihomo yaml, got %v", doc["proxies"])
	}

	sOut, sFmt, sExt, err := DetectAndNormalizeProfile([]byte(encoded), model.CoreSingBox)
	if err != nil {
		t.Fatalf("DetectAndNormalizeProfile uri->singbox: %v", err)
	}
	if sFmt != "sing-box" || sExt != ".json" || len(sOut) == 0 {
		t.Fatalf("expected sing-box/.json, got %q/%q", sFmt, sExt)
	}
}

func TestConvertSingBoxToMihomo(t *testing.T) {
	sb := `{
	  "outbounds": [
	    { "type": "selector", "tag": "Proxy", "outbounds": ["US-01", "direct"] },
	    { "type": "vless", "tag": "US-01", "server": "us.example.com", "server_port": 443, "uuid": "11111111-1111-1111-1111-111111111111", "tls": { "enabled": true, "server_name": "us.example.com" } },
	    { "type": "direct", "tag": "direct" }
	  ],
	  "route": {
	    "rules": [
	      { "domain_suffix": ["google.com"], "outbound": "Proxy" },
	      { "rule_set": ["geosite-cn", "geoip-cn"], "outbound": "direct" },
	      { "ip_is_private": true, "outbound": "direct" }
	    ],
	    "final": "Proxy"
	  }
	}`
	out, err := ConvertSingBoxToMihomo([]byte(sb))
	if err != nil {
		t.Fatalf("ConvertSingBoxToMihomo: %v", err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(out, &doc); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
	dnsObj, _ := doc["dns"].(map[string]any)
	if dnsObj == nil || dnsObj["enable"] != true {
		t.Fatalf("expected converted mihomo yaml to preserve enabled dns block, got %+v", doc["dns"])
	}
	rules := ParseRulesFromConfig(out)
	if len(rules) < 5 {
		t.Fatalf("expected >= 5 parsed rules from mihomo yaml, got %d: %+v", len(rules), rules)
	}
	if rules[0].Type != "DOMAIN-SUFFIX" || rules[0].Payload != "google.com" || rules[0].Proxy != "Proxy" {
		t.Fatalf("unexpected first rule: %+v", rules[0])
	}
	if rules[1].Type != "GEOSITE" || rules[1].Payload != "cn" || rules[1].Proxy != "DIRECT" {
		t.Fatalf("expected GEOSITE,cn,DIRECT rule, got %+v", rules[1])
	}
	if rules[2].Type != "GEOIP" || rules[2].Payload != "CN" || rules[2].Proxy != "DIRECT" {
		t.Fatalf("expected GEOIP,CN,DIRECT rule, got %+v", rules[2])
	}
}
