package app

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"github.com/bihua-university/livery/internal/applog"
	"github.com/bihua-university/livery/internal/clashapi"
	"github.com/bihua-university/livery/internal/core"
	"github.com/bihua-university/livery/internal/diagnostics"
	"github.com/bihua-university/livery/internal/model"
	"github.com/bihua-university/livery/internal/profile"
	"github.com/bihua-university/livery/internal/sysproxy"
	"github.com/bihua-university/livery/internal/theme"
)

const (
	PageOverview    = "overview"
	PageProxies     = "proxies"
	PageProfiles    = "profiles"
	PageConnections = "connections"
	PageRules       = "rules"
	PageLogs        = "logs"
	PageSettings    = "settings"
)

const (
	maxTrafficSamples = 60
	maxVisibleLogs    = 300
	maxClosedConns    = 50
)

// App holds application state and orchestrates core, profile, proxy, and telemetry operations.
type App struct {
	PM       *profile.Manager
	CM       *core.Manager
	LogStore *applog.Store

	win           *mygo.Window
	onTrayRefresh func()

	Page         string
	PrevPage     string
	SettingsData model.AppSettings
	CoreInfo     model.CoreInfo
	Profiles     []model.Profile

	// Live telemetry
	TrafficHistory []model.TrafficSample
	CurrentUp      int64
	CurrentDown    int64
	PeakUp         int64
	PeakDown       int64
	TotalUp        int64
	TotalDown      int64
	MemoryBytes    int64
	AppMemoryBytes int64
	CoreStartedAt  time.Time
	LocalIP        string
	ChartXs        []float32
	ChartYsDown    []float32
	ChartYsUp      []float32
	ChartPtsDown   [][2]float32
	ChartPtsUp     [][2]float32
	ChartSlopes    []float32

	ShowQualityModal bool
	QualityReport    diagnostics.Report
	qualityChecker   *diagnostics.Checker
	qualityCancel    context.CancelFunc

	Groups          []model.ProxyGroup
	Nodes           map[string]model.ProxyNode
	SelectedGroup   string
	ExpandedGroups  map[string]bool
	ProxySearch     string
	ProxySort       int // 0=Default, 1=Latency, 2=Name
	ProxiesScrolled ui.ScrollState

	// Connections state
	Conns          []model.Connection
	ClosedConns    []model.Connection
	ConnFilterTab  int // 0=Active, 1=Closed
	ConnSearch     string
	ConnPaused     bool
	ConnTableState ui.ListState
	SelectedConnID string
	ShowConnModal  bool
	seenConnByID   map[string]model.Connection
	connActiveSet  map[string]struct{}

	// Rules state (lazily loaded asynchronously only while Page == PageRules; released on navigate away or hide)
	Rules            []model.RuleEntry
	RuleSearch       string
	RuleListState    ui.ListState
	RulesLoading     bool
	hasLoadedRules   bool
	rulesCoreSynced  bool
	rulesSyncRetryAt time.Time
	rulesLoadGen     uint64
	ShowRuleModal    bool
	DraftRuleType    string
	DraftRulePat     string
	DraftRuleOut     string
	DraftRulePrep    bool

	// Logs state (disk-backed via LogStore; loaded asynchronously only while Page == PageLogs)
	Logs             []model.LogEntry
	LogLevelIdx      int // 0=All, 1=Info, 2=Warn, 3=Error, 4=Debug
	LogSearch        string
	LogPaused        bool
	LogAutoScroll    bool
	LogListState     ui.ListState
	LogsLoading      bool
	loadedLogRev     uint64
	loadedLogLevel   int
	loadedLogSearch  string
	pendingLogRev    uint64
	pendingLogLevel  int
	pendingLogSearch string
	hasLoadedLogs    bool
	logsLoadGen      uint64

	// Profiles UI & Modals
	pendingClashImports map[string]bool
	SavingDNSMode       bool
	ShowImportModal     bool
	ImportURL           string
	ImportName          string
	ImportUA            string
	ImportUseProxy      bool
	Importing           bool
	ShowNewProfileModal bool
	NewProfileName      string

	EditingProfileID string
	ShowEditorModal  bool
	EditorTitle      string
	EditorReadOnly   bool
	EditorState      *editor.State
	EditorError      string

	ShowMetaModal   bool
	MetaDraft       model.Profile
	ShowDeleteAlert bool
	DeleteTargetID  string

	// Core installer status
	InstallingCore bool
	InstallStatus  string
	CoreStarting   bool
	CoreRestarting bool

	// GeoIP / GeoSite database status
	UpdatingGeo bool
	GeoStatus   string
	GeoFiles    []model.GeoFileStatus

	// App OTA update state (wired in main.go when updater plugin is active)
	AppUpdate   AppUpdateState
	updateHooks AppUpdateHooks

	// Settings draft buffers
	SettingsScroll ui.ScrollState
	ProfilesScroll ui.ScrollState
	MixedPortDraft string
	APIPortDraft   string
	TUNMTUDraft    string
	UIFontDraft    float64
	PendingToasts  []string
	WindowHidden   atomic.Bool

	apiMu           sync.Mutex
	apiClient       *clashapi.Client
	apiClientHost   string
	apiClientPort   int
	apiClientSecret string

	bgMu              sync.Mutex
	coreOpMu          sync.Mutex
	coreOpBusy        atomic.Bool
	sysproxyMu        sync.Mutex
	shuttingDown      atomic.Bool
	shutdownOnce      sync.Once
	proxyGuardWakeCh  chan struct{}
	streamCancel      context.CancelFunc
	syncWakeCh        chan struct{}
	freeMemPending    atomic.Bool
	trimMemPending    atomic.Bool
	hiddenTraffic     [maxTrafficSamples]model.TrafficSample
	hiddenTrafficHead int
	hiddenTrafficLen  int
	hiddenUp          int64
	hiddenDown        int64
	hiddenPeakUp      int64
	hiddenPeakDown    int64
	hasHiddenTraffic  bool
	streamedCoreMem   atomic.Int64
	overviewVisible   atomic.Bool

	mainRunner       func(func())
	mainQueueMu      sync.Mutex
	mainQueue        []func()
	mainQueueRunning bool
}

// New constructs the application controller.
func New(pm *profile.Manager, cm *core.Manager) *App {
	logStore, _ := applog.NewStore(filepath.Join(pm.DataDir(), "logs"))
	s := pm.Settings()
	a := &App{
		PM:               pm,
		CM:               cm,
		LogStore:         logStore,
		Page:             PageOverview,
		PrevPage:         PageOverview,
		SettingsData:     s,
		CoreInfo:         cm.Info(),
		Profiles:         pm.Profiles(),
		ExpandedGroups:   make(map[string]bool),
		LogAutoScroll:    true,
		DraftRuleType:    "domain_suffix",
		DraftRuleOut:     "direct",
		DraftRulePrep:    true,
		MixedPortDraft:   fmt.Sprintf("%d", s.MixedPort),
		APIPortDraft:     fmt.Sprintf("%d", s.ClashAPIPort),
		TUNMTUDraft:      fmt.Sprintf("%d", s.TUNMTU),
		UIFontDraft:      float64(s.UIFontSize),
		LocalIP:          detectLocalIPv4(),
		qualityChecker:   diagnostics.New(pm.DataDir()),
		AppMemoryBytes:   sampleAppMemory(),
		TrafficHistory:   make([]model.TrafficSample, maxTrafficSamples),
		ChartXs:          make([]float32, maxTrafficSamples),
		ChartYsDown:      make([]float32, maxTrafficSamples),
		ChartYsUp:        make([]float32, maxTrafficSamples),
		proxyGuardWakeCh: make(chan struct{}, 1),
		syncWakeCh:       make(chan struct{}, 1),
	}
	now := time.Now()
	for i := range a.TrafficHistory {
		a.TrafficHistory[i] = model.TrafficSample{Timestamp: now.Add(-time.Duration(maxTrafficSamples-1-i) * time.Second)}
	}
	a.GeoFiles = core.InspectGeoFiles(s, pm.DataDir())
	a.LogApp("info", fmt.Sprintf("Livery initialized (data: %s)", pm.DataDir()))
	return a
}

// LogApp appends an application-level log line to the log viewer.
func (a *App) LogApp(level, msg string) {
	a.OnLog(model.LogEntry{
		Time:    time.Now(),
		Level:   level,
		Source:  "livery",
		Message: msg,
	})
}

// Tr returns localized copy for Chinese or English.
func (a *App) Tr(zh, en string) string {
	if a.SettingsData.Language == model.LangEN {
		return en
	}
	return zh
}

// CoreDisplayName returns the human-readable name of the currently selected core engine.
func (a *App) CoreDisplayName() string {
	return model.CoreDisplayName(a.SettingsData.CoreType)
}

