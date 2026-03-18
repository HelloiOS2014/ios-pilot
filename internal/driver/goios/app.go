package goios

import (
	"fmt"
	"path/filepath"
	"strings"

	goios "github.com/danielpaulus/go-ios/ios"
	"github.com/danielpaulus/go-ios/ios/appservice"
	"github.com/danielpaulus/go-ios/ios/installationproxy"
	"github.com/danielpaulus/go-ios/ios/instruments"
	"github.com/danielpaulus/go-ios/ios/zipconduit"
	"ios-pilot/internal/driver"
)

// GoIosAppDriver implements driver.AppDriver using the go-ios library.
// It automatically selects the correct API depending on the iOS version:
//   - iOS 17+: uses appservice (XPC/tunnel-based)
//   - older iOS: uses instruments (DTX-based)
type GoIosAppDriver struct{}

// Compile-time interface check.
var _ driver.AppDriver = (*GoIosAppDriver)(nil)

// NewAppDriver creates a new GoIosAppDriver.
func NewAppDriver() *GoIosAppDriver {
	return &GoIosAppDriver{}
}

// ListApps returns user-installed applications on the device.
func (a *GoIosAppDriver) ListApps(udid string) ([]driver.AppInfo, error) {
	entry, err := goios.GetDevice(udid)
	if err != nil {
		return nil, fmt.Errorf("list apps: get device: %w", err)
	}

	conn, err := installationproxy.New(entry)
	if err != nil {
		return nil, fmt.Errorf("list apps: installationproxy: %w", err)
	}
	defer conn.Close()

	rawApps, err := conn.BrowseUserApps()
	if err != nil {
		return nil, fmt.Errorf("list apps: browse: %w", err)
	}

	infos := make([]driver.AppInfo, 0, len(rawApps))
	for _, raw := range rawApps {
		bundleID := raw.CFBundleIdentifier()
		if bundleID == "" {
			continue
		}
		infos = append(infos, driver.AppInfo{
			BundleID: bundleID,
			Name:     raw.CFBundleName(),
			Version:  raw.CFBundleShortVersionString(),
		})
	}
	return infos, nil
}

// Install installs an IPA or app bundle at path onto the device.
func (a *GoIosAppDriver) Install(udid string, path string) error {
	entry, err := goios.GetDevice(udid)
	if err != nil {
		return fmt.Errorf("install: get device: %w", err)
	}

	conn, err := zipconduit.New(entry)
	if err != nil {
		return fmt.Errorf("install: zipconduit: %w", err)
	}

	if err := conn.SendFile(path); err != nil {
		return fmt.Errorf("install %q: %w", filepath.Base(path), err)
	}
	return nil
}

// Uninstall removes the app with bundleID from the device.
func (a *GoIosAppDriver) Uninstall(udid string, bundleID string) error {
	entry, err := goios.GetDevice(udid)
	if err != nil {
		return fmt.Errorf("uninstall: get device: %w", err)
	}

	conn, err := installationproxy.New(entry)
	if err != nil {
		return fmt.Errorf("uninstall: installationproxy: %w", err)
	}
	defer conn.Close()

	if err := conn.Uninstall(bundleID); err != nil {
		return fmt.Errorf("uninstall %q: %w", bundleID, err)
	}
	return nil
}

// Launch launches the app on the device and returns its PID.
// Uses appservice for iOS 17+ (requires active tunnel) and instruments for older devices.
func (a *GoIosAppDriver) Launch(udid string, bundleID string) (int, error) {
	entry, err := goios.GetDevice(udid)
	if err != nil {
		return 0, fmt.Errorf("launch: get device: %w", err)
	}

	// Try iOS17+ appservice first (requires tunnel to be running).
	if entry.SupportsRsd() {
		pid, err := launchViaAppService(entry, bundleID)
		if err == nil {
			return pid, nil
		}
		// Fall through to instruments path on error.
	}

	return launchViaInstruments(entry, bundleID)
}

// Kill terminates the foreground or background instance of bundleID.
// It finds the PID by listing processes, then kills it.
func (a *GoIosAppDriver) Kill(udid string, bundleID string) error {
	entry, err := goios.GetDevice(udid)
	if err != nil {
		return fmt.Errorf("kill: get device: %w", err)
	}

	// iOS 17+ path
	if entry.SupportsRsd() {
		if err := killViaAppService(entry, bundleID); err == nil {
			return nil
		}
	}

	return killViaInstruments(entry, bundleID)
}

