// Command monorel is a changesets-style release tool for multi-module Go monorepos.
//
// See https://monorel.disaresta.com for documentation.
package main

import (
	"fmt"
	"os"

	"monorel.disaresta.com/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
