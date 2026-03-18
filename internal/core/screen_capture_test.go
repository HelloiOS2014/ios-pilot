package core

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ios-pilot/internal/config"
	"ios-pilot/internal/driver"
)

// ---- mock screenshot driver ----

type mockScreenshotDriver struct {
	data []byte
	err  error
}

func (m *mockScreenshotDriver) TakeScreenshot(_ string) ([]byte, error) {
	return m.data, m.err
}

// ---- WDA mock with element support (used in screen capture tests) ----

type mockWDADriverWithElements struct {
	mockWDADriver
	elements      []driver.WDAElement
	elementsErr   error
	screenshotData []byte
	screenshotErr  error
}

func (m *mockWDADriverWithElements) GetInteractiveElements(_, _ string, _ []string) ([]driver.WDAElement, error) {
	return m.elements, m.elementsErr
}

func (m *mockWDADriverWithElements) GetElementTree(_, _ string) ([]driver.WDAElement, error) {
	return m.elements, m.elementsErr
}

func (m *mockWDADriverWithElements) Screenshot(_, _ string) ([]byte, error) {
	return m.screenshotData, m.screenshotErr
}

// ---- helpers ----

// makePNG creates a minimal valid PNG byte slice of the given dimensions.
func makePNG(w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, color.RGBA{R: 100, G: 150, B: 200, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

// screenshotCfg returns a config pointing to a temp directory for screenshots.
func screenshotCfg(t *testing.T) *config.Config {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Default()
	cfg.Screenshot.Dir = dir
	return &cfg
}

// connectedDegradedManager builds a DeviceManager in degraded mode (no WDA).
func connectedDegradedManager(t *testing.T) *DeviceManager {
	t.Helper()
	dd := &mockDeviceDriver{devices: singleDevice()}
	wd := &mockWDADriver{alive: false}
	dm := NewDeviceManager(dd, nil, wd, nil, defaultCfg())
	if _, err := dm.Connect("abc123"); err != nil {
		t.Fatalf("setup Connect: %v", err)
	}
	return dm
}

// ---- tests ----

func TestTakeScreenshot_SavesFileAndReturnsPath(t *testing.T) {
	pngData := makePNG(100, 100)
	sd := &mockScreenshotDriver{data: pngData}
	cfg := screenshotCfg(t)
	dm := connectedDegradedManager(t)

	sc := NewScreenCapture(sd, nil, dm, cfg)

	path, err := sc.TakeScreenshot()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path == "" {
		t.Fatal("expected non-empty path")
	}
	if !strings.HasPrefix(filepath.Base(path), "screenshot-") {
		t.Errorf("unexpected filename prefix: %q", filepath.Base(path))
	}

	data, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("file not readable: %v", readErr)
	}
	if !bytes.Equal(data, pngData) {
		t.Error("saved file content does not match original PNG bytes")
	}
}

func TestLook_ScreenshotOnly(t *testing.T) {
	pngData := makePNG(200, 300)
	sd := &mockScreenshotDriver{data: pngData}
	cfg := screenshotCfg(t)
	dm := connectedDegradedManager(t)

	sc := NewScreenCapture(sd, nil, dm, cfg)

	result, err := sc.Look(false, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Screenshot == "" {
		t.Error("expected non-empty screenshot path")
	}
	if len(result.Elements) != 0 {
		t.Errorf("expected no elements, got %d", len(result.Elements))
	}
	if result.ScreenSize[0] != 200 || result.ScreenSize[1] != 300 {
		t.Errorf("unexpected screen size: %v", result.ScreenSize)
	}
}

func TestLook_Annotate_FullMode(t *testing.T) {
	pngData := makePNG(400, 600)
	sd := &mockScreenshotDriver{data: pngData}
	cfg := screenshotCfg(t)

	elements := []driver.WDAElement{
		{Type: "button", Label: "OK", Frame: [4]int{10, 20, 80, 40}},
		{Type: "textfield", Label: "Username", Frame: [4]int{10, 80, 200, 44}},
	}
	wd := &mockWDADriverWithElements{
		mockWDADriver: mockWDADriver{alive: true, sessionID: "sess-1"},
		elements:      elements,
	}

	dd := &mockDeviceDriver{devices: singleDevice()}
	dm := NewDeviceManager(dd, nil, wd, nil, defaultCfg())
	if _, err := dm.Connect("abc123"); err != nil {
		t.Fatalf("connect: %v", err)
	}

	sc := NewScreenCapture(sd, wd, dm, cfg)

	result, err := sc.Look(true, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Elements) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(result.Elements))
	}

	// IDs are 1-based.
	if result.Elements[0].ID != 1 {
		t.Errorf("expected first element ID=1, got %d", result.Elements[0].ID)
	}
	if result.Elements[1].ID != 2 {
		t.Errorf("expected second element ID=2, got %d", result.Elements[1].ID)
	}

	// Center = (x + w/2, y + h/2).
	el0 := result.Elements[0]
	expectedCX0 := 10 + 80/2  // 50
	expectedCY0 := 20 + 40/2  // 40
	if el0.Center[0] != expectedCX0 || el0.Center[1] != expectedCY0 {
		t.Errorf("element 0 center: want [%d,%d], got %v", expectedCX0, expectedCY0, el0.Center)
	}

	// Annotated path uses "annotated-" prefix.
	if result.Screenshot == "" {
		t.Error("expected non-empty screenshot path")
	}
	if !strings.Contains(filepath.Base(result.Screenshot), "annotated-") {
		t.Errorf("expected annotated filename, got %q", filepath.Base(result.Screenshot))
	}
	if _, statErr := os.Stat(result.Screenshot); statErr != nil {
		t.Errorf("annotated screenshot file not found: %v", statErr)
	}
}

