// Package cli wires the monorel CLI commands.
package cli

import (
	"github.com/spf13/cobra"
)

// Version is set at build time via -ldflags. Defaults to "dev" for
// `go run` / `go install` invocations.
var Version = "dev"

// Execute runs the monorel CLI. Returns the first non-nil error from
// command resolution or execution; main() prints and sets the exit code.
func Execute() error {
	root := newRootCmd()
	return root.Execute()
}

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "monorel",
		Short: "A changesets-style release tool for multi-module Go monorepos.",
		Long: `monorel manages per-package versions, tags, and changelogs in a Go monorepo
using explicit ".changeset/*.md" files instead of inferring releases from
commit messages. Pair with the disaresta-org/monorel-action GitHub Action
to run an always-open release PR.

Reference docs: https://monorel.disaresta.com`,
		Version:       Version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	addPersistentFlags(cmd)
	cmd.AddCommand(
		newAddCmd(),
		newStatusCmd(),
		newPlanCmd(),
		newReleaseCmd(),
		newPublishCmd(),
		newPreviewCmd(),
		newPreCmd(),
		newInitCmd(),
	)
	return cmd
}
