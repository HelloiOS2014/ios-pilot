// Package cli implements the ios-pilot command-line interface.
// Each command connects to the daemon via Unix socket, sends a JSON-RPC request,
// and prints the result as JSON to stdout.
package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"ios-pilot/internal/client"
	"ios-pilot/internal/config"
	"ios-pilot/internal/core"
	"ios-pilot/internal/daemon"
	"ios-pilot/internal/driver/goios"
	"ios-pilot/internal/driver/wda"
	"ios-pilot/internal/protocol"
)

const usage = `ios-pilot - AI-powered iOS device automation

Usage:
  ios-pilot <command> [options]

Commands:
  device      Device management (list, connect, status, disconnect)
  look        Observe device state (screenshot, UI tree, annotate)
  act         Execute UI actions (tap, swipe, input, press)
  app         Application management (list, install, launch, kill, uninstall)
  log         Logs and crash reports
  check       Assertions and verification
  wda         WebDriverAgent management (setup, status, restart)
  daemon      Daemon management (status, stop)

Use "ios-pilot <command> --help" for more information about a command.
`

// Version is set at build time via -ldflags "-X ios-pilot/internal/cli.Version=vX.Y.Z".
var Version = "dev"

// Run is the main entry point for the CLI. Returns an exit code.
func Run() int {
	args := os.Args[1:]

	if len(args) == 0 || args[0] == "-help" || args[0] == "--help" || args[0] == "help" {
		fmt.Print(usage)
		return 0
	}

	if args[0] == "--version" || args[0] == "-version" || args[0] == "version" {
		fmt.Printf("ios-pilot %s\n", Version)
		return 0
	}

	// Internal command: "daemon serve" runs the daemon in-process.
	if args[0] == "daemon" && len(args) > 1 && args[1] == "serve" {
		return runDaemonServe()
	}

	switch args[0] {
	case "device":
		return cmdDevice(args[1:])
	case "look":
		return cmdLook(args[1:])
	case "act":
		return cmdAct(args[1:])
	case "app":
		return cmdApp(args[1:])
	case "log":
		return cmdLog(args[1:])
	case "check":
		return cmdCheck(args[1:])
	case "wda":
		return cmdWDA(args[1:])
	case "daemon":
		return cmdDaemon(args[1:])
	case "tunnel":
		return cmdTunnel(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "ios-pilot: unknown command %q\n\n", args[0])
		fmt.Print(usage)
		return 1
	}
}

// configDir returns the ios-pilot configuration directory.
// Supports IOS_PILOT_CONFIG_DIR env override for test isolation.
func configDir() string {
	if dir := os.Getenv("IOS_PILOT_CONFIG_DIR"); dir != "" {
		return dir
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "ios-pilot")
}

// sockPath returns the path to the daemon Unix socket.
func sockPath() string {
	return filepath.Join(configDir(), "pilot.sock")
}

// pidPath returns the path to the daemon PID file.
func pidPath() string {
	return filepath.Join(configDir(), "pilot.pid")
}

// lockPath returns the path to the daemon lock file.
func lockPath() string {
	return filepath.Join(configDir(), "pilot.lock")
}

// ensureDaemon connects to an existing daemon or starts a new one.
func ensureDaemon() (*client.Client, error) {
	// Try to connect to existing daemon.
	c, err := client.Dial(sockPath())
	if err == nil {
		return c, nil
	}

	// Daemon not running — try to start one.
	if err := startDaemonProcess(); err != nil {
		return nil, fmt.Errorf("start daemon: %w", err)
	}

	// Wait for socket to become available.
	for i := 0; i < 50; i++ {
		time.Sleep(100 * time.Millisecond)
		c, err = client.Dial(sockPath())
		if err == nil {
			return c, nil
		}
	}
	return nil, fmt.Errorf("daemon started but socket not available after 5s")
}

