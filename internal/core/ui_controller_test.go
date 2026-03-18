package core

import (
	"testing"

	"ios-pilot/internal/protocol"
)

// mockWDADriverUI captures calls to WDA interaction methods.
type mockWDADriverUI struct {
	mockWDADriver

	// call tracking
	tapCalled   bool
	tapX, tapY  int
	tapErr      error

	swipeCalled            bool
	swipeX1, swipeY1       int
	swipeX2, swipeY2       int
	swipeErr               error

	inputCalled bool
	inputText   string
	inputErr    error

	pressCalled bool
	pressKey    string
	pressErr    error
}

func (m *mockWDADriverUI) Tap(_, _ string, x, y int) error {
	m.tapCalled = true
	m.tapX, m.tapY = x, y
	return m.tapErr
}

func (m *mockWDADriverUI) Swipe(_, _ string, x1, y1, x2, y2 int) error {
	m.swipeCalled = true
	m.swipeX1, m.swipeY1 = x1, y1
	m.swipeX2, m.swipeY2 = x2, y2
	return m.swipeErr
}

func (m *mockWDADriverUI) InputText(_, _, text string) error {
	m.inputCalled = true
	m.inputText = text
	return m.inputErr
}

func (m *mockWDADriverUI) PressButton(_, _, key string) error {
	m.pressCalled = true
	m.pressKey = key
	return m.pressErr
}

// ---- helpers ----

func fullModeManager(t *testing.T) *DeviceManager {
	t.Helper()
	dd := &mockDeviceDriver{devices: singleDevice()}
	wd := &mockWDADriver{alive: true, sessionID: "sess-1"}
	dm := NewDeviceManager(dd, nil, wd, nil, defaultCfg())
	if _, err := dm.Connect("abc123"); err != nil {
		t.Fatalf("setup Connect: %v", err)
	}
	return dm
}

func degradedModeManager(t *testing.T) *DeviceManager {
	t.Helper()
	dd := &mockDeviceDriver{devices: singleDevice()}
	wd := &mockWDADriver{alive: false}
	dm := NewDeviceManager(dd, nil, wd, nil, defaultCfg())
	if _, err := dm.Connect("abc123"); err != nil {
		t.Fatalf("setup Connect: %v", err)
	}
	return dm
}

func disconnectedManager() *DeviceManager {
	dd := &mockDeviceDriver{}
	return NewDeviceManager(dd, nil, nil, nil, defaultCfg())
}

// ---- Tap tests ----

func TestTap_FullMode(t *testing.T) {
	dm := fullModeManager(t)
	wd := &mockWDADriverUI{}
	uc := NewUiController(wd, dm)

	if err := uc.Tap(100, 200); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !wd.tapCalled {
		t.Error("expected WDA Tap to be called")
	}
	if wd.tapX != 100 || wd.tapY != 200 {
		t.Errorf("expected tap at (100,200), got (%d,%d)", wd.tapX, wd.tapY)
	}
}

func TestTap_DegradedMode(t *testing.T) {
	dm := degradedModeManager(t)
	wd := &mockWDADriverUI{}
	uc := NewUiController(wd, dm)

	err := uc.Tap(100, 200)
	if err == nil {
		t.Fatal("expected error in degraded mode")
	}
	protoErr, ok := err.(*protocol.Error)
	if !ok {
		t.Fatalf("expected *protocol.Error, got %T: %v", err, err)
	}
	if protoErr.Code != protocol.ErrWDAUnavailable.Code {
		t.Errorf("expected ErrWDAUnavailable (code %d), got code %d", protocol.ErrWDAUnavailable.Code, protoErr.Code)
	}
	if wd.tapCalled {
		t.Error("WDA Tap should not be called in degraded mode")
	}
}

func TestTap_NotConnected(t *testing.T) {
	dm := disconnectedManager()
	wd := &mockWDADriverUI{}
	uc := NewUiController(wd, dm)

	err := uc.Tap(100, 200)
	if err == nil {
		t.Fatal("expected error when not connected")
	}
	protoErr, ok := err.(*protocol.Error)
	if !ok {
		t.Fatalf("expected *protocol.Error, got %T: %v", err, err)
	}
	if protoErr.Code != protocol.ErrDeviceNotConnected.Code {
		t.Errorf("expected ErrDeviceNotConnected (code %d), got code %d", protocol.ErrDeviceNotConnected.Code, protoErr.Code)
	}
}

// ---- Swipe tests ----

func TestSwipe_FullMode(t *testing.T) {
	dm := fullModeManager(t)
	wd := &mockWDADriverUI{}
	uc := NewUiController(wd, dm)

	if err := uc.Swipe(10, 20, 30, 40); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !wd.swipeCalled {
		t.Error("expected WDA Swipe to be called")
	}
	if wd.swipeX1 != 10 || wd.swipeY1 != 20 || wd.swipeX2 != 30 || wd.swipeY2 != 40 {
		t.Errorf("unexpected swipe coords: (%d,%d)->(%d,%d)", wd.swipeX1, wd.swipeY1, wd.swipeX2, wd.swipeY2)
	}
}

