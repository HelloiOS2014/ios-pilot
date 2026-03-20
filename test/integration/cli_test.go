// Package integration contains end-to-end tests that exercise the compiled
// ios-pilot binary against a real iOS device. Gated by IOS_DEVICE_CONNECTED.
package integration

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Global state set up by TestMain
// ---------------------------------------------------------------------------

var (
	pilotBin  string // path to compiled ios-pilot binary
	configDir string // isolated config directory for the test daemon
)

func TestMain(m *testing.M) {
	if os.Getenv("IOS_DEVICE_CONNECTED") == "" {
		fmt.Println("IOS_DEVICE_CONNECTED not set — skipping integration tests")
		os.Exit(0)
	}

	tmpDir, err := os.MkdirTemp("", "ios-pilot-integration-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "create temp dir: %v\n", err)
		os.Exit(1)
	}

	pilotBin = filepath.Join(tmpDir, "ios-pilot")
	configDir = filepath.Join(tmpDir, "config")

	fmt.Printf("Building ios-pilot binary → %s\n", pilotBin)
	build := exec.Command("go", "build", "-o", pilotBin, "./cmd/ios-pilot")
	build.Dir = findProjectRoot()
	build.Stdout = os.Stdout
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "go build failed: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()

	cleanup := exec.Command(pilotBin, "daemon", "stop")
	cleanup.Env = append(os.Environ(), "IOS_PILOT_CONFIG_DIR="+configDir)
	_ = cleanup.Run()
	time.Sleep(500 * time.Millisecond)
	os.RemoveAll(tmpDir)
	os.Exit(code)
}

func findProjectRoot() string {
	dir, _ := os.Getwd()
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			d, _ := os.Getwd()
			return d
		}
		dir = parent
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

type runResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

func runPilot(t *testing.T, args ...string) runResult {
	t.Helper()
	cmd := exec.Command(pilotBin, args...)
	cmd.Env = append(os.Environ(), "IOS_PILOT_CONFIG_DIR="+configDir)

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("exec %v: %v", args, err)
		}
	}

	return runResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode,
	}
}

func runPilotJSON(t *testing.T, args ...string) map[string]any {
	t.Helper()
	r := runPilot(t, args...)
	if r.ExitCode != 0 {
		t.Fatalf("ios-pilot %v exited %d\nstdout: %s\nstderr: %s",
			args, r.ExitCode, r.Stdout, r.Stderr)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(r.Stdout), &result); err != nil {
		t.Fatalf("parse JSON from %v: %v\nraw: %s", args, err, r.Stdout)
	}
	return result
}

func runPilotJSONArray(t *testing.T, args ...string) []any {
	t.Helper()
	r := runPilot(t, args...)
	if r.ExitCode != 0 {
		t.Fatalf("ios-pilot %v exited %d\nstdout: %s\nstderr: %s",
			args, r.ExitCode, r.Stdout, r.Stderr)
	}
	var result []any
	if err := json.Unmarshal([]byte(r.Stdout), &result); err != nil {
		t.Fatalf("parse JSON array from %v: %v\nraw: %s", args, err, r.Stdout)
	}
	return result
}

// takeScreenshotHash calls `look`, reads the PNG file, returns its SHA-256.
func takeScreenshotHash(t *testing.T) string {
	t.Helper()
	result := runPilotJSON(t, "look")
	path, _ := result["screenshot"].(string)
	if path == "" {
		t.Fatal("screenshot path is empty")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read screenshot: %v", err)
	}
	if len(data) < 4 || data[0] != 0x89 || data[1] != 0x50 || data[2] != 0x4E || data[3] != 0x47 {
		t.Fatal("screenshot is not a valid PNG")
	}
	h := sha256.Sum256(data)
	return fmt.Sprintf("%x", h)
}

// getScreenSize returns (width, height) from a `look` result.
func getScreenSize(t *testing.T) (int, int) {
	t.Helper()
	result := runPilotJSON(t, "look")
	ss, ok := result["screen_size"].([]any)
	if !ok || len(ss) != 2 {
		t.Fatal("cannot get screen_size from look")
	}
	w, _ := ss[0].(float64)
	h, _ := ss[1].(float64)
	return int(w), int(h)
}

var wdaMode string

func requireWDA(t *testing.T) {
	t.Helper()
	if wdaMode != "full" {
		t.Skipf("WDA mode is %q (not full) — skipping", wdaMode)
	}
}

