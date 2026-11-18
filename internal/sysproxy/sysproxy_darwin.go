//go:build darwin

package sysproxy

import (
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

const (
	defaultBypass   = "127.0.0.1,localhost,192.168.0.0/16,10.0.0.0/8,172.16.0.0/12,::1,*.local"
	bypassSeparator = ","
)

func listNetworkServices() ([]string, error) {
	out, err := exec.Command("networksetup", "-listallnetworkservices").Output()
	if err != nil {
		return nil, err
	}
	var services []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "An asterisk") || strings.HasPrefix(line, "*") {
			continue
		}
		services = append(services, line)
	}
	return services, nil
}

func setSystemProxy(enable bool, _ string, host string, port int, bypass string) (bool, error) {
	services, err := listNetworkServices()
	if err != nil {
		return false, err
	}
	if len(services) == 0 {
		if !enable {
			return false, nil
		}
		return false, errors.New("no active network service")
	}

	p := strconv.Itoa(port)
	var bypassArgs []string
	for _, part := range ParseBypassList(bypass) {
		if !strings.EqualFold(part, "<local>") {
			bypassArgs = append(bypassArgs, part)
		}
	}

	partial := false
	run := func(args ...string) error {
		if err := exec.Command("networksetup", args...).Run(); err != nil {
			return err
		}
		partial = true
		return nil
	}

	var firstErr error
	for _, svc := range services {
		if enable {
			if err := run("-setautoproxystate", svc, "off"); err != nil {
				return partial, err
			}
			for _, cmd := range [][]string{
				{"-setwebproxy", svc, host, p},
				{"-setsecurewebproxy", svc, host, p},
				{"-setsocksfirewallproxy", svc, host, p},
				{"-setwebproxystate", svc, "on"},
				{"-setsecurewebproxystate", svc, "on"},
				{"-setsocksfirewallproxystate", svc, "on"},
			} {
				if err := run(cmd...); err != nil {
					return partial, err
				}
			}
			if len(bypassArgs) > 0 {
				args := append([]string{"-setproxybypassdomains", svc}, bypassArgs...)
				if err := run(args...); err != nil {
					return partial, err
				}
			}
		} else {
			for _, cmd := range [][]string{
				{"-setwebproxystate", svc, "off"},
				{"-setsecurewebproxystate", svc, "off"},
				{"-setsocksfirewallproxystate", svc, "off"},
			} {
				if err := run(cmd...); err != nil && firstErr == nil {
					firstErr = err
				}
			}
		}
	}
	return partial, firstErr
}

func getSystemProxy() (Status, error) {
	services, err := listNetworkServices()
	if err != nil || len(services) == 0 {
		return Status{}, err
	}
	svc := services[0]
	out, err := exec.Command("networksetup", "-getwebproxy", svc).Output()
	if err != nil {
		return Status{}, err
	}
	var st Status
	var host, port string
	for _, line := range strings.Split(string(out), "\n") {
		if k, v, ok := strings.Cut(line, ":"); ok {
			k, v = strings.TrimSpace(k), strings.TrimSpace(v)
			switch k {
			case "Enabled":
				st.Enabled = strings.EqualFold(v, "Yes")
			case "Server":
				host = v
			case "Port":
				port = v
			}
		}
	}
	if host != "" && port != "" && port != "0" {
		st.Server = fmt.Sprintf("%s:%s", host, port)
	}
	if autoOut, err := exec.Command("networksetup", "-getautoproxyurl", svc).Output(); err == nil {
		var autoOn bool
		var autoURL string
		for _, line := range strings.Split(string(autoOut), "\n") {
			if k, v, ok := strings.Cut(line, ":"); ok {
				k, v = strings.TrimSpace(k), strings.TrimSpace(v)
				switch k {
				case "Enabled":
					autoOn = strings.EqualFold(v, "Yes")
				case "URL":
					autoURL = v
				}
			}
		}
		if autoOn && autoURL != "" && !strings.EqualFold(autoURL, "(null)") {
			st.AutoConfigURL = autoURL
		}
	}
	if bpOut, err := exec.Command("networksetup", "-getproxybypassdomains", svc).Output(); err == nil {
		text := strings.TrimSpace(string(bpOut))
		if text != "" && !strings.Contains(text, "There aren't any bypass domains") {
			var items []string
			for _, line := range strings.Split(text, "\n") {
				line = strings.TrimSpace(line)
				if line != "" {
					items = append(items, line)
				}
			}
			st.Bypass = strings.Join(items, bypassSeparator)
		}
	}
	return st, nil
}
