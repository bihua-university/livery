package core

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/bihua-university/livery/internal/model"
)

// Manager supervises the active proxy core subprocess (mihomo or sing-box).
type Manager struct {
	mu         sync.Mutex
	coreType    model.CoreType
	binDir      string
	workDir     string
	geoDataDir  string
	customPath  string
	cmd         *exec.Cmd
	cancel      context.CancelFunc
	waitDone    chan struct{}
	guard       *processGuard
	startCancel context.CancelFunc
	closed      bool
	info        model.CoreInfo
	onState     func(model.CoreInfo)
	onLog       func(model.LogEntry)
	stopping    bool
	lastErrLine string
}

// NewManager creates a core supervisor defaulting to mihomo.
func NewManager(binDir, workDir string, onState func(model.CoreInfo), onLog func(model.LogEntry)) *Manager {
	return NewManagerWithConfig(model.CoreMihomo, "", "", binDir, workDir, onState, onLog)
}

// NewManagerWithConfig creates a core supervisor preconfigured for the target core type and paths
// so startup probes the binary only once.
func NewManagerWithConfig(coreType model.CoreType, customPath, geoDataDir, binDir, workDir string, onState func(model.CoreInfo), onLog func(model.LogEntry)) *Manager {
	ct := model.NormalizeCoreType(coreType)
	geo := strings.TrimSpace(geoDataDir)
	if geo != "" {
		geo = filepath.Clean(geo)
	}
	m := &Manager{
		coreType:   ct,
		binDir:     binDir,
		workDir:    workDir,
		geoDataDir: geo,
		onState:    onState,
		onLog:      onLog,
		info: model.CoreInfo{
			CoreType:   ct,
			Status:     model.CoreStopped,
			IsElevated: IsElevated(),
		},
	}
	m.RefreshBinaryInfo(customPath)
	return m
}

// Info returns a snapshot of the current core status.
func (m *Manager) Info() model.CoreInfo {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.info
}

// SetGeoDataDir configures a custom GeoData/working directory for core execution (blank = default).
func (m *Manager) SetGeoDataDir(dir string) {
	trimmed := strings.TrimSpace(dir)
	if trimmed != "" {
		trimmed = filepath.Clean(trimmed)
	}
	m.mu.Lock()
	m.geoDataDir = trimmed
	m.mu.Unlock()
}

// SetCoreType switches the target core engine and refreshes binary discovery info.
func (m *Manager) SetCoreType(coreType model.CoreType, customPath string) model.CoreInfo {
	ct := model.NormalizeCoreType(coreType)
	m.mu.Lock()
	m.coreType = ct
	m.info.CoreType = ct
	m.info.LastError = ""
	m.mu.Unlock()
	return m.RefreshBinaryInfo(customPath)
}

// RefreshBinaryInfo resolves the active core binary path and queries its version.
func (m *Manager) RefreshBinaryInfo(customPath string) model.CoreInfo {
	m.mu.Lock()
	m.customPath = strings.TrimSpace(customPath)
	cp := m.customPath
	ct := model.NormalizeCoreType(m.coreType)
	m.mu.Unlock()

	binPath, found := m.resolveBinary(ct, cp)
	version := ""
	if found {
		version = probeVersion(ct, binPath)
	}

	m.mu.Lock()
	m.info.CoreType = ct
	m.info.BinaryPath = binPath
	m.info.Found = found
	m.info.Version = version
	m.info.IsElevated = IsElevated()
	snap := m.info
	m.mu.Unlock()
	m.emitState(snap)
	return snap
}

var disableLocalBinaries bool // Set only by the E2E automation build.

func (m *Manager) resolveBinary(coreType model.CoreType, custom string) (string, bool) {
	if disableLocalBinaries {
		return "", false
	}
	names := binaryNames(coreType)
	custom = strings.TrimSpace(custom)
	if custom != "" {
		if st, err := os.Stat(custom); err == nil {
			if !st.IsDir() {
				return custom, true
			}
			for _, name := range names {
				cand := filepath.Join(custom, name)
				if fst, err := os.Stat(cand); err == nil && !fst.IsDir() {
					return cand, true
				}
			}
		} else if p, err := exec.LookPath(custom); err == nil {
			return p, true
		}
	}
	var dirs []string
	dirs = append(dirs, m.binDir, m.workDir)
	if exe, err := os.Executable(); err == nil {
		dirs = append(dirs, filepath.Dir(exe))
	}
	if wd, err := os.Getwd(); err == nil {
		dirs = append(dirs, wd)
	}
	for _, d := range dirs {
		if d == "" {
			continue
		}
		for _, name := range names {
			cand := filepath.Join(d, name)
			if st, err := os.Stat(cand); err == nil && !st.IsDir() {
				return cand, true
			}
		}
	}
	for _, name := range names {
		if p, err := exec.LookPath(name); err == nil {
			return p, true
		}
	}
	return "", false
}

