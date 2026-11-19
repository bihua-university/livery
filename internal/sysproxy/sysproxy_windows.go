//go:build windows

package sysproxy

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"
)

const (
	legacyDefaultBypass = "localhost;127.*;192.168.*;10.*;172.16.*;172.17.*;172.18.*;172.19.*;172.20.*;172.21.*;172.22.*;172.23.*;172.24.*;172.25.*;172.26.*;172.27.*;172.28.*;172.29.*;172.30.*;172.31.*;*.local;<local>"
	defaultBypass       = "localhost;127.*;192.168.*;10.*;172.16.*;172.17.*;172.18.*;172.19.*;172.20.*;172.21.*;172.22.*;172.23.*;172.24.*;172.25.*;172.26.*;172.27.*;172.28.*;172.29.*;172.30.*;172.31.*;100.64.*;100.65.*;100.66.*;100.67.*;100.68.*;100.69.*;100.70.*;100.71.*;100.72.*;100.73.*;100.74.*;100.75.*;100.76.*;100.77.*;100.78.*;100.79.*;100.80.*;100.81.*;100.82.*;100.83.*;100.84.*;100.85.*;100.86.*;100.87.*;100.88.*;100.89.*;100.90.*;100.91.*;100.92.*;100.93.*;100.94.*;100.95.*;100.96.*;100.97.*;100.98.*;100.99.*;100.100.*;100.101.*;100.102.*;100.103.*;100.104.*;100.105.*;100.106.*;100.107.*;100.108.*;100.109.*;100.110.*;100.111.*;100.112.*;100.113.*;100.114.*;100.115.*;100.116.*;100.117.*;100.118.*;100.119.*;100.120.*;100.121.*;100.122.*;100.123.*;100.124.*;100.125.*;100.126.*;100.127.*;*.ts.net;*.local;<local>"
	bypassSeparator     = ";"

	internetOptionRefresh             = 37
	internetOptionSettingsChg         = 39
	internetOptionPerConnectionOption = 75

	internetPerConnFlags         = 1
	internetPerConnProxyServer   = 2
	internetPerConnProxyBypass   = 3
	internetPerConnAutoConfigURL = 4

	proxyTypeDirect       = 0x00000001
	proxyTypeProxy        = 0x00000002
	proxyTypeAutoProxyURL = 0x00000004

	rasErrorBufferTooSmall = 603
	rasMaxEntryName        = 256
	winMaxPath             = 260

	wmEndSession = 0x0016
)

type internetPerConnOptionW struct {
	dwOption uint32
	value    uint64
}

type internetPerConnOptionListW struct {
	dwSize        uint32
	pszConnection *uint16
	dwOptionCount uint32
	dwOptionError uint32
	pOptions      *internetPerConnOptionW
}

type rasEntryNameW struct {
	dwSize          uint32
	szEntryName     [rasMaxEntryName + 1]uint16
	dwFlags         uint32
	szPhonebookPath [winMaxPath + 1]uint16
}

type wndClassW struct {
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     uintptr
	hIcon         uintptr
	hCursor       uintptr
	hbrBackground uintptr
	lpszMenuName  *uint16
	lpszClassName *uint16
}

type winPoint struct {
	x int32
	y int32
}

type winMsg struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      winPoint
}

var (
	wininet              = syscall.NewLazyDLL("wininet.dll")
	internetSetOptionW   = wininet.NewProc("InternetSetOptionW")
	internetQueryOptionW = wininet.NewProc("InternetQueryOptionW")

	rasapi32        = syscall.NewLazyDLL("rasapi32.dll")
	rasEnumEntriesW = rasapi32.NewProc("RasEnumEntriesW")

	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	getModuleHandleW = kernel32.NewProc("GetModuleHandleW")
	globalFree       = kernel32.NewProc("GlobalFree")
	lstrlenW         = kernel32.NewProc("lstrlenW")
	lstrcpynW        = kernel32.NewProc("lstrcpynW")

	user32           = syscall.NewLazyDLL("user32.dll")
	registerClassW   = user32.NewProc("RegisterClassW")
	createWindowExW  = user32.NewProc("CreateWindowExW")
	getMessageW      = user32.NewProc("GetMessageW")
	translateMessage = user32.NewProc("TranslateMessage")
	dispatchMessageW = user32.NewProc("DispatchMessageW")
	defWindowProcW   = user32.NewProc("DefWindowProcW")

	winSettingsMu    sync.Mutex
	appliedProxy     atomic.Bool
	sessionWatchOnce sync.Once
)