// SwitchCoreType switches between Mihomo and sing-box core modes, restarting if a managed core was running.
func (a *App) SwitchCoreType(ct model.CoreType) {
	ct = model.NormalizeCoreType(ct)
	if a.SettingsData.CoreType == ct {
		return
	}
	a.SettingsData.CoreType = ct
	customPath := a.SettingsData.CorePathFor(ct)
	_ = a.PM.SaveSettings(a.SettingsData)
	a.InstallStatus = ""
	a.ReloadRulesFromActiveProfile()
	a.RefreshTray()

	go func() {
		a.coreOpMu.Lock()
		defer a.coreOpMu.Unlock()

		info := a.CM.Info()
		wasRunning := info.Status == model.CoreRunning || info.Status == model.CoreStarting
		wasExternal := info.External
		if wasRunning {
			a.CM.Stop()
		}
		newInfo := a.CM.SetCoreType(ct, customPath)

		a.UpdateUI(func() {
			a.CoreInfo = newInfo
			a.Groups = nil
			a.Nodes = nil
			a.Conns = nil
			a.ClosedConns = nil
			a.seenConnByID = nil
			a.connActiveSet = nil
			a.GeoStatus = ""
			a.GeoFiles = core.InspectGeoFiles(a.SettingsData, a.PM.DataDir())
			a.Notify(fmt.Sprintf(a.Tr("已切换为 %s 核心模式", "Switched to %s core"), model.CoreDisplayName(ct)))
		})

		if wasRunning && !wasExternal && newInfo.Found {
			a.UpdateUI(func() {
				a.CoreStarting = true
				if a.onTrayRefresh != nil {
					a.onTrayRefresh()
				}
			})
			defer a.UpdateUI(func() {
				a.CoreStarting = false
				if a.onTrayRefresh != nil {
					a.onTrayRefresh()
				}
			})
			a.startCoreLocked(false)
		}
	}()
}

// SetWindow attaches the main MyGo window for thread-safe UI updates.
func (a *App) SetWindow(w *mygo.Window) {
	a.win = w
}

// SetMainRunner registers the host main-thread executor (typically mygo.RunOnMain)
// used to serialize async state mutations when no UI window is open (e.g. silent start or tray mode).
func (a *App) SetMainRunner(fn func(func())) {
	a.mainRunner = fn
}

// Window returns the attached MyGo window, if any.
func (a *App) Window() *mygo.Window {
	return a.win
}

// SetTrayRefresh registers a callback invoked when tray state changes.
func (a *App) SetTrayRefresh(fn func()) {
	a.onTrayRefresh = fn
}

// UpdateUI executes fn on the window's main thread and requests a frame redraw when a window is open.
func (a *App) UpdateUI(fn func()) {
	if a.win != nil && !a.win.IsDestroyed() {
		a.win.Update(fn)
		return
	}
	if a.mainRunner != nil {
		a.dispatchMainAsync(fn)
		return
	}
	fn()
}

func (a *App) dispatchMainAsync(fn func()) {
	a.mainQueueMu.Lock()
	a.mainQueue = append(a.mainQueue, fn)
	start := !a.mainQueueRunning
	if start {
		a.mainQueueRunning = true
	}
	a.mainQueueMu.Unlock()
	if !start {
		return
	}
	go func() {
		for {
			a.mainQueueMu.Lock()
			if len(a.mainQueue) == 0 {
				a.mainQueueRunning = false
				a.mainQueueMu.Unlock()
				return
			}
			batch := a.mainQueue
			a.mainQueue = nil
			a.mainQueueMu.Unlock()
			a.mainRunner(func() {
				for _, f := range batch {
					f()
				}
			})
		}
	}()
}

// Notify queues a toast message for the next UI frame.
func (a *App) Notify(msg string) {
	a.PendingToasts = append(a.PendingToasts, msg)
}

// RefreshTray triggers tray icon and menu synchronization without blocking background workers on the UI thread.
func (a *App) RefreshTray() {
	if a.onTrayRefresh == nil {
		return
	}
	a.UpdateUI(func() {
		if a.onTrayRefresh != nil {
			a.onTrayRefresh()
		}
	})
}

// IsCoreBusy reports whether a core start or restart operation is currently in flight.
func (a *App) IsCoreBusy() bool {
	return a.CoreStarting || a.CoreRestarting || a.coreOpBusy.Load()
}

// Settings returns the current settings snapshot.
func (a *App) Settings() model.AppSettings {
	return a.SettingsData
}

// Info returns the current core info snapshot.
func (a *App) Info() model.CoreInfo {
	return a.CoreInfo
}

// ActivePalette returns the resolved theme palette for the current theme mode.
func (a *App) ActivePalette(systemDark bool) theme.Palette {
	switch a.SettingsData.ThemeMode {
	case model.ThemeLight:
		return theme.Light()
	case model.ThemeDark:
		return theme.Dark()
	default:
		if systemDark {
			return theme.Dark()
		}
		return theme.Light()
	}
}

// SetThemeMode updates the theme mode and syncs the native window appearance when running.
func (a *App) SetThemeMode(mode model.ThemeMode) {
	a.SettingsData.ThemeMode = mode
	_ = a.PM.SaveSettings(a.SettingsData)
	if a.win != nil {
		switch mode {
		case model.ThemeLight:
			mygo.Theme.SetSource(mygo.ThemeLight)
		case model.ThemeDark:
			mygo.Theme.SetSource(mygo.ThemeDark)
		default:
			mygo.Theme.SetSource(mygo.ThemeSystem)
		}
	}
}

// OnCoreState is invoked by core.Manager when the sing-box subprocess transitions.
func (a *App) OnCoreState(info model.CoreInfo) {
	if info.Status != model.CoreRunning {
		a.bgMu.Lock()
		a.hiddenTrafficHead = 0
		a.hiddenTrafficLen = 0
		a.hiddenUp = 0
		a.hiddenDown = 0
		a.hiddenPeakUp = 0
		a.hiddenPeakDown = 0
		a.hasHiddenTraffic = false
		a.bgMu.Unlock()
	}
	a.UpdateUI(func() {
		if info.Status != model.CoreRunning || info.StartedAt != a.CoreInfo.StartedAt {
			a.resetQuality()
		}
		a.CoreInfo = info
		if info.Status == model.CoreRunning {
			if a.CoreStartedAt.IsZero() {
				a.CoreStartedAt = time.Now()
			}
		} else {
			a.CoreStartedAt = time.Time{}
			a.CurrentUp = 0
			a.CurrentDown = 0
			a.PeakUp = 0
			a.PeakDown = 0
			a.MemoryBytes = 0
			a.streamedCoreMem.Store(0)
			if info.Status != model.CoreStarting {
				a.Groups = nil
				a.Nodes = nil
				a.Conns = nil
				a.ClosedConns = nil
				a.seenConnByID = nil
				a.connActiveSet = nil
				a.ShowConnModal = false
				a.FreeOSMemoryAsync()
			}
		}
		if a.onTrayRefresh != nil {
			a.onTrayRefresh()
		}
	})
	if info.Status == model.CoreRunning && !a.WindowHidden.Load() && !a.shuttingDown.Load() {
		a.startStreams()
	} else {
		a.stopStreams()
	}
}

// OnLog persists a log entry directly to disk and wakes the Logs view only while it is visible.
func (a *App) OnLog(entry model.LogEntry) {
	if a.LogStore != nil {
		a.LogStore.Append(entry)
	}
	if a.Page == PageLogs && !a.WindowHidden.Load() && !a.LogPaused {
		if a.win != nil && !a.win.IsDestroyed() {
			a.win.Invalidate()
		}
	}
}

// SyncLogsForView lazily loads only the matching tail window from disk off the UI thread while the Logs page is open.
func (a *App) SyncLogsForView() {
	if a.LogStore == nil || a.Page != PageLogs || a.WindowHidden.Load() {
		return
	}
	rev := a.LogStore.Revision()
	lvl := a.LogLevelIdx
	search := a.LogSearch

	if a.hasLoadedLogs && a.loadedLogRev == rev && a.loadedLogLevel == lvl && a.loadedLogSearch == search {
		return
	}
	if a.LogPaused && a.hasLoadedLogs && a.loadedLogLevel == lvl && a.loadedLogSearch == search {
		return
	}
	if a.LogsLoading && a.pendingLogRev == rev && a.pendingLogLevel == lvl && a.pendingLogSearch == search {
		return
	}

	wantLevel := ""
	switch lvl {
	case 1:
		wantLevel = "info"
	case 2:
		wantLevel = "warn"
	case 3:
		wantLevel = "error"
	case 4:
		wantLevel = "debug"
	}

	a.logsLoadGen++
	gen := a.logsLoadGen
	a.LogsLoading = true
	a.pendingLogRev = rev
	a.pendingLogLevel = lvl
	a.pendingLogSearch = search
	store := a.LogStore

	go func() {
		entries, err := store.Tail(maxVisibleLogs, wantLevel, search)
		a.UpdateUI(func() {
			if gen != a.logsLoadGen {
				return
			}
			a.LogsLoading = false
			if a.Page != PageLogs || a.WindowHidden.Load() {
				return
			}
			if err == nil {
				a.Logs = entries
				a.loadedLogRev = rev
				a.loadedLogLevel = lvl
				a.loadedLogSearch = search
				a.hasLoadedLogs = true
			}
		})
	}()
}

// ReleaseLogsView drops the in-memory log slice when navigating away or hiding the window.
func (a *App) ReleaseLogsView() {
	hadLogs := len(a.Logs) > 0
	if a.Logs != nil || a.hasLoadedLogs || a.LogsLoading {
		a.logsLoadGen++
		a.Logs = nil
		a.hasLoadedLogs = false
		a.LogsLoading = false
		a.LogListState = ui.ListState{}
		if hadLogs {
			a.FreeOSMemoryAsync()
		}
	}
}