func TestSwipe_DegradedMode(t *testing.T) {
	dm := degradedModeManager(t)
	wd := &mockWDADriverUI{}
	uc := NewUiController(wd, dm)

	err := uc.Swipe(0, 0, 100, 100)
	if err == nil {
		t.Fatal("expected error in degraded mode")
	}
	protoErr, ok := err.(*protocol.Error)
	if !ok {
		t.Fatalf("expected *protocol.Error, got %T", err)
	}
	if protoErr.Code != protocol.ErrWDAUnavailable.Code {
		t.Errorf("expected ErrWDAUnavailable, got code %d", protoErr.Code)
	}
}

func TestSwipe_NotConnected(t *testing.T) {
	dm := disconnectedManager()
	wd := &mockWDADriverUI{}
	uc := NewUiController(wd, dm)

	err := uc.Swipe(0, 0, 100, 100)
	if err == nil {
		t.Fatal("expected error when not connected")
	}
	protoErr, ok := err.(*protocol.Error)
	if !ok {
		t.Fatalf("expected *protocol.Error, got %T", err)
	}
	if protoErr.Code != protocol.ErrDeviceNotConnected.Code {
		t.Errorf("expected ErrDeviceNotConnected, got code %d", protoErr.Code)
	}
}

// ---- Input tests ----

func TestInput_FullMode(t *testing.T) {
	dm := fullModeManager(t)
	wd := &mockWDADriverUI{}
	uc := NewUiController(wd, dm)

	if err := uc.Input("hello world"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !wd.inputCalled {
		t.Error("expected WDA InputText to be called")
	}
	if wd.inputText != "hello world" {
		t.Errorf("expected input text %q, got %q", "hello world", wd.inputText)
	}
}

func TestInput_DegradedMode(t *testing.T) {
	dm := degradedModeManager(t)
	wd := &mockWDADriverUI{}
	uc := NewUiController(wd, dm)

	err := uc.Input("text")
	if err == nil {
		t.Fatal("expected error in degraded mode")
	}
	protoErr, ok := err.(*protocol.Error)
	if !ok {
		t.Fatalf("expected *protocol.Error, got %T", err)
	}
	if protoErr.Code != protocol.ErrWDAUnavailable.Code {
		t.Errorf("expected ErrWDAUnavailable, got code %d", protoErr.Code)
	}
}

func TestInput_NotConnected(t *testing.T) {
	dm := disconnectedManager()
	wd := &mockWDADriverUI{}
	uc := NewUiController(wd, dm)

	err := uc.Input("text")
	if err == nil {
		t.Fatal("expected error when not connected")
	}
	protoErr, ok := err.(*protocol.Error)
	if !ok {
		t.Fatalf("expected *protocol.Error, got %T", err)
	}
	if protoErr.Code != protocol.ErrDeviceNotConnected.Code {
		t.Errorf("expected ErrDeviceNotConnected, got code %d", protoErr.Code)
	}
}

// ---- Press tests ----

func TestPress_FullMode(t *testing.T) {
	dm := fullModeManager(t)
	wd := &mockWDADriverUI{}
	uc := NewUiController(wd, dm)

	if err := uc.Press("home"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !wd.pressCalled {
		t.Error("expected WDA PressButton to be called")
	}
	if wd.pressKey != "home" {
		t.Errorf("expected key %q, got %q", "home", wd.pressKey)
	}
}

func TestPress_DegradedMode(t *testing.T) {
	dm := degradedModeManager(t)
	wd := &mockWDADriverUI{}
	uc := NewUiController(wd, dm)

	err := uc.Press("home")
	if err == nil {
		t.Fatal("expected error in degraded mode")
	}
	protoErr, ok := err.(*protocol.Error)
	if !ok {
		t.Fatalf("expected *protocol.Error, got %T", err)
	}
	if protoErr.Code != protocol.ErrWDAUnavailable.Code {
		t.Errorf("expected ErrWDAUnavailable, got code %d", protoErr.Code)
	}
}

func TestPress_NotConnected(t *testing.T) {
	dm := disconnectedManager()
	wd := &mockWDADriverUI{}
	uc := NewUiController(wd, dm)

	err := uc.Press("home")
	if err == nil {
		t.Fatal("expected error when not connected")
	}
	protoErr, ok := err.(*protocol.Error)
	if !ok {
		t.Fatalf("expected *protocol.Error, got %T", err)
	}
	if protoErr.Code != protocol.ErrDeviceNotConnected.Code {
		t.Errorf("expected ErrDeviceNotConnected, got code %d", protoErr.Code)
	}
}