func setSystemProxy(enable bool, server, _ string, _ int, bypass string) (bool, error) {
	if enable {
		watchSessionEnd()
	}

	winSettingsMu.Lock()
	defer winSettingsMu.Unlock()

	if !enable {
		err := stopWinINetLocked()
		return err != nil, err
	}

	sanitizedBypass := SanitizeWinINetBypass(ParseBypassList(bypass))
	serverUTF16, err := syscall.UTF16FromString(server)
	if err != nil {
		return false, err
	}
	bypassUTF16, err := syscall.UTF16FromString(sanitizedBypass)
	if err != nil {
		return false, err
	}

	options := [3]internetPerConnOptionW{
		{dwOption: internetPerConnFlags, value: proxyTypeDirect | proxyTypeProxy},
		{dwOption: internetPerConnProxyServer, value: uint64(uintptr(unsafe.Pointer(&serverUTF16[0])))},
		{dwOption: internetPerConnProxyBypass, value: uint64(uintptr(unsafe.Pointer(&bypassUTF16[0])))},
	}

	appliedProxy.Store(true)
	err = applyToConnections(options[:])
	runtime.KeepAlive(serverUTF16)
	runtime.KeepAlive(bypassUTF16)
	if err != nil {
		return true, err
	}
	return true, nil
}

func stopWinINetLocked() error {
	options := [1]internetPerConnOptionW{
		{dwOption: internetPerConnFlags, value: proxyTypeDirect},
	}
	err := applyToConnections(options[:])
	if err == nil {
		appliedProxy.Store(false)
	}
	return err
}

func applyToConnections(options []internetPerConnOptionW) error {
	var lanErr error
	forEachConnection(func(conn *uint16) error {
		err := setPerConnectionOptions(conn, options)
		if conn == nil {
			lanErr = err
		}
		return err
	})
	if lanErr != nil {
		return lanErr
	}
	return notifyWinINet()
}

func setPerConnectionOptions(conn *uint16, options []internetPerConnOptionW) error {
	list := internetPerConnOptionListW{
		dwSize:        uint32(unsafe.Sizeof(internetPerConnOptionListW{})),
		pszConnection: conn,
		dwOptionCount: uint32(len(options)),
		pOptions:      &options[0],
	}
	r0, _, callErr := internetSetOptionW.Call(
		0,
		internetOptionPerConnectionOption,
		uintptr(unsafe.Pointer(&list)),
		uintptr(list.dwSize),
	)
	if r0 == 0 {
		if callErr != nil && callErr != syscall.Errno(0) {
			return fmt.Errorf("InternetSetOptionW failed: %w", callErr)
		}
		return fmt.Errorf("InternetSetOptionW failed")
	}
	return nil
}

// forEachConnection invokes fn for the LAN settings (conn == nil) and every
// configured dial-up / VPN RAS entry, each of which stores independent WinINet proxy state.
func forEachConnection(fn func(conn *uint16) error) {
	_ = fn(nil)

	if rasEnumEntriesW.Find() != nil {
		return
	}
	var size, count uint32
	ret, _, _ := rasEnumEntriesW.Call(0, 0, 0, uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&count)))
	if ret != rasErrorBufferTooSmall || count == 0 {
		return
	}
	entrySize := uint32(unsafe.Sizeof(rasEntryNameW{}))
	needed := (size + entrySize - 1) / entrySize
	if uint32(count) > needed {
		needed = count
	}
	entries := make([]rasEntryNameW, needed)
	for i := range entries {
		entries[i].dwSize = entrySize
	}
	size = needed * entrySize
	ret, _, _ = rasEnumEntriesW.Call(0, 0, uintptr(unsafe.Pointer(&entries[0])), uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&count)))
	if ret != 0 {
		return
	}
	limit := int(count)
	if limit > len(entries) {
		limit = len(entries)
	}
	for i := range limit {
		_ = fn(&entries[i].szEntryName[0])
	}
}

