package core

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bihua-university/livery/internal/model"
)

func TestMain(m *testing.M) {
	if os.Getenv("LIVERY_TEST_HELPER") == "1" {
		runHelperCore()
		return
	}
	os.Exit(m.Run())
}

func runHelperCore() {
	if len(os.Args) < 2 {
		os.Exit(2)
	}
	switch os.Args[1] {
	case "version":
		fmt.Println("sing-box version 1.14.4")
		os.Exit(0)
	case "check":
		os.Exit(0)
	case "run":
		modeFile := os.Getenv("LIVERY_TEST_MODE_FILE")
		mode := ""
		if modeFile != "" {
			if data, err := os.ReadFile(modeFile); err == nil {
				mode = strings.TrimSpace(string(data))
			}
		}
		if mode == "fail-bind" {
			fmt.Fprintln(os.Stderr, "FATAL[0000] start service: start inbound: listen tcp 127.0.0.1:7890: bind: address already in use")
			os.Exit(1)
		}
		fmt.Fprintln(os.Stdout, "INFO[0000] sing-box started")
		time.Sleep(10 * time.Second)
		os.Exit(0)
	default:
		os.Exit(2)
	}
}

func TestParseVersionOutput(t *testing.T) {
	mihomoOut := "Mihomo Meta v1.19.3 windows amd64 with go1.24.0 Tue Mar 18 00:00:00 UTC 2025\n"
	if got := parseVersionOutput(model.CoreMihomo, mihomoOut); got != "1.19.3" {
		t.Fatalf("expected mihomo version 1.19.3, got %q", got)
	}

	sbOut := "sing-box version 1.11.4\nEnvironment: go1.23.6\n"
	if got := parseVersionOutput(model.CoreSingBox, sbOut); got != "1.11.4" {
		t.Fatalf("expected sing-box version 1.11.4, got %q", got)
	}
}

func TestManagerCoreTypeDefaultsToMihomoAndSwitches(t *testing.T) {
	dir := t.TempDir()
	mgr := NewManager(dir, dir, nil, nil)
	if mgr.Info().CoreType != model.CoreMihomo {
		t.Fatalf("expected initial CoreType mihomo, got %q", mgr.Info().CoreType)
	}

	info := mgr.SetCoreType(model.CoreSingBox, "")
	if info.CoreType != model.CoreSingBox {
		t.Fatalf("expected CoreType sing-box after switch, got %q", info.CoreType)
	}
}

func TestSelectMihomoAsset(t *testing.T) {
	ext := ".gz"
	if runtime.GOOS == "windows" {
		ext = ".zip"
	}
	prefix := fmt.Sprintf("mihomo-%s-%s-", runtime.GOOS, runtime.GOARCH)
	assets := []releaseAsset{
		{Name: prefix + "v1-v1.19.3" + ext, BrowserDownloadURL: "https://example.com/v1"},
		{Name: prefix + "v1.19.3" + ext, BrowserDownloadURL: "https://example.com/base"},
		{Name: prefix + "compatible-v1.19.3" + ext, BrowserDownloadURL: "https://example.com/compat"},
	}
	url, name := selectMihomoAsset(assets, "v1.19.3")
	if url != "https://example.com/base" || name != prefix+"v1.19.3"+ext {
		t.Fatalf("unexpected asset selected: %q (%q)", name, url)
	}
}

func TestManagerStopResetsExternalAndStoppedState(t *testing.T) {
	dir := t.TempDir()
	var states []model.CoreStatus
	mgr := NewManager(dir, dir, func(info model.CoreInfo) {
		states = append(states, info.Status)
	}, nil)

	mgr.AttachExternal("1.19.3", "127.0.0.1:9090")
	if info := mgr.Info(); info.Status != model.CoreRunning || !info.External {
		t.Fatalf("expected attached running external core, got %+v", info)
	}

	mgr.Stop()
	if info := mgr.Info(); info.Status != model.CoreStopped || info.External {
		t.Fatalf("expected stopped non-external core immediately after Stop, got %+v", info)
	}
	if len(states) == 0 || states[len(states)-1] != model.CoreStopped {
		t.Fatalf("expected final emitted state CoreStopped, got %v", states)
	}
}

