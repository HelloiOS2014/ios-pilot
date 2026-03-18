package core

import (
	"context"
	"fmt"
	"io"
	"testing"

	"ios-pilot/internal/config"
	"ios-pilot/internal/driver"
)

// ---- mock drivers ----

type mockDeviceDriver struct {
	devices []driver.DeviceInfo
	listErr error
	getErr  error
}

func (m *mockDeviceDriver) ListDevices() ([]driver.DeviceInfo, error) {
	return m.devices, m.listErr
}

func (m *mockDeviceDriver) GetDevice(udid string) (*driver.DeviceInfo, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	for _, d := range m.devices {
		if d.UDID == udid {
			return &d, nil
		}
	}
	return nil, fmt.Errorf("device not found: %s", udid)
}

type mockTunnelDriver struct {
	ensureErr  error
	running    bool
	forwardErr error
}

func (m *mockTunnelDriver) EnsureTunnel(_ string) error   { return m.ensureErr }
func (m *mockTunnelDriver) IsTunnelRunning(_ string) bool  { return m.running }
func (m *mockTunnelDriver) StopTunnel(_ string) error      { return nil }
func (m *mockTunnelDriver) ForwardPort(_ string, _, _ uint16) (io.Closer, error) {
	if m.forwardErr != nil {
		return nil, m.forwardErr
	}
	return io.NopCloser(nil), nil
}

type mockWDADriver struct {
	alive          bool
	statusErr      error
	sessionID      string
	createErr      error
	deleteErr      error
	deleteCalled   bool
}

func (m *mockWDADriver) Status(_ string) (bool, error) {
	return m.alive, m.statusErr
}

func (m *mockWDADriver) CreateSession(_ string) (string, error) {
	if m.createErr != nil {
		return "", m.createErr
	}
	return m.sessionID, nil
}

func (m *mockWDADriver) DeleteSession(_, _ string) error {
	m.deleteCalled = true
	return m.deleteErr
}

type mockWDAProcessDriver struct {
	startErr    error
	startCalled bool
}

func (m *mockWDAProcessDriver) StartWDA(_ context.Context, _ string) (io.Closer, error) {
	m.startCalled = true
	if m.startErr != nil {
		return nil, m.startErr
	}
	return io.NopCloser(nil), nil
}

// Remaining WDADriver interface methods — unused in these tests.
func (m *mockWDADriver) GetElementTree(_, _ string) ([]driver.WDAElement, error) {
	return nil, nil
}
func (m *mockWDADriver) GetInteractiveElements(_, _ string, _ []string) ([]driver.WDAElement, error) {
	return nil, nil
}
func (m *mockWDADriver) FindElement(_, _, _, _ string) (*driver.WDAElement, error) {
	return nil, nil
}
func (m *mockWDADriver) Tap(_, _ string, _, _ int) error        { return nil }
func (m *mockWDADriver) Swipe(_, _ string, _, _, _, _ int) error { return nil }
func (m *mockWDADriver) InputText(_, _, _ string) error         { return nil }
func (m *mockWDADriver) PressButton(_, _, _ string) error       { return nil }
func (m *mockWDADriver) Screenshot(_, _ string) ([]byte, error) { return nil, nil }

// ---- helpers ----

func defaultCfg() *config.Config {
	c := config.Default()
	return &c
}

func singleDevice() []driver.DeviceInfo {
	return []driver.DeviceInfo{
		{UDID: "abc123", Name: "Test iPhone", IOSVersion: "17.0", ProductType: "iPhone15,2"},
	}
}

// ---- tests ----

func TestConnectWithWDA_FullMode(t *testing.T) {
	dd := &mockDeviceDriver{devices: singleDevice()}
	td := &mockTunnelDriver{}
	wd := &mockWDADriver{alive: true, sessionID: "sess-1"}

	dm := NewDeviceManager(dd, td, wd, nil, defaultCfg())

	status, err := dm.Connect("abc123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !status.Connected {
		t.Error("expected Connected=true")
	}
	if status.WDA.Mode != "full" {
		t.Errorf("expected full mode, got %q", status.WDA.Mode)
	}
	if status.WDA.Status != "running" {
		t.Errorf("expected WDA status running, got %q", status.WDA.Status)
	}
	if dm.WDASessionID() != "sess-1" {
		t.Errorf("expected sessionID=sess-1, got %q", dm.WDASessionID())
	}
	if dm.Mode() != "full" {
		t.Errorf("expected mode=full, got %q", dm.Mode())
	}
}

