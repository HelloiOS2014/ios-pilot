// Package driver defines interfaces for iOS device interaction.
// All driver implementations must satisfy these interfaces for testability.
package driver

import (
	"context"
	"io"
)

// DeviceInfo holds basic information about a connected iOS device.
type DeviceInfo struct {
	UDID        string `json:"udid"`
	Name        string `json:"name"`
	IOSVersion  string `json:"ios_version"`
	ProductType string `json:"product_type"`
}

// DeviceDriver enumerates and describes connected devices.
type DeviceDriver interface {
	ListDevices() ([]DeviceInfo, error)
	GetDevice(udid string) (*DeviceInfo, error)
}

// AppInfo holds basic information about an installed application.
type AppInfo struct {
	BundleID string `json:"bundle_id"`
	Name     string `json:"name"`
	Version  string `json:"version"`
}

// AppDriver manages installed applications on a device.
type AppDriver interface {
	ListApps(udid string) ([]AppInfo, error)
	Install(udid string, path string) error
	Uninstall(udid string, bundleID string) error
	// Launch returns the PID of the launched process.
	Launch(udid string, bundleID string) (int, error)
	Kill(udid string, bundleID string) error
	ForegroundApp(udid string) (string, error)
}

// ScreenshotDriver captures the device screen.
type ScreenshotDriver interface {
	// TakeScreenshot returns raw PNG bytes of the current screen.
	TakeScreenshot(udid string) ([]byte, error)
}

// LogEntry represents a single parsed syslog message.
type LogEntry struct {
	Timestamp string `json:"timestamp"`
	Process   string `json:"process"`
	PID       string `json:"pid"`
	Level     string `json:"level"`
	Message   string `json:"message"`
	Raw       string `json:"raw"`
}

// SyslogDriver streams device syslog entries to the caller.
type SyslogDriver interface {
	// StartSyslog begins streaming log entries into ch.
	// It runs until ctx is cancelled, then returns.
	StartSyslog(ctx context.Context, udid string, ch chan<- LogEntry) error
}

// CrashReport represents a crash report on the device.
type CrashReport struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Process   string `json:"process"`
	Timestamp string `json:"timestamp"`
	Content   string `json:"content,omitempty"`
}

// CrashDriver lists and retrieves crash reports from a device.
type CrashDriver interface {
	ListCrashes(udid string) ([]CrashReport, error)
	GetCrash(udid string, id string) (*CrashReport, error)
}

// TunnelDriver manages the go-ios tunnel agent required for iOS 17+ devices.
type TunnelDriver interface {
	EnsureTunnel(udid string) error
	IsTunnelRunning(udid string) bool
	StopTunnel(udid string) error
	// ForwardPort forwards hostPort on localhost to devicePort on the iOS device.
	// The returned io.Closer stops the forwarding when closed.
	ForwardPort(udid string, hostPort, devicePort uint16) (io.Closer, error)
}

// WDAProcessDriver manages the WDA process lifecycle on a device.
// This is separate from WDADriver which handles HTTP API communication.
type WDAProcessDriver interface {
	// StartWDA launches WDA on the device. Blocks until WDA is ready or ctx is cancelled.
	// Returns a stopper that kills the WDA process when closed.
	StartWDA(ctx context.Context, udid string) (io.Closer, error)
}

// WDAElement describes a UI element returned by WebDriverAgent.
type WDAElement struct {
	Type   string `json:"type"`
	Label  string `json:"label"`
	Frame  [4]int `json:"frame"`
	Center [2]int `json:"center"`
}

// WDADriver communicates with a running WebDriverAgent instance.
// Implementation lives in Task 4.
type WDADriver interface {
	CreateSession(wdaURL string) (string, error)
	Status(wdaURL string) (bool, error)
	DeleteSession(wdaURL string, sessionID string) error
	GetElementTree(wdaURL string, sessionID string) ([]WDAElement, error)
	GetInteractiveElements(wdaURL string, sessionID string, types []string) ([]WDAElement, error)
	FindElement(wdaURL string, sessionID string, using string, value string) (*WDAElement, error)
	Tap(wdaURL string, sessionID string, x, y int) error
	Swipe(wdaURL string, sessionID string, x1, y1, x2, y2 int) error
	InputText(wdaURL string, sessionID string, text string) error
	PressButton(wdaURL string, sessionID string, button string) error
	Screenshot(wdaURL string, sessionID string) ([]byte, error)
}
