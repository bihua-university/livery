//go:build !windows

package app

import (
	"net"
	"os"
	"strings"
)

func networkRouteInfo() string {
	resolvers, _ := os.ReadFile("/etc/resolv.conf")
	parts := []string{string(resolvers)}
	// A connected UDP socket reveals the preferred source without sending packets.
	for _, destination := range []string{"192.0.2.1:53", "[2001:db8::1]:53"} {
		if c, err := net.Dial("udp", destination); err == nil {
			parts = append(parts, c.LocalAddr().(*net.UDPAddr).IP.String())
			c.Close()
		}
	}
	return strings.Join(parts, "\n")
}
