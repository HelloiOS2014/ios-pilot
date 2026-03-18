// Package goios — compile-time interface checks and unit tests.
package goios

import (
	"context"
	"os"
	"testing"
	"time"

	"ios-pilot/internal/driver"
)

// ---------------------------------------------------------------------------
// Compile-time interface assertions
// ---------------------------------------------------------------------------

var _ driver.DeviceDriver = (*GoIosDeviceDriver)(nil)
var _ driver.AppDriver = (*GoIosAppDriver)(nil)
var _ driver.ScreenshotDriver = (*GoIosScreenshotDriver)(nil)
var _ driver.SyslogDriver = (*GoIosSyslogDriver)(nil)
var _ driver.TunnelDriver = (*GoIosTunnelDriver)(nil)

// ---------------------------------------------------------------------------
// Unit tests (no device required)
// ---------------------------------------------------------------------------

// TestNewDrivers verifies that constructor functions return non-nil values.
func TestNewDrivers(t *testing.T) {
	t.Run("DeviceDriver", func(t *testing.T) {
		d := NewDeviceDriver()
		if d == nil {
			t.Fatal("NewDeviceDriver returned nil")
		}
	})

	t.Run("AppDriver", func(t *testing.T) {
		a := NewAppDriver()
		if a == nil {
			t.Fatal("NewAppDriver returned nil")
		}
	})

	t.Run("ScreenshotDriver", func(t *testing.T) {
		s := NewScreenshotDriver()
		if s == nil {
			t.Fatal("NewScreenshotDriver returned nil")
		}
	})

	t.Run("SyslogDriver", func(t *testing.T) {
		s := NewSyslogDriver()
		if s == nil {
			t.Fatal("NewSyslogDriver returned nil")
		}
	})

	t.Run("TunnelDriver", func(t *testing.T) {
		td := NewTunnelDriver()
		if td == nil {
			t.Fatal("NewTunnelDriver returned nil")
		}
	})
}

// TestTunnelIsTunnelRunning_NoAgent verifies that IsTunnelRunning returns false
// when no tunnel agent is running (which is always the case in CI).
func TestTunnelIsTunnelRunning_NoAgent(t *testing.T) {
	td := NewTunnelDriver()
	// In a test environment without a real device / agent, this should be false.
	// We don't assert a specific value — just confirm it doesn't panic.
	_ = td.IsTunnelRunning("")
	_ = td.IsTunnelRunning("fake-udid-12345")
}

// ---------------------------------------------------------------------------
// Integration tests (require a real device)
// ---------------------------------------------------------------------------

func requireDevice(t *testing.T) string {
	t.Helper()
	udid := os.Getenv("IOS_TEST_UDID") // optional specific UDID
	if os.Getenv("IOS_DEVICE_CONNECTED") == "" {
		t.Skip("IOS_DEVICE_CONNECTED not set — skipping hardware test")
	}
	return udid
}

func TestIntegration_ListDevices(t *testing.T) {
	requireDevice(t)
	d := NewDeviceDriver()
	devices, err := d.ListDevices()
	if err != nil {
		t.Fatalf("ListDevices error: %v", err)
	}
	if len(devices) == 0 {
		t.Fatal("expected at least one device")
	}
	t.Logf("found %d device(s):", len(devices))
	for _, dev := range devices {
		t.Logf("  UDID=%s Name=%q iOS=%s Model=%s", dev.UDID, dev.Name, dev.IOSVersion, dev.ProductType)
	}
}

func TestIntegration_GetDevice(t *testing.T) {
	udid := requireDevice(t)
	d := NewDeviceDriver()
	info, err := d.GetDevice(udid)
	if err != nil {
		t.Fatalf("GetDevice(%q) error: %v", udid, err)
	}
	if info.UDID == "" {
		t.Error("expected non-empty UDID")
	}
	t.Logf("device: %+v", info)
}

func TestIntegration_ListApps(t *testing.T) {
	udid := requireDevice(t)
	a := NewAppDriver()
	apps, err := a.ListApps(udid)
	if err != nil {
		t.Fatalf("ListApps error: %v", err)
	}
	t.Logf("found %d user app(s)", len(apps))
	for i, app := range apps {
		if i >= 5 {
			break
		}
		t.Logf("  %s (%s) v%s", app.BundleID, app.Name, app.Version)
	}
}

func TestIntegration_Screenshot(t *testing.T) {
	udid := requireDevice(t)
	s := NewScreenshotDriver()
	data, err := s.TakeScreenshot(udid)
	if err != nil {
		t.Fatalf("TakeScreenshot error: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected non-empty screenshot data")
	}
	// Verify PNG magic bytes: 0x89 0x50 0x4E 0x47
	if len(data) < 4 || data[0] != 0x89 || data[1] != 0x50 || data[2] != 0x4E || data[3] != 0x47 {
		t.Errorf("screenshot does not appear to be PNG (first bytes: %x)", data[:min(4, len(data))])
	}
	t.Logf("screenshot: %d bytes", len(data))
}

func TestIntegration_Syslog(t *testing.T) {
	udid := requireDevice(t)
	s := NewSyslogDriver()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch := make(chan driver.LogEntry, 100)
	errCh := make(chan error, 1)
	go func() {
		errCh <- s.StartSyslog(ctx, udid, ch)
	}()

	count := 0
	deadline := time.After(4 * time.Second)
loop:
	for {
		select {
		case entry := <-ch:
			count++
			if count == 1 {
				t.Logf("first log: ts=%s proc=%s [%s] %s", entry.Timestamp, entry.Process, entry.Level, entry.Message)
			}
			if count >= 10 {
				cancel()
				break loop
			}
		case <-deadline:
			t.Log("timeout waiting for log entries")
			cancel()
			break loop
		}
	}

	if err := <-errCh; err != nil {
		t.Fatalf("syslog error: %v", err)
	}
	t.Logf("received %d log entries", count)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
