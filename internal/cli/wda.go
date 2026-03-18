package cli

import (
	"fmt"
	"os"
)

const wdaUsage = `Usage: ios-pilot wda <subcommand>

Subcommands:
  setup     Show WDA installation guide
  status    Check WDA status
  restart   Restart WDA session
`

const wdaSetupGuide = `WebDriverAgent (WDA) Setup Guide
================================

1. Clone WDA from Facebook:
   git clone https://github.com/appium/WebDriverAgent.git

2. Open WebDriverAgent.xcodeproj in Xcode

3. Select the WebDriverAgentRunner target and configure signing:
   - Set your Team in Signing & Capabilities
   - Change the Bundle Identifier to something unique

4. Build and run on your device:
   xcodebuild build-for-testing test-without-building \
     -project WebDriverAgent.xcodeproj \
     -scheme WebDriverAgentRunner \
     -destination 'id=<YOUR_DEVICE_UDID>'

5. Verify WDA is running:
   curl http://localhost:8100/status

6. Once WDA is running, ios-pilot will auto-detect it on connect:
   ios-pilot device connect

For more details: https://appium.github.io/appium-xcuitest-driver/latest/preparation/real-device-config/
`

func cmdWDA(args []string) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-help" {
		fmt.Print(wdaUsage)
		return 0
	}

	switch args[0] {
	case "setup":
		fmt.Print(wdaSetupGuide)
		return 0

	case "status":
		c, err := ensureDaemon()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
		defer c.Close()
		return handleResponse(c.Call("wda.status", nil))

	case "restart":
		c, err := ensureDaemon()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
		defer c.Close()
		return handleResponse(c.Call("wda.restart", nil))

	default:
		fmt.Fprintf(os.Stderr, "ios-pilot wda: unknown subcommand %q\n\n", args[0])
		fmt.Print(wdaUsage)
		return 1
	}
}