// ReleaseEditorState drops the code editor buffer and syntax state when the editor modal closes.
func (a *App) ReleaseEditorState() {
	hadEditor := a.EditorState != nil
	if hadEditor || a.EditorError != "" || a.EditingProfileID != "" || a.EditorTitle != "" {
		a.EditorState = nil
		a.EditorError = ""
		a.EditingProfileID = ""
		a.EditorTitle = ""
		if hadEditor {
			a.FreeOSMemoryAsync()
		}
	}
}

// SetWindowHidden updates the hidden flag before a window is created (e.g. silent start).
func (a *App) SetWindowHidden(hidden bool) {
	a.WindowHidden.Store(hidden)
	if hidden {
		a.SetOverviewVisible(false)
		a.stopStreams()
	}
}

// OnWindowShown resumes UI telemetry streams and syncs fresh snapshots when the window becomes visible.
func (a *App) OnWindowShown() {
	a.FlushHiddenTraffic()
	if !a.shuttingDown.Load() && a.CM.Info().Status == model.CoreRunning {
		a.bgMu.Lock()
		running := a.streamCancel != nil
		a.bgMu.Unlock()
		if !running {
			a.startStreams()
		}
	}
	a.RequestSnapshotSync()
}

// OnWindowClosed detaches the closed window, stops UI telemetry streams, drops transient UI data, and trims unused OS memory.
func (a *App) OnWindowClosed() {
	a.resetQuality()
	a.win = nil
	a.WindowHidden.Store(true)
	a.SetOverviewVisible(false)
	a.stopStreams()
	a.ReleaseLogsView()
	a.ReleaseRulesView()
	a.ReleaseEditorState()
	a.Groups = nil
	a.Nodes = nil
	a.Conns = nil
	a.ClosedConns = nil
	a.seenConnByID = nil
	a.connActiveSet = nil
	a.ShowConnModal = false
	a.ChartPtsDown = nil
	a.ChartPtsUp = nil
	a.ChartSlopes = nil
	a.FreeOSMemoryAndTrimAsync()
}

// ReleaseHiddenData stops UI telemetry streams, drops transient proxy/connection state when hidden to tray, and returns freed memory to OS.
func (a *App) ReleaseHiddenData() {
	a.resetQuality()
	a.ShowQualityModal = false
	a.stopStreams()
	a.Groups = nil
	a.Nodes = nil
	a.Conns = nil
	a.ClosedConns = nil
	a.seenConnByID = nil
	a.connActiveSet = nil
	a.ShowConnModal = false
	a.FreeOSMemoryAndTrimAsync()
}

// FreeOSMemoryAsync runs GC and scavenges unused heap memory back to the OS asynchronously.
func (a *App) FreeOSMemoryAsync() {
	a.freeOSMemoryAsync(false)
}

// FreeOSMemoryAndTrimAsync runs GC, scavenges unused heap memory, and trims OS working set pages (used when hidden/closed to tray).
func (a *App) FreeOSMemoryAndTrimAsync() {
	a.freeOSMemoryAsync(true)
}

func (a *App) freeOSMemoryAsync(trim bool) {
	if trim {
		a.trimMemPending.Store(true)
	}
	if !a.freeMemPending.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer a.freeMemPending.Store(false)
		runtime.GC()
		debug.FreeOSMemory()
		if a.trimMemPending.Swap(false) {
			// Allow native renderer teardown (e.g. D3D11 device release) to settle before trimming cold working set pages.
			time.Sleep(150 * time.Millisecond)
			runtime.GC()
			debug.FreeOSMemory()
			trimProcessMemory()
		}
	}()
}

// RequestSnapshotSync wakes the background telemetry sync loop immediately when visible.
func (a *App) RequestSnapshotSync() {
	if a.syncWakeCh == nil {
		return
	}
	select {
	case a.syncWakeCh <- struct{}{}:
	default:
	}
}

// SetOverviewVisible tracks whether the Overview page is currently visible and refreshes app RSS on entry.
func (a *App) SetOverviewVisible(visible bool) {
	was := a.overviewVisible.Swap(visible)
	if visible && !was {
		a.RefreshAppMemoryAsync()
	}
}

// RefreshAppMemoryAsync samples current app and core private memory off the UI thread and publishes updates to Overview.
func (a *App) RefreshAppMemoryAsync() {
	go func() {
		if a.shuttingDown.Load() || a.WindowHidden.Load() || !a.overviewVisible.Load() {
			return
		}
		appMem := sampleAppMemory()
		var coreMem int64
		if info := a.CM.Info(); info.Status == model.CoreRunning {
			coreMem = sampleCoreProcessMemory(info, a.PM.Settings().ClashAPIPort)
		}
		a.UpdateUI(func() {
			if a.WindowHidden.Load() || a.Page != PageOverview {
				return
			}
			a.AppMemoryBytes = appMem
			if coreMem > 0 && a.CoreInfo.Status == model.CoreRunning {
				a.MemoryBytes = coreMem
			}
		})
	}()
}

// IsRulesLoaded reports whether the Rules tab has finished at least one load since entering.
func (a *App) IsRulesLoaded() bool { return a.hasLoadedRules }

// IsLogsLoaded reports whether the Logs tab has finished at least one load since entering.
func (a *App) IsLogsLoaded() bool { return a.hasLoadedLogs }

// RefreshRulesNow forces an asynchronous reload of routing rules while on the Rules tab.
func (a *App) RefreshRulesNow() {
	if a.Page != PageRules || a.WindowHidden.Load() {
		return
	}
	a.rulesCoreSynced = false
	a.rulesSyncRetryAt = time.Time{}
	a.startRulesLoadLocked(true)
}

// SyncRulesForView lazily loads routing rules asynchronously only while the Rules page is open.
func (a *App) SyncRulesForView() {
	if a.Page != PageRules || a.WindowHidden.Load() {
		return
	}
	if a.RulesLoading {
		return
	}
	if !a.hasLoadedRules {
		a.startRulesLoadLocked(false)
		return
	}
	if a.CoreInfo.Status == model.CoreRunning && !a.rulesCoreSynced && time.Now().After(a.rulesSyncRetryAt) {
		a.startRulesLoadLocked(false)
	}
}

func (a *App) startRulesLoadLocked(force bool) {
	if a.RulesLoading && !force {
		return
	}
	a.rulesLoadGen++
	gen := a.rulesLoadGen
	a.RulesLoading = true
	coreRunning := a.CoreInfo.Status == model.CoreRunning
	client := a.APIClient()

	go func() {
		var rules []model.RuleEntry
		syncedFromCore := false
		if coreRunning {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			live, err := client.GetRules(ctx)
			cancel()
			if err == nil && len(live) > 0 {
				rules = live
				syncedFromCore = true
			}
		}
		if len(rules) == 0 {
			rules = a.PM.ActiveRules()
		}

		a.UpdateUI(func() {
			if gen != a.rulesLoadGen {
				return
			}
			a.RulesLoading = false
			if a.Page != PageRules || a.WindowHidden.Load() {
				return
			}
			a.Rules = rules
			a.hasLoadedRules = true
			if syncedFromCore {
				a.rulesCoreSynced = true
				a.rulesSyncRetryAt = time.Time{}
			} else if coreRunning {
				a.rulesSyncRetryAt = time.Now().Add(5 * time.Second)
			}
		})
	}()
}

// ReleaseRulesView drops the in-memory rules slice when navigating away or hiding the window.
func (a *App) ReleaseRulesView() {
	hadRules := len(a.Rules) > 0
	if a.Rules != nil || a.hasLoadedRules || a.RulesLoading || a.rulesCoreSynced || !a.rulesSyncRetryAt.IsZero() {
		a.rulesLoadGen++
		a.Rules = nil
		a.hasLoadedRules = false
		a.RulesLoading = false
		a.rulesCoreSynced = false
		a.rulesSyncRetryAt = time.Time{}
		a.RuleListState = ui.ListState{}
		if hadRules {
			a.FreeOSMemoryAsync()
		}
	}
}

// ClearLogs truncates the disk log file and clears the view cache.
func (a *App) ClearLogs() {
	if a.LogStore != nil {
		_ = a.LogStore.Clear()
	}
	a.logsLoadGen++
	a.Logs = nil
	a.LogsLoading = false
	a.hasLoadedLogs = false
}

// APIClient returns a cached Clash API client for the configured management endpoint.
func (a *App) APIClient() *clashapi.Client {
	host := a.SettingsData.ClashAPIHost
	port := a.SettingsData.ClashAPIPort
	secret := a.SettingsData.ClashAPISecret

	a.apiMu.Lock()
	defer a.apiMu.Unlock()
	if a.apiClient == nil || a.apiClientHost != host || a.apiClientPort != port || a.apiClientSecret != secret {
		a.apiClient = clashapi.New(host, port, secret)
		a.apiClientHost = host
		a.apiClientPort = port
		a.apiClientSecret = secret
	}
	return a.apiClient
}

// TryAttachCore probes the configured Clash API management port and attaches if an external core is already active.
func (a *App) TryAttachCore(ctx context.Context) bool {
	if a.SettingsData.ClashAPIPort <= 0 {
		return false
	}
	probeCtx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
	defer cancel()

	client := a.APIClient()
	ver, err := client.Version(probeCtx)
	if err != nil {
		return false
	}
	host := a.SettingsData.ClashAPIHost
	if host == "" {
		host = "127.0.0.1"
	}
	a.CM.AttachExternal(ver, fmt.Sprintf("%s:%d", host, a.SettingsData.ClashAPIPort))
	return true
}

