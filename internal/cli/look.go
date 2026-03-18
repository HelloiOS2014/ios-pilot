package cli

import (
	"fmt"
	"os"
)

const lookUsage = `Usage: ios-pilot look [options]

Take a screenshot of the connected device.

Options:
  --ui        Include UI element tree in output
  --annotate  Draw numbered bounding boxes on interactive elements
  --help      Show this help message
`

func cmdLook(args []string) int {
	// Check for help first.
	for _, a := range args {
		if a == "--help" || a == "-help" {
			fmt.Print(lookUsage)
			return 0
		}
	}

	c, err := ensureDaemon()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	defer c.Close()

	params := map[string]bool{}
	for _, a := range args {
		switch a {
		case "--ui":
			params["ui"] = true
		case "--annotate":
			params["annotate"] = true
		default:
			fmt.Fprintf(os.Stderr, "ios-pilot look: unknown option %q\n\n", a)
			fmt.Print(lookUsage)
			return 1
		}
	}

	return handleResponse(c.Call("look", params))
}