// ---------------------------------------------------------------------------
// Integration test suite
// ---------------------------------------------------------------------------

func TestIntegrationCLI(t *testing.T) {
	t.Cleanup(func() {
		// Best-effort: kill Settings, disconnect, stop daemon.
		run := func(args ...string) {
			cmd := exec.Command(pilotBin, args...)
			cmd.Env = append(os.Environ(), "IOS_PILOT_CONFIG_DIR="+configDir)
			_ = cmd.Run()
		}
		run("app", "kill", "com.apple.Preferences")
		run("device", "disconnect")
		run("daemon", "stop")
	})

	var connectedUDID string

	// 01 — Device List
	t.Run("01_DeviceList", func(t *testing.T) {
		devices := runPilotJSONArray(t, "device", "list")
		if len(devices) == 0 {
			t.Fatal("expected at least one device")
		}
		dev := devices[0].(map[string]any)
		for _, key := range []string{"udid", "name", "ios_version"} {
			if _, exists := dev[key]; !exists {
				t.Errorf("device missing field %q", key)
			}
		}
		t.Logf("found %d device(s), first: %v", len(devices), dev["name"])
	})

	// 02 — Device Connect
	t.Run("02_DeviceConnect", func(t *testing.T) {
		result := runPilotJSON(t, "device", "connect")
		if connected, _ := result["connected"].(bool); !connected {
			t.Fatalf("expected connected=true, got %v", result)
		}
		connectedUDID, _ = result["udid"].(string)
		if connectedUDID == "" {
			t.Fatal("connected but udid is empty")
		}
		if wdaObj, ok := result["wda"].(map[string]any); ok {
			wdaMode, _ = wdaObj["mode"].(string)
		}
		t.Logf("connected: udid=%s wda.mode=%s", connectedUDID, wdaMode)
	})

	// 03 — Device Status
	t.Run("03_DeviceStatus", func(t *testing.T) {
		result := runPilotJSON(t, "device", "status")
		if connected, _ := result["connected"].(bool); !connected {
			t.Fatal("expected connected=true")
		}
		if udid, _ := result["udid"].(string); udid != connectedUDID {
			t.Errorf("udid mismatch: got %q, want %q", udid, connectedUDID)
		}
	})

	// 04 — Screenshot
	t.Run("04_Screenshot", func(t *testing.T) {
		r := runPilot(t, "look")
		if r.ExitCode != 0 {
			// On iOS 17+ in degraded mode, instruments screenshot requires
			// Developer Image which is unavailable. Skip rather than fail.
			if wdaMode == "degraded" && strings.Contains(r.Stderr, "screenshot") {
				t.Skipf("screenshot unavailable in degraded mode: %s", strings.TrimSpace(r.Stderr))
			}
			t.Fatalf("ios-pilot look exited %d\nstdout: %s\nstderr: %s", r.ExitCode, r.Stdout, r.Stderr)
		}
		var result map[string]any
		if err := json.Unmarshal([]byte(r.Stdout), &result); err != nil {
			t.Fatalf("parse JSON: %v\nraw: %s", err, r.Stdout)
		}
		path, _ := result["screenshot"].(string)
		if path == "" {
			t.Fatal("screenshot path is empty")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read screenshot file: %v", err)
		}
		if len(data) < 4 || data[0] != 0x89 || data[1] != 0x50 || data[2] != 0x4E || data[3] != 0x47 {
			t.Fatalf("not a PNG: first bytes %x", data[:4])
		}
		if ss, ok := result["screen_size"].([]any); ok {
			for i, v := range ss {
				if dim, _ := v.(float64); dim <= 0 {
					t.Errorf("screen_size[%d] invalid: %v", i, v)
				}
			}
		}
		t.Logf("screenshot: %s (%d bytes)", path, len(data))
	})

	// 05 — App List
	t.Run("05_AppList", func(t *testing.T) {
		apps := runPilotJSONArray(t, "app", "list")
		for i, a := range apps {
			app, ok := a.(map[string]any)
			if !ok {
				t.Errorf("app[%d] is not an object", i)
				continue
			}
			if _, exists := app["bundle_id"]; !exists {
				t.Errorf("app[%d] missing bundle_id", i)
			}
		}
		t.Logf("found %d user app(s)", len(apps))
	})

	// 06 — App Launch Settings (instruments → WDA fallback)
	t.Run("06_AppLaunch", func(t *testing.T) {
		requireWDA(t) // WDA fallback needs full mode
		result := runPilotJSON(t, "app", "launch", "com.apple.Preferences")
		if status, _ := result["status"].(string); status != "launched" {
			t.Errorf("expected status=launched, got %q", status)
		}
		time.Sleep(2 * time.Second)
		t.Logf("launched Settings: %v", result)
	})

	// 07 — App Foreground
	t.Run("07_AppForeground", func(t *testing.T) {
		requireWDA(t)
		result := runPilotJSON(t, "app", "foreground")
		bid, _ := result["bundle_id"].(string)
		if bid == "" {
			t.Error("expected non-empty foreground bundle_id")
		}
		t.Logf("foreground app: %s", bid)
	})

	// 08 — Check App Running
	t.Run("08_CheckAppRunning", func(t *testing.T) {
		requireWDA(t)
		result := runPilotJSON(t, "check", "app-running", "com.apple.Preferences")
		if pass, _ := result["pass"].(bool); !pass {
			t.Errorf("expected pass=true, got %v (detail: %v)", result["pass"], result["detail"])
		}
	})

	// 09 — Annotated Screenshot with elements (WDA required)
	t.Run("09_AnnotatedScreenshot", func(t *testing.T) {
		requireWDA(t)
		result := runPilotJSON(t, "look", "--annotate")
		path, _ := result["screenshot"].(string)
		if path == "" {
			t.Fatal("screenshot path is empty")
		}
		elements, _ := result["elements"].([]any)
		if len(elements) == 0 {
			t.Fatal("expected non-empty elements — WDA full mode but returned nothing")
		}
		if !strings.Contains(filepath.Base(path), "annotated") {
			t.Errorf("expected filename to contain 'annotated', got %q", filepath.Base(path))
		}
		t.Logf("annotated screenshot: %s, %d elements", path, len(elements))
	})

	// 10 — UI Screenshot with elements (WDA required)
	t.Run("10_UIScreenshot", func(t *testing.T) {
		requireWDA(t)
		result := runPilotJSON(t, "look", "--ui")
		path, _ := result["screenshot"].(string)
		if path == "" {
			t.Fatal("screenshot path is empty")
		}
		if strings.Contains(filepath.Base(path), "annotated") {
			t.Errorf("--ui should not produce 'annotated' filename: %q", filepath.Base(path))
		}
		elements, _ := result["elements"].([]any)
		if len(elements) == 0 {
			t.Fatal("expected non-empty elements for --ui")
		}
		t.Logf("ui screenshot: %s, %d elements", path, len(elements))
	})

	// 11 — Swipe Down in Settings: screenshot must change (WDA required)
	t.Run("11_SwipeDown", func(t *testing.T) {
		requireWDA(t)
		_, h := getScreenSize(t)
		before := takeScreenshotHash(t)

		// Swipe from center-down to center-up (scroll content down).
		midX := 200
		runPilotJSON(t, "act", "swipe",
			fmt.Sprint(midX), fmt.Sprint(h*3/4),
			fmt.Sprint(midX), fmt.Sprint(h/4))
		time.Sleep(2 * time.Second)

		after := takeScreenshotHash(t)
		if before == after {
			t.Error("screen did NOT change after swipe down — action had no visible effect")
		}
	})

	// 12 — Swipe Up: scroll back, screenshot must change (WDA required)
	t.Run("12_SwipeUp", func(t *testing.T) {
		requireWDA(t)
		_, h := getScreenSize(t)
		before := takeScreenshotHash(t)

		midX := 200
		runPilotJSON(t, "act", "swipe",
			fmt.Sprint(midX), fmt.Sprint(h/4),
			fmt.Sprint(midX), fmt.Sprint(h*3/4))
		time.Sleep(2 * time.Second)

		after := takeScreenshotHash(t)
		if before == after {
			t.Error("screen did NOT change after swipe up — action had no visible effect")
		}
	})

	// 13 — Tap on a real element: use `look --ui` to find a target (WDA required)
	t.Run("13_Tap", func(t *testing.T) {
		requireWDA(t)
		// Get element tree, find a tappable element.
		result := runPilotJSON(t, "look", "--ui")
		elements, _ := result["elements"].([]any)
		if len(elements) == 0 {
			t.Fatal("no elements to tap on")
		}

		// Pick the first element with a non-empty label and a valid center.
		var tapX, tapY int
		var tapLabel string
		for _, e := range elements {
			el, ok := e.(map[string]any)
			if !ok {
				continue
			}
			label, _ := el["label"].(string)
			center, _ := el["center"].([]any)
			if label != "" && len(center) == 2 {
				cx, _ := center[0].(float64)
				cy, _ := center[1].(float64)
				if cx > 0 && cy > 0 {
					tapX, tapY = int(cx), int(cy)
					tapLabel = label
					break
				}
			}
		}
		if tapX == 0 && tapY == 0 {
			t.Fatal("could not find a labeled element with valid coordinates")
		}

		before := takeScreenshotHash(t)
		t.Logf("tapping element %q at (%d, %d)", tapLabel, tapX, tapY)

		r := runPilotJSON(t, "act", "tap", fmt.Sprint(tapX), fmt.Sprint(tapY))
		if status, _ := r["status"].(string); status != "ok" {
			t.Fatalf("tap failed: status=%q", status)
		}
		time.Sleep(2 * time.Second)

		after := takeScreenshotHash(t)
		if before == after {
			t.Error("screen did NOT change after tapping element — action had no visible effect")
		}
	})

	// 14 — Press Home: go back to home screen, screenshot must change (WDA required)
	t.Run("14_PressHome", func(t *testing.T) {
		requireWDA(t)
		before := takeScreenshotHash(t)

		r := runPilotJSON(t, "act", "press", "home")
		if status, _ := r["status"].(string); status != "ok" {
			t.Fatalf("press home failed: status=%q", status)
		}
		time.Sleep(2 * time.Second)

		after := takeScreenshotHash(t)
		if before == after {
			t.Error("screen did NOT change after press home")
		}
	})

	// 15 — Screenshot After Home
	t.Run("15_ScreenshotAfterHome", func(t *testing.T) {
		r := runPilot(t, "look")
		if r.ExitCode != 0 {
			if wdaMode == "degraded" && strings.Contains(r.Stderr, "screenshot") {
				t.Skipf("screenshot unavailable in degraded mode: %s", strings.TrimSpace(r.Stderr))
			}
			t.Fatalf("ios-pilot look exited %d\nstdout: %s\nstderr: %s", r.ExitCode, r.Stdout, r.Stderr)
		}
		var result map[string]any
		if err := json.Unmarshal([]byte(r.Stdout), &result); err != nil {
			t.Fatalf("parse JSON: %v\nraw: %s", err, r.Stdout)
		}
		path, _ := result["screenshot"].(string)
		if path == "" {
			t.Fatal("screenshot path is empty")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read screenshot: %v", err)
		}
		if len(data) < 4 || data[0] != 0x89 || data[1] != 0x50 || data[2] != 0x4E || data[3] != 0x47 {
			t.Error("not a valid PNG")
		}
		t.Logf("screenshot after home: %d bytes", len(data))
	})

	// 16 — Check Element (WDA required)
	t.Run("16_CheckElement", func(t *testing.T) {
		requireWDA(t)
		// Re-launch Settings.
		runPilotJSON(t, "app", "launch", "com.apple.Preferences")
		time.Sleep(2 * time.Second)

		result := runPilotJSON(t, "check", "element", "--text", "General")
		if pass, _ := result["pass"].(bool); !pass {
			t.Errorf("expected pass=true for 'General', got %v (detail: %v)", result["pass"], result["detail"])
		}
	})

	// 17 — App Kill
	t.Run("17_AppKill", func(t *testing.T) {
		requireWDA(t)
		result := runPilotJSON(t, "app", "kill", "com.apple.Preferences")
		if status, _ := result["status"].(string); status != "killed" {
			t.Errorf("expected status=killed, got %q", status)
		}
	})

	// 18 — Daemon Status
	t.Run("18_DaemonStatus", func(t *testing.T) {
		result := runPilotJSON(t, "daemon", "status")
		if status, _ := result["status"].(string); status != "running" {
			t.Errorf("expected status=running, got %q", status)
		}
	})

	// 19 — Device Disconnect
	t.Run("19_DeviceDisconnect", func(t *testing.T) {
		result := runPilotJSON(t, "device", "disconnect")
		if status, _ := result["status"].(string); status != "disconnected" {
			t.Errorf("expected status=disconnected, got %q", status)
		}
	})

	// 20 — Status After Disconnect
	t.Run("20_StatusAfterDisconnect", func(t *testing.T) {
		result := runPilotJSON(t, "device", "status")
		if connected, _ := result["connected"].(bool); connected {
			t.Error("expected connected=false after disconnect")
		}
	})
}
