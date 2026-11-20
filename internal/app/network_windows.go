//go:build windows

package app

import (
	"fmt"
	"slices"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

func networkRouteInfo() string {
	size := uint32(15000)
	for {
		buf := make([]byte, size)
		head := (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0]))
		err := windows.GetAdaptersAddresses(windows.AF_UNSPEC, windows.GAA_FLAG_INCLUDE_GATEWAYS, 0, head, &size)
		if err == windows.ERROR_BUFFER_OVERFLOW {
			continue
		}
		if err != nil {
			return ""
		}
		var parts []string
		for a := head; a != nil; a = a.Next {
			if a.OperStatus != windows.IfOperStatusUp {
				continue
			}
			parts = append(parts, fmt.Sprintf("%d/%s/%d/%d", a.IfIndex, a.NetworkGuid.String(), a.Ipv4Metric, a.Ipv6Metric))
			for g := a.FirstGatewayAddress; g != nil; g = g.Next {
				parts = append(parts, fmt.Sprintf("%d gw %s", a.IfIndex, g.Address.IP()))
			}
			for d := a.FirstDnsServerAddress; d != nil; d = d.Next {
				parts = append(parts, fmt.Sprintf("%d dns %s", a.IfIndex, d.Address.IP()))
			}
		}
		slices.Sort(parts)
		return strings.Join(parts, "\n")
	}
}
