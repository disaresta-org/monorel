package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"monorel.disaresta.com/internal/release"
	"monorel.disaresta.com/plan"
)

func newReleaseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "release",
		Short: "Apply pending changesets: bump versions, write changelogs, tag, commit.",
		Long: `Computes the release plan, writes per-package CHANGELOG.md entries,
deletes the consumed .changeset/*.md files, makes a single commit,
and creates per-package git tags at HEAD.

In pre-release mode (when .changeset/pre.json exists), the
CHANGELOGs are NOT written and the changeset files are NOT deleted;
instead, the per-package counters in pre.json are incremented and a
suffixed tag (e.g. transports/foo/v1.6.0-rc.0) is created. The
accumulated changes are emitted to CHANGELOGs at the next stable
release after pre exit.

Idempotency: if a planned tag already exists, the command aborts
with a clear error rather than re-tagging. Push is the caller's
responsibility: this command does not push commits or tags. Use
the CI wrapper or the local pipeline:

    monorel release
    git push --follow-tags
    monorel publish    # creates one forge release per tag`,
		RunE: runRelease,
	}
}

func runRelease(cmd *cobra.Command, _ []string) error {
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
		fmt.Fprintln(cmd.OutOrStdout(), "No pending changesets. Nothing to release.")
		return nil
	}

	res, err := release.Apply(release.Options{
		Plan:         p,
		Repo:         rt.Repo,
		RepoDir:      rt.RepoDir,
		ChangesetDir: rt.ChangesetDir,
		PreState:     rt.PreState,
	})
	if err != nil {
		return err
	}

	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Released %d package(s) at %s:\n", len(res.Releases), short(res.CommitSHA))
	for _, r := range res.Releases {
		fmt.Fprintf(out, "  %s\n", r.Tag)
	}
	fmt.Fprintln(out, "Run `git push --follow-tags && monorel publish` to publish.")
	return nil
}

// short returns the first 7 characters of a SHA, or the whole string
// if it's shorter. Cosmetic only.
func short(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}
