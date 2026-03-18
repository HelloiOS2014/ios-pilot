package core

import (
	"fmt"
	"strings"

	"ios-pilot/internal/driver"
)

// ScreenCaptureInterface is the minimal interface Checker needs from the
// screenshot subsystem (Task 6).  Defined here so Checker can be tested
// independently.
type ScreenCaptureInterface interface {
	// TakeScreenshot returns the file path of the saved screenshot.
	TakeScreenshot() (string, error)
}

// DeviceManagerInterface is a read-only view of DeviceManager needed by Checker.
type DeviceManagerInterface interface {
	IsConnected() bool
	ConnectedDevice() *driver.DeviceInfo
	Mode() string
	WDAURL() string
	WDASessionID() string
}

// CheckResult is the structured outcome of a check operation.
// Pass is a *bool so that nil marshals to JSON null (used by Screen where
// the LLM decides pass/fail from the screenshot).
type CheckResult struct {
	Pass       *bool  `json:"pass"`
	Detail     string `json:"detail"`
	Screenshot string `json:"screenshot"`
}

// boolPtr is a convenience helper that returns a pointer to v.
func boolPtr(v bool) *bool { return &v }

// Checker performs assertion-style checks against a connected device.
type Checker struct {
	screenCapture ScreenCaptureInterface
	wdaDriver     driver.WDADriver
	appDriver     driver.AppDriver
	crashDrv      driver.CrashDriver
	deviceManager DeviceManagerInterface
}

// NewChecker constructs a Checker.
func NewChecker(
	sc ScreenCaptureInterface,
	wd driver.WDADriver,
	ad driver.AppDriver,
	cd driver.CrashDriver,
	dm DeviceManagerInterface,
) *Checker {
	return &Checker{
		screenCapture: sc,
		wdaDriver:     wd,
		appDriver:     ad,
		crashDrv:      cd,
		deviceManager: dm,
	}
}

// screenshot is a helper that takes a screenshot and returns its path.
// Returns an empty string on error (non-fatal; caller decides).
func (c *Checker) screenshot() string {
	if c.screenCapture == nil {
		return ""
	}
	path, err := c.screenCapture.TakeScreenshot()
	if err != nil {
		return ""
	}
	return path
}

// Screen takes a screenshot and returns a CheckResult with pass=nil so that
// an LLM can decide pass/fail from the image.
func (c *Checker) Screen() (*CheckResult, error) {
	shot := c.screenshot()
	return &CheckResult{
		Pass:       nil,
		Detail:     "screenshot taken; pass/fail determined by LLM",
		Screenshot: shot,
	}, nil
}

// Element searches the WDA element tree for an element whose Label or Name
// contains text (case-insensitive substring match).
// pass=true if found, pass=false if not found.
func (c *Checker) Element(text string) (*CheckResult, error) {
	if c.wdaDriver == nil {
		return nil, fmt.Errorf("WDA driver not available")
	}

	wdaURL := c.deviceManager.WDAURL()
	sessionID := c.deviceManager.WDASessionID()

	elements, err := c.wdaDriver.GetElementTree(wdaURL, sessionID)
	if err != nil {
		return nil, fmt.Errorf("get element tree: %w", err)
	}

	found := false
	lower := strings.ToLower(text)
	for _, el := range elements {
		if strings.Contains(strings.ToLower(el.Label), lower) ||
			strings.Contains(strings.ToLower(el.Type), lower) {
			found = true
			break
		}
	}

	detail := fmt.Sprintf("element %q not found", text)
	if found {
		detail = fmt.Sprintf("element %q found", text)
	}

	return &CheckResult{
		Pass:       boolPtr(found),
		Detail:     detail,
		Screenshot: c.screenshot(),
	}, nil
}

// AppRunning checks whether bundleID is the foreground application.
// pass=true if it is, pass=false otherwise.
func (c *Checker) AppRunning(bundleID string) (*CheckResult, error) {
	if c.appDriver == nil {
		return nil, fmt.Errorf("app driver not available")
	}

	var udid string
	if dev := c.deviceManager.ConnectedDevice(); dev != nil {
		udid = dev.UDID
	}

	fg, err := c.appDriver.ForegroundApp(udid)
	if err != nil {
		return nil, fmt.Errorf("foreground app: %w", err)
	}

	match := fg == bundleID
	detail := fmt.Sprintf("foreground app is %q, expected %q", fg, bundleID)
	if match {
		detail = fmt.Sprintf("app %q is in foreground", bundleID)
	}

	return &CheckResult{
		Pass:       boolPtr(match),
		Detail:     detail,
		Screenshot: c.screenshot(),
	}, nil
}

// NoCrash checks whether any crash reports exist for bundleID.
// pass=true if no crashes found, pass=false if crashes exist.
func (c *Checker) NoCrash(bundleID string) (*CheckResult, error) {
	if c.crashDrv == nil {
		return nil, fmt.Errorf("crash driver not available")
	}

	var udid string
	if dev := c.deviceManager.ConnectedDevice(); dev != nil {
		udid = dev.UDID
	}

	crashes, err := c.crashDrv.ListCrashes(udid)
	if err != nil {
		return nil, fmt.Errorf("list crashes: %w", err)
	}

	// Filter crashes that belong to the given bundleID (match on Process field).
	relevant := 0
	for _, cr := range crashes {
		if strings.Contains(cr.Process, bundleID) || strings.Contains(cr.Name, bundleID) {
			relevant++
		}
	}

	noCrash := relevant == 0
	detail := fmt.Sprintf("no crashes found for %q", bundleID)
	if !noCrash {
		detail = fmt.Sprintf("%d crash(es) found for %q", relevant, bundleID)
	}

	return &CheckResult{
		Pass:       boolPtr(noCrash),
		Detail:     detail,
		Screenshot: c.screenshot(),
	}, nil
}
