package cli

import (
	"errors"

	"github.com/spf13/cobra"
)

func newAddCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add",
		Short: "Create a new changeset describing pending package changes.",
		Long: `Interactively prompts for affected packages, per-package bump level,
and changelog body. Writes a .changeset/<random-name>.md file.

Use --package and --message for non-interactive use.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return errors.New("not implemented yet (Phase 5)")
		},
	}
}
