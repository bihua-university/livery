package profile

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/bihua-university/livery/internal/model"
)

func TestCompileInjectsClashAPITUNAndCustomRules(t *testing.T) {
	settings := model.DefaultSettings()
	settings.MixedPort = 17890
	settings.AllowLAN = true
	settings.ClashAPIPort = 19090
	settings.ClashAPISecret = "topsecret"
	settings.TUNEnabled = true
	settings.ProxyMode = model.ModeGlobal
	settings.CustomRules = []model.CustomRule{
		{ID: "r1", Enabled: true, MatchType: "domain_suffix", Pattern: "example.com", Outbound: "direct", Prepend: true},
		{ID: "r2", Enabled: true, MatchType: "ip_cidr", Pattern: "8.8.8.8/32", Outbound: "block", Prepend: false},
	}

	prof := model.Profile{
		Override: model.ProfileOverride{
			OverrideInbounds: true,
			InjectClashAPI:   true,
			OverrideLog:      true,
		},
	}

	compiled, err := Compile([]byte(defaultStarterConfig), prof, settings)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(compiled, &parsed); err != nil {
		t.Fatalf("Unmarshal compiled: %v", err)
	}

	inbounds := parsed["inbounds"].([]any)
	if len(inbounds) != 2 {
		t.Fatalf("expected 2 inbounds (mixed + tun), got %d", len(inbounds))
	}
	mixed := inbounds[0].(map[string]any)
	if mixed["listen"] != "0.0.0.0" || int(mixed["listen_port"].(float64)) != 17890 {
		t.Fatalf("unexpected mixed inbound: %+v", mixed)
	}

	exp := parsed["experimental"].(map[string]any)
	clash := exp["clash_api"].(map[string]any)
	if clash["external_controller"] != "127.0.0.1:19090" || clash["secret"] != "topsecret" {
		t.Fatalf("unexpected clash_api: %+v", clash)
	}

	rules := ParseRulesFromConfig(compiled)
	if len(rules) < 4 {
		t.Fatalf("expected parsed rules >= 4, got %d", len(rules))
	}
	if rules[0].Type != "domain_suffix" || rules[0].Payload != "example.com" || rules[0].Proxy != "direct" {
		t.Fatalf("expected prepended custom rule first, got %+v", rules[0])
	}
}

func TestMigrateLegacyDNSFormat(t *testing.T) {
	legacy := `{
	  "dns": {
	    "servers": [
	      { "tag": "dns-direct", "address": "223.5.5.5", "detour": "direct" },
	      { "tag": "dns-remote", "address": "https://1.1.1.1/dns-query", "detour": "proxy" }
	    ],
	    "rules": [
	      { "outbound": "any", "server": "dns-direct" }
	    ]
	  },
	  "outbounds": [ { "type": "direct", "tag": "direct" } ],
	  "route": { "final": "direct" }
	}`
	compiled, err := Compile([]byte(legacy), model.Profile{}, model.DefaultSettings())
	if err != nil {
		t.Fatalf("Compile legacy: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(compiled, &parsed); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	dnsObj := parsed["dns"].(map[string]any)
	servers := dnsObj["servers"].([]any)
	if len(servers) != 2 {
		t.Fatalf("expected 2 migrated DNS servers, got %d", len(servers))
	}
	s0 := servers[0].(map[string]any)
	s1 := servers[1].(map[string]any)
	if s0["type"] != "udp" || s0["server"] != "223.5.5.5" {
		t.Fatalf("unexpected migrated server 0: %+v", s0)
	}
	if s1["type"] != "https" || s1["server"] != "1.1.1.1" || s1["path"] != "/dns-query" {
		t.Fatalf("unexpected migrated server 1: %+v", s1)
	}
	routeObj := parsed["route"].(map[string]any)
	if routeObj["default_domain_resolver"] != "dns-direct" {
		t.Fatalf("expected default_domain_resolver=dns-direct, got %v", routeObj["default_domain_resolver"])
	}
}

func TestManagerRemoteImportAndPersistence(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Subscription-Userinfo", "upload=1024; download=2048; total=10240; expire=1893456000")
		w.Header().Set("Content-Disposition", `attachment; filename="MySub.json"`)
		_, _ = w.Write([]byte(defaultStarterConfig))
	}))
	defer srv.Close()

	dir := t.TempDir()
	mgr, err := NewManager(dir)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if len(mgr.Profiles()) != 1 {
		t.Fatalf("expected 1 default profile, got %d", len(mgr.Profiles()))
	}

	p, err := mgr.ImportRemote(context.Background(), "", srv.URL+"/sub", "LiveryTest", false)
	if err != nil {
		t.Fatalf("ImportRemote: %v", err)
	}
	if p.Name != "MySub" {
		t.Fatalf("expected name from Content-Disposition 'MySub', got %q", p.Name)
	}
	if p.Usage.Upload != 1024 || p.Usage.Download != 2048 || p.Usage.Total != 10240 {
		t.Fatalf("unexpected usage: %+v", p.Usage)
	}
	if err := mgr.SetActive(p.ID); err != nil {
		t.Fatalf("SetActive: %v", err)
	}
	_, compiled, err := mgr.CompileActive()
	if err != nil || len(compiled) == 0 {
		t.Fatalf("CompileActive failed: %v", err)
	}
}