func TestManagerUnexpectedExitAndRestartDoesNotDeadlock(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}

	dir := t.TempDir()
	modeFile := filepath.Join(dir, "mode.txt")
	if err := os.WriteFile(modeFile, []byte("fail-bind"), 0o644); err != nil {
		t.Fatalf("write mode file: %v", err)
	}
	cfgFile := filepath.Join(dir, "config.json")
	if err := os.WriteFile(cfgFile, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	t.Setenv("LIVERY_TEST_HELPER", "1")
	t.Setenv("LIVERY_TEST_MODE_FILE", modeFile)

	var (
		mu       sync.Mutex
		logs     []model.LogEntry
		errState = make(chan model.CoreInfo, 4)
	)
	mgr := NewManager(dir, dir, func(info model.CoreInfo) {
		if info.Status == model.CoreError {
			select {
			case errState <- info:
			default:
			}
		}
	}, func(entry model.LogEntry) {
		mu.Lock()
		logs = append(logs, entry)
		mu.Unlock()
	})
	mgr.SetCoreType(model.CoreSingBox, exe)

	if err := mgr.Start(cfgFile); err != nil {
		t.Fatalf("Start failed before process spawn: %v", err)
	}

	var gotErr model.CoreInfo
	select {
	case gotErr = <-errState:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for CoreError (possible Manager mutex deadlock)")
	}

	if !strings.Contains(gotErr.LastError, "bind: address already in use") {
		t.Fatalf("expected LastError to contain port conflict detail, got %q", gotErr.LastError)
	}

	// Verify Manager methods and subsequent restart return promptly without hanging.
	if err := os.WriteFile(modeFile, []byte("run"), 0o644); err != nil {
		t.Fatalf("update mode file: %v", err)
	}

	restarted := make(chan error, 1)
	go func() {
		mgr.Stop()
		restarted <- mgr.Start(cfgFile)
	}()

	select {
	case err := <-restarted:
		if err != nil {
			t.Fatalf("restart Start after port conflict failed: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out restarting Manager after port-conflict failure")
	}

	stopped := make(chan struct{})
	go func() {
		mgr.Stop()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out stopping restarted core")
	}

	mu.Lock()
	defer mu.Unlock()
	foundExitLog := false
	for _, entry := range logs {
		if entry.Level == "error" && strings.Contains(entry.Message, "exited unexpectedly") {
			foundExitLog = true
			break
		}
	}
	if !foundExitLog {
		t.Fatalf("expected unexpected exit log entry to be emitted, got logs: %+v", logs)
	}
}

func TestResolveCoreDownloadTargetAndDirResolution(t *testing.T) {
	defaultBin := filepath.Join(t.TempDir(), "bin")
	defaultName := defaultCoreBinaryName(model.CoreSingBox)

	// 1. Blank uses defaultBin + defaultName.
	dir, name := ResolveCoreDownloadTarget(model.CoreSingBox, defaultBin, "")
	if dir != defaultBin || name != defaultName {
		t.Fatalf("blank target = (%q, %q), want (%q, %q)", dir, name, defaultBin, defaultName)
	}

	// 2. Existing directory uses that directory + defaultName.
	customDir := filepath.Join(t.TempDir(), "custom-cores")
	if err := os.MkdirAll(customDir, 0o755); err != nil {
		t.Fatalf("mkdir customDir: %v", err)
	}
	dir, name = ResolveCoreDownloadTarget(model.CoreSingBox, defaultBin, customDir)
	if dir != customDir || name != defaultName {
		t.Fatalf("existing dir target = (%q, %q), want (%q, %q)", dir, name, customDir, defaultName)
	}

	// 3. Explicit file path preserves directory and custom filename.
	customFile := filepath.Join(customDir, "my-sing-box.exe")
	dir, name = ResolveCoreDownloadTarget(model.CoreSingBox, defaultBin, customFile)
	if dir != customDir || name != "my-sing-box.exe" {
		t.Fatalf("file target = (%q, %q), want (%q, %q)", dir, name, customDir, "my-sing-box.exe")
	}

	// 4. resolveBinary finds binary inside custom directory and falls back to default binDir.
	workDir := t.TempDir()
	mgr := NewManager(defaultBin, workDir, nil, nil)
	binInCustomDir := filepath.Join(customDir, defaultName)
	if err := os.WriteFile(binInCustomDir, []byte("stub"), 0o755); err != nil {
		t.Fatalf("write custom dir bin: %v", err)
	}
	gotPath, ok := mgr.resolveBinary(model.CoreSingBox, customDir)
	if !ok || gotPath != binInCustomDir {
		t.Fatalf("resolveBinary(customDir) = (%q, %v), want (%q, true)", gotPath, ok, binInCustomDir)
	}

	if err := os.MkdirAll(defaultBin, 0o755); err != nil {
		t.Fatalf("mkdir defaultBin: %v", err)
	}
	binInDefault := filepath.Join(defaultBin, defaultName)
	if err := os.WriteFile(binInDefault, []byte("stub"), 0o755); err != nil {
		t.Fatalf("write default bin: %v", err)
	}
	emptyDir := t.TempDir()
	gotFallback, ok := mgr.resolveBinary(model.CoreSingBox, emptyDir)
	if !ok || gotFallback != binInDefault {
		t.Fatalf("resolveBinary fallback to defaultBin = (%q, %v), want (%q, true)", gotFallback, ok, binInDefault)
	}

	// 5. SetGeoDataDir overrides both sing-box and mihomo work dirs and clears back to default.
	geoDir := filepath.Join(t.TempDir(), "geo")
	mgr.SetGeoDataDir(geoDir)
	if mgr.singBoxWorkDir() != geoDir || mgr.mihomoWorkDir() != geoDir {
		t.Fatalf("expected SetGeoDataDir %q, got singBox=%q mihomo=%q", geoDir, mgr.singBoxWorkDir(), mgr.mihomoWorkDir())
	}
	mgr.SetGeoDataDir("")
	if mgr.singBoxWorkDir() != workDir || mgr.mihomoWorkDir() != filepath.Join(workDir, "mihomo") {
		t.Fatalf("expected default workDirs after reset, got singBox=%q mihomo=%q", mgr.singBoxWorkDir(), mgr.mihomoWorkDir())
	}
}
