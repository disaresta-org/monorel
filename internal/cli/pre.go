package cli

import (
	"errors"

	"github.com/spf13/cobra"
)

func newPreCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pre",
		Short: "Manage pre-release mode (rc / beta / alpha).",
		Long: `Pre-release mode causes monorel release to append a "-<channel>.N" suffix
to next versions and increment N per release. While in pre-release mode,
no stable tag is produced; "monorel pre exit" returns to stable releases.

Subcommands:
  monorel pre enter <channel>   start pre-release mode (writes .changeset/pre.json)
  monorel pre exit              return to stable releases (clears state)
  monorel pre status            print current pre-release state, if any`,
	}

	cmd.AddCommand(
		&cobra.Command{
			Use:   "enter <channel>",
			Short: "Start pre-release mode with the given channel name (e.g. rc, beta, alpha).",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				return errors.New("not implemented yet (Phase 3.5)")
			},
		},
		&cobra.Command{
			Use:   "exit",
			Short: "Exit pre-release mode. Next release will be a stable version.",
			RunE: func(cmd *cobra.Command, args []string) error {
				return errors.New("not implemented yet (Phase 3.5)")
			},
		},
		&cobra.Command{
			Use:   "status",
			Short: "Print the current pre-release mode state, if any.",
			RunE: func(cmd *cobra.Command, args []string) error {
				return errors.New("not implemented yet (Phase 3.5)")
			},
		},
	)
	return cmd
}
