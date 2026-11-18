//go:build !windows && !darwin && !linux

package sysproxy

import "errors"

const (
	legacyDefaultBypass = "localhost,127.0.0.1,192.168.0.0/16,10.0.0.0/8,172.16.0.0/12,::1"
	defaultBypass       = "localhost,127.0.0.1,192.168.0.0/16,10.0.0.0/8,172.16.0.0/12,100.64.0.0/10,::1,*.ts.net"
	bypassSeparator     = ","
)

func setSystemProxy(bool, string, string, int, string) (bool, error) {
	return false, errors.New("system proxy is not supported on this platform")
}

func getSystemProxy() (Status, error) {
	return Status{}, errors.New("system proxy is not supported on this platform")
}
