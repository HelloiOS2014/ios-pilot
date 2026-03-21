package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	goios "github.com/danielpaulus/go-ios/ios"
	"github.com/danielpaulus/go-ios/ios/tunnel"
)

// cmdTunnel handles the hidden "tunnel" subcommand used internally by
// go-ios's RunAgent to spawn the tunnel agent process.
//
// Usage: ios-pilot tunnel start [--userspace]
func cmdTunnel(args []string) int {
	if len(args) == 0 || args[0] != "start" {
		fmt.Fprintf(os.Stderr, "usage: ios-pilot tunnel start [--userspace]\n")
		return 1
	}

	userspace := false
	for _, a := range args[1:] {
		if a == "--userspace" {
			userspace = true
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sigCh
		cancel()
	}()

	pm, err := tunnel.NewPairRecordManager("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: create pair record manager: %v\n", err)
		return 1
	}

	tm := tunnel.NewTunnelManager(pm, userspace)

	// Periodically update tunnels for connected devices.
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := tm.UpdateTunnels(ctx); err != nil {
					slog.Warn("failed to update tunnels", "error", err)
				}
			}
		}
	}()

	// Serve tunnel info HTTP API.
	go func() {
		if err := tunnel.ServeTunnelInfo(tm, goios.HttpApiPort()); err != nil {
			fmt.Fprintf(os.Stderr, "error: tunnel info server: %v\n", err)
			cancel()
		}
	}()

	slog.Info("tunnel agent started", "port", goios.HttpApiPort(), "userspace", userspace)
	<-ctx.Done()
	return 0
}
