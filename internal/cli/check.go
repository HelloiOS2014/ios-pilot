package cli

import (
	"fmt"
	"os"
	"strings"
)

const checkUsage = `Usage: ios-pilot check <subcommand> [args...]

Subcommands:
  screen                      Take a screenshot for LLM verification
  element --text "<text>"     Check if a UI element with text exists
  app-running <bundle_id>     Check if an app is in the foreground
  no-crash <bundle_id>        Check that no crashes exist for the app
`

func cmdCheck(args []string) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-help" {
		fmt.Print(checkUsage)
		return 0
	}

	c, err := ensureDaemon()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	defer c.Close()

	switch args[0] {
	case "screen":
		return handleResponse(c.Call("check.screen", nil))

	case "element":
		text := parseCheckElementText(args[1:])
		if text == "" {
			fmt.Fprintf(os.Stderr, "usage: ios-pilot check element --text \"<text>\"\n")
			return 1
		}
		return handleResponse(c.Call("check.element", map[string]string{"text": text}))

	case "app-running":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "usage: ios-pilot check app-running <bundle_id>\n")
			return 1
		}
		return handleResponse(c.Call("check.app_running", map[string]string{"bundle_id": args[1]}))

	case "no-crash":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "usage: ios-pilot check no-crash <bundle_id>\n")
			return 1
		}
		return handleResponse(c.Call("check.no_crash", map[string]string{"bundle_id": args[1]}))

	default:
		fmt.Fprintf(os.Stderr, "ios-pilot check: unknown subcommand %q\n\n", args[0])
		fmt.Print(checkUsage)
		return 1
	}
}

// parseCheckElementText extracts the --text value from args.
func parseCheckElementText(args []string) string {
	for i := 0; i < len(args); i++ {
		if args[i] == "--text" && i+1 < len(args) {
			return strings.Join(args[i+1:], " ")
		}
	}
	// If no --text flag, treat remaining args as the text.
	if len(args) > 0 {
		return strings.Join(args, " ")
	}
	return ""
}
