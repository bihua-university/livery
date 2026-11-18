//go:build linux

package sysproxy

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

const (
	defaultBypass   = "localhost,127.0.0.1,192.168.0.0/16,10.0.0.0/8,172.16.0.0/12,::1"
	bypassSeparator = ","
)

func setSystemProxy(enable bool, _ string, host string, port int, bypass string) (bool, error) {
	if _, err := exec.LookPath("gsettings"); err != nil {
		return false, fmt.Errorf("gsettings not found: %w", err)
	}
	if !enable {
		err := exec.Command("gsettings", "set", "org.gnome.system.proxy", "mode", "none").Run()
		return err == nil, err
	}

	p := strconv.Itoa(port)
	partial := false
	for _, proto := range []string{"http", "https", "socks"} {
		schema := "org.gnome.system.proxy." + proto
		if err := exec.Command("gsettings", "set", schema, "host", host).Run(); err != nil {
			return partial, err
		}
		partial = true
		if err := exec.Command("gsettings", "set", schema, "port", p).Run(); err != nil {
			return partial, err
		}
	}

	var ignores []string
	for _, part := range ParseBypassList(bypass) {
		if !strings.EqualFold(part, "<local>") {
			ignores = append(ignores, fmt.Sprintf("%q", part))
		}
	}
	if len(ignores) > 0 {
		if err := exec.Command("gsettings", "set", "org.gnome.system.proxy", "ignore-hosts", "["+strings.Join(ignores, ", ")+"]").Run(); err != nil {
			return partial, err
		}
	}

	if err := exec.Command("gsettings", "set", "org.gnome.system.proxy", "mode", "manual").Run(); err != nil {
		return partial, err
	}
	return true, nil
}

func getSystemProxy() (Status, error) {
	out, err := exec.Command("gsettings", "get", "org.gnome.system.proxy", "mode").Output()
	if err != nil {
		return Status{}, err
	}
	mode := strings.Trim(strings.TrimSpace(string(out)), "'\"")
	if mode == "auto" {
		uOut, _ := exec.Command("gsettings", "get", "org.gnome.system.proxy", "autoconfig-url").Output()
		return Status{
			Enabled:       false,
			AutoConfigURL: strings.Trim(strings.TrimSpace(string(uOut)), "'\""),
		}, nil
	}
	if mode != "manual" {
		return Status{Enabled: false}, nil
	}
	hOut, _ := exec.Command("gsettings", "get", "org.gnome.system.proxy.http", "host").Output()
	pOut, _ := exec.Command("gsettings", "get", "org.gnome.system.proxy.http", "port").Output()
	iOut, _ := exec.Command("gsettings", "get", "org.gnome.system.proxy", "ignore-hosts").Output()
	host := strings.Trim(strings.TrimSpace(string(hOut)), "'\"")
	port := strings.TrimSpace(string(pOut))

	rawIgnore := strings.TrimSpace(string(iOut))
	rawIgnore = strings.TrimPrefix(rawIgnore, "@as ")
	rawIgnore = strings.Trim(rawIgnore, "[]")
	var items []string
	for _, part := range strings.Split(rawIgnore, ",") {
		part = strings.Trim(strings.TrimSpace(part), "'\"")
		if part != "" {
			items = append(items, part)
		}
	}

	return Status{
		Enabled: true,
		Server:  host + ":" + port,
		Bypass:  strings.Join(items, bypassSeparator),
	}, nil
}
