package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	gh "github.com/disaresta-org/monorel/internal/github"
	"github.com/disaresta-org/monorel/internal/plan"
	"github.com/disaresta-org/monorel/internal/release"
)

func newReleaseCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "release",
		Short: "Apply pending changesets: bump versions, write changelogs, tag, commit.",
		Long: `Computes the release plan, writes per-package CHANGELOG.md entries,
deletes the consumed .changeset/*.md files, makes a single commit, and
creates per-package git tags at HEAD.

In pre-release mode (when .changeset/pre.json exists), the CHANGELOGs
are NOT written and the changeset files are NOT deleted; instead, the
per-package counters in pre.json are incremented and a suffixed tag
(e.g. transports/foo/v1.6.0-rc.0) is created. The accumulated changes
are emitted to CHANGELOGs at the next stable release after pre exit.

Idempotency: if a planned tag already exists, the command aborts with
a clear error rather than re-tagging. Push is the caller's
responsibility — this command does not push commits or tags. Use the
GitHub Action wrapper or run "git push --follow-tags" yourself.`,
		RunE: runRelease,
	}
	cmd.Flags().Bool("github", false,
		"Also create one GitHub Release per tag. "+
			"Requires GITHUB_TOKEN (or GH_TOKEN) and that tags have been pushed.")
	return cmd
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
	fmt.Fprintf(out, "Released %d package(s) at %s:\n", len(res.Tags), short(res.CommitSHA))
	for _, tag := range res.Tags {
		fmt.Fprintf(out, "  %s\n", tag)
	}

	if doGitHub, _ := cmd.Flags().GetBool("github"); doGitHub {
		token := firstNonEmpty(os.Getenv("GITHUB_TOKEN"), os.Getenv("GH_TOKEN"))
		if token == "" {
			return fmt.Errorf("--github requires GITHUB_TOKEN or GH_TOKEN in the environment")
		}
		client, err := gh.New(cmd.Context(), gh.Options{
			Owner: rt.Config.GitHub.Owner,
			Repo:  rt.Config.GitHub.Repo,
			Token: token,
		})
		if err != nil {
			return fmt.Errorf("github client: %w", err)
		}
		ctx := cmd.Context()
		if ctx == nil {
			ctx = context.Background()
		}
		published, err := gh.PublishReleases(ctx, client, res, p)
		if err != nil {
			fmt.Fprintf(out, "Created %d/%d GitHub Releases before failing.\n", len(published), len(res.Tags))
			return err
		}
		fmt.Fprintf(out, "Published %d GitHub Release(s).\n", len(published))
		return nil
	}

	fmt.Fprintln(out, "Run `git push --follow-tags` to publish.")
	return nil
}

// firstNonEmpty returns the first non-empty argument, or "".
func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return ""
}

// short returns the first 7 characters of a SHA, or the whole string
// if it's shorter. Cosmetic only.
func short(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}