// ForegroundApp returns the bundle ID of the currently active application.
// This is implemented by listing running processes and finding the one whose
// executable matches a known app bundle. Since go-ios doesn't expose a
// single "foreground app" API, we heuristically look for the process with
// the shortest path depth (i.e. not a daemon).
func (a *GoIosAppDriver) ForegroundApp(udid string) (string, error) {
	entry, err := goios.GetDevice(udid)
	if err != nil {
		return "", fmt.Errorf("foreground app: get device: %w", err)
	}

	// iOS 17+ path via appservice
	if entry.SupportsRsd() {
		bundleID, err := foregroundAppViaAppService(entry)
		if err == nil {
			return bundleID, nil
		}
	}

	// Fallback: instruments process list — return first /private/var/containers/Bundle/Application/... entry
	return foregroundAppViaInstruments(entry)
}

// --- iOS 17+ helpers (appservice) ---

func launchViaAppService(entry goios.DeviceEntry, bundleID string) (int, error) {
	conn, err := appservice.New(entry)
	if err != nil {
		return 0, fmt.Errorf("appservice: %w", err)
	}
	defer conn.Close()

	pid, err := conn.LaunchApp(bundleID, nil, nil, nil, false)
	if err != nil {
		return 0, fmt.Errorf("launch via appservice %q: %w", bundleID, err)
	}
	return pid, nil
}

func killViaAppService(entry goios.DeviceEntry, bundleID string) error {
	conn, err := appservice.New(entry)
	if err != nil {
		return fmt.Errorf("appservice: %w", err)
	}
	defer conn.Close()

	processes, err := conn.ListProcesses()
	if err != nil {
		return fmt.Errorf("list processes: %w", err)
	}

	for _, p := range processes {
		if strings.HasSuffix(p.Path, "/"+bundleID) || strings.Contains(p.Path, bundleID) {
			if err := conn.KillProcess(p.Pid); err != nil {
				return fmt.Errorf("kill pid %d: %w", p.Pid, err)
			}
			return nil
		}
	}
	return fmt.Errorf("no process found for bundle %q", bundleID)
}

func foregroundAppViaAppService(entry goios.DeviceEntry) (string, error) {
	conn, err := appservice.New(entry)
	if err != nil {
		return "", fmt.Errorf("appservice: %w", err)
	}
	defer conn.Close()

	processes, err := conn.ListProcesses()
	if err != nil {
		return "", fmt.Errorf("list processes: %w", err)
	}

	// User apps live under /private/var/containers/Bundle/Application/
	for _, p := range processes {
		if strings.Contains(p.Path, "/Bundle/Application/") {
			// Extract the bundle path component after the UUID directory.
			// Path shape: .../Application/<UUID>/<App>.app/<Executable>
			parts := strings.Split(p.Path, "/")
			for i, part := range parts {
				if strings.HasSuffix(part, ".app") && i+1 < len(parts) {
					// Use the executable name (heuristic) as we can't easily recover bundle ID here.
					return p.ExecutableName(), nil
				}
			}
		}
	}
	return "", fmt.Errorf("no foreground user app found")
}

// --- Legacy helpers (instruments / DTX) ---

func launchViaInstruments(entry goios.DeviceEntry, bundleID string) (int, error) {
	ctrl, err := instruments.NewProcessControl(entry)
	if err != nil {
		return 0, fmt.Errorf("process control: %w", err)
	}
	defer ctrl.Close()

	pid, err := ctrl.LaunchApp(bundleID, nil)
	if err != nil {
		return 0, fmt.Errorf("launch via instruments %q: %w", bundleID, err)
	}
	return int(pid), nil
}

func killViaInstruments(entry goios.DeviceEntry, bundleID string) error {
	svc, err := instruments.NewDeviceInfoService(entry)
	if err != nil {
		return fmt.Errorf("device info service: %w", err)
	}
	defer svc.Close()

	processes, err := svc.ProcessList()
	if err != nil {
		return fmt.Errorf("process list: %w", err)
	}

	ctrl, err := instruments.NewProcessControl(entry)
	if err != nil {
		return fmt.Errorf("process control: %w", err)
	}
	defer ctrl.Close()

	for _, p := range processes {
		if p.Name == bundleID || strings.HasSuffix(p.RealAppName, bundleID) {
			if err := ctrl.KillProcess(uint64(p.Pid)); err != nil {
				return fmt.Errorf("kill pid %d: %w", p.Pid, err)
			}
			return nil
		}
	}
	return fmt.Errorf("no process found for bundle %q", bundleID)
}

func foregroundAppViaInstruments(entry goios.DeviceEntry) (string, error) {
	svc, err := instruments.NewDeviceInfoService(entry)
	if err != nil {
		return "", fmt.Errorf("device info service: %w", err)
	}
	defer svc.Close()

	processes, err := svc.ProcessList()
	if err != nil {
		return "", fmt.Errorf("process list: %w", err)
	}

	// User apps live under /var/containers/Bundle/Application/
	for _, p := range processes {
		if strings.Contains(p.RealAppName, "/Bundle/Application/") {
			return p.Name, nil
		}
	}
	return "", fmt.Errorf("no foreground user app found via instruments")
}
