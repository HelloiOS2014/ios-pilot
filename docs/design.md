# ios-pilot Design Spec

## Overview

ios-pilot is a CLI tool + auto-managed daemon for AI agents to operate real iOS devices. It enables LLMs to perform development self-testing (build → install → UI interaction → verification → debug) and occasional device control tasks.

**Replaces**: ios-debug-mcp (deprecated)

**Design Principles**:
- Visual-first: screenshots are the primary way LLMs understand device state; UI tree is optional enhancement
- Minimal tool surface: 6 primary commands + 2 management commands, LLMs don't need to understand iOS automation internals
- Progressive dependency: go-ios required (device ops + screenshots + logs), WDA required for UI interaction (tap/swipe/input) but auto-managed, xcodebuild optional (builds, managed by LLM directly)
- Auto-managed lifecycle: daemon starts on first use, exits on idle timeout

## Architecture

```
User / LLM
    │
    │  ios-pilot <command>
    │
    ▼
┌─────────────────────────────────────────────────────┐
│  CLI (thin client)                                   │
│  Connects to daemon via Unix domain socket           │
│  Auto-starts daemon if not running                   │
└──────────────────────┬──────────────────────────────┘
                       │ ~/.config/ios-pilot/pilot.sock
                       │ JSON-RPC over Unix socket
                       ▼
┌─────────────────────────────────────────────────────┐
│  Daemon (auto-managed, idle timeout)                 │
│                                                      │
│  ┌─────────────────────────────────────────────┐    │
│  │  Tool Layer (6 primary + 2 management)        │    │
│  │  device │ look │ act │ app │ log │ check    │    │
│  │  wda │ daemon                                │    │
│  └──────────────────────┬──────────────────────┘    │
│                         │                            │
│  ┌──────────────────────┴──────────────────────┐    │
│  │  Core Layer                                  │    │
│  │  DeviceManager ── device discovery/session   │    │
│  │  ScreenCapture ── screenshot + annotation    │    │
│  │  UiController  ── UI ops dispatch + degrade  │    │
│  │  AppManager    ── app lifecycle              │    │
│  │  LogManager    ── syslog stream + crash logs │    │
│  └──────────────────────┬──────────────────────┘    │
│                         │                            │
│  ┌──────────────────────┴──────────────────────┐    │
│  │  Driver Layer                                │    │
│  │  GoIosDriver ── go-ios as Go library import  │    │
│  │  WdaDriver   ── WDA W3C WebDriver HTTP       │    │
│  └─────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────┘
```

### Layer Responsibilities

**CLI Layer**: Thin client. Checks if daemon is running (via socket), auto-starts if not, sends JSON-RPC request, prints response. No business logic.

**Daemon**: Long-running background process managing all stateful resources (device connection, WDA session, syslog stream). Auto-starts on first CLI call, auto-exits after idle timeout.

**Tool Layer**: 6 commands implementing the user-facing API. Each command maps to one or more Core Layer calls.

**Core Layer**: Business logic modules, each with a single responsibility. Communicates with drivers through interfaces (swappable).

**Driver Layer**: Wraps external tools behind stable interfaces.
- `GoIosDriver`: imports go-ios as a Go library (not CLI calls). Handles device enumeration, app management, screenshots, syslog, iOS 17+ tunnel management.
- `GoIosWDAProcessDriver`: uses go-ios `testmanagerd.RunTestWithConfig` to programmatically launch WDA on the device, without needing xcodebuild or go-ios CLI. Automatically discovers the WDA bundle ID from installed apps.
- `WdaDriver`: HTTP client for WebDriverAgent W3C WebDriver protocol. Handles all UI interactions (tap, swipe, input), UI element tree, text input. **WDA is the only way to inject touch events on real iOS devices.**

### Key Design Decision: go-ios as Library

The current ios-debug-mcp calls pymobiledevice3 CLI and parses text output with regex. This is the single biggest source of fragility.

ios-pilot imports go-ios as a Go package (`github.com/danielpaulus/go-ios/ios`), getting structured data directly. No CLI text parsing, no subprocess spawning for device operations.

### Key Design Decision: iOS 17+ Tunnel Handling

