package daemon

import (
	"encoding/json"
	"fmt"
	"runtime"
	"time"

	"ios-pilot/internal/config"
	"ios-pilot/internal/core"
)

// State holds shared references to all core managers.
// Populated when daemon starts, used by handler registration.
type State struct {
	DeviceManager *core.DeviceManager
	AppManager    *core.AppManager
	ScreenCapture *core.ScreenCapture
	UiController  *core.UiController
	LogManager    *core.LogManager
	Checker       *core.Checker
	Config        *config.Config
	StartTime     time.Time
}

// RegisterHandlers wires JSON-RPC methods to State's managers.
func (s *State) RegisterHandlers(srv *Server) {
	// Device commands
	srv.Handle("device.list", s.handleDeviceList)
	srv.Handle("device.connect", s.handleDeviceConnect)
	srv.Handle("device.status", s.handleDeviceStatus)
	srv.Handle("device.disconnect", s.handleDeviceDisconnect)

	// Look commands
	srv.Handle("look", s.handleLook)

	// Act commands
	srv.Handle("act", s.handleAct)

	// App commands
	srv.Handle("app.list", s.handleAppList)
	srv.Handle("app.install", s.handleAppInstall)
	srv.Handle("app.launch", s.handleAppLaunch)
	srv.Handle("app.kill", s.handleAppKill)
	srv.Handle("app.uninstall", s.handleAppUninstall)
	srv.Handle("app.foreground", s.handleAppForeground)

	// Log commands
	srv.Handle("log", s.handleLog)
	srv.Handle("log.crash.list", s.handleCrashList)
	srv.Handle("log.crash.get", s.handleCrashGet)

	// Check commands
	srv.Handle("check.screen", s.handleCheckScreen)
	srv.Handle("check.element", s.handleCheckElement)
	srv.Handle("check.app_running", s.handleCheckAppRunning)
	srv.Handle("check.no_crash", s.handleCheckNoCrash)

	// WDA commands
	srv.Handle("wda.status", s.handleWDAStatus)
	srv.Handle("wda.restart", s.handleWDARestart)

	// Daemon commands
	srv.Handle("daemon.status", s.handleDaemonStatus)
}

// --- Device handlers ---

func (s *State) handleDeviceList(params json.RawMessage) (any, error) {
	devices, err := s.DeviceManager.ListDevices()
	if err != nil {
		return nil, err
	}
	return devices, nil
}

func (s *State) handleDeviceConnect(params json.RawMessage) (any, error) {
	var p struct {
		UDID string `json:"udid"`
	}
	if len(params) > 0 {
		json.Unmarshal(params, &p)
	}
	status, err := s.DeviceManager.Connect(p.UDID)
	if err != nil {
		return nil, err
	}

	// Auto-start log capture on connect.
	if status.Connected && s.LogManager != nil {
		_ = s.LogManager.Start(status.UDID)
	}

	return status, nil
}

func (s *State) handleDeviceStatus(params json.RawMessage) (any, error) {
	return s.DeviceManager.Status(), nil
}

func (s *State) handleDeviceDisconnect(params json.RawMessage) (any, error) {
	if s.LogManager != nil {
		s.LogManager.Stop()
	}
	if err := s.DeviceManager.Disconnect(); err != nil {
		return nil, err
	}
	return map[string]string{"status": "disconnected"}, nil
}

// --- Look handler ---

func (s *State) handleLook(params json.RawMessage) (any, error) {
	var p struct {
		UI       bool `json:"ui"`
		Annotate bool `json:"annotate"`
	}
	if len(params) > 0 {
		json.Unmarshal(params, &p)
	}
	return s.ScreenCapture.Look(p.Annotate, p.UI)
}

// --- Act handler ---