func TestLook_Annotate_DegradedMode(t *testing.T) {
	pngData := makePNG(100, 100)
	sd := &mockScreenshotDriver{data: pngData}
	cfg := screenshotCfg(t)
	dm := connectedDegradedManager(t)

	sc := NewScreenCapture(sd, nil, dm, cfg)

	// annotate=true but in degraded mode — should return plain screenshot, no elements.
	result, err := sc.Look(true, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Mode != "degraded" {
		t.Errorf("expected degraded mode, got %q", result.Mode)
	}
	if len(result.Elements) != 0 {
		t.Errorf("expected no elements in degraded mode, got %d", len(result.Elements))
	}
	if result.Screenshot == "" {
		t.Error("expected non-empty screenshot path even in degraded mode")
	}
}

func TestLook_UI_FullMode(t *testing.T) {
	pngData := makePNG(300, 500)
	sd := &mockScreenshotDriver{data: pngData}
	cfg := screenshotCfg(t)

	elements := []driver.WDAElement{
		{Type: "cell", Label: "Item 1", Frame: [4]int{0, 0, 300, 44}},
	}
	wd := &mockWDADriverWithElements{
		mockWDADriver: mockWDADriver{alive: true, sessionID: "sess-1"},
		elements:      elements,
	}

	dd := &mockDeviceDriver{devices: singleDevice()}
	dm := NewDeviceManager(dd, nil, wd, nil, defaultCfg())
	if _, err := dm.Connect("abc123"); err != nil {
		t.Fatalf("connect: %v", err)
	}

	sc := NewScreenCapture(sd, wd, dm, cfg)

	// ui=true, annotate=false — elements returned but screenshot is plain.
	result, err := sc.Look(false, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Elements) != 1 {
		t.Fatalf("expected 1 element, got %d", len(result.Elements))
	}
	if strings.Contains(filepath.Base(result.Screenshot), "annotated-") {
		t.Errorf("did not expect annotated screenshot for ui-only look, got %q", result.Screenshot)
	}
}

func TestDrawAnnotations_ProducesValidPNG(t *testing.T) {
	cfg := screenshotCfg(t)
	// ScreenCapture is only used for its drawAnnotations method here.
	sc := &ScreenCapture{config: cfg}

	src := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			src.SetRGBA(x, y, color.RGBA{R: 128, G: 128, B: 128, A: 255})
		}
	}

	elements := []ElementInfo{
		{ID: 1, Type: "button", Label: "A", Frame: [4]int{5, 5, 40, 20}, Center: [2]int{25, 15}},
		{ID: 2, Type: "button", Label: "B", Frame: [4]int{55, 5, 40, 20}, Center: [2]int{75, 15}},
	}

	out, err := sc.drawAnnotations(src, elements)
	if err != nil {
		t.Fatalf("drawAnnotations error: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("expected non-empty PNG output")
	}

	decoded, decErr := png.Decode(bytes.NewReader(out))
	if decErr != nil {
		t.Fatalf("output is not a valid PNG: %v", decErr)
	}
	b := decoded.Bounds()
	if b.Dx() != 100 || b.Dy() != 100 {
		t.Errorf("annotated image size changed: want 100×100, got %dx%d", b.Dx(), b.Dy())
	}
}

