package goios

import (
	"fmt"
	"net/http"
	"time"

	goios "github.com/danielpaulus/go-ios/ios"
	"github.com/danielpaulus/go-ios/ios/tunnel"
	"ios-pilot/internal/driver"
)

// GoIosTunnelDriver implements driver.TunnelDriver using the go-ios tunnel agent.
//
// The tunnel agent is a separate process (go-ios itself) that maintains a
// TLS tunnel for each connected iOS 17+ device.  This driver manages that
// agent via the HTTP API that go-ios exposes on localhost.
type GoIosTunnelDriver struct {
	// httpClient is used to talk to the go-ios agent HTTP API.
	httpClient *http.Client
}

// Compile-time interface check.
var _ driver.TunnelDriver = (*GoIosTunnelDriver)(nil)

// NewTunnelDriver creates a new GoIosTunnelDriver.
func NewTunnelDriver() *GoIosTunnelDriver {
	return &GoIosTunnelDriver{
		httpClient: &http.Client{Timeout: 2 * time.Second},
	}
}

// IsTunnelRunning reports whether the go-ios tunnel agent is running and
// has an active tunnel for the given device UDID.
// If udid is empty, it only checks whether the agent itself is running.
func (t *GoIosTunnelDriver) IsTunnelRunning(udid string) bool {
	if !tunnel.IsAgentRunning() {
		return false
	}
	if udid == "" {
		return true
	}
	// Query the agent for a specific device tunnel.
	info, err := tunnel.TunnelInfoForDevice(udid, goios.HttpApiHost(), goios.HttpApiPort())
	if err != nil {
		return false
	}
	return info.Udid == udid
}

// EnsureTunnel starts the go-ios tunnel agent in userspace mode if it is
// not already running and waits until it reports ready.
// If udid is non-empty, this does not wait for a device-specific tunnel;
// the agent will establish one automatically when the device connects.
func (t *GoIosTunnelDriver) EnsureTunnel(udid string) error {
	if err := tunnel.RunAgent("user"); err != nil {
		return fmt.Errorf("ensure tunnel: start agent: %w", err)
	}
	if !tunnel.WaitUntilAgentReady() {
		return fmt.Errorf("ensure tunnel: agent did not become ready")
	}
	return nil
}

// StopTunnel stops an individual device tunnel (if udid is non-empty) or
// shuts down the entire tunnel agent (if udid is empty).
func (t *GoIosTunnelDriver) StopTunnel(udid string) error {
	if udid == "" {
		return tunnel.CloseAgent()
	}

	// Delete the device-specific tunnel via the agent REST API.
	req, err := http.NewRequest(
		http.MethodDelete,
		fmt.Sprintf("http://%s:%d/tunnel/%s", goios.HttpApiHost(), goios.HttpApiPort(), udid),
		nil,
	)
	if err != nil {
		return fmt.Errorf("stop tunnel %q: build request: %w", udid, err)
	}

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("stop tunnel %q: http: %w", udid, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("stop tunnel %q: agent returned status %d", udid, resp.StatusCode)
	}
	return nil
}