func (s *State) handleAct(params json.RawMessage) (any, error) {
	var p struct {
		Action string `json:"action"`
		X      int    `json:"x"`
		Y      int    `json:"y"`
		X1     int    `json:"x1"`
		Y1     int    `json:"y1"`
		X2     int    `json:"x2"`
		Y2     int    `json:"y2"`
		Text   string `json:"text"`
		Key    string `json:"key"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid act params: %w", err)
	}

	switch p.Action {
	case "tap":
		if err := s.UiController.Tap(p.X, p.Y); err != nil {
			return nil, err
		}
		return map[string]string{"status": "ok", "action": "tap"}, nil
	case "swipe":
		if err := s.UiController.Swipe(p.X1, p.Y1, p.X2, p.Y2); err != nil {
			return nil, err
		}
		return map[string]string{"status": "ok", "action": "swipe"}, nil
	case "input":
		if err := s.UiController.Input(p.Text); err != nil {
			return nil, err
		}
		return map[string]string{"status": "ok", "action": "input"}, nil
	case "press":
		if err := s.UiController.Press(p.Key); err != nil {
			return nil, err
		}
		return map[string]string{"status": "ok", "action": "press"}, nil
	default:
		return nil, fmt.Errorf("unknown action: %q", p.Action)
	}
}

// --- App handlers ---

func (s *State) handleAppList(params json.RawMessage) (any, error) {
	apps, err := s.AppManager.List()
	if err != nil {
		return nil, err
	}
	return apps, nil
}

func (s *State) handleAppInstall(params json.RawMessage) (any, error) {
	var p struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	if p.Path == "" {
		return nil, fmt.Errorf("path is required")
	}
	if err := s.AppManager.Install(p.Path); err != nil {
		return nil, err
	}
	return map[string]string{"status": "installed", "path": p.Path}, nil
}

func (s *State) handleAppLaunch(params json.RawMessage) (any, error) {
	var p struct {
		BundleID string `json:"bundle_id"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	pid, err := s.AppManager.Launch(p.BundleID)
	if err != nil {
		return nil, err
	}
	return map[string]any{"status": "launched", "bundle_id": p.BundleID, "pid": pid}, nil
}

func (s *State) handleAppKill(params json.RawMessage) (any, error) {
	var p struct {
		BundleID string `json:"bundle_id"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	if err := s.AppManager.Kill(p.BundleID); err != nil {
		return nil, err
	}
	return map[string]string{"status": "killed", "bundle_id": p.BundleID}, nil
}

func (s *State) handleAppUninstall(params json.RawMessage) (any, error) {
	var p struct {
		BundleID string `json:"bundle_id"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	if err := s.AppManager.Uninstall(p.BundleID); err != nil {
		return nil, err
	}
	return map[string]string{"status": "uninstalled", "bundle_id": p.BundleID}, nil
}

func (s *State) handleAppForeground(params json.RawMessage) (any, error) {
	bundleID, err := s.AppManager.Foreground()
	if err != nil {
		return nil, err
	}
	return map[string]string{"bundle_id": bundleID}, nil
}

// --- Log handlers ---

func (s *State) handleLog(params json.RawMessage) (any, error) {
	var p struct {
		N      int    `json:"n"`
		Filter string `json:"filter"`
		Level  string `json:"level"`
		Search string `json:"search"`
	}
	if len(params) > 0 {
		json.Unmarshal(params, &p)
	}
	if p.N <= 0 {
		p.N = 50
	}

	entries := s.LogManager.GetLogs(p.N, core.LogFilter{
		BundleID: p.Filter,
		Level:    p.Level,
		Search:   p.Search,
	})
	return entries, nil
}

func (s *State) handleCrashList(params json.RawMessage) (any, error) {
	crashes, err := s.LogManager.ListCrashes()
	if err != nil {
		return nil, err
	}
	return crashes, nil
}

func (s *State) handleCrashGet(params json.RawMessage) (any, error) {
	var p struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	crash, err := s.LogManager.GetCrash(p.ID)
	if err != nil {
		return nil, err
	}
	return crash, nil
}

// --- Check handlers ---

func (s *State) handleCheckScreen(params json.RawMessage) (any, error) {
	return s.Checker.Screen()
}

func (s *State) handleCheckElement(params json.RawMessage) (any, error) {
	var p struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	return s.Checker.Element(p.Text)
}

func (s *State) handleCheckAppRunning(params json.RawMessage) (any, error) {
	var p struct {
		BundleID string `json:"bundle_id"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	return s.Checker.AppRunning(p.BundleID)
}

func (s *State) handleCheckNoCrash(params json.RawMessage) (any, error) {
	var p struct {
		BundleID string `json:"bundle_id"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	return s.Checker.NoCrash(p.BundleID)
}

// --- WDA handlers ---

func (s *State) handleWDAStatus(params json.RawMessage) (any, error) {
	status := s.DeviceManager.Status()
	return status.WDA, nil
}

func (s *State) handleWDARestart(params json.RawMessage) (any, error) {
	// Disconnect and reconnect to re-probe WDA.
	dev := s.DeviceManager.ConnectedDevice()
	if dev == nil {
		return nil, fmt.Errorf("no device connected")
	}
	udid := dev.UDID

	_ = s.DeviceManager.Disconnect()
	newStatus, err := s.DeviceManager.Connect(udid)
	if err != nil {
		return nil, fmt.Errorf("reconnect failed: %w", err)
	}
	return newStatus.WDA, nil
}

// --- Daemon handler ---

func (s *State) handleDaemonStatus(params json.RawMessage) (any, error) {
	uptime := time.Since(s.StartTime).Truncate(time.Second).String()

	deviceStatus := s.DeviceManager.Status()
	logRunning := false
	if s.LogManager != nil {
		logRunning = s.LogManager.IsRunning()
	}

	return map[string]any{
		"status":       "running",
		"uptime":       uptime,
		"go_version":   runtime.Version(),
		"device":       deviceStatus,
		"log_capture":  logRunning,
	}, nil
}