iOS 17+ devices require a tunnel daemon for device communication. This is an iOS platform constraint that no tool can eliminate. The difference from the old approach:

- **Old (pymobiledevice3)**: User must manually run `sudo pymobiledevice3 remote tunneld --protocol quic` in a separate terminal
- **New (go-ios)**: The ios-pilot daemon auto-manages the tunnel via go-ios's `tunnel` package. On `device connect`, if no tunnel is detected, the daemon attempts to start one. If elevated privileges are required, the user is prompted once with a clear message.

The tunnel runs as part of the daemon process and is cleaned up on daemon exit.

### Key Design Decision: No MCP

ios-pilot manages a physical device (stateful: device connection, WDA session, log streams). MCP's model of "each LLM host spawns its own child process" doesn't fit stateful physical resource management. Multiple LLM hosts would fight over the same device.

Instead: single daemon process, CLI access via Unix socket. All LLM platforms (Claude Code, Gemini CLI, Codex) call CLI commands via their shell/bash execution capability.

### Key Design Decision: No Build Wrapping

xcodebuild is not wrapped by ios-pilot because:
- LLMs already know how to run xcodebuild via shell
- Build configurations vary wildly between projects
- Build is stateless, no benefit from daemon model
- ios-pilot focuses on what LLMs can't easily do themselves: device interaction, WDA management, screenshots

## Communication Protocol

**Transport**: Unix domain socket at `~/.config/ios-pilot/pilot.sock`

**Protocol**: JSON-RPC 2.0

**Request/Response example**:
```json
// Request
{"jsonrpc":"2.0","id":1,"method":"look","params":{"annotate":true}}

// Response
{"jsonrpc":"2.0","id":1,"result":{
  "screenshot":"~/.config/ios-pilot/screenshots/annotated-1710648000.png",
  "mode":"full",
  "screen_size":[390,844],
  "elements":[
    {"id":1,"type":"button","label":"Login","frame":[150,400,90,44]},
    {"id":2,"type":"textfield","label":"Username","frame":[50,300,290,40]}
  ]
}}
```

**Streaming** (for `log --follow`):

Uses JSON-RPC notifications (no `id`) for stream data, keeping the protocol spec-compliant:

```json
// Request
{"jsonrpc":"2.0","id":2,"method":"log","params":{"follow":true}}

// Ack response (confirms stream started)
{"jsonrpc":"2.0","id":2,"result":{"stream_id":"s1","status":"streaming"}}

// Stream data as notifications (no id, spec-compliant)
{"jsonrpc":"2.0","method":"log.line","params":{"stream_id":"s1","line":"2026-03-17 10:00:01 MyApp[1234] INFO: started"}}
{"jsonrpc":"2.0","method":"log.line","params":{"stream_id":"s1","line":"2026-03-17 10:00:02 MyApp[1234] ERROR: nil pointer"}}
// ... continues until client disconnects or sends cancel
```

## Tool Specifications

### `device` — Device Management

```bash
ios-pilot device list                    # List all connected iOS devices
ios-pilot device connect [udid]          # Connect to device (auto-select if only one)
ios-pilot device status                  # Connection status + WDA availability + foreground app
ios-pilot device disconnect              # Disconnect
```

**On connect**:
1. go-ios connects to device
2. Ensure tunnel (iOS 17+)
3. Forward port 8100 (localhost → device WDA)
4. Probe WDA → if not responding, launch WDA via `testmanagerd.RunTestWithConfig` (auto-discovers bundle ID)
5. Create WDA session → report mode: "full" (with WDA) or "degraded" (without WDA)

**Output** (`device status`):
```json
{
  "connected": true,
  "udid": "00008030-000A1234567890AB",
  "name": "iPhone 15 Pro",
  "ios_version": "18.2",
  "wda": {"status": "running", "mode": "full"},
  "foreground_app": "com.example.myapp",
  "daemon": {"uptime": "12m", "idle_timeout": "30m"}
}
```

### `look` — Observe Device State

```bash
ios-pilot look                           # Screenshot only (fast, no WDA needed)
ios-pilot look --ui                      # Screenshot + UI element tree as JSON
ios-pilot look --annotate                # Screenshot with numbered interactive element overlays
```