func probeVersion(coreType model.CoreType, binPath string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	arg := "-v"
	if model.NormalizeCoreType(coreType) == model.CoreSingBox {
		arg = "version"
	}
	cmd := exec.CommandContext(ctx, binPath, arg)
	configureCommand(cmd)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return parseVersionOutput(coreType, string(out))
}

func parseVersionOutput(coreType model.CoreType, raw string) string {
	if model.NormalizeCoreType(coreType) == model.CoreSingBox {
		for _, line := range strings.Split(raw, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(strings.ToLower(line), "sing-box version") {
				return strings.TrimSpace(strings.TrimPrefix(line, "sing-box version"))
			}
		}
	} else {
		lines := strings.Split(strings.TrimSpace(raw), "\n")
		if len(lines) > 0 {
			for _, field := range strings.Fields(lines[0]) {
				if strings.HasPrefix(field, "v") && len(field) > 1 && field[1] >= '0' && field[1] <= '9' {
					return strings.TrimPrefix(field, "v")
				}
			}
			return strings.TrimSpace(lines[0])
		}
	}
	lines := strings.Split(strings.TrimSpace(raw), "\n")
	if len(lines) > 0 {
		return strings.TrimSpace(lines[0])
	}
	return ""
}

func (m *Manager) mihomoWorkDir() string {
	m.mu.Lock()
	custom := m.geoDataDir
	base := m.workDir
	m.mu.Unlock()
	return ResolveGeoDataDir(model.CoreMihomo, base, custom)
}

func (m *Manager) singBoxWorkDir() string {
	m.mu.Lock()
	custom := m.geoDataDir
	base := m.workDir
	m.mu.Unlock()
	return ResolveGeoDataDir(model.CoreSingBox, base, custom)
}

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

func stripANSI(s string) string {
	if !strings.ContainsRune(s, '\x1b') {
		return s
	}
	return ansiPattern.ReplaceAllString(s, "")
}

func (m *Manager) emitLog(level, msg string) {
	if m.onLog == nil {
		return
	}
	msg = strings.TrimSpace(stripANSI(msg))
	if msg == "" {
		return
	}
	m.mu.Lock()
	src := string(model.NormalizeCoreType(m.coreType))
	m.mu.Unlock()
	m.onLog(model.LogEntry{
		Time:    time.Now(),
		Level:   level,
		Source:  src,
		Message: msg,
	})
}

// CheckConfig validates config syntax & schema using the active core binary.
func (m *Manager) CheckConfig(configPath string) error {
	return m.checkConfigContext(context.Background(), configPath)
}

func (m *Manager) checkConfigContext(parent context.Context, configPath string) error {
	info := m.Info()
	ct := model.NormalizeCoreType(info.CoreType)
	coreName := model.CoreDisplayName(ct)
	if !info.Found {
		return fmt.Errorf("%s binary not found", coreName)
	}
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()

	var cmd *exec.Cmd
	if ct == model.CoreSingBox {
		workDir := m.singBoxWorkDir()
		_ = os.MkdirAll(workDir, 0o755)
		cmd = exec.CommandContext(ctx, info.BinaryPath, "check", "-c", configPath, "-D", workDir)
	} else {
		homeDir := m.mihomoWorkDir()
		_ = os.MkdirAll(homeDir, 0o755)
		cmd = exec.CommandContext(ctx, info.BinaryPath, "-t", "-d", homeDir, "-f", configPath)
	}
	configureCommand(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(stripANSI(string(out)))
		if msg == "" {
			msg = err.Error()
		}
		for _, line := range strings.Split(msg, "\n") {
			m.emitLog("error", line)
		}
		return errors.New(msg)
	}
	return nil
}

