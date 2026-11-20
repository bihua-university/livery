package profile

import (
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"os"
	"strconv"
)

// SingBoxProxyURL reads the actual inbound from the running core's compiled
// configuration. Profiles may preserve their own port or have only a TUN inbound.
func SingBoxProxyURL(path string) (*url.URL, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var config struct {
		Inbounds []struct {
			Type   string
			Listen string
			Port   int `json:"listen_port"`
			Users  []struct{ Username, Password string }
		}
	}
	if err := json.Unmarshal(raw, &config); err != nil {
		return nil, err
	}
	for _, in := range config.Inbounds {
		if in.Port <= 0 || in.Port > 65535 {
			continue
		}
		scheme := "http"
		switch in.Type {
		case "http", "mixed":
		case "socks":
			scheme = "socks5"
		default:
			continue
		}
		host := in.Listen
		switch host {
		case "", "0.0.0.0":
			host = "127.0.0.1"
		case "::":
			host = "::1"
		}
		endpoint := &url.URL{Scheme: scheme, Host: net.JoinHostPort(host, strconv.Itoa(in.Port))}
		if len(in.Users) > 0 {
			endpoint.User = url.UserPassword(in.Users[0].Username, in.Users[0].Password)
		}
		return endpoint, nil
	}
	return nil, errors.New("no HTTP, mixed or SOCKS inbound in the active sing-box profile")
}
