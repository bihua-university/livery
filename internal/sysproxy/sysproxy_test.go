package sysproxy

import "testing"

func TestNormalizeHost(t *testing.T) {
	for _, in := range []string{"", " 0.0.0.0 ", "::"} {
		if got := NormalizeHost(in); got != "127.0.0.1" {
			t.Fatalf("NormalizeHost(%q) = %q, want 127.0.0.1", in, got)
		}
	}
	if got := NormalizeHost("127.0.0.1"); got != "127.0.0.1" {
		t.Fatalf("NormalizeHost(127.0.0.1) = %q", got)
	}
}

func TestParseAndNormalizeBypass(t *testing.T) {
	got := ParseBypassList(" localhost ; 127.* , LOCALHOST \n 10.* ; ")
	if len(got) != 3 || got[0] != "localhost" || got[1] != "127.*" || got[2] != "10.*" {
		t.Fatalf("unexpected parsed bypass list: %#v", got)
	}

	if norm := NormalizeBypass("   "); norm != DefaultBypass() {
		t.Fatalf("empty NormalizeBypass = %q, want default %q", norm, DefaultBypass())
	}

	want := "localhost" + BypassSeparator() + "example.com"
	if norm := NormalizeBypass("localhost, example.com"); norm != want {
		t.Fatalf("NormalizeBypass = %q, want %q", norm, want)
	}
}

func TestFormatBypass(t *testing.T) {
	if got := FormatBypass(false, ""); got != DefaultBypass() {
		t.Fatalf("FormatBypass(false, empty) = %q, want default", got)
	}
	if got := FormatBypass(false, "example.com; foo.local"); got != "example.com"+BypassSeparator()+"foo.local" {
		t.Fatalf("FormatBypass(false, custom) = %q", got)
	}
	withDef := FormatBypass(true, "example.com")
	if !EqualBypass(withDef, DefaultBypass()+BypassSeparator()+"example.com") {
		t.Fatalf("FormatBypass(true, custom) = %q", withDef)
	}
}

func TestStatusMatchesTarget(t *testing.T) {
	targetBypass := DefaultBypass()
	st := Status{
		Enabled: true,
		Server:  "127.0.0.1:7890",
		Bypass:  targetBypass,
	}
	if !st.MatchesTarget("127.0.0.1", 7890, targetBypass) {
		t.Fatal("expected exact target to match")
	}
	if !st.MatchesTarget("0.0.0.0", 7890, targetBypass) {
		t.Fatal("expected wildcard host to normalize and match 127.0.0.1")
	}
	if st.MatchesTarget("127.0.0.1", 7891, targetBypass) {
		t.Fatal("expected different port not to match")
	}
	if st.MatchesTarget("127.0.0.1", 7890, "localhost") {
		t.Fatal("expected different bypass list not to match")
	}

	pacConflict := st
	pacConflict.AutoConfigURL = "http://127.0.0.1:9000/pac"
	if pacConflict.MatchesTarget("127.0.0.1", 7890, targetBypass) {
		t.Fatal("expected active AutoConfigURL to prevent MatchesTarget")
	}
}

func TestDefaultBypassIncludesTailscaleAndLegacyUpgrade(t *testing.T) {
	items := ParseBypassList(DefaultBypass())
	hasTS := false
	hasCGNAT := false
	for _, it := range items {
		if it == "*.ts.net" {
			hasTS = true
		}
		if it == "100.64.*" || it == "100.64.0.0/10" {
			hasCGNAT = true
		}
	}
	if !hasTS || !hasCGNAT {
		t.Fatalf("expected DefaultBypass to include *.ts.net and Tailscale CGNAT, got: %q", DefaultBypass())
	}
	if !IsLegacyDefaultBypass(legacyDefaultBypass) {
		t.Fatal("expected IsLegacyDefaultBypass to match legacyDefaultBypass")
	}
	if IsLegacyDefaultBypass("localhost, custom.internal") {
		t.Fatal("expected custom bypass not to be treated as legacy default")
	}
}

func TestSanitizeWinINetBypass(t *testing.T) {
	items := []string{
		" example.test/ ",
		"/",
		"::1",
		"fe80::1%eth0",
		"[fd00::1]",
		"Example.TEST",
		"[::1]",
		"<local>",
	}
	got := SanitizeWinINetBypass(items)
	want := "example.test;[::1];[fe80::1%eth0];[fd00::1];<local>"
	if got != want {
		t.Fatalf("SanitizeWinINetBypass = %q, want %q", got, want)
	}
}

