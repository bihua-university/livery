package app

import (
	"context"
	"net"
	"slices"
	"strings"
	"time"

	"github.com/bihua-university/livery/internal/model"
)

// NetworkChanged records reconnects even if DHCP assigns the old address.
func (a *App) NetworkChanged() { a.networkChanged.Store(true) }

func (a *App) FlushDNSCaches() {
	info := a.CoreInfo
	if a.FlushingDNS || info.Status != model.CoreRunning || info.CoreType != model.CoreMihomo {
		return
	}
	a.FlushingDNS = true
	go func() {
		err := a.flushSessionCaches(context.Background(), info)
		a.UpdateUI(func() {
			a.FlushingDNS = false
			if a.CoreInfo.StartedAt != info.StartedAt || a.CoreInfo.Status != model.CoreRunning {
				return
			}
			if err != nil {
				a.Notify(a.Tr("缓存清理失败：", "Cache flush failed: ") + err.Error())
			} else {
				a.Notify(a.Tr("已清理 DNS 和 Fake-IP 缓存", "DNS and Fake-IP caches cleared"))
			}
		})
	}()
}

func (a *App) flushSessionCaches(ctx context.Context, session model.CoreInfo) error {
	a.coreOpMu.Lock()
	defer a.coreOpMu.Unlock()
	info := a.CM.Info()
	if a.shuttingDown.Load() || info.Status != model.CoreRunning || info.CoreType != model.CoreMihomo || info.StartedAt != session.StartedAt {
		return context.Canceled
	}
	return a.apiClientFor(a.PM.Settings()).FlushDNSCaches(ctx)
}

type dnsNetworkWatch struct {
	session time.Time
	last    string
	pending bool
}

// Wait for a second, stable sample so one handoff produces one flush.
func (w *dnsNetworkWatch) changed(session time.Time, network string, reconnected bool) bool {
	if w.session != session {
		*w = dnsNetworkWatch{session: session, last: network}
		return false
	}
	if network != w.last || reconnected {
		w.last, w.pending = network, true
		return false
	}
	if network == "" || !w.pending {
		return false
	}
	w.pending = false
	return true
}

func (a *App) watchDNSNetwork(ctx context.Context) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	var watch dnsNetworkWatch
	for {
		info := a.CM.Info()
		reconnected := a.networkChanged.Swap(false)
		if info.Status != model.CoreRunning || info.CoreType != model.CoreMihomo || !a.PM.Settings().FlushDNSOnNetworkChange {
			watch = dnsNetworkWatch{}
		} else if network, ok := networkSignature(); ok && watch.changed(info.StartedAt, network, reconnected) {
			if err := a.flushSessionCaches(ctx, info); err != nil && ctx.Err() == nil && err != context.Canceled {
				a.LogApp("warning", "DNS cache flush after network change: "+err.Error())
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func networkSignature() (string, bool) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", false
	}
	var parts []string
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			return "", false
		}
		for _, addr := range addrs {
			parts = append(parts, iface.Name+" "+addr.String())
		}
	}
	if len(parts) == 0 {
		return "", true
	}
	slices.Sort(parts)
	return strings.Join(parts, "\n") + "\n" + networkRouteInfo(), true
}
