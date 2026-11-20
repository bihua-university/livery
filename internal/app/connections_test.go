package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/bihua-university/livery/internal/model"
)

func TestConnectionSort(t *testing.T) {
	a := model.Connection{ID: "a", Download: 1024, Upload: 2, Start: time.Unix(2, 0), Rule: "DOMAIN", Chains: []string{"z"}, Metadata: model.ConnectionMetadata{Host: "a.example", Network: "udp"}}
	b := model.Connection{ID: "b", Download: 99, Upload: 11, Start: time.Unix(10, 0), Rule: "MATCH", Chains: []string{"a"}, Metadata: model.ConnectionMetadata{Host: "b.example", Network: "tcp"}}
	for column, first := range map[string]string{"host": "a", "type": "b", "rule": "a", "chains": "b", "down": "b", "up": "a", "duration": "b"} {
		for _, desc := range []bool{false, true} {
			rows := filterSortConnections([]model.Connection{a, b}, connectionQuery{sort: ui.SortOrder{Column: column, Descending: desc}, search: "example"})
			if (rows[0].ID == first) == desc {
				t.Errorf("%s descending=%v first=%s", column, desc, rows[0].ID)
			}
		}
	}
	if rows := filterSortConnections([]model.Connection{a, b}, connectionQuery{search: "b.example"}); len(rows) != 1 || rows[0].ID != "b" {
		t.Fatalf("filtered rows = %v", rows)
	}
}

func TestConnectionRuleTarget(t *testing.T) {
	for _, tc := range []struct{ host, ip, kind, pattern string }{
		{"Example.COM.", "192.0.2.4", "domain", "example.com"},
		{"192.0.2.1", "", "ip_cidr", "192.0.2.1/32"},
		{"", "2001:db8::1", "ip_cidr", "2001:db8::1/128"},
		{"bad,host", "192.0.2.4", "ip_cidr", "192.0.2.4/32"},
		{"", "", "", ""},
	} {
		k, p := ConnectionRuleTarget(model.ConnectionMetadata{Host: tc.host, DestinationIP: tc.ip})
		if k != tc.kind || p != tc.pattern {
			t.Errorf("%q %q: got %s %s", tc.host, tc.ip, k, p)
		}
	}
}
