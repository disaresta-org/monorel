package cli

import (
	"errors"

	"github.com/spf13/cobra"
)

func newPreviewCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "preview",
		Short: "Render the proposed releases as markdown for an always-open release PR body.",
		Long: `Equivalent to "plan" but rendered as the markdown body for an always-open
release PR. Used by the GitHub Action wrapping monorel.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return errors.New("not implemented yet (Phase 9)")
		},
	}
}
