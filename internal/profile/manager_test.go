package profile

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/bihua-university/livery/internal/model"
)

func TestMihomoDNSModeOverride(t *testing.T) {
	for _, tun := range []bool{false, true} {
		settings := model.DefaultSettings()
		settings.TUNEnabled = tun
		for _, mode := range []string{"", "redir-host", "fake-ip"} {
			raw := []byte("proxies: []\nrules: [MATCH]\ndns:\n  enable: false\n  enhanced-mode: redir-host\n  nameserver: [192.0.2.53]\n  respect-rules: true\n")
			prof := model.Profile{Override: model.ProfileOverride{DNSMode: mode}}
			out, err := CompileMihomo(raw, prof, settings)
			if err != nil {
				t.Fatal(err)
			}
			var config struct {
				DNS struct {
					Enable       bool
					Mode         string `yaml:"enhanced-mode"`
					Nameserver   []string
					RespectRules bool `yaml:"respect-rules"`
				}
			}
			if err := yaml.Unmarshal(out, &config); err != nil {
				t.Fatal(err)
			}
			wantMode := mode
			if mode == "" {
				wantMode = "redir-host"
			}
			dns := config.DNS
			if dns.Mode != wantMode || dns.Enable != (tun || mode != "") || len(dns.Nameserver) != 1 || dns.Nameserver[0] != "192.0.2.53" || !dns.RespectRules {
				t.Fatalf("tun=%v mode=%q: %+v", tun, mode, dns)
			}
		}
	}
}

func TestParseClashInstallURL(t *testing.T) {
	target, name, ok := ParseClashInstallURL("CLASH://INSTALL-CONFIG/?url=https%3A%2F%2Fexample.org%2Fs%3Fk%3Da%252Bb%26v%3D1&name=My+Profile")
	if !ok || target != "https://example.org/s?k=a%2Bb&v=1" || name != "My Profile" {
		t.Fatalf("unexpected subscription: %q, %q, %v", target, name, ok)
	}
	for _, raw := range []string{
		"clash://unknown?url=https://example.org/",
		"clash://install-config",
		"clash://install-config?url=file:///tmp/config",
		"clash://install-config?url=https:///missing-host",
		"clash://install-config?url=https://one.example/&url=https://two.example/",
		"clash://install-config?url=%garbled",
	} {
		if _, _, ok := ParseClashInstallURL(raw); ok {
			t.Errorf("accepted invalid install link %q", raw)
		}
	}
}