func TestConnectWithoutWDA_DegradedMode(t *testing.T) {
	dd := &mockDeviceDriver{devices: singleDevice()}
	td := &mockTunnelDriver{}
	wd := &mockWDADriver{alive: false} // WDA not responsive

	dm := NewDeviceManager(dd, td, wd, nil, defaultCfg())

	status, err := dm.Connect("abc123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !status.Connected {
		t.Error("expected Connected=true even in degraded mode")
	}
	if status.WDA.Mode != "degraded" {
		t.Errorf("expected degraded mode, got %q", status.WDA.Mode)
	}
	if dm.WDASessionID() != "" {
		t.Errorf("expected empty sessionID in degraded mode, got %q", dm.WDASessionID())
	}
}

func TestConnectAutoSelectsSingleDevice(t *testing.T) {
	dd := &mockDeviceDriver{devices: singleDevice()}
	td := &mockTunnelDriver{}
	wd := &mockWDADriver{alive: false}

	dm := NewDeviceManager(dd, td, wd, nil, defaultCfg())

	// Pass empty udid — should auto-select.
	status, err := dm.Connect("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if status.UDID != "abc123" {
		t.Errorf("expected UDID=abc123, got %q", status.UDID)
	}
	if dm.ConnectedDevice().UDID != "abc123" {
		t.Errorf("connected device UDID mismatch")
	}
}

func TestConnectWithSpecificUDID(t *testing.T) {
	devices := []driver.DeviceInfo{
		{UDID: "aaa", Name: "Device A"},
		{UDID: "bbb", Name: "Device B"},
	}
	dd := &mockDeviceDriver{devices: devices}
	td := &mockTunnelDriver{}
	wd := &mockWDADriver{alive: false}

	dm := NewDeviceManager(dd, td, wd, nil, defaultCfg())

	status, err := dm.Connect("bbb")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.UDID != "bbb" {
		t.Errorf("expected UDID=bbb, got %q", status.UDID)
	}
}

func TestDisconnectClearsState(t *testing.T) {
	dd := &mockDeviceDriver{devices: singleDevice()}
	td := &mockTunnelDriver{}
	wd := &mockWDADriver{alive: true, sessionID: "sess-42"}

	dm := NewDeviceManager(dd, td, wd, nil, defaultCfg())

	if _, err := dm.Connect("abc123"); err != nil {
		t.Fatalf("connect: %v", err)
	}
	if !dm.IsConnected() {
		t.Fatal("expected connected after Connect")
	}

	if err := dm.Disconnect(); err != nil {
		t.Fatalf("disconnect: %v", err)
	}

	if dm.IsConnected() {
		t.Error("expected not connected after Disconnect")
	}
	if dm.ConnectedDevice() != nil {
		t.Error("expected nil ConnectedDevice after Disconnect")
	}
	if dm.WDASessionID() != "" {
		t.Error("expected empty sessionID after Disconnect")
	}
	if dm.WDAURL() != "" {
		t.Error("expected empty WDAURL after Disconnect")
	}
	if dm.Mode() != "" {
		t.Error("expected empty mode after Disconnect")
	}
	if !wd.deleteCalled {
		t.Error("expected WDA DeleteSession to be called on Disconnect")
	}
}

func TestStatusWhenNotConnected(t *testing.T) {
	dd := &mockDeviceDriver{}
	dm := NewDeviceManager(dd, nil, nil, nil, defaultCfg())

	s := dm.Status()
	if s.Connected {
		t.Error("expected Connected=false when not connected")
	}
	if s.UDID != "" {
		t.Error("expected empty UDID when not connected")
	}
}

func TestListDevicesDelegatesToDriver(t *testing.T) {
	expected := []driver.DeviceInfo{
		{UDID: "x1", Name: "X1"},
		{UDID: "x2", Name: "X2"},
	}
	dd := &mockDeviceDriver{devices: expected}
	dm := NewDeviceManager(dd, nil, nil, nil, defaultCfg())

	got, err := dm.ListDevices()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != len(expected) {
		t.Errorf("expected %d devices, got %d", len(expected), len(got))
	}
	for i, d := range got {
		if d.UDID != expected[i].UDID {
			t.Errorf("device[%d] UDID mismatch: want %q got %q", i, expected[i].UDID, d.UDID)
		}
	}
}

