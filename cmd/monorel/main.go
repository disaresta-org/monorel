// Command monorel is a changesets-style release tool for multi-module Go monorepos.
//
// See https://github.com/disaresta-org/monorel for documentation.
package main

import (
	"fmt"
	"os"

	"github.com/disaresta-org/monorel/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
