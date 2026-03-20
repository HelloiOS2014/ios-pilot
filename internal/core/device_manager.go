// Package core provides business logic that ties driver implementations together.
package core

import (
	"context"
	"fmt"
	"io"
	"sync"

	"ios-pilot/internal/config"
	"ios-pilot/internal/driver"
)

const defaultWDAPort = "8100"

// WDAStatus describes the state of the WebDriverAgent process.
type WDAStatus struct {
	Status string `json:"status"` // "running", "stopped", "not_installed"
	Mode   string `json:"mode"`   // "full", "degraded"
}

// DeviceStatus is the composite status returned by Connect and Status.
type DeviceStatus struct {
	Connected     bool      `json:"connected"`
	UDID          string    `json:"udid,omitempty"`
	Name          string    `json:"name,omitempty"`
	IOSVersion    string    `json:"ios_version,omitempty"`
	WDA           WDAStatus `json:"wda"`
	ForegroundApp string    `json:"foreground_app,omitempty"`
}

// DeviceManager orchestrates device connection, tunnel setup, and WDA lifecycle.
type DeviceManager struct {
	deviceDriver     driver.DeviceDriver
	tunnelDriver     driver.TunnelDriver
	wdaDriver        driver.WDADriver
	wdaProcessDriver driver.WDAProcessDriver
	config           *config.Config

	mu            sync.RWMutex
	connected     *driver.DeviceInfo
	wdaSessionID  string
	wdaURL        string
	mode          string // "full" or "degraded"
	portForwarder io.Closer
	wdaProcess    io.Closer
}

// NewDeviceManager constructs a DeviceManager backed by the supplied drivers.
func NewDeviceManager(
	dd driver.DeviceDriver,
	td driver.TunnelDriver,
	wd driver.WDADriver,
	wpd driver.WDAProcessDriver,
	cfg *config.Config,
) *DeviceManager {
	return &DeviceManager{
		deviceDriver:     dd,
		tunnelDriver:     td,
		wdaDriver:        wd,
		wdaProcessDriver: wpd,
		config:           cfg,
	}
}

// ListDevices returns all currently connected iOS devices.
func (dm *DeviceManager) ListDevices() ([]driver.DeviceInfo, error) {
	return dm.deviceDriver.ListDevices()
}

// Connect establishes a session with the device identified by udid.
// If udid is empty and exactly one device is connected, it is auto-selected.
// WDA availability is probed; the call succeeds even if WDA is unavailable
// (degraded mode).
func (dm *DeviceManager) Connect(udid string) (*DeviceStatus, error) {
	// Resolve UDID.
	if udid == "" {
		devices, err := dm.deviceDriver.ListDevices()
		if err != nil {
			return nil, fmt.Errorf("list devices: %w", err)
		}
		if len(devices) == 0 {
			return nil, fmt.Errorf("no devices connected")
		}
		if len(devices) > 1 {
			return nil, fmt.Errorf("multiple devices connected; specify a UDID")
		}
		udid = devices[0].UDID
	}

	// Fetch device info.
	info, err := dm.deviceDriver.GetDevice(udid)
	if err != nil {
		return nil, fmt.Errorf("get device %s: %w", udid, err)
	}

	// Attempt tunnel — failure is non-fatal.
	if dm.tunnelDriver != nil {
		_ = dm.tunnelDriver.EnsureTunnel(udid)
	}

	// Forward device WDA port to localhost — failure is non-fatal.
	if dm.tunnelDriver != nil {
		fwd, fwdErr := dm.tunnelDriver.ForwardPort(udid, 8100, 8100)
		if fwdErr == nil {
			dm.mu.Lock()
			dm.portForwarder = fwd
			dm.mu.Unlock()
		}
	}

	// Determine WDA URL.
	wdaURL := fmt.Sprintf("http://localhost:%s", defaultWDAPort)

	// Probe WDA — if not running, try to start it.
	var sessionID string
	var mode string

	if dm.wdaDriver != nil {
		alive, _ := dm.wdaDriver.Status(wdaURL)

		// WDA not responding — try to launch it via WDAProcessDriver.
		if !alive && dm.wdaProcessDriver != nil {
			proc, startErr := dm.wdaProcessDriver.StartWDA(context.Background(), udid)
			if startErr == nil {
				dm.mu.Lock()
				dm.wdaProcess = proc
				dm.mu.Unlock()
				alive, _ = dm.wdaDriver.Status(wdaURL)
			}
		}

		if alive {
			sid, serr := dm.wdaDriver.CreateSession(wdaURL)
			if serr == nil {
				sessionID = sid
				mode = "full"
			} else {
				mode = "degraded"
			}
		} else {
			mode = "degraded"
		}
	} else {
		mode = "degraded"
	}

	// Commit state.
	dm.mu.Lock()
	dm.connected = info
	dm.wdaURL = wdaURL
	dm.wdaSessionID = sessionID
	dm.mode = mode
	dm.mu.Unlock()

	return dm.buildStatus(), nil
}

// Disconnect tears down the active WDA session and clears all state.
func (dm *DeviceManager) Disconnect() error {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	if dm.connected == nil {
		return nil
	}

	// Best-effort session deletion.
	if dm.wdaDriver != nil && dm.wdaSessionID != "" {
		_ = dm.wdaDriver.DeleteSession(dm.wdaURL, dm.wdaSessionID)
	}

	// Kill WDA process.
	if dm.wdaProcess != nil {
		_ = dm.wdaProcess.Close()
		dm.wdaProcess = nil
	}

	// Stop port forwarding.
	if dm.portForwarder != nil {
		_ = dm.portForwarder.Close()
		dm.portForwarder = nil
	}

	dm.connected = nil
	dm.wdaSessionID = ""
	dm.wdaURL = ""
	dm.mode = ""
	return nil
}

// Status returns the current device status without modifying state.
func (dm *DeviceManager) Status() *DeviceStatus {
	return dm.buildStatus()
}

// IsConnected reports whether a device session is active.
func (dm *DeviceManager) IsConnected() bool {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	return dm.connected != nil
}

// Mode returns the current operating mode ("full", "degraded", or "" if not connected).
func (dm *DeviceManager) Mode() string {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	return dm.mode
}

// ConnectedDevice returns a copy of the connected DeviceInfo, or nil.
func (dm *DeviceManager) ConnectedDevice() *driver.DeviceInfo {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	if dm.connected == nil {
		return nil
	}
	cp := *dm.connected
	return &cp
}

// WDADriver returns the WDA driver instance.
func (dm *DeviceManager) WDADriver() driver.WDADriver {
	return dm.wdaDriver
}

// WDASessionID returns the active WDA session identifier.
func (dm *DeviceManager) WDASessionID() string {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	return dm.wdaSessionID
}

// WDAURL returns the base URL of the WebDriverAgent instance.
func (dm *DeviceManager) WDAURL() string {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	return dm.wdaURL
}

// buildStatus constructs a DeviceStatus snapshot from the current state.
func (dm *DeviceManager) buildStatus() *DeviceStatus {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	if dm.connected == nil {
		return &DeviceStatus{Connected: false}
	}

	wdaStatus := "stopped"
	if dm.mode == "full" {
		wdaStatus = "running"
	}

	return &DeviceStatus{
		Connected:  true,
		UDID:       dm.connected.UDID,
		Name:       dm.connected.Name,
		IOSVersion: dm.connected.IOSVersion,
		WDA: WDAStatus{
			Status: wdaStatus,
			Mode:   dm.mode,
		},
	}
}
