package cli

import (
	"errors"

	"github.com/spf13/cobra"
)

func newReleaseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "release",
		Short: "Apply pending changesets: bump versions, write changelogs, tag, push.",
		Long: `Consumes every .changeset/*.md file in the working tree, writes per-package
CHANGELOG.md entries, deletes the consumed files, makes a single commit,
creates per-package git tags, and pushes commits + tags.

Optionally creates GitHub Releases when GITHUB_TOKEN is set or --github
is passed (one Release per tag, body sourced from the package's changelog
entry).

Idempotency: if a planned tag already exists, the command aborts with a
clear error rather than re-tagging.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return errors.New("not implemented yet (Phase 7)")
		},
	}
}