func TestConnectMultipleDevicesNoUDID_Error(t *testing.T) {
	devices := []driver.DeviceInfo{
		{UDID: "aaa"},
		{UDID: "bbb"},
	}
	dd := &mockDeviceDriver{devices: devices}
	dm := NewDeviceManager(dd, nil, nil, nil, defaultCfg())

	_, err := dm.Connect("")
	if err == nil {
		t.Error("expected error when multiple devices and no UDID given")
	}
}

func TestConnectNoDevices_Error(t *testing.T) {
	dd := &mockDeviceDriver{devices: []driver.DeviceInfo{}}
	dm := NewDeviceManager(dd, nil, nil, nil, defaultCfg())

	_, err := dm.Connect("")
	if err == nil {
		t.Error("expected error when no devices connected")
	}
}

func TestConnectStartsWDAWhenNotRunning(t *testing.T) {
	dd := &mockDeviceDriver{devices: singleDevice()}
	td := &mockTunnelDriver{}
	// WDA initially not alive, but becomes alive after StartWDA.
	wd := &mockWDADriver{alive: false, sessionID: "sess-auto"}
	wpd := &mockWDAProcessDriver{}

	dm := NewDeviceManager(dd, td, wd, wpd, defaultCfg())

	// After StartWDA is called, simulate WDA becoming alive.
	origStatus := wd.Status
	_ = origStatus
	// We need WDA to return false on first call, true on second.
	// Use a counter to simulate this.
	callCount := 0
	dm.wdaDriver = &statusToggleWDADriver{
		inner:     wd,
		callCount: &callCount,
	}

	status, err := dm.Connect("abc123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !wpd.startCalled {
		t.Error("expected StartWDA to be called")
	}
	if !status.Connected {
		t.Error("expected Connected=true")
	}
	if status.WDA.Mode != "full" {
		t.Errorf("expected full mode after auto-start, got %q", status.WDA.Mode)
	}
}

// statusToggleWDADriver returns false on first Status call, true on subsequent.
type statusToggleWDADriver struct {
	inner     *mockWDADriver
	callCount *int
}

func (s *statusToggleWDADriver) Status(_ string) (bool, error) {
	*s.callCount++
	if *s.callCount == 1 {
		return false, nil
	}
	return true, nil
}
func (s *statusToggleWDADriver) CreateSession(url string) (string, error) {
	return s.inner.CreateSession(url)
}
func (s *statusToggleWDADriver) DeleteSession(url, sid string) error {
	return s.inner.DeleteSession(url, sid)
}
func (s *statusToggleWDADriver) GetElementTree(_, _ string) ([]driver.WDAElement, error) {
	return nil, nil
}
func (s *statusToggleWDADriver) GetInteractiveElements(_, _ string, _ []string) ([]driver.WDAElement, error) {
	return nil, nil
}
func (s *statusToggleWDADriver) FindElement(_, _, _, _ string) (*driver.WDAElement, error) {
	return nil, nil
}
func (s *statusToggleWDADriver) Tap(_, _ string, _, _ int) error        { return nil }
func (s *statusToggleWDADriver) Swipe(_, _ string, _, _, _, _ int) error { return nil }
func (s *statusToggleWDADriver) InputText(_, _, _ string) error         { return nil }
func (s *statusToggleWDADriver) PressButton(_, _, _ string) error       { return nil }
func (s *statusToggleWDADriver) Screenshot(_, _ string) ([]byte, error) { return nil, nil }

func TestTunnelFailureIsNonFatal(t *testing.T) {
	dd := &mockDeviceDriver{devices: singleDevice()}
	td := &mockTunnelDriver{ensureErr: fmt.Errorf("tunnel error")}
	wd := &mockWDADriver{alive: false}

	dm := NewDeviceManager(dd, td, wd, nil, defaultCfg())

	// Tunnel fails but Connect should still succeed.
	status, err := dm.Connect("abc123")
	if err != nil {
		t.Fatalf("expected connect to succeed despite tunnel failure: %v", err)
	}
	if !status.Connected {
		t.Error("expected Connected=true despite tunnel failure")
	}
}
