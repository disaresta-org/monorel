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
	err := cli.Execute()
	// Only print the error text for non-zero exits where the error
	// itself is meaningful. Exit-code-only errors (validate's --strict
	// path returns one) shouldn't print "Error: exit 2".
	if err != nil && !cli.IsSilentExit(err) {
		fmt.Fprintln(os.Stderr, err)
	}
	os.Exit(cli.ExitCode(err))
}
