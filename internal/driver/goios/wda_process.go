package goios

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	goios "github.com/danielpaulus/go-ios/ios"
	"github.com/danielpaulus/go-ios/ios/installationproxy"
	"github.com/danielpaulus/go-ios/ios/testmanagerd"
	"github.com/danielpaulus/go-ios/ios/tunnel"
	"ios-pilot/internal/driver"
)

// GoIosWDAProcessDriver implements driver.WDAProcessDriver using go-ios testmanagerd.
type GoIosWDAProcessDriver struct{}

// Compile-time interface check.
var _ driver.WDAProcessDriver = (*GoIosWDAProcessDriver)(nil)

// NewWDAProcessDriver creates a new GoIosWDAProcessDriver.
func NewWDAProcessDriver() *GoIosWDAProcessDriver {
	return &GoIosWDAProcessDriver{}
}

// wdaCloser cancels the context to kill the WDA process.
type wdaCloser struct {
	cancel context.CancelFunc
}

func (c *wdaCloser) Close() error {
	c.cancel()
	return nil
}

// StartWDA launches WDA on the device using testmanagerd.RunTestWithConfig.
// It blocks until WDA responds on localhost:8100 or ctx is cancelled.
// For iOS 17+ devices, it enriches the device entry with tunnel/RSD info.
func (d *GoIosWDAProcessDriver) StartWDA(ctx context.Context, udid string) (io.Closer, error) {
	entry, err := goios.GetDevice(udid)
	if err != nil {
		return nil, fmt.Errorf("start wda: get device %q: %w", udid, err)
	}

	// For iOS 17+ devices, enrich the device entry with tunnel info.
	if entry.SupportsRsd() {
		enriched, enrichErr := enrichWithTunnel(entry, udid)
		if enrichErr == nil {
			entry = enriched
		}
		// If enrichment fails, proceed anyway — testmanagerd will report the actual error.
	}

	// Discover WDA bundle ID from installed apps.
	bundleID, err := findWDABundleID(entry)
	if err != nil {
		return nil, fmt.Errorf("start wda: %w", err)
	}

	// Create a child context so we can kill WDA on Close().
	wdaCtx, cancel := context.WithCancel(ctx)

	listener := testmanagerd.NewTestListener(io.Discard, io.Discard, "")
	testConfig := testmanagerd.TestConfig{
		TestRunnerBundleId: bundleID,
		XctestConfigName:   "WebDriverAgentRunner.xctest",
		Device:             entry,
		Listener:           listener,
	}

	// Launch WDA in a background goroutine — it blocks until ctx is cancelled.
	errCh := make(chan error, 1)
	go func() {
		_, runErr := testmanagerd.RunTestWithConfig(wdaCtx, testConfig)
		errCh <- runErr
	}()

	// Wait for WDA to become ready on localhost:8100.
	if err := waitForWDA(wdaCtx, errCh); err != nil {
		cancel()
		return nil, fmt.Errorf("start wda: %w", err)
	}

	return &wdaCloser{cancel: cancel}, nil
}

// enrichWithTunnel queries the tunnel agent for this device's tunnel info
// and returns a DeviceEntry that includes the RSD provider, so testmanagerd
// can connect through the tunnel.
func enrichWithTunnel(entry goios.DeviceEntry, udid string) (goios.DeviceEntry, error) {
	info, err := tunnel.TunnelInfoForDevice(udid, goios.HttpApiHost(), goios.HttpApiPort())
	if err != nil {
		return entry, fmt.Errorf("get tunnel info: %w", err)
	}

	// Connect to RSD via the tunnel address.
	rsdService, err := goios.NewWithAddrPortDevice(info.Address, info.RsdPort, entry)
	if err != nil {
		return entry, fmt.Errorf("connect to RSD: %w", err)
	}
	defer rsdService.Close()

	rsdProvider, err := rsdService.Handshake()
	if err != nil {
		return entry, fmt.Errorf("RSD handshake: %w", err)
	}

	enriched, err := goios.GetDeviceWithAddress(udid, info.Address, rsdProvider)
	if err != nil {
		return entry, fmt.Errorf("get device with address: %w", err)
	}

	enriched.UserspaceTUN = info.UserspaceTUN
	enriched.UserspaceTUNPort = info.UserspaceTUNPort

	return enriched, nil
}

// findWDABundleID lists installed apps and returns the first bundle ID containing "WebDriverAgent".
func findWDABundleID(device goios.DeviceEntry) (string, error) {
	conn, err := installationproxy.New(device)
	if err != nil {
		return "", fmt.Errorf("connect to installation proxy: %w", err)
	}

	apps, err := conn.BrowseAllApps()
	if err != nil {
		return "", fmt.Errorf("browse apps: %w", err)
	}

	for _, app := range apps {
		bid := app.CFBundleIdentifier()
		if strings.Contains(bid, "WebDriverAgent") {
			return bid, nil
		}
	}

	return "", fmt.Errorf("WebDriverAgent not installed on device (no bundle ID containing 'WebDriverAgent' found)")
}

// waitForWDA polls localhost:8100/status until WDA responds or the context/process fails.
func waitForWDA(ctx context.Context, errCh <-chan error) error {
	client := &http.Client{Timeout: 2 * time.Second}
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	timeout := time.After(30 * time.Second)

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("context cancelled while waiting for WDA")
		case runErr := <-errCh:
			return fmt.Errorf("WDA process exited: %w", runErr)
		case <-timeout:
			return fmt.Errorf("WDA did not become ready within 30s")
		case <-ticker.C:
			resp, err := client.Get("http://localhost:8100/status")
			if err == nil {
				resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					return nil
				}
			}
		}
	}
}