// StartBackground launches periodic background tasks.
func (a *App) StartBackground(ctx context.Context) {
	go func() {
		a.coreOpMu.Lock()
		attached := a.TryAttachCore(ctx)
		a.coreOpMu.Unlock()
		s := a.PM.Settings()
		if !attached && s.AutoStartCore && a.CM.Info().Found {
			a.StartCore()
		}
		a.reconcileSystemProxyOnStartup()
	}()
	go func() {
		guardTimer := time.NewTimer(a.proxyGuardDuration())
		subTicker := time.NewTicker(5 * time.Minute)
		defer guardTimer.Stop()
		defer subTicker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-a.proxyGuardWakeCh:
				if a.shuttingDown.Load() {
					return
				}
				if !guardTimer.Stop() {
					select {
					case <-guardTimer.C:
					default:
					}
				}
				guardTimer.Reset(a.proxyGuardDuration())
			case <-guardTimer.C:
				if a.shuttingDown.Load() {
					return
				}
				a.runProxyGuardTick()
				guardTimer.Reset(a.proxyGuardDuration())
			case <-subTicker.C:
				a.checkAutoUpdates(ctx)
			}
		}
	}()
	go func() {
		// Return transient startup allocations (SVG parsing, config loading, first frame) to the OS once settled.
		startupScavenge := time.NewTimer(2500 * time.Millisecond)
		defer startupScavenge.Stop()
		// Poll slower than MyGo's 2s frameIdle so idle shaping/frame caches can release cleanly.
		memTicker := time.NewTicker(5 * time.Second)
		defer memTicker.Stop()
		var lastMem int64
		for {
			select {
			case <-ctx.Done():
				return
			case <-startupScavenge.C:
				if !a.shuttingDown.Load() {
					if a.WindowHidden.Load() {
						a.FreeOSMemoryAndTrimAsync()
					} else {
						a.FreeOSMemoryAsync()
					}
				}
			case <-memTicker.C:
				if a.shuttingDown.Load() || a.WindowHidden.Load() || !a.overviewVisible.Load() {
					continue
				}
				if a.CM.Info().Status == model.CoreRunning {
					continue
				}
				appMem := sampleAppMemory()
				diff := appMem - lastMem
				if diff < 0 {
					diff = -diff
				}
				if lastMem > 0 && diff < 64*1024 {
					continue
				}
				lastMem = appMem
				a.UpdateUI(func() {
					if a.WindowHidden.Load() || a.Page != PageOverview {
						return
					}
					a.AppMemoryBytes = appMem
				})
			}
		}
	}()
}

// WakeProxyGuard resets the proxy guard timer immediately after settings changes.
func (a *App) WakeProxyGuard() {
	if a.proxyGuardWakeCh == nil {
		return
	}
	select {
	case a.proxyGuardWakeCh <- struct{}{}:
	default:
	}
}

func (a *App) proxyGuardDuration() time.Duration {
	s := a.PM.Settings()
	if !s.ProxyGuard || !s.SystemProxyEnabled {
		return 24 * time.Hour
	}
	sec := s.ProxyGuardInterval
	if sec <= 0 {
		sec = 10
	}
	if sec < 3 {
		sec = 3
	}
	return time.Duration(sec) * time.Second
}

func effectiveMixedPort(s model.AppSettings) int {
	if s.MixedPort > 0 && s.MixedPort <= 65535 {
		return s.MixedPort
	}
	return 7890
}

func (a *App) reconcileSystemProxyOnStartup() {
	a.sysproxyMu.Lock()
	defer a.sysproxyMu.Unlock()
	if a.shuttingDown.Load() {
		return
	}
	s := a.PM.Settings()
	if !s.SystemProxyEnabled {
		return
	}
	if a.CM.Info().Status == model.CoreRunning {
		port := effectiveMixedPort(s)
		if err := sysproxy.Enable("127.0.0.1", port, s.SystemProxyBypass); err != nil {
			a.LogApp("warning", "Startup system proxy sync failed: "+err.Error())
			s.SystemProxyEnabled = false
			_ = a.PM.SaveSettings(s)
			a.UpdateUI(func() {
				a.SettingsData.SystemProxyEnabled = false
				a.Notify(a.Tr("系统代理恢复失败: ", "System proxy restore failed: ") + err.Error())
				if a.onTrayRefresh != nil {
					a.onTrayRefresh()
				}
			})
		}
		return
	}
	if st, err := sysproxy.Current(); err == nil && !st.Enabled {
		s.SystemProxyEnabled = false
		_ = a.PM.SaveSettings(s)
		a.UpdateUI(func() {
			a.SettingsData.SystemProxyEnabled = false
			if a.onTrayRefresh != nil {
				a.onTrayRefresh()
			}
		})
	}
}

func (a *App) runProxyGuardTick() {
	a.sysproxyMu.Lock()
	defer a.sysproxyMu.Unlock()
	if a.shuttingDown.Load() {
		return
	}
	s := a.PM.Settings()
	if !s.ProxyGuard || !s.SystemProxyEnabled {
		return
	}
	port := effectiveMixedPort(s)
	bypass := sysproxy.NormalizeBypass(s.SystemProxyBypass)
	st, err := sysproxy.Current()
	if err == nil && st.MatchesTarget("127.0.0.1", port, bypass) {
		return
	}
	if err := sysproxy.Enable("127.0.0.1", port, bypass); err != nil {
		a.LogApp("warning", "Proxy guard failed to restore system proxy: "+err.Error())
		return
	}
	a.LogApp("info", fmt.Sprintf("Proxy guard restored system proxy (127.0.0.1:%d)", port))
}

func (a *App) checkAutoUpdates(ctx context.Context) {
	for _, p := range a.PM.Profiles() {
		if p.Kind != "remote" || !p.AutoUpdate || p.UpdateIntervalH <= 0 {
			continue
		}
		if time.Since(p.UpdatedAt) >= time.Duration(p.UpdateIntervalH)*time.Hour {
			_, _ = a.PM.RefreshRemote(ctx, p.ID)
			a.UpdateUI(func() {
				a.Profiles = a.PM.Profiles()
			})
		}
	}

	s := a.PM.Settings()
	if s.GeoAutoUpdate && s.GeoUpdateIntervalH > 0 {
		files := core.InspectGeoFiles(s, a.PM.DataDir())
		needsUpdate := false
		for _, f := range files {
			if !f.Exists || time.Since(f.UpdatedAt) >= time.Duration(s.GeoUpdateIntervalH)*time.Hour {
				needsUpdate = true
				break
			}
		}
		if needsUpdate {
			a.UpdateUI(func() {
				a.StartGeoUpdate()
			})
		}
	}
}

func (a *App) startStreams() {
	a.stopStreams()
	ctx, cancel := context.WithCancel(context.Background())
	a.bgMu.Lock()
	a.streamCancel = cancel
	a.bgMu.Unlock()

	client := a.APIClient()

	go func() {
		for {
			if ctx.Err() != nil {
				return
			}
			_ = client.StreamTraffic(ctx, func(sample model.TrafficSample) {
				if a.WindowHidden.Load() {
					a.recordHiddenTraffic(sample)
					return
				}
				a.UpdateUI(func() {
					if a.WindowHidden.Load() {
						a.recordHiddenTraffic(sample)
						return
					}
					a.pushTrafficSample(sample)
				})
			})
			select {
			case <-ctx.Done():
				return
			case <-time.After(1500 * time.Millisecond):
			}
		}
	}()

	// Live core memory stream (/memory for Mihomo; sing-box reports memory in /connections).
	go func() {
		for {
			if ctx.Err() != nil {
				return
			}
			err := client.StreamMemory(ctx, func(mem int64) {
				a.streamedCoreMem.Store(mem)
				if a.WindowHidden.Load() {
					return
				}
				displayMem := mem
				if osMem := sampleCoreProcessMemory(a.CM.Info(), a.PM.Settings().ClashAPIPort); osMem > 0 {
					displayMem = osMem
				}
				a.UpdateUI(func() {
					if a.WindowHidden.Load() || a.CoreInfo.Status != model.CoreRunning {
						return
					}
					a.MemoryBytes = displayMem
				})
			})
			if errors.Is(err, clashapi.ErrUnsupported) {
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(1500 * time.Millisecond):
			}
		}
	}()

	// Periodic snapshot sync (/proxies, /connections); Rules are loaded strictly on-demand when opening the Rules tab.
	go func() {
		select {
		case <-ctx.Done():
			return
		case <-time.After(350 * time.Millisecond):
		}

		syncOnce := func(includeProxies bool) {
			if a.WindowHidden.Load() {
				return
			}
			if includeProxies {
				if groups, nodes, err := client.GetProxies(ctx); err == nil {
					a.UpdateUI(func() {
						if a.WindowHidden.Load() || a.CoreInfo.Status != model.CoreRunning {
							return
						}
						a.SetGroups(groups)
						a.Nodes = nodes
					})
				}
			}
			if a.Page == PageConnections || a.Page == PageOverview {
				if snap, err := client.GetConnections(ctx); err == nil {
					appMem := sampleAppMemory()
					coreMem := sampleCoreProcessMemory(a.CM.Info(), a.PM.Settings().ClashAPIPort)
					a.UpdateUI(func() {
						if a.WindowHidden.Load() || a.CoreInfo.Status != model.CoreRunning {
							return
						}
						a.TotalUp = snap.UploadTotal
						a.TotalDown = snap.DownloadTotal
						mem := coreMem
						if mem <= 0 {
							mem = snap.Memory
						}
						if mem <= 0 {
							mem = a.streamedCoreMem.Load()
						}
						a.MemoryBytes = mem
						a.AppMemoryBytes = appMem
						if !a.ConnPaused {
							a.reconcileConnections(snap.Connections)
						}
					})
				}
			}
		}

		syncOnce(a.Page == PageProxies || a.Page == PageOverview)
		ticker := time.NewTicker(1500 * time.Millisecond)
		defer ticker.Stop()
		ticks := 0
		extFailStreak := 0
		for {
			select {
			case <-ctx.Done():
				return
			case <-a.syncWakeCh:
				if !a.WindowHidden.Load() {
					includeProxies := a.Page == PageProxies || a.Page == PageOverview
					syncOnce(includeProxies)
				}
			case <-ticker.C:
				ticks++
				if a.CM.Info().External && ticks%3 == 0 {
					probeCtx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
					_, err := client.Version(probeCtx)
					cancel()
					if err != nil {
						extFailStreak++
						if extFailStreak >= 2 {
							a.CM.DetachExternal()
							return
						}
					} else {
						extFailStreak = 0
					}
				}
				if a.WindowHidden.Load() {
					continue
				}
				includeProxies := a.Page == PageProxies || a.Page == PageOverview
				syncOnce(includeProxies)
			}
		}
	}()
}