**`--annotate` process**:
1. Take screenshot via go-ios
2. Fetch UI tree via WDA
3. Filter interactive elements (button, textfield, switch, link, cell)
4. Draw semi-transparent bounding boxes + numbered labels on screenshot using Go `image/draw`
5. Return annotated image path + element list JSON

**Output**:
```json
{
  "screenshot": "~/.config/ios-pilot/screenshots/annotated-1710648000.png",
  "mode": "full",
  "screen_size": [390, 844],
  "elements": [
    {"id": 1, "type": "button", "label": "Login", "frame": [150, 400, 90, 44], "center": [195, 422]},
    {"id": 2, "type": "textfield", "label": "Username", "frame": [50, 300, 290, 40], "center": [195, 320]},
    {"id": 3, "type": "textfield", "label": "Password", "frame": [50, 350, 290, 40], "center": [195, 370]},
    {"id": 4, "type": "switch", "label": "Remember me", "frame": [50, 450, 51, 31], "center": [75, 465]}
  ]
}
```

Each element includes pre-computed `center` coordinates. The LLM uses these directly with `act tap <x> <y>` — no stateful element ID cache needed. The numbered labels on the annotated screenshot help the LLM visually identify which element is which, then it uses the corresponding center coordinates.

**Degraded mode** (no WDA): returns screenshot only, `"mode": "degraded"`, empty elements array. LLM uses its own vision capability to analyze the screenshot.

### `act` — Execute Actions

**All `act` commands require WDA.** There is no public API for injecting touch events on real iOS devices without WDA. This is an iOS platform constraint.

```bash
ios-pilot act tap <x> <y>               # Tap at coordinates (via WDA)
ios-pilot act swipe <x1> <y1> <x2> <y2> # Swipe gesture (via WDA)
ios-pilot act input "<text>"             # Type text into focused field (via WDA)
ios-pilot act press <key>                # Press key: home, lock, volumeup, volumedown
```

**Typical workflow with `look --annotate`**:
```
LLM: ios-pilot look --annotate
     → sees element 1 "Login" at center [195, 422]
LLM: ios-pilot act tap 195 422
```

The LLM reads center coordinates from the `look --annotate` output and passes them to `act tap`. No stateful element cache — fully stateless, safe for concurrent clients.

**`act press`**: Hardware button presses (home, lock, volume) may work without WDA via go-ios `instruments` package. If unavailable, falls back to WDA.

### `app` — Application Management

```bash
ios-pilot app list                       # List installed apps
ios-pilot app install <path>             # Install .app or .ipa
ios-pilot app launch <bundle_id>         # Launch app
ios-pilot app kill <bundle_id>           # Terminate app
ios-pilot app uninstall <bundle_id>      # Uninstall app
ios-pilot app foreground                 # Get foreground app bundle ID
```

All operations via go-ios library. No WDA dependency.

### `log` — Logs & Crash Reports

```bash
ios-pilot log                            # Recent logs (default 50 lines)
ios-pilot log -n 200                     # Recent 200 lines
ios-pilot log --follow                   # Stream logs in real-time
ios-pilot log --filter <bundle_id>       # Filter by app
ios-pilot log --level error              # Filter by level
ios-pilot log --search "<text>"          # Text search in buffer
ios-pilot log crash                      # List crash reports
ios-pilot log crash <id>                 # View specific crash report
```

**Syslog collection**: Daemon continuously collects syslog via go-ios into an in-memory ring buffer (configurable size, default 2000 entries). CLI queries read from this buffer.

**`--follow`**: Opens a streaming JSON-RPC connection. New log lines pushed to client in real-time.

**Crash reports**: Fetched on-demand from device via go-ios.

**Output** (`log crash`):
```json
[
  {"id": "crash-001", "name": "MyApp-2026-03-17-100001.ips", "process": "MyApp", "timestamp": "2026-03-17T10:00:01Z"},
  {"id": "crash-002", "name": "MyApp-2026-03-17-093012.ips", "process": "MyApp", "timestamp": "2026-03-17T09:30:12Z"}
]
```
`log crash <id>` accepts the `id` field from the listing output.

### `check` — Assertions

