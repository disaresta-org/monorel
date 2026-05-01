package cli

import (
	"errors"

	"github.com/spf13/cobra"
)

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Print the pending changesets and which packages they affect.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return errors.New("not implemented yet (Phase 4)")
		},
	}
}
