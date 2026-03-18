package cli

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

const actUsage = `Usage: ios-pilot act <action> [args...]

Actions:
  tap <x> <y>                      Tap at coordinates
  swipe <x1> <y1> <x2> <y2>       Swipe between coordinates
  input "<text>"                    Type text into focused element
  press <key>                       Press a button (home, volumeUp, volumeDown)
`

func cmdAct(args []string) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-help" {
		fmt.Print(actUsage)
		return 0
	}

	c, err := ensureDaemon()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	defer c.Close()

	switch args[0] {
	case "tap":
		if len(args) < 3 {
			fmt.Fprintf(os.Stderr, "usage: ios-pilot act tap <x> <y>\n")
			return 1
		}
		x, err1 := strconv.Atoi(args[1])
		y, err2 := strconv.Atoi(args[2])
		if err1 != nil || err2 != nil {
			fmt.Fprintf(os.Stderr, "error: x and y must be integers\n")
			return 1
		}
		return handleResponse(c.Call("act", map[string]any{
			"action": "tap",
			"x":      x,
			"y":      y,
		}))

	case "swipe":
		if len(args) < 5 {
			fmt.Fprintf(os.Stderr, "usage: ios-pilot act swipe <x1> <y1> <x2> <y2>\n")
			return 1
		}
		x1, e1 := strconv.Atoi(args[1])
		y1, e2 := strconv.Atoi(args[2])
		x2, e3 := strconv.Atoi(args[3])
		y2, e4 := strconv.Atoi(args[4])
		if e1 != nil || e2 != nil || e3 != nil || e4 != nil {
			fmt.Fprintf(os.Stderr, "error: all coordinates must be integers\n")
			return 1
		}
		return handleResponse(c.Call("act", map[string]any{
			"action": "swipe",
			"x1":     x1,
			"y1":     y1,
			"x2":     x2,
			"y2":     y2,
		}))

	case "input":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "usage: ios-pilot act input \"<text>\"\n")
			return 1
		}
		text := strings.Join(args[1:], " ")
		return handleResponse(c.Call("act", map[string]any{
			"action": "input",
			"text":   text,
		}))

	case "press":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "usage: ios-pilot act press <key>\n")
			return 1
		}
		return handleResponse(c.Call("act", map[string]any{
			"action": "press",
			"key":    args[1],
		}))

	default:
		fmt.Fprintf(os.Stderr, "ios-pilot act: unknown action %q\n\n", args[0])
		fmt.Print(actUsage)
		return 1
	}
}