```bash
ios-pilot check screen                   # Take screenshot for LLM visual verification
ios-pilot check element --text "<text>"  # UI tree check: element with text exists? (requires WDA)
ios-pilot check app-running <bundle_id>  # Is app in foreground?
ios-pilot check no-crash <bundle_id>     # No recent crashes for this app?
```

**`check screen`**: Takes a screenshot and returns it. The LLM uses its own multimodal vision to verify the screen content. This aligns with the visual-first design principle and avoids introducing an OCR dependency.

**`check element`**: Searches the WDA UI tree for an element matching the text. Requires WDA.

**Output**:
```json
{
  "pass": true,
  "detail": "Found element with text 'Welcome'",
  "screenshot": "~/.config/ios-pilot/screenshots/check-1710648100.png"
}
```

For `check screen`, `pass` is always `null` (LLM decides), with the screenshot path provided. For `check element`, `check app-running`, and `check no-crash`, `pass` is `true`/`false` determined programmatically.

Every check captures a screenshot for evidence, regardless of pass/fail.

## WDA Management

### First-Time Setup

WDA must be installed once on the device (build-for-testing via Xcode). ios-pilot provides a guided setup:

```bash
ios-pilot wda setup
# 1. Checks if WDA is already installed on device
# 2. If not, guides user through Xcode signing + build-for-testing
# 3. Verifies WDA is accessible
# 4. Saves WDA bundle ID to config
```

After initial installation, WDA is fully auto-managed — `device connect` launches it via `testmanagerd.RunTestWithConfig`, no need to keep xcodebuild running.

### Automatic Lifecycle

```
device connect
    │
    ├── WDA installed? → No → degraded mode, suggest "ios-pilot wda setup"
    │
    ├── WDA running? → Yes → verify session → full mode
    │
    └── WDA not running? → testmanagerd.RunTestWithConfig → wait ready → full mode

daemon running:
    │
    ├── Health check every 30s (GET /status)
    ├── WDA crashed → auto-restart (max 3 attempts)
    ├── 3 failures → switch to degraded mode, log warning
    └── WDA session expired → auto-recreate
```

### Degraded Mode

When WDA is unavailable, ios-pilot continues working with observation-only capability. **All touch/gesture/input operations require WDA** — there is no public API for touch injection on real iOS devices without it.

| Command | Full Mode | Degraded Mode |
|---------|-----------|---------------|
| `look` | screenshot + UI tree + annotate | screenshot only |
| `look --annotate` | works | unavailable (no UI tree) |
| `act tap` | works | **unavailable** |
| `act swipe` | works | **unavailable** |
| `act input` | works | **unavailable** |
| `act press` | works | partial (hardware buttons only, via go-ios) |
| `check screen` | works | works (screenshot for LLM visual check) |
| `check element` | works | unavailable |
| `check app-running` | works | works |
| `check no-crash` | works | works |
| `app/*` | works | works |
| `log/*` | works | works |
| `device/*` | works | works |

**Degraded mode is still useful for**: installing apps, launching them, taking screenshots to verify initial state, reading logs to debug crashes. The LLM can do a "build → install → launch → screenshot → read logs" cycle without WDA. But multi-step UI interaction flows (login, navigate, fill forms) require WDA.

**Implication**: WDA setup should be strongly encouraged. The `device connect` output clearly indicates WDA status and provides setup instructions if missing.

### `wda` — WDA Management (setup command)

```bash
ios-pilot wda setup                      # Guided WDA installation on device
ios-pilot wda status                     # WDA running? Session valid? Bundle ID?
ios-pilot wda restart                    # Force restart WDA
```

`wda setup` guides the user through first-time WDA installation:
1. Check if WDA bundle is already on device
2. If not, open WDA Xcode project, guide signing and build
3. Verify WDA responds on device
4. Save bundle ID to config

This is a setup-only command, not used during normal operation (WDA is auto-managed by daemon).

### `daemon` — Daemon Management

```bash
ios-pilot daemon status                  # Running? PID? Uptime? Idle time? Connected device?
ios-pilot daemon stop                    # Graceful shutdown
```

