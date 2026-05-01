package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"monorel.disaresta.com/internal/forge"
	"monorel.disaresta.com/internal/forge/factory"
	"monorel.disaresta.com/internal/release"
)

func newPublishCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "publish",
		Short: "Create one forge release per tag pointing at HEAD.",
		Long: `Reads tags pointing at the current HEAD, matches each to a configured
package, and creates a forge release using the matching CHANGELOG
entry as the release notes. Pre-release tags (those carrying a SemVer
pre-release suffix) are flagged accordingly.

This is the post-push step of a monorel release pipeline:

    monorel release       # write CHANGELOGs, delete changesets, commit, tag
    git push --follow-tags
    monorel publish       # create one forge release per tag

Splitting publish from release ensures tags are on the remote before
the forge tries to validate them when creating the Release. Requires
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
		fmt.Fprintln(cmd.OutOrStdout(), "No tags at HEAD. Nothing to publish.")
		return nil
	}

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	provider := forge.ResolveProvider(rt.Config.Forge.Provider)
	token := forge.TokenFromEnv(provider)
	if token == "" {
		envVars := strings.Join(forge.TokenEnvVars(provider), " or ")
		return fmt.Errorf("publish: provider %q requires %s in the environment", provider, envVars)
	}
	client, err := factory.New(ctx, rt.Config.Forge, token)
	if err != nil {
		return fmt.Errorf("forge client: %w", err)
	}

	res := &release.Result{Releases: infos}
	published, err := release.PublishReleases(ctx, client, res)
	out := cmd.OutOrStdout()
	if err != nil {
		fmt.Fprintf(out, "Created %d/%d releases before failing.\n", len(published), len(infos))
		return err
	}
	fmt.Fprintf(out, "Published %d release(s):\n", len(published))
	for _, r := range published {
		fmt.Fprintf(out, "  %s\n", r.Tag)
	}
	return nil
}
