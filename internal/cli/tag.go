package cli

import (
	"github.com/spf13/cobra"

	"monorel.disaresta.com/internal/release"
)

func newTagCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tag",
		Short: "Create per-package tags at HEAD from the release commit's trailers.",
		Long: `Reads HEAD's commit message looking for monorel-Release: trailers
written by "monorel apply", and creates one annotated git tag per
trailer using the package's configured tag_prefix.

This is the post-merge step of the always-open release-PR pattern:

    # in release-pr.yml (on every push to main):
    monorel apply               # writes files + commits, NO tags
    git push -f monorel/release # speculative state on the head branch
    monorel preview --upsert    # opens / updates the release PR

    # in release.yml (on the release PR's merge commit):
    monorel tag                 # reads trailers, creates tags
    git push --follow-tags      # publishes the tags
    monorel publish             # creates a Release per tag

Errors out if HEAD's commit has no monorel-Release: trailers
(meaning it's not a release commit), if a trailer names a package
that no longer exists in monorel.toml, or if any derived tag
already exists in the repository.

Push is the caller's responsibility; this command does not push.`,
		RunE: runTag,
	}
}

func runTag(cmd *cobra.Command, _ []string) error {
	rt, err := loadRuntime(cmd)
	if err != nil {
		return err
	}

	res, err := release.Tag(release.TagOptions{
		Config: rt.Config,
		Repo:   rt.Repo,
	})
	if err != nil {
		return err
	}

	// Headline summary: keep the wording stable (the GitHub Action
	// wrapper greps for these lines).
	rt.Log.Info("Tagged %d release(s) at %s:", len(res.Releases), short(res.CommitSHA))
	for _, r := range res.Releases {
		rt.Log.Info("  %s", r.Tag)
	}
	rt.Log.Info("Run `git push --follow-tags && monorel publish` to publish.")
	return nil
}
