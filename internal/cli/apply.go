package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"monorel.disaresta.com/internal/release"
	"monorel.disaresta.com/plan"
)

func newApplyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "apply",
		Short: "Stage a release: write CHANGELOGs, delete consumed changesets, commit. No tags.",
		Long: `Computes the release plan and applies it: writes per-package CHANGELOG
entries, deletes the consumed .changeset/*.md files, makes a single
commit. The commit body carries machine-readable trailers
(monorel-Release: ...) so a later "monorel tag" can derive the
per-package tags without needing the plan in memory.

apply does NOT create tags; that's "monorel tag"'s job. The split
exists for the always-open release-PR pattern: the CI wrapper runs
"monorel apply" on a staging branch (so the always-open release PR's
diff IS the file changes the release will produce) and force-pushes.
After the release PR is merged, "monorel tag" runs on the merge
commit to create the tags.

In pre-release mode (when .changeset/pre.json exists), CHANGELOGs are
NOT written and changesets are NOT deleted; instead, per-package
counters in pre.json are incremented. Tag creation still goes through
"monorel tag".

Push is the caller's responsibility; this command does not push.`,
		RunE: runApply,
	}
}

func runApply(cmd *cobra.Command, _ []string) error {
	rt, err := loadRuntime(cmd)
	if err != nil {
		return err
	}

	clean, err := rt.Repo.IsClean()
	if err != nil {
		return fmt.Errorf("check working tree: %w", err)
	}
	if !clean {
		return fmt.Errorf("working tree has uncommitted changes; commit or stash first")
	}

	p, err := plan.Plan(rt.Config, rt.Changesets, rt.Tags, rt.PreState)
	if err != nil {
		return err
	}
	if p.IsEmpty() {
		fmt.Fprintln(cmd.OutOrStdout(), "No pending changesets. Nothing to apply.")
		return nil
	}

	res, err := release.Apply(release.Options{
		Plan:         p,
		Config:       rt.Config,
		Repo:         rt.Repo,
		RepoDir:      rt.RepoDir,
		ChangesetDir: rt.ChangesetDir,
		PreState:     rt.PreState,
	})
	if err != nil {
		return err
	}

	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Applied %d release(s) at %s:\n", len(res.Releases), short(res.CommitSHA))
	for _, r := range res.Releases {
		// Apply doesn't create tags; the planned tag name is what
		// `monorel tag` will produce for this release.
		fmt.Fprintf(out, "  staged: %s\n", r.Tag)
	}
	fmt.Fprintln(out, "Run `monorel tag` (typically post-merge) to create tags from this commit's trailers.")
	return nil
}
