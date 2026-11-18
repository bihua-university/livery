//go:build windows

package sysproxy

import (
	"fmt"
	"syscall"
	"unsafe"
)

const (
	legacyDefaultBypass = "localhost;127.*;192.168.*;10.*;172.16.*;172.17.*;172.18.*;172.19.*;172.20.*;172.21.*;172.22.*;172.23.*;172.24.*;172.25.*;172.26.*;172.27.*;172.28.*;172.29.*;172.30.*;172.31.*;*.local;<local>"
	defaultBypass       = "localhost;127.*;192.168.*;10.*;172.16.*;172.17.*;172.18.*;172.19.*;172.20.*;172.21.*;172.22.*;172.23.*;172.24.*;172.25.*;172.26.*;172.27.*;172.28.*;172.29.*;172.30.*;172.31.*;100.64.*;100.65.*;100.66.*;100.67.*;100.68.*;100.69.*;100.70.*;100.71.*;100.72.*;100.73.*;100.74.*;100.75.*;100.76.*;100.77.*;100.78.*;100.79.*;100.80.*;100.81.*;100.82.*;100.83.*;100.84.*;100.85.*;100.86.*;100.87.*;100.88.*;100.89.*;100.90.*;100.91.*;100.92.*;100.93.*;100.94.*;100.95.*;100.96.*;100.97.*;100.98.*;100.99.*;100.100.*;100.101.*;100.102.*;100.103.*;100.104.*;100.105.*;100.106.*;100.107.*;100.108.*;100.109.*;100.110.*;100.111.*;100.112.*;100.113.*;100.114.*;100.115.*;100.116.*;100.117.*;100.118.*;100.119.*;100.120.*;100.121.*;100.122.*;100.123.*;100.124.*;100.125.*;100.126.*;100.127.*;*.ts.net;*.local;<local>"
	bypassSeparator     = ";"

	hkeyCurrentUser            = uintptr(0x80000001)
	keyRead                    = 0x20019
	keySetValue                = 0x0002
	regSZ                      = 1
	regDword                   = 4
	errorFileNotFound          = syscall.Errno(2)
	internetOptionSettingsChg  = 39
	internetOptionRefresh      = 37
	internetSettingsSubKeyPath = `Software\Microsoft\Windows\CurrentVersion\Internet Settings`
)

var (
	advapi32         = syscall.NewLazyDLL("advapi32.dll")
	regOpenKeyExW    = advapi32.NewProc("RegOpenKeyExW")
	regQueryValueExW = advapi32.NewProc("RegQueryValueExW")
	regSetValueExW   = advapi32.NewProc("RegSetValueExW")
	regDeleteValueW  = advapi32.NewProc("RegDeleteValueW")
	regCloseKey      = advapi32.NewProc("RegCloseKey")

	wininet            = syscall.NewLazyDLL("wininet.dll")
	internetSetOptionW = wininet.NewProc("InternetSetOptionW")
)

func setSystemProxy(enable bool, server, _ string, _ int, bypass string) (bool, error) {
	subKey, err := syscall.UTF16PtrFromString(internetSettingsSubKeyPath)
	if err != nil {
		return false, err
	}
	var hKey uintptr
	r0, _, _ := regOpenKeyExW.Call(hkeyCurrentUser, uintptr(unsafe.Pointer(subKey)), 0, keySetValue|keyRead, uintptr(unsafe.Pointer(&hKey)))
	if r0 != 0 {
		return false, fmt.Errorf("RegOpenKeyExW failed: %w", syscall.Errno(r0))
	}
	defer regCloseKey.Call(hKey)

	partial := false
	if enable {
		// Clear stale PAC URL before enabling global proxy so WinINet does not prioritize AutoConfigURL.
		if err := regDeleteValue(hKey, "AutoConfigURL"); err != nil {
			return false, err
		}
		if err := regSetSZ(hKey, "ProxyServer", server); err != nil {
			return partial, err
		}
		partial = true
		if err := regSetSZ(hKey, "ProxyOverride", bypass); err != nil {
			return partial, err
		}
		if err := regSetDword(hKey, "ProxyEnable", 1); err != nil {
			return partial, err
		}
	} else {
		if err := regSetDword(hKey, "ProxyEnable", 0); err != nil {
			return false, err
		}
		partial = true
	}

	notifyWinINet()
	return partial, nil
}

func notifyWinINet() {
	internetSetOptionW.Call(0, internetOptionSettingsChg, 0, 0)
	internetSetOptionW.Call(0, internetOptionRefresh, 0, 0)
}

