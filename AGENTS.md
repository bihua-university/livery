# AGENTS.md

Guidance for coding agents working in **Livery** (`github.com/bihua-university/livery`), a cross-platform desktop proxy client (supporting `mihomo` and `sing-box` cores) built with Go and [`github.com/egoist/mygo`](https://github.com/egoist/mygo).

## Build, Test & Verification Commands

- **Vet (includes MyGo UI static checks):**
  ```sh
  go tool mygo vet .
  ```
- **Run all tests (unit + real E2E via `mygo/plugins/automation`):**
  ```sh
  CGO_ENABLED=0 go test ./...
  ```
- **Run unit tests only (skip E2E):**
  ```sh
  CGO_ENABLED=0 go test -short ./...
  ```
- **Run locally:**
  ```sh
  go run .
  ```
- **Build desktop binary / installer (`mygo.json` outputs to `build/`):**
  ```sh
  go tool mygo build -platform windows/amd64 -skip-installer
  ```
  Omit `-skip-installer` to also generate platform installers/archives.

Note: `automation_e2e.go` (`//go:build e2e`) enables `github.com/egoist/mygo/plugins/automation` exclusively for `-tags e2e` test builds and is excluded from normal/release builds. E2E launches accept `LIVERY_DATA_DIR` and `MYGO_AUTOMATION` for isolated UI testing.

## Architecture & Directory Layout

- `main.go`: App entry point. Sets `debug.SetGCPercent(25)`, acquires single-instance lock, initializes `profile.Manager`, `core.Manager`, `app.App`, and the native MyGo OTA updater (`plugins/updater/native`), configures main window / system tray / global shortcut (`CmdOrCtrl+Shift+M`), and manages minimize-to-tray behavior.
- `mygo.json`: MyGo app manifest (`name: "Livery"`, `slug: "livery"`, `identifier: "io.github.bihua-university.livery"`, `out: "build"`, signed GitHub OTA `updates`). Signed release uploads (`go tool mygo build -upload`) require `MYGO_UPDATER_PRIVATE_KEY`.
- `internal/app/`: Central controller (`app.go`). Coordinates core lifecycle, Clash API polling, live traffic history, lazy/released state when hidden or switching away from `Logs`/`Rules`/editor modals, and marshals state updates back to the UI thread via `Window.Update`.
- `internal/core/`: Core subprocess supervisor (`manager.go`, `process_windows.go`, `process_other.go`), core binary downloader (`installer.go`), and GeoIP/GeoSite/ruleset downloader (`geodata.go`).
- `internal/profile/`: Persistent settings & profile storage (`manager.go`), runtime config compiler (`compiler.go`), and subscription/share-link converter (`converter.go`).
- `internal/clashapi/`: REST and streaming client for the local Clash/Mihomo controller API.
- `internal/sysproxy/`: Cross-platform OS proxy and bypass configuration (`sysproxy.go`, `sysproxy_windows.go`, `sysproxy_darwin.go`, `sysproxy_linux.go`, `sysproxy_other.go`).
- `internal/applog/`: Disk-backed log ring/store (`store.go`) to keep memory footprint low.
- `internal/model/`: Shared domain structs, enums, and default settings (`types.go`).
- `internal/theme/`, `internal/icons/`: Light/dark color palettes, typography scaling, and vector/tray icons.
- `internal/ui/`: MyGo UI layer:
  - `shell.go`: Main window frame, sidebar navigation, titlebar, and page routing.
  - `components/`: Reusable UI primitives (`card.go`, `buttons.go`, `badges.go`, `inputs.go`, `sliding.go`, `format.go`).
  - `pages/`: Feature views (`overview.go`, `proxies.go`, `profiles.go`, `connections.go`, `rules.go`, `logs.go`, `settings.go`, `modals.go`).
- `.github/workflows/release.yml`: Tag-triggered (`v*`) release workflow building Windows (`amd64`/`arm64`) and Linux (`amd64`/`arm64`) artifacts matching `mygo.json`'s `version`.

## MyGo UI & Concurrency Conventions

Detailed rules live in `.agents/skills/mygo-maintenance/SKILL.md` (read it before major UI edits or `mygo` upgrades), and interactive E2E driving instructions live in `.agents/skills/mygo-automation/SKILL.md`:

- **Context & Element lifetimes:** Never store `*ui.Context` or `ui.Element` across build passes or capture them in goroutines. Store `ui.Handle` (`.Bind(&handle)`) and persistent model data; use `c.Services()` or `win.Update(...)` for async callbacks.
- **Stable keys:** Assign unique `c.Key(...)` before constructing stateful/animated widgets and list items so collapse/expand animations and list scroll states remain stable across rebuilds.
- **Threading & I/O:** Keep network calls, subprocess start/stop/restart, and disk I/O off the UI thread; guard against concurrent restarts/port conflicts in `core.Manager` and `app.App`, and publish results asynchronously via `Window.Update`.
- **Verification before finishing:** Always run `go tool mygo vet .` and `CGO_ENABLED=0 go test ./...` (or `-short` in headless environments) after code changes.