func (a *App) stopStreams() {
	a.bgMu.Lock()
	cancel := a.streamCancel
	a.streamCancel = nil
	a.bgMu.Unlock()
	if cancel != nil {
		cancel()
	}
	a.apiMu.Lock()
	client := a.apiClient
	a.apiMu.Unlock()
	if client != nil {
		client.CloseIdleConnections()
	}
}

func (a *App) recordHiddenTraffic(sample model.TrafficSample) {
	a.bgMu.Lock()
	if a.hiddenTrafficLen < maxTrafficSamples {
		idx := (a.hiddenTrafficHead + a.hiddenTrafficLen) % maxTrafficSamples
		a.hiddenTraffic[idx] = sample
		a.hiddenTrafficLen++
	} else {
		a.hiddenTraffic[a.hiddenTrafficHead] = sample
		a.hiddenTrafficHead = (a.hiddenTrafficHead + 1) % maxTrafficSamples
	}
	a.hiddenUp = sample.Up
	a.hiddenDown = sample.Down
	if sample.Up > a.hiddenPeakUp {
		a.hiddenPeakUp = sample.Up
	}
	if sample.Down > a.hiddenPeakDown {
		a.hiddenPeakDown = sample.Down
	}
	a.hasHiddenTraffic = true
	a.bgMu.Unlock()
}

// FlushHiddenTraffic applies traffic samples collected while the window was hidden.
func (a *App) FlushHiddenTraffic() {
	a.bgMu.Lock()
	if !a.hasHiddenTraffic {
		a.bgMu.Unlock()
		return
	}
	n := a.hiddenTrafficLen
	head := a.hiddenTrafficHead
	var buf [maxTrafficSamples]model.TrafficSample
	for i := range n {
		buf[i] = a.hiddenTraffic[(head+i)%maxTrafficSamples]
	}
	up := a.hiddenUp
	down := a.hiddenDown
	peakUp := a.hiddenPeakUp
	peakDown := a.hiddenPeakDown
	a.hiddenTrafficHead = 0
	a.hiddenTrafficLen = 0
	a.hiddenPeakUp = 0
	a.hiddenPeakDown = 0
	a.hasHiddenTraffic = false
	a.bgMu.Unlock()

	a.CurrentUp = up
	a.CurrentDown = down
	if peakUp > a.PeakUp {
		a.PeakUp = peakUp
	}
	if peakDown > a.PeakDown {
		a.PeakDown = peakDown
	}
	for i := range n {
		a.appendTrafficHistory(buf[i])
	}
}

func (a *App) pushTrafficSample(sample model.TrafficSample) {
	a.CurrentUp = sample.Up
	a.CurrentDown = sample.Down
	if sample.Up > a.PeakUp {
		a.PeakUp = sample.Up
	}
	if sample.Down > a.PeakDown {
		a.PeakDown = sample.Down
	}
	a.appendTrafficHistory(sample)
}

func (a *App) appendTrafficHistory(sample model.TrafficSample) {
	if len(a.TrafficHistory) < maxTrafficSamples {
		a.TrafficHistory = append(a.TrafficHistory, sample)
		return
	}
	copy(a.TrafficHistory, a.TrafficHistory[1:])
	a.TrafficHistory[len(a.TrafficHistory)-1] = sample
}

func (a *App) reconcileConnections(active []model.Connection) {
	if a.connActiveSet == nil {
		a.connActiveSet = make(map[string]struct{}, len(active))
	} else {
		clear(a.connActiveSet)
	}
	if a.seenConnByID == nil {
		a.seenConnByID = make(map[string]model.Connection, len(active))
	}
	prevSeenCount := len(a.seenConnByID)
	for _, c := range active {
		a.connActiveSet[c.ID] = struct{}{}
		a.seenConnByID[c.ID] = c
	}
	for id, prev := range a.seenConnByID {
		if _, ok := a.connActiveSet[id]; !ok {
			a.prependClosedConn(prev)
			delete(a.seenConnByID, id)
		}
	}
	if len(a.seenConnByID) == 0 && len(active) == 0 {
		a.seenConnByID = nil
		a.connActiveSet = nil
	} else if prevSeenCount > 256 && len(active) < 32 {
		shrunk := make(map[string]model.Connection, len(active))
		for k, v := range a.seenConnByID {
			shrunk[k] = v
		}
		a.seenConnByID = shrunk
		shrunkSet := make(map[string]struct{}, len(active))
		for k := range a.connActiveSet {
			shrunkSet[k] = struct{}{}
		}
		a.connActiveSet = shrunkSet
	}
	a.Conns = active
}

func (a *App) prependClosedConn(prev model.Connection) {
	if len(a.ClosedConns) < maxClosedConns {
		a.ClosedConns = append(a.ClosedConns, model.Connection{})
	}
	copy(a.ClosedConns[1:], a.ClosedConns[:len(a.ClosedConns)-1])
	a.ClosedConns[0] = prev
}

// StartCore attaches to an active management port if reachable, or compiles and launches the active core.
func (a *App) StartCore() {
	if !a.coreOpBusy.CompareAndSwap(false, true) {
		return
	}
	defer a.coreOpBusy.Store(false)

	a.coreOpMu.Lock()
	defer a.coreOpMu.Unlock()

	a.UpdateUI(func() {
		a.CoreStarting = true
		if a.onTrayRefresh != nil {
			a.onTrayRefresh()
		}
	})
	defer a.UpdateUI(func() {
		a.CoreStarting = false
		if a.onTrayRefresh != nil {
			a.onTrayRefresh()
		}
	})

	a.startCoreLocked(true)
}

func (a *App) startCoreLocked(tryAttach bool) {
	if a.shuttingDown.Load() || a.CM.Info().Status == model.CoreRunning {
		return
	}
	coreName := a.CoreDisplayName()
	if tryAttach && a.TryAttachCore(context.Background()) {
		a.UpdateUI(func() {
			a.Notify(a.Tr("检测到后台核心运行中，已通过管理端口连接", "Connected to running core via Clash API"))
		})
		return
	}

	runtimePath, _, err := a.PM.CompileActive()
	if err != nil {
		a.LogApp("error", "Compile active profile failed: "+err.Error())
		a.UpdateUI(func() {
			a.Notify(a.Tr("配置编译失败: ", "Compile failed: ") + err.Error())
		})
		return
	}
	if a.shuttingDown.Load() {
		return
	}
	a.UpdateUI(func() {
		a.ReloadRulesFromActiveProfile()
	})
	if err := a.CM.Start(runtimePath); err != nil {
		if a.shuttingDown.Load() {
			return
		}
		a.UpdateUI(func() {
			a.Notify(a.Tr("启动失败: ", "Start failed: ") + err.Error())
		})
		return
	}
	a.UpdateUI(func() {
		a.Notify(fmt.Sprintf(a.Tr("%s 核心已启动", "%s core started"), coreName))
	})
}

// StopCore terminates the active core (or disconnects if attached to an external core).
func (a *App) StopCore() {
	a.coreOpMu.Lock()
	wasExternal := a.CM.Info().External
	coreName := a.CoreDisplayName()
	a.CM.Stop()
	a.coreOpMu.Unlock()
	a.UpdateUI(func() {
		if wasExternal {
			a.Notify(fmt.Sprintf(a.Tr("已断开外部 %s 核心连接", "Disconnected from external %s core"), coreName))
		} else {
			a.Notify(fmt.Sprintf(a.Tr("%s 核心已停止", "%s core stopped"), coreName))
		}
	})
}

