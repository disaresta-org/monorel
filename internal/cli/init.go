package cli

import (
	"errors"

	"github.com/spf13/cobra"
)

func newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Scaffold a monorel.toml + .changeset/ directory in the current repo.",
		Long: `Creates an initial monorel.toml by walking go.work / go.mod files in the
current repo, plus an empty .changeset/ directory. Skip if monorel.toml
already exists.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return errors.New("not implemented yet (Phase 11+)")
		},
	}
}
