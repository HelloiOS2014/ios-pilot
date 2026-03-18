package cli

import (
	"fmt"
	"os"
)

const deviceUsage = `Usage: ios-pilot device <subcommand>

Subcommands:
  list        List connected iOS devices
  connect     Connect to a device (auto-selects if only one)
  status      Show current device connection status
  disconnect  Disconnect from the active device
`

func cmdDevice(args []string) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-help" {
		fmt.Print(deviceUsage)
		return 0
	}

	c, err := ensureDaemon()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	defer c.Close()

	switch args[0] {
	case "list":
		return handleResponse(c.Call("device.list", nil))

	case "connect":
		var udid string
		if len(args) > 1 {
			udid = args[1]
		}
		return handleResponse(c.Call("device.connect", map[string]string{"udid": udid}))

	case "status":
		return handleResponse(c.Call("device.status", nil))

	case "disconnect":
		return handleResponse(c.Call("device.disconnect", nil))

	default:
		fmt.Fprintf(os.Stderr, "ios-pilot device: unknown subcommand %q\n\n", args[0])
		fmt.Print(deviceUsage)
		return 1
	}
}