func TestWithoutWinINetLocal(t *testing.T) {
	got := WithoutWinINetLocal([]string{"localhost", " <local> ", "<LOCAL>", "127.0.0.1", " "})
	if len(got) != 2 || got[0] != "localhost" || got[1] != "127.0.0.1" {
		t.Fatalf("WithoutWinINetLocal = %#v", got)
	}
}

func TestFormatGSettingsStringList(t *testing.T) {
	if got := formatGSettingsStringList(nil); got != "[]" {
		t.Fatalf("formatGSettingsStringList(nil) = %q, want []", got)
	}
	got := formatGSettingsStringList([]string{`a\b`, `it's`, `100.64.0.0/10`})
	want := `['a\\b', 'it\'s', '100.64.0.0/10']`
	if got != want {
		t.Fatalf("formatGSettingsStringList = %q, want %q", got, want)
	}
}

func TestResolveLinuxBackend(t *testing.T) {
	has := func(names ...string) func(string) bool {
		set := make(map[string]bool, len(names))
		for _, n := range names {
			set[n] = true
		}
		return func(name string) bool { return set[name] }
	}

	b, tool, ok := resolveLinuxBackend("KDE", has("gsettings", "kwriteconfig5", "kwriteconfig6"))
	if !ok || b != backendKDE || tool != "kwriteconfig6" {
		t.Fatalf("KDE with kwriteconfig6 = (%v, %q, %v)", b, tool, ok)
	}

	b, tool, ok = resolveLinuxBackend("KDE", has("gsettings", "kwriteconfig5"))
	if !ok || b != backendKDE || tool != "kwriteconfig5" {
		t.Fatalf("KDE fallback to kwriteconfig5 = (%v, %q, %v)", b, tool, ok)
	}

	if _, _, ok := resolveLinuxBackend("KDE", has("gsettings")); ok {
		t.Fatal("expected KDE without kwriteconfig6/kwriteconfig5 not to fall back to gsettings")
	}

	b, tool, ok = resolveLinuxBackend("ubuntu:GNOME", has("gsettings", "kwriteconfig6"))
	if !ok || b != backendGNOME || tool != "gsettings" {
		t.Fatalf("ubuntu:GNOME = (%v, %q, %v)", b, tool, ok)
	}

	b, tool, ok = resolveLinuxBackend("X-Cinnamon:MATE", has("gsettings"))
	if !ok || b != backendMATE || tool != "gsettings" {
		t.Fatalf("X-Cinnamon:MATE = (%v, %q, %v)", b, tool, ok)
	}

	b, tool, ok = resolveLinuxBackend("XFCE", has("kwriteconfig6"))
	if !ok || b != backendKDE || tool != "kwriteconfig6" {
		t.Fatalf("unknown desktop fallback to KDE = (%v, %q, %v)", b, tool, ok)
	}
}

