package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"monorel.disaresta.com/config"
	"monorel.disaresta.com/internal/provider"
	"monorel.disaresta.com/internal/provider/factory"
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

	// Build a provider client when the auth token is in the
	// environment. The provider is only used for the universal PR-body
	// trailers fallback (when HEAD's commit message has no trailers,
	// e.g. squash-merge rewrote it). Tag works without a provider in
	// the common case; we only need it for recovery.
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	var providerClient provider.Client
	providerName := config.ResolveProvider(rt.Config.Provider.Name)
	if token := provider.TokenFromEnv(providerName); token != "" {
		providerClient, err = factory.New(ctx, rt.Config.Provider, token)
		if err != nil {
			return fmt.Errorf("provider client: %w", err)
		}
	}

	res, err := release.Tag(release.TagOptions{
		Config:   rt.Config,
		Repo:     rt.Repo,
		Provider: providerClient,
	})
	if err != nil {
		// When the trailers are missing AND no provider client was
		// built (no token in env), the PR-body fallback couldn't even
		// be tried. Hint the operator at the env var that would
		// enable it; advisory only, the exit code stays the same.
		if errors.Is(err, release.ErrNoReleaseCommit) && providerClient == nil {
			if envVars := provider.TokenEnvVars(providerName); len(envVars) > 0 {
				rt.Log.Warn("Tip: set %s to enable the PR-body trailers fallback (covers squash-merged release PRs).",
					strings.Join(envVars, " or "))
			}
		}
		return err
	}

	// Headline summary: write directly to stdout (the GitHub Action
	// wrapper greps for these lines, and `-q` suppresses Info, so
	// routing through the logger would silently break the contract).
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Tagged %d release(s) at %s:\n", len(res.Releases), short(res.CommitSHA))
	for _, r := range res.Releases {
		fmt.Fprintf(out, "  %s\n", r.Tag)
	}
	rt.Log.Info("Run `git push --follow-tags && monorel publish` to publish.")
	return nil
}
