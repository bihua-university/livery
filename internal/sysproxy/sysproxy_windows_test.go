//go:build windows

package sysproxy

import (
	"testing"
	"unsafe"
)

func TestWindowsStructLayouts(t *testing.T) {
	is64 := unsafe.Sizeof(uintptr(0)) == 8
	wantOptSize := uintptr(12)
	wantListSize := uintptr(20)
	wantMsgSize := uintptr(28)
	if is64 {
		wantOptSize = 16
		wantListSize = 32
		wantMsgSize = 48
	}

	if got := unsafe.Sizeof(internetPerConnOptionW{}); got != wantOptSize {
		t.Fatalf("sizeof(internetPerConnOptionW) = %d, want %d", got, wantOptSize)
	}
	wantOptValueOffset := uintptr(4)
	if is64 {
		wantOptValueOffset = 8
	}
	if off := unsafe.Offsetof(internetPerConnOptionW{}.value); off != wantOptValueOffset {
		t.Fatalf("offsetof(internetPerConnOptionW.value) = %d, want %d", off, wantOptValueOffset)
	}
	if got := unsafe.Sizeof(internetPerConnOptionListW{}); got != wantListSize {
		t.Fatalf("sizeof(internetPerConnOptionListW) = %d, want %d", got, wantListSize)
	}
	if got := unsafe.Sizeof(rasEntryNameW{}); got != 1048 {
		t.Fatalf("sizeof(rasEntryNameW) = %d, want 1048", got)
	}
	if got := unsafe.Sizeof(winMsg{}); got != wantMsgSize {
		t.Fatalf("sizeof(winMsg) = %d, want %d", got, wantMsgSize)
	}
}

func TestWinINetStatusMatchesTargetSanitization(t *testing.T) {
	st := Status{
		Enabled: true,
		Server:  "127.0.0.1:7890",
		Bypass:  "example.test;[::1];<local>",
	}
	if !st.MatchesTarget("127.0.0.1", 7890, "example.test/;::1;<local>") {
		t.Fatal("expected MatchesTarget to account for WinINet trailing-slash trim and IPv6 bracketing")
	}
}