// Start launches the active core (mihomo or sing-box) with the compiled runtime config file.
func (m *Manager) Start(configPath string) error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return errors.New("core manager is shutting down")
	}
	m.mu.Unlock()

	m.Stop()

	startCtx, startCancel := context.WithCancel(context.Background())
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		startCancel()
		return errors.New("core manager is shutting down")
	}
	m.startCancel = startCancel
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		m.startCancel = nil
		m.mu.Unlock()
		startCancel()
	}()

	info := m.RefreshBinaryInfo(m.customPath)
	ct := model.NormalizeCoreType(info.CoreType)
	coreName := model.CoreDisplayName(ct)
	if !info.Found {
		err := fmt.Errorf("%s binary not found; configure Core Path or download %s in Settings", coreName, coreName)
		m.emitLog("error", err.Error())
		m.setError(err.Error())
		return err
	}

	m.emitLog("info", fmt.Sprintf("Checking config: %s", configPath))
	if err := m.checkConfigContext(startCtx, configPath); err != nil {
		if m.isClosed() {
			return errors.New("core manager is shutting down")
		}
		m.setError(err.Error())
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	var cmd *exec.Cmd
	if ct == model.CoreSingBox {
		workDir := m.singBoxWorkDir()
		_ = os.MkdirAll(workDir, 0o755)
		cmd = exec.CommandContext(ctx, info.BinaryPath, "run", "-c", configPath, "-D", workDir)
	} else {
		homeDir := m.mihomoWorkDir()
		_ = os.MkdirAll(homeDir, 0o755)
		cmd = exec.CommandContext(ctx, info.BinaryPath, "-d", homeDir, "-f", configPath)
	}
	configureCommand(cmd)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		m.setError(err.Error())
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		m.setError(err.Error())
		return err
	}

	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		cancel()
		return errors.New("core manager is shutting down")
	}
	m.info.Status = model.CoreStarting
	m.info.LastError = ""
	m.info.RuntimePath = configPath
	m.stopping = false
	m.lastErrLine = ""
	m.mu.Unlock()
	m.emitState(m.Info())

	if err := cmd.Start(); err != nil {
		cancel()
		m.emitLog("error", fmt.Sprintf("Failed to start %s: %v", coreName, err))
		m.setError(err.Error())
		return err
	}
	guard := attachProcessGuard(cmd)

	waitDone := make(chan struct{})
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		terminateProcessForce(cmd, guard)
		cancel()
		_ = cmd.Wait()
		guard.Close()
		return errors.New("core manager is shutting down")
	}
	m.cmd = cmd
	m.cancel = cancel
	m.waitDone = waitDone
	m.guard = guard
	m.info.Status = model.CoreRunning
	m.info.External = false
	m.info.PID = cmd.Process.Pid
	m.info.StartedAt = time.Now()
	m.info.LastError = ""
	snap := m.info
	m.mu.Unlock()

	m.emitLog("info", fmt.Sprintf("%s %s started (PID %d)", coreName, info.Version, cmd.Process.Pid))
	m.emitState(snap)

	var logWG sync.WaitGroup
	logWG.Add(2)
	go func() {
		defer logWG.Done()
		m.pipeLogs(stdout, "info")
	}()
	go func() {
		defer logWG.Done()
		m.pipeLogs(stderr, "warning")
	}()

	go func(c *exec.Cmd, g *processGuard, name string, wd chan struct{}) {
		defer close(wd)
		defer g.Close()
		logWG.Wait()
		waitErr := c.Wait()
		m.mu.Lock()
		// Ignore exit if a newer process has replaced this one.
		if m.cmd != c {
			m.mu.Unlock()
			return
		}
		wasStopping := m.stopping || m.closed
		errDetail := m.lastErrLine
		m.cmd = nil
		m.cancel = nil
		m.waitDone = nil
		m.guard = nil
		m.info.PID = 0
		var logLevel, logMsg string
		if wasStopping || waitErr == nil {
			m.info.Status = model.CoreStopped
			logLevel = "info"
			logMsg = name + " stopped"
		} else {
			m.info.Status = model.CoreError
			if errDetail != "" {
				m.info.LastError = errDetail
			} else {
				m.info.LastError = waitErr.Error()
			}
			logLevel = "error"
			logMsg = name + " exited unexpectedly: " + waitErr.Error()
		}
		snap := m.info
		m.mu.Unlock()

		m.emitLog(logLevel, logMsg)
		m.emitState(snap)
	}(cmd, guard, coreName, waitDone)

	return nil
}

func (m *Manager) isClosed() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.closed
}

