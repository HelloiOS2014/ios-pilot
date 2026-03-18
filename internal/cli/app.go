package cli

import (
	"fmt"
	"os"
)

const appUsage = `Usage: ios-pilot app <subcommand> [args...]

Subcommands:
  list                    List installed applications
  install <path>          Install an IPA or app bundle
  launch <bundle_id>      Launch an application
  kill <bundle_id>        Kill a running application
  uninstall <bundle_id>   Uninstall an application
  foreground              Show the foreground application
`

func cmdApp(args []string) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-help" {
		fmt.Print(appUsage)
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
		return handleResponse(c.Call("app.list", nil))

	case "install":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "usage: ios-pilot app install <path>\n")
			return 1
		}
		return handleResponse(c.Call("app.install", map[string]string{"path": args[1]}))

	case "launch":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "usage: ios-pilot app launch <bundle_id>\n")
			return 1
		}
		return handleResponse(c.Call("app.launch", map[string]string{"bundle_id": args[1]}))

	case "kill":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "usage: ios-pilot app kill <bundle_id>\n")
			return 1
		}
		return handleResponse(c.Call("app.kill", map[string]string{"bundle_id": args[1]}))

	case "uninstall":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "usage: ios-pilot app uninstall <bundle_id>\n")
			return 1
		}
		return handleResponse(c.Call("app.uninstall", map[string]string{"bundle_id": args[1]}))

	case "foreground":
		return handleResponse(c.Call("app.foreground", nil))

	default:
		fmt.Fprintf(os.Stderr, "ios-pilot app: unknown subcommand %q\n\n", args[0])
		fmt.Print(appUsage)
		return 1
	}
}