**`daemon status`**: Reads PID file at `~/.config/ios-pilot/pilot.pid`. If PID is alive, connects to socket for full status. If daemon is not running, reports "not running" without error.

**`daemon stop`**: Sends SIGTERM to the daemon PID. Daemon performs graceful shutdown (disconnect WDA, stop syslog, remove socket, remove PID file).

**Output** (`daemon status`):
```json
{
  "running": true,
  "pid": 12345,
  "uptime": "12m",
  "idle_time": "3m",
  "idle_timeout": "30m",
  "device": "iPhone 15 Pro (00008030-...)",
  "wda": "running"
}
```

## Error Handling

### JSON-RPC Error Format

All errors follow JSON-RPC 2.0 standard:
```json
{"jsonrpc":"2.0","id":1,"error":{"code":-32001,"message":"Device not connected","data":{"hint":"Run: ios-pilot device connect"}}}
```

### Error Codes

| Code | Name | Description |
|------|------|-------------|
| -32001 | `DEVICE_NOT_CONNECTED` | No device connected. Run `device connect` first. |
| -32002 | `DEVICE_NOT_FOUND` | Specified UDID not found in connected devices. |
| -32003 | `WDA_UNAVAILABLE` | WDA not running. Command requires WDA. |
| -32004 | `WDA_NOT_INSTALLED` | WDA not installed on device. Run `wda setup`. |
| -32005 | `OPERATION_TIMEOUT` | Command timed out (default 30s). |
| -32006 | `APP_NOT_FOUND` | Bundle ID not found on device. |
| -32007 | `TUNNEL_FAILED` | iOS 17+ tunnel setup failed. May need elevated privileges. |
| -32600 | `INVALID_REQUEST` | Malformed request (JSON-RPC standard). |
| -32601 | `METHOD_NOT_FOUND` | Unknown command (JSON-RPC standard). |

Each error includes a `data.hint` field with actionable guidance for the LLM.

## Service Lifecycle

### Auto-Start

```
ios-pilot <any command>
    │
    ├── Check pilot.sock exists and is connectable?
    │   ├── Yes → send request
    │   └── No → acquire file lock (~/.config/ios-pilot/pilot.lock)
    │          → double-check sock (another process may have started daemon)
    │          → fork daemon in background
    │          → daemon writes PID to ~/.config/ios-pilot/pilot.pid
    │          → wait for sock to appear (timeout 5s)
    │          → release lock
    │          → send request
```

The file lock prevents race conditions when multiple CLI invocations happen simultaneously before the daemon is up. The 5s timeout covers daemon startup only — device/WDA connection happens lazily on `device connect`, not on daemon start.

### Auto-Stop (Idle Timeout)

```
Daemon running
    │
    ├── Each request resets idle timer
    ├── Timer expires (default 30 min) →
    │   ├── Disconnect WDA session
    │   ├── Stop syslog stream
    │   ├── Disconnect device
    │   ├── Remove pilot.sock
    │   └── Process exit
```

### Manual Control

```bash
ios-pilot daemon status                  # Daemon running? PID? Uptime? Idle time?
ios-pilot daemon stop                    # Graceful shutdown
```

## Configuration

**File**: `~/.config/ios-pilot/config.json`

```json
{
  "idle_timeout": "30m",
  "log_buffer_size": 2000,
  "wda": {
    "auto_start": true,
    "bundle_id": "com.facebook.WebDriverAgentRunner.xctrunner",
    "health_interval": "30s",
    "max_restart": 3
  },
  "screenshot": {
    "dir": "~/.config/ios-pilot/screenshots",
    "retention_hours": 24,
    "max_count": 200
  },
  "annotate": {
    "box_color": "#FF0000",
    "label_size": 14,
    "interactive_types": ["button", "textfield", "switch", "link", "cell"]
  }
}
```

Zero configuration required for default usage.

## LLM Integration

### Discovery Layers

| Layer | Mechanism | Platforms |
|-------|-----------|-----------|
| **Awareness** | CLAUDE.md / AGENTS.md / GEMINI.md mention | All |
| **Discovery** | `ios-pilot --help` self-description | All |
| **Workflow** | Claude Code Skill with full self-test workflow | Claude Code (primary) |

