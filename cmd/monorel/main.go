// Command monorel is a changesets-style release tool for multi-module Go monorepos.
//
// See https://monorel.disaresta.com for documentation.
package main

import (
	"errors"
	"fmt"
	"os"

	"monorel.disaresta.com/internal/cli"
)

func main() {
	err := cli.Execute()
	code := cli.ExitCode(err)
	// Only print the error text for non-zero exits where the error
	// itself is meaningful. Exit-code-only errors (e.g. validate's
	// --strict 2) shouldn't print a "Error: exit 2" line.
	if err != nil && !errors.As(err, new(silentExitError)) {
		fmt.Fprintln(os.Stderr, err)
	}
	os.Exit(code)
}

// silentExitError matches any error type that opts into silent
// exit-code-only behavior. Currently only cli.errExit (unexported);
// using a duck-typed interface here avoids exporting it.
type silentExitError interface {
	error
	silentExit()
}