// startDaemonProcess forks the current binary with "daemon serve".
func startDaemonProcess() error {
	// Ensure config dir exists.
	if err := os.MkdirAll(configDir(), 0o755); err != nil {
		return err
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}

	cmd := exec.Command(exe, "daemon", "serve")
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true, // detach from parent session
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("fork daemon: %w", err)
	}

	// Release the child so it continues running after we exit.
	_ = cmd.Process.Release()
	return nil
}

// printJSON marshals v as indented JSON to stdout.
func printJSON(v any) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: marshal output: %v\n", err)
		return
	}
	fmt.Println(string(data))
}

// handleResponse checks for errors in the JSON-RPC response,
// prints the result as JSON, and returns an exit code.
func handleResponse(resp *protocol.Response, err error) int {
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	if resp.Error != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", resp.Error.Message)
		if resp.Error.Data != nil {
			fmt.Fprintf(os.Stderr, "  detail: %v\n", resp.Error.Data)
		}
		return 1
	}
	printJSON(resp.Result)
	return 0
}

// runDaemonServe runs the daemon in-process (foreground). This is the
// internal command invoked by startDaemonProcess.
func runDaemonServe() int {
	// Ensure config dir exists.
	if err := os.MkdirAll(configDir(), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "error: create config dir: %v\n", err)
		return 1
	}

	// Acquire lock to prevent multiple daemons.
	lockFile, err := daemon.AcquireLock(lockPath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: another daemon is already running\n")
		return 1
	}
	defer daemon.ReleaseLock(lockFile)

	// Load configuration.
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: load config: %v (using defaults)\n", err)
		cfg = config.Default()
	}

	// Create drivers.
	deviceDrv := goios.NewDeviceDriver()
	appDrv := goios.NewAppDriver()
	screenshotDrv := goios.NewScreenshotDriver()
	syslogDrv := goios.NewSyslogDriver()
	tunnelDrv := goios.NewTunnelDriver()
	wdaClient := wda.NewWDAClient()
	wdaProcessDrv := goios.NewWDAProcessDriver()

	// Create core managers.
	deviceMgr := core.NewDeviceManager(deviceDrv, tunnelDrv, wdaClient, wdaProcessDrv, &cfg)
	appMgr := core.NewAppManager(appDrv, deviceMgr)
	screenCapture := core.NewScreenCapture(screenshotDrv, wdaClient, deviceMgr, &cfg)
	uiCtrl := core.NewUiController(wdaClient, deviceMgr)
	logMgr := core.NewLogManager(syslogDrv, nil, cfg.LogBufferSize) // no crash driver yet
	checker := core.NewChecker(screenCapture, wdaClient, appDrv, nil, deviceMgr)

	// Create daemon state and server.
	state := &daemon.State{
		DeviceManager: deviceMgr,
		AppManager:    appMgr,
		ScreenCapture: screenCapture,
		UiController:  uiCtrl,
		LogManager:    logMgr,
		Checker:       checker,
		Config:        &cfg,
		StartTime:     time.Now(),
	}

	srv := daemon.NewServer(sockPath())
	state.RegisterHandlers(srv)

	// Set idle timer (auto-shutdown after idle timeout).
	idleDuration := 30 * time.Minute
	if d, err := time.ParseDuration(cfg.IdleTimeout); err == nil {
		idleDuration = d
	}
	idleTimer := daemon.NewIdleTimer(idleDuration, func() {
		srv.Stop()
	})
	srv.SetIdleTimer(idleTimer)

	// Write PID file.
	if err := daemon.WritePIDFile(pidPath()); err != nil {
		fmt.Fprintf(os.Stderr, "error: write PID file: %v\n", err)
		return 1
	}
	defer daemon.RemovePIDFile(pidPath())

	// Start server.
	if err := srv.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "error: start server: %v\n", err)
		return 1
	}

	// Wait for shutdown signal.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	<-sigCh

	// Graceful shutdown.
	idleTimer.Stop()
	logMgr.Stop()
	srv.Stop()

	return 0
}