func getSystemProxy() (Status, error) {
	subKey, err := syscall.UTF16PtrFromString(internetSettingsSubKeyPath)
	if err != nil {
		return Status{}, err
	}
	var hKey uintptr
	r0, _, _ := regOpenKeyExW.Call(hkeyCurrentUser, uintptr(unsafe.Pointer(subKey)), 0, keyRead, uintptr(unsafe.Pointer(&hKey)))
	if r0 != 0 {
		return Status{}, fmt.Errorf("RegOpenKeyExW failed: %w", syscall.Errno(r0))
	}
	defer regCloseKey.Call(hKey)

	enabled, err := regGetDword(hKey, "ProxyEnable")
	if err != nil {
		return Status{}, err
	}
	server, err := regGetSZ(hKey, "ProxyServer")
	if err != nil {
		return Status{}, err
	}
	bypass, err := regGetSZ(hKey, "ProxyOverride")
	if err != nil {
		return Status{}, err
	}
	autoConfigURL, err := regGetSZ(hKey, "AutoConfigURL")
	if err != nil {
		return Status{}, err
	}
	return Status{
		Enabled:       enabled != 0,
		Server:        server,
		Bypass:        bypass,
		AutoConfigURL: autoConfigURL,
	}, nil
}

func regDeleteValue(hKey uintptr, name string) error {
	nPtr, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return err
	}
	r0, _, _ := regDeleteValueW.Call(hKey, uintptr(unsafe.Pointer(nPtr)))
	if r0 != 0 && syscall.Errno(r0) != errorFileNotFound {
		return fmt.Errorf("RegDeleteValueW(%s) failed: %w", name, syscall.Errno(r0))
	}
	return nil
}

func regSetDword(hKey uintptr, name string, val uint32) error {
	nPtr, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return err
	}
	r0, _, _ := regSetValueExW.Call(hKey, uintptr(unsafe.Pointer(nPtr)), 0, regDword, uintptr(unsafe.Pointer(&val)), 4)
	if r0 != 0 {
		return fmt.Errorf("RegSetValueExW(%s) failed: %w", name, syscall.Errno(r0))
	}
	return nil
}

func regSetSZ(hKey uintptr, name, val string) error {
	nPtr, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return err
	}
	vSlice, err := syscall.UTF16FromString(val)
	if err != nil {
		return err
	}
	r0, _, _ := regSetValueExW.Call(hKey, uintptr(unsafe.Pointer(nPtr)), 0, regSZ, uintptr(unsafe.Pointer(&vSlice[0])), uintptr(len(vSlice)*2))
	if r0 != 0 {
		return fmt.Errorf("RegSetValueExW(%s) failed: %w", name, syscall.Errno(r0))
	}
	return nil
}

func regGetDword(hKey uintptr, name string) (uint32, error) {
	nPtr, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return 0, err
	}
	var typ, val, size uint32
	size = 4
	r0, _, _ := regQueryValueExW.Call(hKey, uintptr(unsafe.Pointer(nPtr)), 0, uintptr(unsafe.Pointer(&typ)), uintptr(unsafe.Pointer(&val)), uintptr(unsafe.Pointer(&size)))
	if r0 != 0 {
		if syscall.Errno(r0) == errorFileNotFound {
			return 0, nil
		}
		return 0, fmt.Errorf("RegQueryValueExW(%s) failed: %w", name, syscall.Errno(r0))
	}
	return val, nil
}

func regGetSZ(hKey uintptr, name string) (string, error) {
	nPtr, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return "", err
	}
	var typ, size uint32
	r0, _, _ := regQueryValueExW.Call(hKey, uintptr(unsafe.Pointer(nPtr)), 0, uintptr(unsafe.Pointer(&typ)), 0, uintptr(unsafe.Pointer(&size)))
	if r0 != 0 {
		if syscall.Errno(r0) == errorFileNotFound {
			return "", nil
		}
		return "", fmt.Errorf("RegQueryValueExW(%s) failed: %w", name, syscall.Errno(r0))
	}
	if size == 0 {
		return "", nil
	}
	buf := make([]uint16, size/2+1)
	r0, _, _ = regQueryValueExW.Call(hKey, uintptr(unsafe.Pointer(nPtr)), 0, uintptr(unsafe.Pointer(&typ)), uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)))
	if r0 != 0 {
		return "", fmt.Errorf("RegQueryValueExW(%s) failed: %w", name, syscall.Errno(r0))
	}
	return syscall.UTF16ToString(buf), nil
}