func TestSingBoxProxyURLUsesProfileInbound(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime.json")
	for _, tc := range []struct{ raw, want string }{
		{`{"inbounds":[{"type":"tun"}]}`, ""},
		{`{"inbounds":[{"type":"tun"},{"type":"socks","listen":"::","listen_port":11999}]}`, "socks5://[::1]:11999"},
		{`{"inbounds":[{"type":"mixed","listen_port":12000,"users":[{"username":"test","password":"fixture"}]}]}`, "http://test:fixture@127.0.0.1:12000"},
	} {
		if err := os.WriteFile(path, []byte(tc.raw), 0o600); err != nil {
			t.Fatal(err)
		}
		got, err := SingBoxProxyURL(path)
		if tc.want == "" {
			if err == nil {
				t.Fatal("used unrelated inbound for TUN-only profile")
			}
		} else if err != nil || got.String() != tc.want {
			t.Fatalf("got %v, %v; want %s", got, err, tc.want)
		}
	}
}

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
	excludes := strSlice(tunObj["route-exclude-address"])
	if !containsStr(excludes, "100.64.0.0/10") || !containsStr(excludes, "fd7a:115c:a1e0::/48") {
		t.Fatalf("expected Tailscale CIDRs in tun.route-exclude-address, got %v", excludes)
	}

	dnsObj, _ := parsed["dns"].(map[string]any)
	if dnsObj == nil || !containsStr(strSlice(dnsObj["fake-ip-filter"]), "+.ts.net") {
		t.Fatalf("expected +.ts.net in dns.fake-ip-filter, got %+v", dnsObj)
	}
	policy, _ := dnsObj["nameserver-policy"].(map[string]any)
	if policy == nil || policy["+.ts.net"] != "system" {
		t.Fatalf("expected +.ts.net=system in dns.nameserver-policy, got %+v", policy)
	}

	rules := ParseRulesFromConfig(compiled)
	if len(rules) != 7 {
		t.Fatalf("expected 7 rules (1 prepended + 3 Tailscale direct + 1 existing + 1 appended + 1 MATCH), got %d: %+v", len(rules), rules)
	}
	if rules[0].Payload != "example.com" || rules[0].Proxy != "DIRECT" {
		t.Fatalf("expected prepended rule first, got %+v", rules[0])
	}
	if rules[1].Type != "DOMAIN-SUFFIX" || rules[1].Payload != "ts.net" || rules[1].Proxy != "DIRECT" {
		t.Fatalf("expected Tailscale domain rule second, got %+v", rules[1])
	}
	if rules[2].Type != "IP-CIDR" || rules[2].Payload != "100.64.0.0/10" || rules[2].Proxy != "DIRECT" {
		t.Fatalf("expected Tailscale IPv4 CIDR rule third, got %+v", rules[2])
	}
	if rules[5].Payload != "8.8.8.8/32" || rules[5].Proxy != "REJECT" {
		t.Fatalf("expected appended rule before MATCH, got %+v", rules[5])
	}
	if rules[6].Type != "MATCH" {
		t.Fatalf("expected MATCH last, got %+v", rules[6])
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

func TestCompileMihomoNoDuplicateTailscaleRulesAndLegacyBypass(t *testing.T) {
	settings := model.DefaultSettings()
	settings.CoreType = model.CoreMihomo
	settings.TUNEnabled = false

	compiled, err := CompileForCore([]byte(defaultMihomoStarterConfig), model.Profile{}, settings)
	if err != nil {
		t.Fatalf("CompileForCore: %v", err)
	}
	rules := ParseRulesFromConfig(compiled)
	tsCount := 0
	for _, r := range rules {
		if r.Type == "DOMAIN-SUFFIX" && r.Payload == "ts.net" {
			tsCount++
		}
	}
	if tsCount != 1 {
		t.Fatalf("expected exactly 1 ts.net rule without duplication, got %d: %+v", tsCount, rules)
	}

	dir := t.TempDir()
	legacyBypass := "localhost,127.0.0.1,192.168.0.0/16,10.0.0.0/8,172.16.0.0/12,::1"
	switch runtime.GOOS {
	case "windows":
		legacyBypass = "localhost;127.*;192.168.*;10.*;172.16.*;172.17.*;172.18.*;172.19.*;172.20.*;172.21.*;172.22.*;172.23.*;172.24.*;172.25.*;172.26.*;172.27.*;172.28.*;172.29.*;172.30.*;172.31.*;*.local;<local>"
	case "darwin":
		legacyBypass = "127.0.0.1,localhost,192.168.0.0/16,10.0.0.0/8,172.16.0.0/12,::1,*.local"
	}
	legacyJSON := `{"system_proxy_bypass":"` + legacyBypass + `"}`
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(legacyJSON), 0o644); err != nil {
		t.Fatalf("write legacy settings: %v", err)
	}
	mgr, err := NewManager(dir)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if got := mgr.Settings().SystemProxyBypass; !strings.Contains(got, "*.ts.net") {
		t.Fatalf("expected legacy default bypass to be upgraded with *.ts.net, got %q", got)
	}
}

func containsStr(list []string, target string) bool {
	for _, s := range list {
		if s == target {
			return true
		}
	}
	return false
}

