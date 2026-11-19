//go:build linux

package sysproxy

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	legacyDefaultBypass = "localhost,127.0.0.1,192.168.0.0/16,10.0.0.0/8,172.16.0.0/12,::1"
	defaultBypass       = "localhost,127.0.0.1,192.168.0.0/16,10.0.0.0/8,172.16.0.0/12,100.64.0.0/10,::1,*.ts.net"
	bypassSeparator     = ","
)

func linuxEnvironment() (home string, backend linuxBackend, tool string, err error) {
	home = strings.TrimSpace(os.Getenv("HOME"))
	if home == "" {
		var hErr error
		home, hErr = os.UserHomeDir()
		if hErr != nil || strings.TrimSpace(home) == "" {
			return "", backendGNOME, "", errors.New("HOME directory not set")
		}
	}
	desktop := os.Getenv("XDG_CURRENT_DESKTOP")
	b, t, ok := resolveLinuxBackend(desktop, func(name string) bool {
		_, err := exec.LookPath(name)
		return err == nil
	})
	if !ok {
		return "", backendGNOME, "", errors.New("no supported desktop proxy tool found (gsettings or kwriteconfig6/kwriteconfig5)")
	}
	return home, b, t, nil
}

func setSystemProxy(enable bool, _ string, host string, port int, bypass string) (bool, error) {
	home, backend, tool, err := linuxEnvironment()
	if err != nil {
		return false, err
	}
	var cmds []sysCommand
	if enable {
		cmds = linuxManualCommands(backend, tool, home, host, port, bypass)
	} else {
		cmds = linuxStopCommands(backend, tool, home)
	}

	partial := false
	for _, cmd := range cmds {
		if err := exec.Command(cmd.Program, cmd.Args...).Run(); err != nil {
			if cmd.Optional {
				continue
			}
			return partial, err
		}
		partial = true
	}
	return partial, nil
}

func getSystemProxy() (Status, error) {
	home, backend, _, err := linuxEnvironment()
	if err != nil {
		return Status{}, err
	}
	if backend == backendKDE {
		data, err := os.ReadFile(filepath.Join(home, ".config", "kioslaverc"))
		if err != nil {
			if os.IsNotExist(err) {
				return Status{Enabled: false}, nil
			}
			return Status{}, err
		}
		return parseKioslavercStatus(string(data)), nil
	}

	schema := linuxGSettingsSchema(backend)
	out, err := exec.Command("gsettings", "get", schema, "mode").Output()
	if err != nil {
		return Status{}, err
	}
	mode := strings.Trim(strings.TrimSpace(string(out)), "'\"")
	if mode == "auto" {
		uOut, _ := exec.Command("gsettings", "get", schema, "autoconfig-url").Output()
		return Status{
			Enabled:       false,
			AutoConfigURL: strings.Trim(strings.TrimSpace(string(uOut)), "'\""),
		}, nil
	}
	if mode != "manual" {
		return Status{Enabled: false}, nil
	}
	hOut, _ := exec.Command("gsettings", "get", schema+".http", "host").Output()
	pOut, _ := exec.Command("gsettings", "get", schema+".http", "port").Output()
	iOut, _ := exec.Command("gsettings", "get", schema, "ignore-hosts").Output()
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
