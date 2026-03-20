package core

import (
	"ios-pilot/internal/driver"
	"ios-pilot/internal/protocol"
)

// AppManager wraps AppDriver with connectivity guards.
// When instruments-based operations fail, it falls back to WDA endpoints
// (which do not require Developer Image).
type AppManager struct {
	appDriver     driver.AppDriver
	deviceManager *DeviceManager
}

// NewAppManager constructs an AppManager.
func NewAppManager(ad driver.AppDriver, dm *DeviceManager) *AppManager {
	return &AppManager{
		appDriver:     ad,
		deviceManager: dm,
	}
}

// requireConnected returns the connected device's UDID or a protocol error.
func (am *AppManager) requireConnected() (string, error) {
	if !am.deviceManager.IsConnected() {
		return "", protocol.ErrDeviceNotConnected.ToError(nil)
	}
	return am.deviceManager.ConnectedDevice().UDID, nil
}

// List returns all installed applications on the connected device.
func (am *AppManager) List() ([]driver.AppInfo, error) {
	udid, err := am.requireConnected()
	if err != nil {
		return nil, err
	}
	return am.appDriver.ListApps(udid)
}

// Install installs the application at path onto the connected device.
func (am *AppManager) Install(path string) error {
	udid, err := am.requireConnected()
	if err != nil {
		return err
	}
	return am.appDriver.Install(udid, path)
}

// Launch starts the application identified by bundleID and returns its PID.
// Falls back to WDA if instruments/appservice fails.
func (am *AppManager) Launch(bundleID string) (int, error) {
	udid, err := am.requireConnected()
	if err != nil {
		return 0, err
	}
	pid, err := am.appDriver.Launch(udid, bundleID)
	if err == nil {
		return pid, nil
	}

	// Instruments failed — try WDA fallback (no Developer Image needed).
	if am.deviceManager.Mode() == "full" {
		wdaDrv := am.deviceManager.WDADriver()
		if wdaDrv != nil {
			wdaErr := wdaDrv.LaunchApp(
				am.deviceManager.WDAURL(),
				am.deviceManager.WDASessionID(),
				bundleID,
			)
			if wdaErr == nil {
				return 0, nil // WDA doesn't return PID, return 0
			}
		}
	}
	return 0, err // return original instruments error
}

// Kill stops the application identified by bundleID.
// Falls back to WDA if instruments/appservice fails.
func (am *AppManager) Kill(bundleID string) error {
	udid, err := am.requireConnected()
	if err != nil {
		return err
	}
	err = am.appDriver.Kill(udid, bundleID)
	if err == nil {
		return nil
	}

	// Instruments failed — try WDA fallback.
	if am.deviceManager.Mode() == "full" {
		wdaDrv := am.deviceManager.WDADriver()
		if wdaDrv != nil {
			wdaErr := wdaDrv.KillApp(
				am.deviceManager.WDAURL(),
				am.deviceManager.WDASessionID(),
				bundleID,
			)
			if wdaErr == nil {
				return nil
			}
		}
	}
	return err
}

// Uninstall removes the application identified by bundleID from the device.
func (am *AppManager) Uninstall(bundleID string) error {
	udid, err := am.requireConnected()
	if err != nil {
		return err
	}
	return am.appDriver.Uninstall(udid, bundleID)
}

// Foreground returns the bundle ID of the app currently in the foreground.
func (am *AppManager) Foreground() (string, error) {
	udid, err := am.requireConnected()
	if err != nil {
		return "", err
	}
	return am.appDriver.ForegroundApp(udid)
}
