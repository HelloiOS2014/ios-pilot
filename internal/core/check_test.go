package core

import (
	"encoding/json"
	"fmt"
	"testing"

	"ios-pilot/internal/driver"
)

// ---- mock ScreenCaptureInterface ----

type mockScreenCapture struct {
	path string
	err  error
}

func (m *mockScreenCapture) TakeScreenshot() (string, error) {
	return m.path, m.err
}

// ---- mock DeviceManagerInterface ----

type mockDeviceManager struct {
	connected bool
	device    *driver.DeviceInfo
	mode      string
	wdaURL    string
	sessionID string
}

func (m *mockDeviceManager) IsConnected() bool              { return m.connected }
func (m *mockDeviceManager) ConnectedDevice() *driver.DeviceInfo { return m.device }
func (m *mockDeviceManager) Mode() string                   { return m.mode }
func (m *mockDeviceManager) WDAURL() string                 { return m.wdaURL }
func (m *mockDeviceManager) WDASessionID() string           { return m.sessionID }

// ---- mock WDADriver for Checker ----

type mockCheckerWDA struct {
	elements  []driver.WDAElement
	treeErr   error
	findEl    *driver.WDAElement
	findErr   error
}

func (m *mockCheckerWDA) Status(_ string) (bool, error)      { return true, nil }
func (m *mockCheckerWDA) CreateSession(_ string) (string, error) { return "sid", nil }
func (m *mockCheckerWDA) DeleteSession(_, _ string) error    { return nil }
func (m *mockCheckerWDA) GetElementTree(_, _ string) ([]driver.WDAElement, error) {
	return m.elements, m.treeErr
}
func (m *mockCheckerWDA) GetInteractiveElements(_, _ string, _ []string) ([]driver.WDAElement, error) {
	return m.elements, m.treeErr
}
func (m *mockCheckerWDA) FindElement(_, _, _, _ string) (*driver.WDAElement, error) {
	return m.findEl, m.findErr
}
func (m *mockCheckerWDA) Tap(_, _ string, _, _ int) error         { return nil }
func (m *mockCheckerWDA) Swipe(_, _ string, _, _, _, _ int) error { return nil }
func (m *mockCheckerWDA) InputText(_, _, _ string) error          { return nil }
func (m *mockCheckerWDA) PressButton(_, _, _ string) error        { return nil }
func (m *mockCheckerWDA) Screenshot(_, _ string) ([]byte, error)  { return nil, nil }

// ---- mock AppDriver for Checker ----

type mockCheckerApp struct {
	foreground string
	fgErr      error
}

func (m *mockCheckerApp) ListApps(_ string) ([]driver.AppInfo, error) { return nil, nil }
func (m *mockCheckerApp) Install(_, _ string) error                    { return nil }
func (m *mockCheckerApp) Uninstall(_, _ string) error                  { return nil }
func (m *mockCheckerApp) Launch(_, _ string) (int, error)              { return 0, nil }
func (m *mockCheckerApp) Kill(_, _ string) error                       { return nil }
func (m *mockCheckerApp) ForegroundApp(_ string) (string, error)       { return m.foreground, m.fgErr }

// ---- mock CrashDriver for Checker ----

type mockCheckerCrash struct {
	crashes  []driver.CrashReport
	listErr  error
}

func (m *mockCheckerCrash) ListCrashes(_ string) ([]driver.CrashReport, error) {
	return m.crashes, m.listErr
}
func (m *mockCheckerCrash) GetCrash(_, _ string) (*driver.CrashReport, error) {
	return nil, fmt.Errorf("not implemented")
}

// ---- helpers ----

func defaultDevManager() *mockDeviceManager {
	return &mockDeviceManager{
		connected: true,
		device:    &driver.DeviceInfo{UDID: "test-device"},
		mode:      "full",
		wdaURL:    "http://localhost:8100",
		sessionID: "sess-abc",
	}
}

// ---- tests ----

func TestCheckScreen_PassIsNull(t *testing.T) {
	sc := &mockScreenCapture{path: "/tmp/screen.png"}
	checker := NewChecker(sc, nil, nil, nil, defaultDevManager())

	result, err := checker.Screen()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Pass != nil {
		t.Errorf("expected pass=nil for Screen(), got %v", *result.Pass)
	}

	// Verify JSON serialization produces "pass":null
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	passVal, exists := m["pass"]
	if !exists {
		t.Fatal("expected 'pass' key in JSON output")
	}
	if passVal != nil {
		t.Errorf("expected JSON null for pass, got %v", passVal)
	}
}

func TestCheckElement_Found(t *testing.T) {
	sc := &mockScreenCapture{path: "/tmp/screen.png"}
	wda := &mockCheckerWDA{
		elements: []driver.WDAElement{
			{Type: "XCUIElementTypeButton", Label: "Login"},
		},
	}
	checker := NewChecker(sc, wda, nil, nil, defaultDevManager())

	result, err := checker.Element("Login")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Pass == nil || !*result.Pass {
		t.Error("expected pass=true when element is found")
	}
}

func TestCheckElement_NotFound(t *testing.T) {
	sc := &mockScreenCapture{path: "/tmp/screen.png"}
	wda := &mockCheckerWDA{
		elements: []driver.WDAElement{
			{Type: "XCUIElementTypeButton", Label: "Cancel"},
		},
	}
	checker := NewChecker(sc, wda, nil, nil, defaultDevManager())

	result, err := checker.Element("NonExistentElement")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Pass == nil || *result.Pass {
		t.Error("expected pass=false when element is not found")
	}
}

func TestCheckAppRunning_True(t *testing.T) {
	sc := &mockScreenCapture{path: "/tmp/screen.png"}
	app := &mockCheckerApp{foreground: "com.example.myapp"}
	checker := NewChecker(sc, nil, app, nil, defaultDevManager())

	result, err := checker.AppRunning("com.example.myapp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Pass == nil || !*result.Pass {
		t.Error("expected pass=true when foreground matches")
	}
}

func TestCheckAppRunning_False(t *testing.T) {
	sc := &mockScreenCapture{path: "/tmp/screen.png"}
	app := &mockCheckerApp{foreground: "com.apple.mobilesafari"}
	checker := NewChecker(sc, nil, app, nil, defaultDevManager())

	result, err := checker.AppRunning("com.example.myapp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Pass == nil || *result.Pass {
		t.Error("expected pass=false when foreground does not match")
	}
}

func TestCheckNoCrash_NoCrashes(t *testing.T) {
	sc := &mockScreenCapture{path: "/tmp/screen.png"}
	cd := &mockCheckerCrash{crashes: []driver.CrashReport{}}
	checker := NewChecker(sc, nil, nil, cd, defaultDevManager())

	result, err := checker.NoCrash("com.example.myapp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Pass == nil || !*result.Pass {
		t.Error("expected pass=true when no crashes found")
	}
}

func TestCheckNoCrash_HasCrashes(t *testing.T) {
	sc := &mockScreenCapture{path: "/tmp/screen.png"}
	cd := &mockCheckerCrash{
		crashes: []driver.CrashReport{
			{ID: "cr1", Name: "com.example.myapp_crash", Process: "com.example.myapp"},
		},
	}
	checker := NewChecker(sc, nil, nil, cd, defaultDevManager())

	result, err := checker.NoCrash("com.example.myapp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Pass == nil || *result.Pass {
		t.Error("expected pass=false when crashes are found")
	}
}