func TestLinuxCommandsAndKioslavercStatus(t *testing.T) {
	cmds := linuxManualCommands(backendKDE, "kwriteconfig6", "/home/u", "127.0.0.1", 7890, "localhost,*.ts.net,<local>")
	if len(cmds) != 14 {
		t.Fatalf("expected 14 commands for KDE (5 writes + dbus + 8 optional gsettings), got %d", len(cmds))
	}
	if cmds[0].Program != "kwriteconfig6" || cmds[0].Args[len(cmds[0].Args)-1] != "localhost,*.ts.net" {
		t.Fatalf("unexpected first KDE command (NoProxyFor without <local>): %+v", cmds[0])
	}
	if cmds[4].Args[len(cmds[4].Args)-1] != "1" || cmds[4].Args[len(cmds[4].Args)-2] != "ProxyType" {
		t.Fatalf("expected ProxyType=1 as 5th KDE command, got %+v", cmds[4])
	}
	if cmds[5].Program != "dbus-send" || !cmds[5].Optional {
		t.Fatalf("expected optional dbus-send command, got %+v", cmds[5])
	}
	if !cmds[6].Optional || cmds[6].Program != "gsettings" || cmds[6].Args[1] != "org.gnome.system.proxy" {
		t.Fatalf("expected optional GNOME mirror command, got %+v", cmds[6])
	}

	stop := linuxStopCommands(backendKDE, "kwriteconfig5", "/home/u")
	if len(stop) != 3 || stop[0].Program != "kwriteconfig5" || stop[0].Args[len(stop[0].Args)-1] != "0" || !stop[1].Optional || !stop[2].Optional {
		t.Fatalf("unexpected KDE stop commands: %+v", stop)
	}

	mate := linuxManualCommands(backendMATE, "gsettings", "/home/u", "127.0.0.1", 7890, "localhost")
	if len(mate) != 8 || mate[0].Args[1] != "org.mate.system.proxy" || mate[len(mate)-1].Args[2] != "mode" || mate[len(mate)-1].Args[3] != "manual" {
		t.Fatalf("unexpected MATE manual commands: %+v", mate)
	}

	kioManual := `
[General]
Something=1

[Proxy Settings]
ProxyType=1
httpProxy=http://127.0.0.1:7890
NoProxyFor=localhost,*.ts.net
`
	st := parseKioslavercStatus(kioManual)
	if !st.Enabled || st.Server != "127.0.0.1:7890" || st.Bypass != "localhost,*.ts.net" {
		t.Fatalf("parseKioslavercStatus manual = %+v", st)
	}
	if !st.MatchesTarget("127.0.0.1", 7890, "localhost,*.ts.net") {
		t.Fatal("expected parsed KDE status to match target")
	}

	kioPAC := "[Proxy Settings]\nProxyType=2\nProxy Config Script=http://127.0.0.1:9000/pac\n"
	pacSt := parseKioslavercStatus(kioPAC)
	if pacSt.Enabled || pacSt.AutoConfigURL != "http://127.0.0.1:9000/pac" {
		t.Fatalf("parseKioslavercStatus PAC = %+v", pacSt)
	}

	kioOff := "[Proxy Settings]\nProxyType=0\nhttpProxy=http://127.0.0.1:7890\n"
	if offSt := parseKioslavercStatus(kioOff); offSt.Enabled {
		t.Fatalf("expected ProxyType=0 to be disabled, got %+v", offSt)
	}
}

func TestDarwinNetworkServicesAndCommands(t *testing.T) {
	services := parseDarwinNetworkServices("An asterisk (*) denotes that a network service is disabled.\nWi-Fi\n*Thunderbolt Bridge\nUSB 10/100/1000 LAN\n")
	if len(services) != 2 || services[0] != "Wi-Fi" || services[1] != "USB 10/100/1000 LAN" {
		t.Fatalf("parseDarwinNetworkServices = %#v", services)
	}

	cmds := darwinManualCommands("Wi-Fi", "127.0.0.1", 7890, "localhost,<local>,*.ts.net")
	if len(cmds) != 8 {
		t.Fatalf("expected 8 darwin manual commands, got %d", len(cmds))
	}
	if cmds[0].Args[0] != "-setautoproxystate" || cmds[0].Args[2] != "off" {
		t.Fatalf("expected autoproxy disabled first, got %+v", cmds[0])
	}
	if cmds[4].Args[0] != "-setproxybypassdomains" || len(cmds[4].Args) != 4 || cmds[4].Args[2] != "localhost" || cmds[4].Args[3] != "*.ts.net" {
		t.Fatalf("expected bypass domains without <local> before enabling proxy, got %+v", cmds[4])
	}
	if cmds[5].Args[0] != "-setwebproxystate" || cmds[5].Args[2] != "on" {
		t.Fatalf("expected webproxystate on after bypass domains, got %+v", cmds[5])
	}

	emptyBypass := darwinManualCommands("Wi-Fi", "127.0.0.1", 7890, "<local>")
	if len(emptyBypass[4].Args) != 3 || emptyBypass[4].Args[2] != "Empty" {
		t.Fatalf("expected Empty sentinel when only <local> provided, got %+v", emptyBypass[4])
	}

	stop := darwinStopCommands("Wi-Fi")
	if len(stop) != 5 || stop[4].Args[0] != "-setproxybypassdomains" || stop[4].Args[2] != "Empty" {
		t.Fatalf("unexpected darwin stop commands: %+v", stop)
	}
}