// RestartCore compiles and restarts a managed core instance cleanly under lock.
func (a *App) RestartCore() {
	if a.shuttingDown.Load() {
		return
	}
	if !a.coreOpBusy.CompareAndSwap(false, true) {
		return
	}
	defer a.coreOpBusy.Store(false)

	a.coreOpMu.Lock()
	defer a.coreOpMu.Unlock()
	if a.shuttingDown.Load() {
		return
	}

	if a.CM.Info().External {
		a.UpdateUI(func() {
			a.Notify(a.Tr("当前已连接后台外部核心，请先关闭外部进程再由 Livery 启动", "Attached to external core; stop the external process before restarting"))
		})
		return
	}

	a.UpdateUI(func() {
		a.CoreRestarting = true
		if a.onTrayRefresh != nil {
			a.onTrayRefresh()
		}
	})
	defer a.UpdateUI(func() {
		a.CoreRestarting = false
		if a.onTrayRefresh != nil {
			a.onTrayRefresh()
		}
	})

	a.CM.Stop()
	a.startCoreLocked(false)
}

// SetSystemProxy enables or disables the OS system proxy.
func (a *App) SetSystemProxy(enable bool) {
	a.sysproxyMu.Lock()
	defer a.sysproxyMu.Unlock()
	if a.shuttingDown.Load() {
		return
	}

	s := a.PM.Settings()
	port := effectiveMixedPort(s)
	var err error
	if enable {
		err = sysproxy.Enable("127.0.0.1", port, s.SystemProxyBypass)
	} else {
		err = sysproxy.Disable()
	}
	if err != nil {
		a.LogApp("error", "System proxy update failed: "+err.Error())
		a.UpdateUI(func() {
			a.Notify(a.Tr("系统代理设置失败: ", "System proxy error: ") + err.Error())
			if a.onTrayRefresh != nil {
				a.onTrayRefresh()
			}
		})
		return
	}
	s.SystemProxyEnabled = enable
	_ = a.PM.SaveSettings(s)
	a.UpdateUI(func() {
		a.SettingsData.SystemProxyEnabled = enable
		if enable {
			a.Notify(fmt.Sprintf(a.Tr("已开启系统代理 (127.0.0.1:%d)", "System proxy enabled (127.0.0.1:%d)"), port))
		} else {
			a.Notify(a.Tr("已关闭系统代理", "System proxy disabled"))
		}
		if a.onTrayRefresh != nil {
			a.onTrayRefresh()
		}
	})
	a.WakeProxyGuard()
}

// SetTUNMode toggles TUN virtual interface and restarts core if running.
func (a *App) SetTUNMode(enable bool) {
	if enable && !core.IsElevated() {
		a.UpdateUI(func() {
			a.Notify(a.Tr("提示：TUN 模式在多数系统上需要管理员/Root 权限", "Note: TUN mode usually requires Administrator/Root privileges"))
		})
	}
	a.SettingsData.TUNEnabled = enable
	_ = a.PM.SaveSettings(a.SettingsData)
	a.RefreshTray()
	if a.CoreInfo.Status == model.CoreRunning {
		go a.RestartCore()
	}
}

// SetProxyMode updates proxy mode (rule/global/direct) via Clash API & settings.
func (a *App) SetProxyMode(mode model.ProxyMode) {
	if mode != a.SettingsData.ProxyMode {
		a.resetQuality()
	}
	a.SettingsData.ProxyMode = mode
	sortGroups(a.Groups, mode)
	_ = a.PM.SaveSettings(a.SettingsData)
	a.RefreshTray()
	if a.CoreInfo.Status == model.CoreRunning {
		go func() {
			_ = a.APIClient().SetMode(context.Background(), mode)
		}()
	}
}

// Shutdown cleans up core, drains proxy guard, disables system proxy, and closes logs before exit.
func (a *App) Shutdown() {
	a.shuttingDown.Store(true)
	a.shutdownOnce.Do(func() {
		a.WakeProxyGuard()
		a.stopStreams()

		info := a.CM.Info()
		s := a.PM.Settings()
		a.CM.Close()
		if info.External {
			terminateOwnedExternalCore(s.ClashAPIPort, a.PM.BinDir(), info.BinaryPath)
		}

		a.sysproxyMu.Lock()
		if s.SystemProxyEnabled || a.SettingsData.SystemProxyEnabled {
			_ = sysproxy.Disable()
		}
		a.sysproxyMu.Unlock()

		if a.LogStore != nil {
			_ = a.LogStore.Close()
		}
	})
}

// SelectProxy selects an outbound node inside a selector group.
func (a *App) SelectProxy(group, target string) {
	previous := ""
	closeConnections := false
	for i := range a.Groups {
		if a.Groups[i].Name == group {
			previous = a.Groups[i].Now
			if previous == target {
				return
			}
			closeConnections = a.SettingsData.CloseConnectionsOnSwitch
			a.resetQuality()
			a.Groups[i].Now = target
			break
		}
	}
	client := a.APIClient()
	go func() {
		ctx := context.Background()
		if err := client.SelectProxy(ctx, group, target); err != nil {
			a.UpdateUI(func() {
				for i := range a.Groups {
					if a.Groups[i].Name == group && a.Groups[i].Now == target {
						a.Groups[i].Now = previous
						break
					}
				}
				a.Notify(a.Tr("切换节点失败: ", "Switch failed: ") + err.Error())
			})
			return
		}
		if closeConnections {
			if err := client.CloseAllConnections(ctx); err != nil {
				a.UpdateUI(func() {
					a.Notify(a.Tr("节点已切换，断开连接失败: ", "Node switched, but closing connections failed: ") + err.Error())
				})
			}
		}
	}()
}

// IsGroupExpanded returns whether a proxy group accordion card is currently expanded.
func (a *App) IsGroupExpanded(name string) bool {
	if a.ExpandedGroups == nil {
		return false
	}
	return a.ExpandedGroups[name]
}

// ToggleGroupExpanded toggles the expanded state of a proxy group accordion card.
func (a *App) ToggleGroupExpanded(name string) {
	if a.ExpandedGroups == nil {
		a.ExpandedGroups = make(map[string]bool)
	}
	a.ExpandedGroups[name] = !a.ExpandedGroups[name]
}

// SetAllGroupsExpanded expands or collapses all loaded proxy groups.
func (a *App) SetAllGroupsExpanded(expanded bool) {
	if a.ExpandedGroups == nil {
		a.ExpandedGroups = make(map[string]bool)
	}
	for _, g := range a.Groups {
		a.ExpandedGroups[g.Name] = expanded
	}
}

// TestNodeDelay measures latency for a single node.
func (a *App) TestNodeDelay(name string) {
	if n, ok := a.Nodes[name]; ok {
		n.Testing = true
		a.Nodes[name] = n
	}
	go func() {
		d, err := a.APIClient().TestDelay(context.Background(), name, a.SettingsData.DelayTestURL, a.SettingsData.DelayTestTimeoutMs)
		a.UpdateUI(func() {
			if n, ok := a.Nodes[name]; ok {
				n.Testing = false
				if err != nil {
					n.Delay = 0
				} else {
					n.Delay = d
				}
				a.Nodes[name] = n
			}
		})
	}()
}

// TestGroupDelay measures latency for all nodes in a group.
func (a *App) TestGroupDelay(groupName string) {
	for i := range a.Groups {
		if a.Groups[i].Name == groupName {
			a.Groups[i].Testing = true
			for _, member := range a.Groups[i].All {
				if n, ok := a.Nodes[member]; ok {
					n.Testing = true
					a.Nodes[member] = n
				}
			}
			break
		}
	}
	go func() {
		delays, err := a.APIClient().TestGroupDelay(context.Background(), groupName, a.SettingsData.DelayTestURL, a.SettingsData.DelayTestTimeoutMs)
		a.UpdateUI(func() {
			for i := range a.Groups {
				if a.Groups[i].Name == groupName {
					a.Groups[i].Testing = false
					for _, member := range a.Groups[i].All {
						if n, ok := a.Nodes[member]; ok {
							n.Testing = false
							if err == nil {
								if d, exists := delays[member]; exists {
									n.Delay = d
								} else {
									n.Delay = 0
								}
							}
							a.Nodes[member] = n
						}
					}
					break
				}
			}
			if err != nil {
				a.Notify(a.Tr("测速失败: ", "Latency test failed: ") + err.Error())
			}
		})
	}()
}

// ReloadRulesFromActiveProfile invalidates cached rules and refreshes them asynchronously only while viewing the Rules page.
func (a *App) ReloadRulesFromActiveProfile() {
	a.rulesCoreSynced = false
	a.rulesSyncRetryAt = time.Time{}
	if a.Page != PageRules || a.WindowHidden.Load() {
		a.ReleaseRulesView()
		return
	}
	a.startRulesLoadLocked(true)
}

// SetGroups replaces and pre-sorts the active proxy groups slice.
func (a *App) SetGroups(groups []model.ProxyGroup) {
	sortGroups(groups, a.SettingsData.ProxyMode)
	if len(a.Groups) > 0 && (a.QualityChecking() || !a.QualityReport.CheckedAt.IsZero()) && !slices.EqualFunc(a.Groups, groups, func(old, next model.ProxyGroup) bool {
		return old.Name == next.Name && old.Now == next.Now
	}) {
		a.resetQuality()
	}
	a.Groups = groups
	if a.SelectedGroup == "" && len(a.Groups) > 0 {
		a.SelectedGroup = a.Groups[0].Name
	}
}

// ActivePrimaryNode returns the primary selected outbound name for overview display.
func (a *App) ActivePrimaryNode() string {
	for _, g := range a.OrderedGroups() {
		if g.Name != "GLOBAL" && g.Now != "" {
			return g.Now
		}
	}
	if len(a.Groups) > 0 && a.Groups[0].Now != "" {
		return a.Groups[0].Now
	}
	return "--"
}

