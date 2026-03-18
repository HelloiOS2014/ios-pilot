package cli

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

const logUsage = `Usage: ios-pilot log [options]
       ios-pilot log crash [id]

Show device logs or crash reports.

Options:
  -n <count>        Number of log entries (default: 50)
  --filter <name>   Filter by process/bundle name
  --level <level>   Filter by log level (e.g. error, info)
  --search <text>   Search in log messages
  --follow          Continuously poll for new logs
  --help            Show this help message

Crash subcommands:
  crash             List crash reports
  crash <id>        Show a specific crash report
`

func cmdLog(args []string) int {
	// Check for help.
	for _, a := range args {
		if a == "--help" || a == "-help" {
			fmt.Print(logUsage)
			return 0
		}
	}

	// Handle "log crash" subcommand.
	if len(args) > 0 && args[0] == "crash" {
		return cmdLogCrash(args[1:])
	}

	c, err := ensureDaemon()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	defer c.Close()

	// Parse log options.
	params := map[string]any{}
	follow := false
	n := 50

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-n":
			if i+1 >= len(args) {
				fmt.Fprintf(os.Stderr, "error: -n requires a value\n")
				return 1
			}
			i++
			val, err := strconv.Atoi(args[i])
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: -n must be an integer\n")
				return 1
			}
			n = val
		case "--filter":
			if i+1 >= len(args) {
				fmt.Fprintf(os.Stderr, "error: --filter requires a value\n")
				return 1
			}
			i++
			params["filter"] = args[i]
		case "--level":
			if i+1 >= len(args) {
				fmt.Fprintf(os.Stderr, "error: --level requires a value\n")
				return 1
			}
			i++
			params["level"] = args[i]
		case "--search":
			if i+1 >= len(args) {
				fmt.Fprintf(os.Stderr, "error: --search requires a value\n")
				return 1
			}
			i++
			params["search"] = args[i]
		case "--follow":
			follow = true
		default:
			fmt.Fprintf(os.Stderr, "ios-pilot log: unknown option %q\n\n", args[i])
			fmt.Print(logUsage)
			return 1
		}
	}

	params["n"] = n

	if !follow {
		return handleResponse(c.Call("log", params))
	}

	// Follow mode: poll every second.
	for {
		params["n"] = 10
		resp, err := c.Call("log", params)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
		if resp.Error != nil {
			fmt.Fprintf(os.Stderr, "error: %s\n", resp.Error.Message)
			return 1
		}
		printJSON(resp.Result)
		time.Sleep(1 * time.Second)
	}
}

func cmdLogCrash(args []string) int {
	c, err := ensureDaemon()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	defer c.Close()

	if len(args) == 0 {
		// List crashes.
		return handleResponse(c.Call("log.crash.list", nil))
	}

	// Get specific crash by ID.
	return handleResponse(c.Call("log.crash.get", map[string]string{"id": args[0]}))
}