### Skill Workflow (Claude Code)

Triggered when LLM needs to verify functionality on a real iOS device.

```
1. Confirm device: ios-pilot device status
2. Build & install: xcodebuild ... → ios-pilot app install → ios-pilot app launch
3. Observe: ios-pilot look --annotate
4. Interact: ios-pilot act tap <x> <y> (using center coords from annotate output)
5. Verify: ios-pilot check screen (LLM visually verifies screenshot)
6. Debug (on failure): ios-pilot log --filter <bundle_id>
7. Iterate: back to step 3

Principles:
- Always look before acting
- Use look --annotate to get element coordinates, then act tap with those coordinates
- check screen returns a screenshot — LLM decides pass/fail visually
- On check failure, examine logs before concluding it's a code bug
```

### Integration with dev-pipeline

The ios-pilot skill can be invoked during dev-pipeline's self-test phase for iOS platform tasks.

## Project Structure

```
ios-pilot/
├── cmd/
│   └── ios-pilot/
│       └── main.go              # CLI entry point + daemon fork
├── internal/
│   ├── cli/                     # CLI command parsing + socket client
│   │   ├── root.go
│   │   ├── device.go
│   │   ├── look.go
│   │   ├── act.go
│   │   ├── app.go
│   │   ├── log.go
│   │   ├── check.go
│   │   ├── wda.go
│   │   └── daemon.go
│   ├── daemon/                  # Daemon lifecycle + JSON-RPC server
│   │   ├── server.go            # Unix socket listener + request router
│   │   ├── lifecycle.go         # Auto-start, idle timeout, graceful shutdown
│   │   └── state.go             # Device session state (no mutable annotation cache)
│   ├── core/                    # Business logic
│   │   ├── device_manager.go    # Device discovery, connection, session
│   │   ├── screen_capture.go    # Screenshot + annotation drawing
│   │   ├── ui_controller.go     # UI ops dispatch (WDA or degraded)
│   │   ├── app_manager.go       # App lifecycle
│   │   ├── log_manager.go       # Syslog ring buffer + crash logs
│   │   └── check.go             # Assertion logic
│   └── driver/                  # External tool wrappers
│       ├── goios/               # go-ios library integration
│       │   ├── device.go
│       │   ├── app.go
│       │   ├── screenshot.go
│       │   ├── syslog.go
│       │   ├── tunnel.go       # iOS 17+ tunnel management
│       │   └── wda_process.go  # WDA process launch via testmanagerd
│       └── wda/                 # WDA HTTP client
│           ├── client.go        # W3C WebDriver protocol
│           ├── session.go       # Session management + health check
│           └── elements.go      # Element tree parsing
├── skills/
│   └── ios-pilot/
│       └── SKILL.md             # Claude Code skill for iOS self-test workflow
├── config.example.json
├── Makefile
├── go.mod
└── README.md
```

## Dependencies

**Required**:
- Go 1.22+ (build time only)
- go-ios (`github.com/danielpaulus/go-ios`) — imported as Go library
- macOS (for Xcode toolchain access)
- iOS device connected via USB

**Optional**:
- WebDriverAgent installed on device (for full mode)
- Xcode (for building apps and initial WDA setup)

**No runtime dependencies**: single compiled binary, no Node.js/Python required.

## Migration from ios-debug-mcp

ios-debug-mcp is fully replaced. Key differences:

| Aspect | ios-debug-mcp (old) | ios-pilot (new) |
|--------|---------------------|-----------------|
| Language | TypeScript | Go |
| iOS backend | pymobiledevice3 CLI + regex parsing | go-ios as Go library import |
| Communication | MCP (stdin/stdout JSON-RPC) | Unix socket daemon + CLI |
| WDA setup | Manual build + tunnel + port forward | One-time install, auto-managed |
| Tunnel | Required for iOS 17+ (manual `sudo tunneld`) | Auto-managed by daemon (go-ios tunnel package) |
| Tool count | 33 granular tools | 6 primary + 2 management commands |
| WDA dependency | Required for most operations | Optional (degraded mode available) |
| Service model | Spawned per LLM host | Single daemon, shared by all clients |
| Screenshot annotation | None | Built-in element labeling |