func TestDefaultCoreTypeIsMihomo(t *testing.T) {
	def := model.DefaultSettings()
	if def.CoreType != model.CoreMihomo {
		t.Fatalf("expected default core type mihomo, got %q", def.CoreType)
	}

	dir := t.TempDir()
	// Simulate legacy settings.json without core_type
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"mixed_port": 7890}`), 0o644); err != nil {
		t.Fatalf("write legacy settings: %v", err)
	}
	mgr, err := NewManager(dir)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if mgr.Settings().CoreType != model.CoreMihomo {
		t.Fatalf("expected legacy settings to default to mihomo, got %q", mgr.Settings().CoreType)
	}
	if !strings.HasSuffix(mgr.RuntimeConfigPath(), "runtime.yaml") {
		t.Fatalf("expected runtime.yaml in mihomo mode, got %q", mgr.RuntimeConfigPath())
	}
	profs := mgr.Profiles()
	if len(profs) != 1 || filepath.Ext(profs[0].FileName) != ".yaml" {
		t.Fatalf("expected default profile to be .yaml, got %+v", profs)
	}
}

func TestCompileMihomoInjectsControllerTUNAndRules(t *testing.T) {
	settings := model.DefaultSettings()
	settings.CoreType = model.CoreMihomo
	settings.MixedPort = 17890
	settings.AllowLAN = true
	settings.ClashAPIPort = 19090
	settings.ClashAPISecret = "topsecret"
	settings.TUNEnabled = true
	settings.ProxyMode = model.ModeGlobal
	settings.LogLevel = "warn"
	settings.CustomRules = []model.CustomRule{
		{ID: "r1", Enabled: true, MatchType: "domain_suffix", Pattern: "example.com", Outbound: "direct", Prepend: true},
		{ID: "r2", Enabled: true, MatchType: "ip_cidr", Pattern: "8.8.8.8/32", Outbound: "block", Prepend: false},
	}

	raw := `port: 7890
socks-port: 7891
proxies: []
proxy-groups:
  - name: Proxy
    type: select
    proxies:
      - DIRECT
rules:
  - DOMAIN-SUFFIX,openai.com,Proxy
  - MATCH,Proxy
