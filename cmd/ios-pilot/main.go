// Command ios-pilot is the CLI entry point for the ios-pilot daemon.
package main

import (
	"os"

	"ios-pilot/internal/cli"
)

func main() {
	os.Exit(cli.Run())
}