// OrderedGroups returns proxy groups prioritized with primary selector groups on top.
func (a *App) OrderedGroups() []model.ProxyGroup {
	return a.Groups
}

func sortGroups(groups []model.ProxyGroup, mode model.ProxyMode) {
	if len(groups) < 2 {
		return
	}
	sort.SliceStable(groups, func(i, j int) bool {
		return groupOrderRank(groups[i], mode) < groupOrderRank(groups[j], mode)
	})
}

func groupOrderRank(g model.ProxyGroup, mode model.ProxyMode) int {
	if strings.EqualFold(g.Name, "GLOBAL") {
		if mode == model.ModeGlobal {
			return 2
		}
		return 4
	}
	if strings.EqualFold(g.Type, "Selector") {
		name := strings.TrimSpace(g.Name)
		if strings.Contains(name, "选择节点") || strings.Contains(name, "节点选择") {
			return 0
		}
		if name == "默认代理" || name == "手动选择" || name == "手动切换" ||
			strings.EqualFold(name, "proxy") || strings.EqualFold(name, "select") || strings.EqualFold(name, "proxies") {
			return 1
		}
	}
	return 3
}

func detectLocalIPv4() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "127.0.0.1"
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip4 := ip.To4(); ip4 != nil && !ip4.IsLoopback() {
				return ip4.String()
			}
		}
	}
	return "127.0.0.1"
}

// ModeLabel returns a localized label for a proxy mode.
func (a *App) ModeLabel(m model.ProxyMode) string {
	switch m {
	case model.ModeGlobal:
		return a.Tr("全局", "Global")
	case model.ModeDirect:
		return a.Tr("直连", "Direct")
	default:
		return a.Tr("规则", "Rule")
	}
}

// ActivateProfile switches the active profile and restarts the core if running.
func (a *App) ActivateProfile(id string) {
	if err := a.PM.SetActive(id); err != nil {
		a.Notify(err.Error())
		return
	}
	a.SettingsData = a.PM.Settings()
	a.ReloadRulesFromActiveProfile()
	a.Notify(a.Tr("已切换配置", "Profile activated"))
	if a.CoreInfo.Status == model.CoreRunning {
		go a.RestartCore()
	}
}

// ImportClashLink adds a subscription opened by the OS to the profile library.
func (a *App) ImportClashLink(raw string) {
	a.Page = PageProfiles
	target, name, ok := profile.ParseClashInstallURL(raw)
	if !ok {
		a.Notify(a.Tr("无效的 Clash 订阅导入链接", "Invalid Clash subscription link"))
		return
	}
	for _, p := range a.PM.Profiles() {
		if p.Kind == "remote" && p.URL == target {
			a.Notify(a.Tr("订阅已存在：", "Subscription already exists: ") + p.Name)
			return
		}
	}
	if a.pendingClashImports[target] {
		return
	}
	if a.pendingClashImports == nil {
		a.pendingClashImports = make(map[string]bool)
	}
	a.pendingClashImports[target] = true
	a.Notify(a.Tr("正在导入订阅…", "Importing subscription…"))
	go func() {
		p, err := a.PM.ImportRemote(context.Background(), name, target, "", false)
		a.UpdateUI(func() {
			delete(a.pendingClashImports, target)
			if err != nil {
				a.Notify(a.Tr("订阅导入失败，请检查地址和网络后重试", "Import failed. Check the subscription address and network, then try again."))
				return
			}
			a.Profiles = a.PM.Profiles()
			a.Notify(a.Tr("已导入订阅：", "Subscription imported: ") + p.Name)
		})
	}()
}

// PickLocalProfile opens a file chooser to import a local sing-box JSON profile.
func (a *App) PickLocalProfile() {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Parent: a.win,
		Title:  a.Tr("选择配置文件 (sing-box / Clash / URI)", "Select config file (sing-box / Clash / URI)"),
		Filters: []mygo.FileFilter{
			{Name: "Subscription / Config", Extensions: []string{"json", "jsonc", "yaml", "yml", "txt"}},
		},
	})
	if err != nil || len(paths) == 0 {
		return
	}
	p, err := a.PM.ImportLocal("", paths[0])
	a.UpdateUI(func() {
		if err != nil {
			a.Notify(a.Tr("导入失败: ", "Import failed: ") + err.Error())
			return
		}
		a.Profiles = a.PM.Profiles()
		a.Notify(a.Tr("已导入配置: ", "Imported profile: ") + p.Name)
	})
}

// RefreshProfileAsync updates a remote subscription profile in the background.
func (a *App) RefreshProfileAsync(id string) {
	a.Notify(a.Tr("正在更新订阅...", "Updating subscription..."))
	go func() {
		updated, err := a.PM.RefreshRemote(context.Background(), id)
		a.UpdateUI(func() {
			a.Profiles = a.PM.Profiles()
			if err != nil {
				a.Notify(a.Tr("更新失败: ", "Update failed: ") + err.Error())
				return
			}
			a.Notify(a.Tr("订阅已更新: ", "Updated: ") + updated.Name)
			if updated.ID == a.SettingsData.ActiveProfileID && a.CoreInfo.Status == model.CoreRunning {
				go a.RestartCore()
			}
		})
	}()
}

// RefreshAllProfilesAsync updates every remote subscription profile.
func (a *App) RefreshAllProfilesAsync() {
	a.Notify(a.Tr("正在更新全部远程订阅...", "Updating all remote profiles..."))
	go func() {
		count := 0
		for _, p := range a.PM.Profiles() {
			if p.Kind == "remote" {
				if _, err := a.PM.RefreshRemote(context.Background(), p.ID); err == nil {
					count++
				}
			}
		}
		a.UpdateUI(func() {
			a.Profiles = a.PM.Profiles()
			a.Notify(fmt.Sprintf(a.Tr("已更新 %d 个订阅", "Updated %d remote profiles"), count))
		})
	}()
}

// OpenProfileEditor loads a profile's YAML or JSON into the code editor modal.
func (a *App) OpenProfileEditor(id string) {
	raw, err := a.PM.ReadProfileRaw(id)
	if err != nil {
		a.Notify(err.Error())
		return
	}
	text := string(raw)
	isJSON := strings.HasPrefix(strings.TrimSpace(text), "{")
	buf := editor.NewBuffer(text)
	if isJSON {
		buf.SetLanguage(editor.LanguageFor("config.json"))
		a.EditorTitle = a.Tr("编辑配置 JSON", "Edit Profile JSON")
	} else {
		buf.SetLanguage(editor.LanguageFor("config.yaml"))
		a.EditorTitle = a.Tr("编辑配置 YAML", "Edit Profile YAML")
	}
	a.EditingProfileID = id
	a.EditorReadOnly = false
	a.EditorError = ""
	a.EditorState = &editor.State{Buffer: buf}
	a.ShowEditorModal = true
}

// OpenRuntimePreview loads the compiled runtime config into the read-only code editor modal.
func (a *App) OpenRuntimePreview() {
	_, compiled, err := a.PM.CompileActive()
	if err != nil {
		a.Notify(err.Error())
		return
	}
	buf := editor.NewBuffer(string(compiled))
	if model.NormalizeCoreType(a.SettingsData.CoreType) == model.CoreSingBox {
		buf.SetLanguage(editor.LanguageFor("runtime.json"))
	} else {
		buf.SetLanguage(editor.LanguageFor("runtime.yaml"))
	}
	a.EditingProfileID = ""
	a.EditorTitle = a.Tr("编译后的运行时配置 (只读)", "Compiled Runtime Config (Read-Only)")
	a.EditorReadOnly = true
	a.EditorError = ""
	a.EditorState = &editor.State{Buffer: buf}
	a.ShowEditorModal = true
}

// FindProfile returns a profile by ID.
func (a *App) FindProfile(id string) (model.Profile, bool) {
	for _, p := range a.Profiles {
		if p.ID == id {
			return p, true
		}
	}
	return model.Profile{}, false
}

// ApplySettings persists SettingsData and optionally restarts the core.
func (a *App) ApplySettings(restartIfRunning bool) {
	_ = a.PM.SaveSettings(a.SettingsData)
	a.WakeProxyGuard()
	if restartIfRunning && a.CoreInfo.Status == model.CoreRunning {
		go a.RestartCore()
	}
}

// SetDNSMode applies a per-profile override to Livery-managed Mihomo configurations.
func (a *App) SetDNSMode(id, mode string) {
	if a.SavingDNSMode || a.IsCoreBusy() {
		return
	}
	a.SavingDNSMode = true
	go func() {
		err := a.PM.SetDNSMode(id, mode)
		a.UpdateUI(func() {
			a.SavingDNSMode = false
			if err != nil {
				a.Notify(a.Tr("保存 DNS 模式失败：", "Could not save DNS mode: ") + err.Error())
				return
			}
			a.Profiles = a.PM.Profiles()
			if a.SettingsData.ActiveProfileID != id || model.NormalizeCoreType(a.SettingsData.CoreType) != model.CoreMihomo || a.CoreInfo.Status != model.CoreRunning {
				return
			}
			if a.CoreInfo.External {
				a.Notify(a.Tr("DNS 模式已保存，下次由 Livery 启动核心时生效", "DNS mode saved for the next Livery-managed start"))
				return
			}
			go a.RestartCore()
		})
	}()
}