`
	prof := model.Profile{
		Override: model.ProfileOverride{
			OverrideInbounds: true,
			InjectClashAPI:   true,
			OverrideLog:      true,
		},
	}

	compiled, err := CompileForCore([]byte(raw), prof, settings)
	if err != nil {
		t.Fatalf("CompileForCore: %v", err)
	}

	var parsed map[string]any
	if err := yaml.Unmarshal(compiled, &parsed); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
	if parsed["external-controller"] != "127.0.0.1:19090" || parsed["secret"] != "topsecret" {
		t.Fatalf("unexpected controller/secret: %v / %v", parsed["external-controller"], parsed["secret"])
	}
	if parsed["mode"] != "global" || parsed["log-level"] != "warning" {
		t.Fatalf("unexpected mode/log-level: %v / %v", parsed["mode"], parsed["log-level"])
	}
	if intVal(parsed["mixed-port"]) != 17890 || parsed["allow-lan"] != true {
		t.Fatalf("unexpected mixed-port/allow-lan: %v / %v", parsed["mixed-port"], parsed["allow-lan"])
	}
	if _, hasPort := parsed["port"]; hasPort {
		t.Fatalf("expected legacy port to be removed when OverrideInbounds is true")
	}
	tunObj, _ := parsed["tun"].(map[string]any)
	if tunObj == nil || tunObj["enable"] != true {
		t.Fatalf("expected tun.enable=true, got %+v", tunObj)
	}

	rules := ParseRulesFromConfig(compiled)
	if len(rules) != 4 {
		t.Fatalf("expected 4 rules, got %d: %+v", len(rules), rules)
	}
	if rules[0].Payload != "example.com" || rules[0].Proxy != "DIRECT" {
		t.Fatalf("expected prepended rule first, got %+v", rules[0])
	}
	if rules[2].Payload != "8.8.8.8/32" || rules[2].Proxy != "REJECT" {
		t.Fatalf("expected appended rule before MATCH, got %+v", rules[2])
	}
	if rules[3].Type != "MATCH" {
		t.Fatalf("expected MATCH last, got %+v", rules[3])
	}
}

func TestCompileForCoreCrossModeSwitching(t *testing.T) {
	clashYAML := `proxies:
  - name: HK-SS
    type: ss
    server: 1.2.3.4
    port: 8388
    cipher: aes-256-gcm
    password: pass
proxy-groups:
  - name: Proxy
    type: select
    proxies:
      - HK-SS
rules:
  - MATCH,Proxy
`
	sbSettings := model.DefaultSettings()
	sbSettings.CoreType = model.CoreSingBox
	sbOut, err := CompileForCore([]byte(clashYAML), model.Profile{Override: model.ProfileOverride{OverrideInbounds: true, InjectClashAPI: true}}, sbSettings)
	if err != nil {
		t.Fatalf("CompileForCore YAML -> sing-box: %v", err)
	}
	var sbParsed map[string]any
	if err := json.Unmarshal(sbOut, &sbParsed); err != nil {
		t.Fatalf("expected valid sing-box JSON: %v", err)
	}

	mhSettings := model.DefaultSettings()
	mhSettings.CoreType = model.CoreMihomo
	mhOut, err := CompileForCore([]byte(defaultStarterConfig), model.Profile{Override: model.ProfileOverride{OverrideInbounds: true, InjectClashAPI: true}}, mhSettings)
	if err != nil {
		t.Fatalf("CompileForCore sing-box -> mihomo: %v", err)
	}
	var mhParsed map[string]any
	if err := yaml.Unmarshal(mhOut, &mhParsed); err != nil {
		t.Fatalf("expected valid mihomo YAML: %v", err)
	}
}

func TestCompileMihomoInjectsGeoSettings(t *testing.T) {
	settings := model.DefaultSettings()
	settings.CoreType = model.CoreMihomo
	settings.GeoDataMode = true
	settings.GeoAutoUpdate = true
	settings.GeoUpdateIntervalH = 12
	settings.GeoIPURL = "https://mirror.example.com/geoip.dat"
	settings.GeoSiteURL = "https://mirror.example.com/geosite.dat"
	settings.MMDBURL = "https://mirror.example.com/geoip.metadb"
	settings.ASNURL = "https://mirror.example.com/GeoLite2-ASN.mmdb"

	compiled, err := CompileForCore([]byte(defaultMihomoStarterConfig), model.Profile{}, settings)
	if err != nil {
		t.Fatalf("CompileForCore: %v", err)
	}
	var parsed map[string]any
	if err := yaml.Unmarshal(compiled, &parsed); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
	if parsed["geodata-mode"] != true {
		t.Fatalf("expected geodata-mode=true, got %v", parsed["geodata-mode"])
	}
	if parsed["geo-auto-update"] != true || intVal(parsed["geo-update-interval"]) != 12 {
		t.Fatalf("unexpected geo-auto-update settings: %v / %v", parsed["geo-auto-update"], parsed["geo-update-interval"])
	}
	geox, _ := parsed["geox-url"].(map[string]any)
	if geox == nil || geox["geoip"] != settings.GeoIPURL || geox["geosite"] != settings.GeoSiteURL || geox["mmdb"] != settings.MMDBURL || geox["asn"] != settings.ASNURL {
		t.Fatalf("unexpected geox-url: %+v", geox)
	}
}
