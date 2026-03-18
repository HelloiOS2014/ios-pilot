package core

import (
	"fmt"
	"testing"

	"ios-pilot/internal/driver"
	"ios-pilot/internal/protocol"
)

// mockAppDriver records calls and returns configured responses.
type mockAppDriver struct {
	apps        []driver.AppInfo
	listErr     error
	installErr  error
	launchPID   int
	launchErr   error
	killErr     error
	uninstallErr error
	foreground  string
	foregroundErr error

	// call tracking
	installedPath   string
	launchedBundle  string
	killedBundle    string
	uninstalledBundle string
}

func (m *mockAppDriver) ListApps(_ string) ([]driver.AppInfo, error) {
	return m.apps, m.listErr
}

func (m *mockAppDriver) Install(_ string, path string) error {
	m.installedPath = path
	return m.installErr
}

func (m *mockAppDriver) Launch(_ string, bundleID string) (int, error) {
	m.launchedBundle = bundleID
	return m.launchPID, m.launchErr
}

func (m *mockAppDriver) Kill(_ string, bundleID string) error {
	m.killedBundle = bundleID
	return m.killErr
}

func (m *mockAppDriver) Uninstall(_ string, bundleID string) error {
	m.uninstalledBundle = bundleID
	return m.uninstallErr
}

func (m *mockAppDriver) ForegroundApp(_ string) (string, error) {
	return m.foreground, m.foregroundErr
}

// ---- helpers ----

// connectedManager returns a DeviceManager pre-connected to a mock device.
func connectedManager(t *testing.T) *DeviceManager {
	t.Helper()
	dd := &mockDeviceDriver{devices: singleDevice()}
	wd := &mockWDADriver{alive: false}
	dm := NewDeviceManager(dd, nil, wd, defaultCfg())
	if _, err := dm.Connect("abc123"); err != nil {
		t.Fatalf("setup Connect: %v", err)
	}
	return dm
}

// ---- tests ----

func TestAppList_WhenConnected(t *testing.T) {
	apps := []driver.AppInfo{
		{BundleID: "com.example.app1", Name: "App1", Version: "1.0"},
		{BundleID: "com.example.app2", Name: "App2", Version: "2.0"},
	}
	ad := &mockAppDriver{apps: apps}
	am := NewAppManager(ad, connectedManager(t))

	got, err := am.List()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != len(apps) {
		t.Errorf("expected %d apps, got %d", len(apps), len(got))
	}
	if got[0].BundleID != "com.example.app1" {
		t.Errorf("unexpected first bundle ID: %q", got[0].BundleID)
	}
}

func TestAppList_WhenNotConnected(t *testing.T) {
	ad := &mockAppDriver{}
	dm := NewDeviceManager(&mockDeviceDriver{}, nil, nil, defaultCfg())
	am := NewAppManager(ad, dm)

	_, err := am.List()
	if err == nil {
		t.Fatal("expected error when not connected")
	}

	protoErr, ok := err.(*protocol.Error)
	if !ok {
		t.Fatalf("expected *protocol.Error, got %T: %v", err, err)
	}
	if protoErr.Code != protocol.ErrDeviceNotConnected.Code {
		t.Errorf("expected code %d, got %d", protocol.ErrDeviceNotConnected.Code, protoErr.Code)
	}
}

func TestAppInstall_DelegatesToDriver(t *testing.T) {
	ad := &mockAppDriver{}
	am := NewAppManager(ad, connectedManager(t))

	if err := am.Install("/tmp/my.ipa"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ad.installedPath != "/tmp/my.ipa" {
		t.Errorf("expected path /tmp/my.ipa, got %q", ad.installedPath)
	}
}

func TestAppInstall_WhenNotConnected(t *testing.T) {
	ad := &mockAppDriver{}
	dm := NewDeviceManager(&mockDeviceDriver{}, nil, nil, defaultCfg())
	am := NewAppManager(ad, dm)

	err := am.Install("/tmp/app.ipa")
	if err == nil {
		t.Fatal("expected error when not connected")
	}
}

func TestAppLaunch_ReturnsPID(t *testing.T) {
	ad := &mockAppDriver{launchPID: 1234}
	am := NewAppManager(ad, connectedManager(t))

	pid, err := am.Launch("com.example.app")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pid != 1234 {
		t.Errorf("expected PID=1234, got %d", pid)
	}
	if ad.launchedBundle != "com.example.app" {
		t.Errorf("expected bundle com.example.app, got %q", ad.launchedBundle)
	}
}

func TestAppLaunch_WhenNotConnected(t *testing.T) {
	ad := &mockAppDriver{}
	dm := NewDeviceManager(&mockDeviceDriver{}, nil, nil, defaultCfg())
	am := NewAppManager(ad, dm)

	_, err := am.Launch("com.example.app")
	if err == nil {
		t.Fatal("expected error when not connected")
	}
}

func TestAppKill_DelegatesToDriver(t *testing.T) {
	ad := &mockAppDriver{}
	am := NewAppManager(ad, connectedManager(t))

	if err := am.Kill("com.example.app"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ad.killedBundle != "com.example.app" {
		t.Errorf("expected bundle com.example.app, got %q", ad.killedBundle)
	}
}

func TestAppKill_WhenNotConnected(t *testing.T) {
	ad := &mockAppDriver{}
	dm := NewDeviceManager(&mockDeviceDriver{}, nil, nil, defaultCfg())
	am := NewAppManager(ad, dm)

	err := am.Kill("com.example.app")
	if err == nil {
		t.Fatal("expected error when not connected")
	}
}

func TestAppUninstall_DelegatesToDriver(t *testing.T) {
	ad := &mockAppDriver{}
	am := NewAppManager(ad, connectedManager(t))

	if err := am.Uninstall("com.example.app"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ad.uninstalledBundle != "com.example.app" {
		t.Errorf("expected bundle com.example.app, got %q", ad.uninstalledBundle)
	}
}

func TestAppForeground_ReturnsBundle(t *testing.T) {
	ad := &mockAppDriver{foreground: "com.apple.mobilesafari"}
	am := NewAppManager(ad, connectedManager(t))

	bundle, err := am.Foreground()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bundle != "com.apple.mobilesafari" {
		t.Errorf("expected com.apple.mobilesafari, got %q", bundle)
	}
}

func TestAppForeground_WhenNotConnected(t *testing.T) {
	ad := &mockAppDriver{}
	dm := NewDeviceManager(&mockDeviceDriver{}, nil, nil, defaultCfg())
	am := NewAppManager(ad, dm)

	_, err := am.Foreground()
	if err == nil {
		t.Fatal("expected error when not connected")
	}
}

func TestAppDriverErrors_Propagated(t *testing.T) {
	driverErr := fmt.Errorf("driver unavailable")
	ad := &mockAppDriver{listErr: driverErr}
	am := NewAppManager(ad, connectedManager(t))

	_, err := am.List()
	if err == nil {
		t.Fatal("expected driver error to propagate")
	}
	if err.Error() != driverErr.Error() {
		t.Errorf("expected %q, got %q", driverErr.Error(), err.Error())
	}
}