func notifyWinINet() error {
	r0, _, err0 := internetSetOptionW.Call(0, internetOptionSettingsChg, 0, 0)
	r1, _, err1 := internetSetOptionW.Call(0, internetOptionRefresh, 0, 0)
	if r0 == 0 {
		if err0 != nil && err0 != syscall.Errno(0) {
			return fmt.Errorf("InternetSetOptionW(SETTINGS_CHANGED) failed: %w", err0)
		}
		return fmt.Errorf("InternetSetOptionW(SETTINGS_CHANGED) failed")
	}
	if r1 == 0 {
		if err1 != nil && err1 != syscall.Errno(0) {
			return fmt.Errorf("InternetSetOptionW(REFRESH) failed: %w", err1)
		}
		return fmt.Errorf("InternetSetOptionW(REFRESH) failed")
	}
	return nil
}

func getSystemProxy() (Status, error) {
	options := [4]internetPerConnOptionW{
		{dwOption: internetPerConnFlags},
		{dwOption: internetPerConnProxyServer},
		{dwOption: internetPerConnProxyBypass},
		{dwOption: internetPerConnAutoConfigURL},
	}
	size := uint32(unsafe.Sizeof(internetPerConnOptionListW{}))
	list := internetPerConnOptionListW{
		dwSize:        size,
		dwOptionCount: uint32(len(options)),
		pOptions:      &options[0],
	}
	r0, _, callErr := internetQueryOptionW.Call(
		0,
		internetOptionPerConnectionOption,
		uintptr(unsafe.Pointer(&list)),
		uintptr(unsafe.Pointer(&size)),
	)
	if r0 == 0 {
		if callErr != nil && callErr != syscall.Errno(0) {
			return Status{}, fmt.Errorf("InternetQueryOptionW failed: %w", callErr)
		}
		return Status{}, fmt.Errorf("InternetQueryOptionW failed")
	}

	flags := uint32(options[0].value)
	server := takeGlobalUTF16String(uintptr(options[1].value))
	bypass := takeGlobalUTF16String(uintptr(options[2].value))
	autoConfigURL := takeGlobalUTF16String(uintptr(options[3].value))

	st := Status{
		Enabled: (flags & proxyTypeProxy) != 0,
		Server:  server,
		Bypass:  bypass,
	}
	if (flags & proxyTypeAutoProxyURL) != 0 {
		st.AutoConfigURL = autoConfigURL
	}
	return st, nil
}

func takeGlobalUTF16String(ptr uintptr) string {
	if ptr == 0 {
		return ""
	}
	defer globalFree.Call(ptr)
	n, _, _ := lstrlenW.Call(ptr)
	if n == 0 {
		return ""
	}
	buf := make([]uint16, n+1)
	lstrcpynW.Call(uintptr(unsafe.Pointer(&buf[0])), ptr, n+1)
	return syscall.UTF16ToString(buf)
}

func watchSessionEnd() {
	sessionWatchOnce.Do(func() {
		go runSessionWindow()
	})
}

func runSessionWindow() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	instance, _, _ := getModuleHandleW.Call(0)
	className, err := syscall.UTF16PtrFromString("LiverySystemProxySession")
	if err != nil {
		return
	}
	wc := wndClassW{
		lpfnWndProc:   syscall.NewCallback(sessionWindowProc),
		hInstance:     instance,
		lpszClassName: className,
	}
	atom, _, _ := registerClassW.Call(uintptr(unsafe.Pointer(&wc)))
	if atom == 0 {
		return
	}
	hwnd, _, _ := createWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		0,
		0,
		0, 0, 0, 0,
		0, 0,
		instance,
		0,
	)
	if hwnd == 0 {
		return
	}
	var msg winMsg
	for {
		ret, _, _ := getMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(ret) <= 0 {
			break
		}
		translateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		dispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

func sessionWindowProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	if msg == wmEndSession && wParam != 0 && appliedProxy.Load() {
		winSettingsMu.Lock()
		_ = stopWinINetLocked()
		winSettingsMu.Unlock()
		return 0
	}
	ret, _, _ := defWindowProcW.Call(hwnd, msg, wParam, lParam)
	return ret
}