func TestLegacySettingsInheritTUNBypassPrivateRouteTrue(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"mixed_port": 7890}`), 0o644); err != nil {
		t.Fatalf("write legacy settings: %v", err)
	}
	mgr, err := NewManager(dir)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if !mgr.Settings().TUNBypassPrivateRoute {
		t.Fatal("expected legacy settings.json without tun_bypass_private_route to inherit true")
	}
}

func TestCompileSingBoxTUNAlignmentAndTailscale(t *testing.T) {
	base := `{
	  "inbounds": [
	    {
	      "type": "tun",
	      "tag": "tun-in",
	      "route_exclude_address": ["203.0.113.0/24", "100.64.0.0/10"]
	    }
	  ],
	  "dns": {
	    "servers": [
	      { "type": "https", "tag": "dns-remote", "server": "1.1.1.1" }
	    ],
	    "rules": []
	  },
	  "outbounds": [
	    { "type": "direct", "tag": "direct-out" }
	  ],
	  "route": {
	    "rules": [
	      { "protocol": "dns", "action": "hijack-dns" }
	    ],
	    "final": "direct-out"
	  }
	}`

	settings := model.DefaultSettings()
	settings.CoreType = model.CoreSingBox
	settings.TUNEnabled = true
	settings.TUNBypassPrivateRoute = true

	compiled, err := Compile([]byte(base), model.Profile{}, settings)
	if err != nil {
		t.Fatalf("Compile(TUNBypassPrivateRoute=true): %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(compiled, &parsed); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	tunIn := findInboundByType(toSlice(parsed["inbounds"]), "tun")
	if tunIn == nil {
		t.Fatal("expected tun inbound")
	}
	excludes := strSlice(tunIn["route_exclude_address"])
	for _, want := range []string{"203.0.113.0/24", "10.0.0.0/8", "192.168.0.0/16", "fc00::/7", "100.64.0.0/10", "fd7a:115c:a1e0::/48"} {
		if !containsStr(excludes, want) {
			t.Fatalf("expected %q in route_exclude_address, got %v", want, excludes)
		}
	}
	routeObj := parsed["route"].(map[string]any)
	if routeObj["auto_detect_interface"] != true {
		t.Fatalf("expected route.auto_detect_interface=true, got %v", routeObj["auto_detect_interface"])
	}
	rules := toSlice(routeObj["rules"])
	if len(rules) < 3 {
		t.Fatalf("expected hijack-dns + tailscale rules, got %d", len(rules))
	}
	if !isSingBoxHijackDNSRule(rules[0]) {
		t.Fatalf("expected hijack-dns rule to remain before injected Tailscale rules, got %+v", rules[0])
	}

	dnsObj := parsed["dns"].(map[string]any)
	var hasLocalServer bool
	for _, srvRaw := range toSlice(dnsObj["servers"]) {
		if srv, ok := srvRaw.(map[string]any); ok && srv["type"] == "local" {
			hasLocalServer = true
		}
	}
	if !hasLocalServer {
		t.Fatalf("expected local dns server injected for Tailscale MagicDNS, got %+v", dnsObj["servers"])
	}
	var hasTSRule bool
	for _, rRaw := range toSlice(dnsObj["rules"]) {
		if r, ok := rRaw.(map[string]any); ok && ruleHasStringValue(r["domain_suffix"], "ts.net") {
			hasTSRule = true
		}
	}
	if !hasTSRule {
		t.Fatalf("expected ts.net dns rule, got %+v", dnsObj["rules"])
	}

	// Now verify TUNBypassPrivateRoute = false omits private CIDRs while keeping user & Tailscale excludes.
	settings.TUNBypassPrivateRoute = false
	compiledNoPriv, err := Compile([]byte(base), model.Profile{}, settings)
	if err != nil {
		t.Fatalf("Compile(TUNBypassPrivateRoute=false): %v", err)
	}
	var parsedNoPriv map[string]any
	if err := json.Unmarshal(compiledNoPriv, &parsedNoPriv); err != nil {
		t.Fatalf("Unmarshal noPriv: %v", err)
	}
	tunNoPriv := findInboundByType(toSlice(parsedNoPriv["inbounds"]), "tun")
	excludesNoPriv := strSlice(tunNoPriv["route_exclude_address"])
	if containsStr(excludesNoPriv, "192.168.0.0/16") {
		t.Fatalf("did not expect private CIDR 192.168.0.0/16 when TUNBypassPrivateRoute=false, got %v", excludesNoPriv)
	}
	if !containsStr(excludesNoPriv, "203.0.113.0/24") || !containsStr(excludesNoPriv, "100.64.0.0/10") || !containsStr(excludesNoPriv, "fd7a:115c:a1e0::/48") {
		t.Fatalf("expected pre-existing and Tailscale excludes preserved, got %v", excludesNoPriv)
	}
}

func TestCompileMihomoTUNPrivateRouteAndNCSI(t *testing.T) {
	settings := model.DefaultSettings()
	settings.CoreType = model.CoreMihomo
	settings.TUNEnabled = true
	settings.TUNBypassPrivateRoute = true

	raw := `proxies: []
