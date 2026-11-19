//go:build darwin

package sysproxy

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

const (
	legacyDefaultBypass = "127.0.0.1,localhost,192.168.0.0/16,10.0.0.0/8,172.16.0.0/12,::1,*.local"
	defaultBypass       = "127.0.0.1,localhost,192.168.0.0/16,10.0.0.0/8,172.16.0.0/12,100.64.0.0/10,::1,*.ts.net,*.local"
	bypassSeparator     = ","
)

const networksetupBin = "/usr/sbin/networksetup"

func listNetworkServices() ([]string, error) {
	out, err := exec.Command(networksetupBin, "-listallnetworkservices").Output()
	if err != nil {
		return nil, err
	}
	return parseDarwinNetworkServices(string(out)), nil
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

	partial := false
	runCmd := func(c sysCommand) error {
		if err := exec.Command(c.Program, c.Args...).Run(); err != nil {
			if c.Optional {
				return nil
			}
			return err
		}
		partial = true
		return nil
	}

	var firstErr error
	for _, svc := range services {
		if enable {
			for _, cmd := range darwinManualCommands(svc, host, port, bypass) {
				if err := runCmd(cmd); err != nil {
					return partial, err
				}
			}
		} else {
			for _, cmd := range darwinStopCommands(svc) {
				if err := runCmd(cmd); err != nil && firstErr == nil {
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
	out, err := exec.Command(networksetupBin, "-getwebproxy", svc).Output()
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
	if autoOut, err := exec.Command(networksetupBin, "-getautoproxyurl", svc).Output(); err == nil {
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
	if bpOut, err := exec.Command(networksetupBin, "-getproxybypassdomains", svc).Output(); err == nil {
		text := strings.TrimSpace(string(bpOut))
		if text != "" && !strings.Contains(text, "There aren't any bypass domains") && !strings.EqualFold(text, "Empty") {
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
