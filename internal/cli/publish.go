package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"monorel.disaresta.com/config"
	"monorel.disaresta.com/internal/provider"
	"monorel.disaresta.com/internal/provider/factory"
	"monorel.disaresta.com/internal/release"
)

func newPublishCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "publish",
		Short: "Create one provider release per tag pointing at HEAD.",
		Long: `Reads tags pointing at the current HEAD, matches each to a configured
package, and creates a provider release using the matching CHANGELOG
entry as the release notes. Pre-release tags (those carrying a SemVer
pre-release suffix) are flagged accordingly.

This is the post-push step of a monorel release pipeline:

    monorel release       # write CHANGELOGs, delete changesets, commit, tag
    git push --follow-tags
    monorel publish       # create one provider release per tag

Splitting publish from release ensures tags are on the remote before
the provider tries to validate them when creating the Release. Requires
the configured provider's auth token in the environment.`,
		RunE: runPublish,
	}
}

func runPublish(cmd *cobra.Command, _ []string) error {
	rt, err := loadRuntime(cmd)
	if err != nil {
		return err
	}
	infos, err := release.DiscoverPublishables(rt.Config, rt.Repo, rt.RepoDir)
	if err != nil {
		return err
	}
	if len(infos) == 0 {
		rt.Log.Info("No tags at HEAD. Nothing to publish.")
		return nil
	}

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	providerName := config.ResolveProvider(rt.Config.Provider.Name)
	token := provider.TokenFromEnv(providerName)
	if token == "" {
		envVars := strings.Join(provider.TokenEnvVars(providerName), " or ")
		return fmt.Errorf("publish: provider %q requires %s in the environment", providerName, envVars)
	}
	client, err := factory.New(ctx, rt.Config.Provider, token)
	if err != nil {
		return fmt.Errorf("provider client: %w", err)
	}

	res := &release.Result{Releases: infos}
	published, err := release.PublishReleases(ctx, client, res)
	if err != nil {
		// Partial-progress headline goes to stderr (warn) so a CI
		// log scraper can detect the failure cleanly while still
		// seeing the count.
		rt.Log.Warn("Created %d/%d releases before failing.", len(published), len(infos))
		return err
	}
	// Headline summary: write directly to stdout (the GitHub Action
	// wrapper greps for these lines, and `-q` suppresses Info, so
	// routing through the logger would silently break the contract).
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Published %d release(s):\n", len(published))
	for _, r := range published {
		fmt.Fprintf(out, "  %s\n", r.Tag)
	}
	return nil
}