// AttachExternal marks the core as running by connecting to an already-active external Clash API instance.
func (m *Manager) AttachExternal(version, endpoint string) {
	m.mu.Lock()
	if m.cmd != nil || m.closed {
		m.mu.Unlock()
		return
	}
	m.info.Status = model.CoreRunning
	m.info.External = true
	m.info.PID = 0
	m.info.StartedAt = time.Now()
	m.info.LastError = ""
	if strings.TrimSpace(version) != "" {
		m.info.Version = strings.TrimSpace(version)
	}
	snap := m.info
	coreName := model.CoreDisplayName(m.coreType)
	m.mu.Unlock()
	m.emitLog("info", fmt.Sprintf("Attached to existing %s core via Clash API (%s, v%s)", coreName, endpoint, snap.Version))
	m.emitState(snap)
}

// DetachExternal transitions an attached external core state back to stopped without killing external processes.
func (m *Manager) DetachExternal() {
	m.mu.Lock()
	if m.cmd != nil || !m.info.External {
		m.mu.Unlock()
		return
	}
	m.info.Status = model.CoreStopped
	m.info.External = false
	m.info.PID = 0
	snap := m.info
	coreName := model.CoreDisplayName(m.coreType)
	m.mu.Unlock()
	m.emitLog("info", fmt.Sprintf("Disconnected from external %s core", coreName))
	m.emitState(snap)
}

// Close permanently shuts down the manager, cancels any in-flight start, and terminates any running core process.
func (m *Manager) Close() {
	m.mu.Lock()
	m.closed = true
	startCancel := m.startCancel
	m.mu.Unlock()
	if startCancel != nil {
		startCancel()
	}
	m.Stop()
}

// Stop terminates the running core process cleanly, or detaches if external.
func (m *Manager) Stop() {
	m.mu.Lock()
	cmd := m.cmd
	cancel := m.cancel
	waitDone := m.waitDone
	guard := m.guard
	coreName := model.CoreDisplayName(m.coreType)
	if cmd == nil {
		wasExternal := m.info.External
		m.info.Status = model.CoreStopped
		m.info.External = false
		m.info.PID = 0
		snap := m.info
		m.mu.Unlock()
		if wasExternal {
			m.emitLog("info", fmt.Sprintf("Disconnected from external %s core", coreName))
		}
		m.emitState(snap)
		return
	}
	m.stopping = true
	m.mu.Unlock()

	if terminateProcessGraceful(cmd) && waitDone != nil {
		select {
		case <-waitDone:
			guard.Close()
			return
		case <-time.After(2 * time.Second):
		}
	}

	if cancel != nil {
		cancel()
	}
	terminateProcessForce(cmd, guard)
	if waitDone != nil {
		select {
		case <-waitDone:
			guard.Close()
			return
		case <-time.After(2 * time.Second):
		}
	}
	guard.Close()

	m.mu.Lock()
	if m.cmd == cmd {
		m.cmd = nil
		m.cancel = nil
		m.waitDone = nil
		m.guard = nil
		m.info.PID = 0
		m.info.Status = model.CoreStopped
		snap := m.info
		m.mu.Unlock()
		m.emitLog("info", coreName+" stopped")
		m.emitState(snap)
		return
	}
	m.mu.Unlock()
}

func (m *Manager) setError(msg string) {
	m.mu.Lock()
	m.info.Status = model.CoreError
	m.info.External = false
	m.info.LastError = msg
	m.info.PID = 0
	snap := m.info
	m.mu.Unlock()
	m.emitState(snap)
}

func (m *Manager) emitState(info model.CoreInfo) {
	if m.onState != nil {
		m.onState(info)
	}
}

func (m *Manager) pipeLogs(r io.Reader, defaultLevel string) {
	scanner := bufio.NewScanner(r)
	buf := make([]byte, 0, 32*1024)
	scanner.Buffer(buf, 512*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(stripANSI(scanner.Text()))
		if line == "" {
			continue
		}
		level := defaultLevel
		lower := strings.ToLower(line)
		switch {
		case strings.Contains(lower, "error") || strings.Contains(lower, "fatal"):
			level = "error"
		case strings.Contains(lower, "warn"):
			level = "warning"
		case strings.Contains(lower, "debug"):
			level = "debug"
		case strings.Contains(lower, "info"):
			level = "info"
		}
		if level == "error" {
			m.mu.Lock()
			m.lastErrLine = line
			m.mu.Unlock()
		}
		m.emitLog(level, line)
	}
}

// FormatError cleanly formats errors for UI display.
func FormatError(err error) string {
	if err == nil {
		return ""
	}
	return fmt.Sprintf("%v", err)
}