// StartInstallCore downloads the latest release binary for the active core in the background.
// When a custom core path or directory is configured, it downloads directly to that location.
func (a *App) StartInstallCore() {
	if a.InstallingCore {
		return
	}
	targetCore := model.NormalizeCoreType(a.SettingsData.CoreType)
	coreName := model.CoreDisplayName(targetCore)
	configured := strings.TrimSpace(a.SettingsData.CorePathFor(targetCore))
	a.SettingsData.SetCorePathFor(targetCore, configured)
	_ = a.PM.SaveSettings(a.SettingsData)
	targetDir, targetName := core.ResolveCoreDownloadTarget(targetCore, a.PM.BinDir(), configured)

	a.InstallingCore = true
	a.InstallStatus = a.Tr("正在连接 GitHub Releases...", "Connecting to GitHub Releases...")
	go func() {
		binPath, err := core.DownloadLatestTo(context.Background(), targetCore, targetDir, targetName, func(msg string) {
			a.UpdateUI(func() {
				a.InstallStatus = msg
			})
		})
		a.FreeOSMemoryAsync()
		a.UpdateUI(func() {
			a.InstallingCore = false
			if err != nil {
				a.InstallStatus = a.Tr("下载失败: ", "Download failed: ") + err.Error()
				a.Notify(a.InstallStatus)
				return
			}
			activePath := strings.TrimSpace(a.SettingsData.CorePathFor(targetCore))
			a.SettingsData.SetCorePathFor(targetCore, activePath)
			_ = a.PM.SaveSettings(a.SettingsData)
			if a.SettingsData.CoreType == targetCore {
				a.CoreInfo = a.CM.RefreshBinaryInfo(activePath)
			}
			a.InstallStatus = a.Tr("核心安装成功: ", "Installed: ") + binPath
			a.Notify(fmt.Sprintf(a.Tr("%s 核心安装成功", "%s core installed"), coreName))
		})
	}()
}

// RefreshGeoStatus refreshes the cached status of local GeoIP/GeoSite files.
func (a *App) RefreshGeoStatus() {
	a.GeoFiles = core.InspectGeoFiles(a.SettingsData, a.PM.DataDir())
}

// ApplyGeoDataDir persists the configured GeoData directory, updates the core manager, and refreshes status.
func (a *App) ApplyGeoDataDir() {
	a.SettingsData.GeoDataDir = strings.TrimSpace(a.SettingsData.GeoDataDir)
	_ = a.PM.SaveSettings(a.SettingsData)
	a.CM.SetGeoDataDir(a.SettingsData.GeoDataDir)
	a.RefreshGeoStatus()
	a.Notify(a.Tr("Geo 数据目录已更新", "GeoData directory updated"))
	if a.CoreInfo.Status == model.CoreRunning {
		go a.RestartCore()
	}
}

// ResetGeoURLs restores the default GeoIP/GeoSite/MMDB/ASN download URLs and persists settings.
func (a *App) ResetGeoURLs() {
	a.SettingsData.GeoIPURL = model.DefaultGeoIPURL
	a.SettingsData.GeoSiteURL = model.DefaultGeoSiteURL
	a.SettingsData.MMDBURL = model.DefaultMMDBURL
	a.SettingsData.ASNURL = model.DefaultASNURL
	_ = a.PM.SaveSettings(a.SettingsData)
	a.Notify(a.Tr("已恢复默认 Geo 数据库源", "Default Geo database URLs restored"))
}

// StartGeoUpdate updates GeoIP/GeoSite databases via Mihomo's /configs/geo API or direct download.
func (a *App) StartGeoUpdate() {
	if a.UpdatingGeo {
		return
	}
	a.SettingsData.GeoDataDir = strings.TrimSpace(a.SettingsData.GeoDataDir)
	_ = a.PM.SaveSettings(a.SettingsData)
	a.CM.SetGeoDataDir(a.SettingsData.GeoDataDir)
	settings := a.SettingsData
	running := a.CoreInfo.Status == model.CoreRunning
	dataDir := a.PM.DataDir()

	a.UpdatingGeo = true
	a.GeoStatus = a.Tr("正在更新 GeoIP 数据库...", "Updating GeoIP databases...")

	go func() {
		if model.NormalizeCoreType(settings.CoreType) == model.CoreMihomo && running {
			a.UpdateUI(func() {
				a.GeoStatus = a.Tr("正在通过 Mihomo API 更新 GeoIP 数据库...", "Updating GeoIP via Mihomo API...")
			})
			if err := a.APIClient().UpdateGeo(context.Background()); err == nil {
				files := core.InspectGeoFiles(settings, dataDir)
				a.LogApp("info", "GeoIP/GeoSite databases updated via Mihomo API")
				a.UpdateUI(func() {
					a.UpdatingGeo = false
					a.GeoFiles = files
					a.GeoStatus = a.Tr("GeoIP 数据库已更新并热重载", "GeoIP databases updated and reloaded")
					a.Notify(a.GeoStatus)
				})
				return
			}
		}

		files, err := core.DownloadGeoDatabases(context.Background(), settings, dataDir, running, func(msg string) {
			a.UpdateUI(func() {
				a.GeoStatus = msg
			})
		})
		a.FreeOSMemoryAsync()
		a.UpdateUI(func() {
			a.UpdatingGeo = false
			a.GeoFiles = files
			if err != nil {
				a.GeoStatus = a.Tr("更新失败: ", "Update failed: ") + err.Error()
				a.LogApp("error", "GeoIP update failed: "+err.Error())
				a.Notify(a.GeoStatus)
				return
			}
			a.LogApp("info", "GeoIP/GeoSite databases downloaded to "+core.EffectiveGeoDataDir(settings, dataDir))
			a.GeoStatus = a.Tr("GeoIP 数据库更新完成", "GeoIP databases updated")
			a.Notify(a.GeoStatus)
		})
	}()
}

// OpenFolderInOS reveals a folder in the OS file manager.
func OpenFolderInOS(dir string) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return
	}
	_ = os.MkdirAll(dir, 0o755)
	switch runtime.GOOS {
	case "windows":
		_ = exec.Command("explorer", dir).Start()
	case "darwin":
		_ = exec.Command("open", dir).Start()
	default:
		_ = exec.Command("xdg-open", dir).Start()
	}
}

// DeleteCustomRuleAt removes a custom rule at index idx and reloads rules/core.
func (a *App) DeleteCustomRuleAt(idx int) {
	if idx < 0 || idx >= len(a.SettingsData.CustomRules) {
		return
	}
	a.SettingsData.CustomRules = slices.Delete(a.SettingsData.CustomRules, idx, idx+1)
	_ = a.PM.SaveSettings(a.SettingsData)
	a.ReloadRulesFromActiveProfile()
	if a.CoreInfo.Status == model.CoreRunning {
		go a.RestartCore()
	}
}

// AppUpdateState summarizes OTA update availability and preferences for UI/tray rendering.
type AppUpdateState struct {
	Supported          bool
	AutomaticChecks    bool
	AutomaticDownloads bool
	AvailableVersion   string
	LastCheck          time.Time
}

// AppUpdateHooks connects App to the MyGo updater plugin without coupling unit tests to plugin registration.
type AppUpdateHooks struct {
	State                 func() AppUpdateState
	CheckForUpdates       func()
	SetAutomaticChecks    func(bool)
	SetAutomaticDownloads func(bool)
	SetPrereleases        func(bool)
}

// SetAppUpdateHooks registers OTA callbacks and initializes current update state.
func (a *App) SetAppUpdateHooks(h AppUpdateHooks) {
	a.updateHooks = h
	if h.SetPrereleases != nil {
		h.SetPrereleases(a.SettingsData.UpdatePrereleases)
	}
	if h.State != nil {
		a.AppUpdate = h.State()
	}
}

// RefreshAppUpdateState reloads OTA update state from the updater plugin and refreshes UI/tray.
func (a *App) RefreshAppUpdateState() {
	if a.updateHooks.State == nil {
		return
	}
	st := a.updateHooks.State()
	a.UpdateUI(func() {
		a.AppUpdate = st
		a.RefreshTray()
	})
}

// CheckAppUpdates opens the native OTA update window and checks for new releases.
func (a *App) CheckAppUpdates() {
	if a.updateHooks.CheckForUpdates == nil {
		a.Notify(a.Tr("当前运行环境未启用应用更新器", "App updater is not active"))
		return
	}
	a.updateHooks.CheckForUpdates()
}

// SetUpdateAutomaticChecks toggles periodic background OTA update checks.
func (a *App) SetUpdateAutomaticChecks(on bool) {
	a.AppUpdate.AutomaticChecks = on
	if a.updateHooks.SetAutomaticChecks != nil {
		a.updateHooks.SetAutomaticChecks(on)
	}
	a.RefreshAppUpdateState()
}

// SetUpdateAutomaticDownloads toggles automatic background download and install of updates.
func (a *App) SetUpdateAutomaticDownloads(on bool) {
	a.AppUpdate.AutomaticDownloads = on
	if a.updateHooks.SetAutomaticDownloads != nil {
		a.updateHooks.SetAutomaticDownloads(on)
	}
	a.RefreshAppUpdateState()
}

// SetUpdatePrereleases toggles receiving pre-release builds for OTA updates.
func (a *App) SetUpdatePrereleases(on bool) {
	a.SettingsData.UpdatePrereleases = on
	_ = a.PM.SaveSettings(a.SettingsData)
	if a.updateHooks.SetPrereleases != nil {
		a.updateHooks.SetPrereleases(on)
	}
}
