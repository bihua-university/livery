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
