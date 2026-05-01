package cli

import (
	"errors"

	"github.com/spf13/cobra"
)

func newPlanCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "plan",
		Short: "Compute the proposed releases without applying them (dry-run).",
		Long: `Reads .changeset/*.md, the configured packages, and the latest matching
git tag per package. Prints the proposed bump for each affected package
plus the changelog excerpts that will be written.

Pass --json to emit a machine-readable plan.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return errors.New("not implemented yet (Phase 4)")
		},
	}
}
