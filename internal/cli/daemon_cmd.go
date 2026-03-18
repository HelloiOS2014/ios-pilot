package cli

import (
	"fmt"
	"os"
	"syscall"

	"ios-pilot/internal/daemon"
)

const daemonUsage = `Usage: ios-pilot daemon <subcommand>

Subcommands:
  status    Show daemon status (PID, uptime, device info)
  stop      Stop the running daemon
`

func cmdDaemon(args []string) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-help" {
		fmt.Print(daemonUsage)
		return 0
	}

	switch args[0] {
	case "status":
		return cmdDaemonStatus()

	case "stop":
		return cmdDaemonStop()

	default:
		fmt.Fprintf(os.Stderr, "ios-pilot daemon: unknown subcommand %q\n\n", args[0])
		fmt.Print(daemonUsage)
		return 1
	}
}

func cmdDaemonStatus() int {
	// First check if daemon is running via PID file.
	if !daemon.IsRunning(pidPath()) {
		printJSON(map[string]string{"status": "stopped"})
		return 0
	}

	pid, _ := daemon.ReadPID(pidPath())

	// Try to connect and get full status.
	c, err := ensureDaemon()
	if err != nil {
		// PID file exists but can't connect.
		printJSON(map[string]any{
			"status": "running",
			"pid":    pid,
			"note":   "daemon running but not responding",
		})
		return 0
	}
	defer c.Close()

	resp, err := c.Call("daemon.status", nil)
	if err != nil {
		printJSON(map[string]any{
			"status": "running",
			"pid":    pid,
			"note":   fmt.Sprintf("connected but status call failed: %v", err),
		})
		return 0
	}
	if resp.Error != nil {
		printJSON(map[string]any{
			"status": "running",
			"pid":    pid,
			"note":   resp.Error.Message,
		})
		return 0
	}

	// Enrich the daemon status with PID.
	if m, ok := resp.Result.(map[string]any); ok {
		m["pid"] = pid
		printJSON(m)
	} else {
		printJSON(resp.Result)
	}
	return 0
}

func cmdDaemonStop() int {
	pid, err := daemon.ReadPID(pidPath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "daemon is not running (no PID file)\n")
		return 0
	}

	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		fmt.Fprintf(os.Stderr, "error: send SIGTERM to PID %d: %v\n", pid, err)
		// Clean up stale PID file.
		daemon.RemovePIDFile(pidPath())
		return 1
	}

	fmt.Fprintf(os.Stderr, "sent SIGTERM to daemon (PID %d)\n", pid)
	return 0
}