tun:
  route-exclude-address:
    - 198.51.100.0/24
rules:
  - MATCH,DIRECT
`
	compiled, err := CompileMihomo([]byte(raw), model.Profile{}, settings)
	if err != nil {
		t.Fatalf("CompileMihomo(true): %v", err)
	}
	var parsed map[string]any
	if err := yaml.Unmarshal(compiled, &parsed); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
	tunObj := parsed["tun"].(map[string]any)
	excludes := strSlice(tunObj["route-exclude-address"])
	for _, want := range []string{"198.51.100.0/24", "192.168.0.0/16", "fc00::/7", "100.64.0.0/10", "fd7a:115c:a1e0::/48"} {
		if !containsStr(excludes, want) {
			t.Fatalf("expected %q in mihomo route-exclude-address, got %v", want, excludes)
		}
	}
	hijacks := strSlice(tunObj["dns-hijack"])
	if !containsStr(hijacks, "any:53") {
		t.Fatalf("expected default dns-hijack any:53, got %v", hijacks)
	}
	if runtime.GOOS == "darwin" && tunObj["dns-mode"] != "native" {
		t.Fatalf("expected darwin mihomo tun.dns-mode=native, got %v", tunObj["dns-mode"])
	}
	dnsObj := parsed["dns"].(map[string]any)
	filters := strSlice(dnsObj["fake-ip-filter"])
	for _, ncsi := range microsoftNCSIFakeIPFilters {
		if !containsStr(filters, ncsi) {
			t.Fatalf("expected NCSI domain %q in fake-ip-filter, got %v", ncsi, filters)
		}
	}

	settings.TUNBypassPrivateRoute = false
	compiledNoPriv, err := CompileMihomo([]byte(raw), model.Profile{}, settings)
	if err != nil {
		t.Fatalf("CompileMihomo(false): %v", err)
	}
	var parsedNoPriv map[string]any
	if err := yaml.Unmarshal(compiledNoPriv, &parsedNoPriv); err != nil {
		t.Fatalf("yaml.Unmarshal noPriv: %v", err)
	}
	excludesNoPriv := strSlice(parsedNoPriv["tun"].(map[string]any)["route-exclude-address"])
	if containsStr(excludesNoPriv, "192.168.0.0/16") {
		t.Fatalf("did not expect 192.168.0.0/16 when TUNBypassPrivateRoute=false, got %v", excludesNoPriv)
	}
	if !containsStr(excludesNoPriv, "198.51.100.0/24") || !containsStr(excludesNoPriv, "100.64.0.0/10") {
		t.Fatalf("expected custom and Tailscale CIDRs preserved, got %v", excludesNoPriv)
	}
}
