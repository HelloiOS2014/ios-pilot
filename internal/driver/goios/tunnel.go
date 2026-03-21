package goios

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	goios "github.com/danielpaulus/go-ios/ios"
	"github.com/danielpaulus/go-ios/ios/forward"
	"github.com/danielpaulus/go-ios/ios/tunnel"
	"ios-pilot/internal/driver"
)

// GoIosTunnelDriver implements driver.TunnelDriver by running the
// go-ios TunnelManager in-process. This is critical for userspace TUN
// on iOS 17+ because the TUN network stack is process-local — a
// separate tunnel agent process cannot share its TUN interface.
type GoIosTunnelDriver struct {
	mu     sync.Mutex
	tm     *tunnel.TunnelManager
	cancel context.CancelFunc
}

// Compile-time interface check.
var _ driver.TunnelDriver = (*GoIosTunnelDriver)(nil)

// NewTunnelDriver creates a new GoIosTunnelDriver.
func NewTunnelDriver() *GoIosTunnelDriver {
	return &GoIosTunnelDriver{}
}

// EnsureTunnel starts the in-process TunnelManager if not already running,
// then waits for a device-specific tunnel to appear.
func (t *GoIosTunnelDriver) EnsureTunnel(udid string) error {
	if err := t.ensureManager(); err != nil {
		return fmt.Errorf("ensure tunnel: %w", err)
	}

	if udid == "" {
		return nil
	}

	// Wait for the device-specific tunnel to appear (up to 15 s).
	for i := 0; i < 30; i++ {
		tun, err := t.tm.FindTunnel(udid)
		if err == nil && tun.Udid == udid {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("ensure tunnel: device tunnel for %s did not appear within 15s", udid)
}

// IsTunnelRunning reports whether the TunnelManager is running and
// (optionally) has an active tunnel for the given device UDID.
func (t *GoIosTunnelDriver) IsTunnelRunning(udid string) bool {
	t.mu.Lock()
	tm := t.tm
	t.mu.Unlock()

	if tm == nil {
		return false
	}
	if udid == "" {
		return true
	}
	tun, err := tm.FindTunnel(udid)
	if err != nil {
		return false
	}
	return tun.Udid == udid
}

// StopTunnel stops an individual device tunnel (if udid is non-empty)
// or shuts down the entire TunnelManager (if udid is empty).
func (t *GoIosTunnelDriver) StopTunnel(udid string) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.tm == nil {
		return nil
	}

	if udid == "" {
		if t.cancel != nil {
			t.cancel()
			t.cancel = nil
		}
		_ = t.tm.Close()
		t.tm = nil
		return nil
	}

	return t.tm.RemoveTunnel(context.Background(), udid)
}

// ForwardPort forwards hostPort on localhost to devicePort on the given iOS device.
// For iOS 17+ with userspace TUN, the device entry is enriched with tunnel info
// so that forwarding goes through the in-process tunnel.
func (t *GoIosTunnelDriver) ForwardPort(udid string, hostPort, devicePort uint16) (io.Closer, error) {
	entry, err := goios.GetDevice(udid)
	if err != nil {
		return nil, fmt.Errorf("forward port: get device %q: %w", udid, err)
	}

	// Enrich entry with tunnel info so forwarding goes through the tunnel.
	t.mu.Lock()
	tm := t.tm
	t.mu.Unlock()
	if tm != nil {
		tun, findErr := tm.FindTunnel(udid)
		if findErr == nil && tun.Udid == udid {
			entry.UserspaceTUN = tun.UserspaceTUN
			entry.UserspaceTUNPort = tun.UserspaceTUNPort
		}
	}

	listener, err := forward.Forward(entry, hostPort, devicePort)
	if err != nil {
		return nil, fmt.Errorf("forward port: %d -> %d: %w", hostPort, devicePort, err)
	}
	return listener, nil
}

// ensureManager creates and starts the in-process TunnelManager if it
// hasn't been started yet. It runs UpdateTunnels in a background goroutine
// and also starts the tunnel HTTP API so enrichWithTunnel can query it.
func (t *GoIosTunnelDriver) ensureManager() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.tm != nil {
		return nil
	}

	pm, err := tunnel.NewPairRecordManager("")
	if err != nil {
		return fmt.Errorf("create pair record manager: %w", err)
	}

	tm := tunnel.NewTunnelManager(pm, true) // userspace TUN
	ctx, cancel := context.WithCancel(context.Background())

	// Perform the first update synchronously so tunnels are ready before returning.
	if err := tm.UpdateTunnels(ctx); err != nil {
		slog.Warn("initial tunnel update", "error", err)
	}

	// Continue updating in the background.
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := tm.UpdateTunnels(ctx); err != nil {
					slog.Warn("tunnel update", "error", err)
				}
			}
		}
	}()

	// Serve tunnel HTTP API so enrichWithTunnel (in StartWDA) can query it.
	go func() {
		if err := tunnel.ServeTunnelInfo(tm, goios.HttpApiPort()); err != nil {
			slog.Error("tunnel info server", "error", err)
		}
	}()

	t.tm = tm
	t.cancel = cancel
	slog.Info("in-process tunnel manager started", "port", goios.HttpApiPort())
	return nil
}
