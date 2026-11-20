package app

import (
	"cmp"
	"crypto/rand"
	"net"
	"net/netip"
	"slices"
	"strings"
	"unicode"

	"github.com/egoist/mygo/ui"

	"github.com/bihua-university/livery/internal/model"
)

type connectionQuery struct {
	tab      int
	search   string
	sort     ui.SortOrder
	revision uint64
}

type connectionView struct {
	request *connectionQuery
	shown   connectionQuery
	rows    []model.Connection
}

// ReleaseConnectionView drops the sorted snapshot and invalidates an outstanding sort.
func (a *App) ReleaseConnectionView() {
	a.connView = connectionView{}
	a.ConnTableState.Key = nil
}

func (a *App) ConnectionRows() []model.Connection {
	src := a.Conns
	if a.ConnFilterTab == 1 {
		src = a.ClosedConns
	}
	q := connectionQuery{a.ConnFilterTab, strings.ToLower(strings.TrimSpace(a.ConnSearch)), a.ConnSort, a.connRevision}
	if len(src) == 0 || (q.search == "" && q.sort.Column == "") {
		a.ReleaseConnectionView()
		return src
	}
	v := &a.connView
	if v.request == nil || *v.request != q {
		v.request = &q
		// Active snapshots are replaced wholesale. Closed history is updated
		// in place, so take its small bounded copy on the UI thread.
		rows := src
		if q.tab == 1 {
			rows = slices.Clone(src)
		}
		go func() {
			if q.tab == 0 {
				rows = slices.Clone(rows)
			}
			rows = filterSortConnections(rows, q)
			a.UpdateUI(func() {
				if v.request == &q {
					v.rows, v.shown = rows, q
				}
			})
		}()
	}
	if v.shown.tab == q.tab && v.shown.search == q.search && v.rows != nil {
		return v.rows
	}
	if q.search == "" {
		return src
	}
	return nil
}

func filterSortConnections(rows []model.Connection, q connectionQuery) []model.Connection {
	if q.search != "" {
		rows = slices.DeleteFunc(rows, func(c model.Connection) bool {
			text := strings.Join([]string{c.Metadata.Host, c.Metadata.DestinationIP, c.Metadata.ProcessPath, c.Rule, c.RulePayload, strings.Join(c.Chains, " ")}, " ")
			return !strings.Contains(strings.ToLower(text), q.search)
		})
	}
	if q.sort.Column != "" {
		slices.SortStableFunc(rows, func(a, b model.Connection) int {
			if q.sort.Descending {
				a, b = b, a
			}
			switch q.sort.Column {
			case "host":
				return cmp.Or(strings.Compare(connectionHost(a), connectionHost(b)), strings.Compare(a.Metadata.DestinationPort, b.Metadata.DestinationPort))
			case "type":
				return cmp.Or(strings.Compare(a.Metadata.Network, b.Metadata.Network), strings.Compare(a.Metadata.Type, b.Metadata.Type))
			case "rule":
				return cmp.Or(strings.Compare(a.Rule, b.Rule), strings.Compare(a.RulePayload, b.RulePayload))
			case "chains":
				return slices.Compare(a.Chains, b.Chains)
			case "down":
				return cmp.Compare(a.Download, b.Download)
			case "up":
				return cmp.Compare(a.Upload, b.Upload)
			case "duration":
				return b.Start.Compare(a.Start)
			}
			return 0
		})
	}
	return rows
}

func connectionHost(c model.Connection) string {
	return cmp.Or(c.Metadata.Host, c.Metadata.DestinationIP)
}

func ConnectionAddress(c model.Connection) string {
	host := connectionHost(c)
	if c.Metadata.DestinationPort != "" {
		return net.JoinHostPort(host, c.Metadata.DestinationPort)
	}
	return host
}

// ConnectionRuleTarget matches exactly the destination, never an entire subnet.
func ConnectionRuleTarget(m model.ConnectionMetadata) (kind, pattern string) {
	host := strings.TrimSuffix(strings.TrimSpace(m.Host), ".")
	for _, value := range []string{host, m.DestinationIP} {
		if ip, err := netip.ParseAddr(strings.Trim(value, "[]")); err == nil {
			ip = ip.Unmap().WithZone("")
			return "ip_cidr", netip.PrefixFrom(ip, ip.BitLen()).String()
		}
		if value == host && host != "" && strings.IndexFunc(host, func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsDigit(r) && !strings.ContainsRune(".-_", r)
		}) < 0 {
			return "domain", strings.ToLower(host)
		}
	}
	return "", ""
}

func (a *App) AddCustomRule(rule model.CustomRule) {
	rule.ID = "cr-" + rand.Text()
	a.SavePreference(func(s *model.AppSettings) {
		if rule.FromConnection {
			// Replace a previous shortcut for the same destination and give
			// the latest choice priority over broader matching rules.
			s.CustomRules = slices.DeleteFunc(s.CustomRules, func(old model.CustomRule) bool {
				return old.FromConnection && old.MatchType == rule.MatchType && old.Pattern == rule.Pattern
			})
			s.CustomRules = slices.Insert(s.CustomRules, 0, rule)
		} else {
			s.CustomRules = append(s.CustomRules, rule)
		}
	}, func() {
		a.ShowRuleModal = false
		a.ReloadRulesFromActiveProfile()
		a.Notify(a.Tr("已添加规则：", "Added rule: ") + rule.Pattern + " → " + rule.Outbound)
		a.ApplyProfileChanges()
	})
}