func TestTakeScreenshot_ErrorPropagates(t *testing.T) {
	captureErr := fmt.Errorf("capture failed")
	sd := &mockScreenshotDriver{err: captureErr}
	cfg := screenshotCfg(t)
	dm := connectedDegradedManager(t)

	sc := NewScreenCapture(sd, nil, dm, cfg)

	_, err := sc.TakeScreenshot()
	if err == nil {
		t.Fatal("expected error from TakeScreenshot when driver fails")
	}
}

func TestCaptureRaw_FallbackToWDA(t *testing.T) {
	// instruments fails
	sd := &mockScreenshotDriver{err: fmt.Errorf("instruments: no developer image")}

	// WDA screenshot succeeds
	wdaPNG := makePNG(100, 100)
	wd := &mockWDADriverWithElements{
		mockWDADriver:  mockWDADriver{alive: true, sessionID: "sess-1"},
		screenshotData: wdaPNG,
	}

	dd := &mockDeviceDriver{devices: singleDevice()}
	dm := NewDeviceManager(dd, nil, wd, nil, defaultCfg())
	if _, err := dm.Connect("abc123"); err != nil {
		t.Fatalf("connect: %v", err)
	}

	cfg := screenshotCfg(t)
	sc := NewScreenCapture(sd, wd, dm, cfg)

	path, err := sc.TakeScreenshot()
	if err != nil {
		t.Fatalf("expected WDA fallback to succeed, got: %v", err)
	}

	data, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("file not readable: %v", readErr)
	}
	if !bytes.Equal(data, wdaPNG) {
		t.Error("saved file should contain WDA screenshot data")
	}
}

func TestCaptureRaw_BothFail(t *testing.T) {
	instrumentsErr := fmt.Errorf("instruments: no developer image")
	sd := &mockScreenshotDriver{err: instrumentsErr}

	wd := &mockWDADriverWithElements{
		mockWDADriver: mockWDADriver{alive: true, sessionID: "sess-1"},
		screenshotErr: fmt.Errorf("wda: connection refused"),
	}

	dd := &mockDeviceDriver{devices: singleDevice()}
	dm := NewDeviceManager(dd, nil, wd, nil, defaultCfg())
	if _, err := dm.Connect("abc123"); err != nil {
		t.Fatalf("connect: %v", err)
	}

	cfg := screenshotCfg(t)
	sc := NewScreenCapture(sd, wd, dm, cfg)

	_, err := sc.TakeScreenshot()
	if err == nil {
		t.Fatal("expected error when both instruments and WDA fail")
	}
	// Should wrap the instruments error (more diagnostic value).
	if !strings.Contains(err.Error(), "instruments") {
		t.Errorf("expected instruments error in message, got: %v", err)
	}
}
