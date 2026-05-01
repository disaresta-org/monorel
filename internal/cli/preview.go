package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/disaresta-org/monorel/internal/changelog"
	"github.com/disaresta-org/monorel/internal/forge"
	"github.com/disaresta-org/monorel/internal/forge/factory"
	"github.com/disaresta-org/monorel/internal/orchestrator"
	"github.com/disaresta-org/monorel/internal/plan"
	"github.com/disaresta-org/monorel/internal/release"
)

func newPreviewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "preview",
		Short: "Render the proposed releases as markdown for an always-open release PR.",
		Long: `Equivalent to "plan" but rendered as the markdown body of the
always-open release PR. Default writes the body to stdout; pipe it
into a comment, attach to a custom workflow, or pass --upsert to
have monorel call the configured forge directly.

The CI wrapper (under ci/<provider>/) typically calls --upsert after
force-pushing the speculative-version commits to the release branch.`,
		RunE: runPreview,
	}
	cmd.Flags().Bool("upsert", false,
		"After rendering, create or update the release PR via the configured forge. "+
			"Closes any open release PR when the plan is empty. Requires the provider's "+
			"auth token in the environment.")
	cmd.Flags().String("head-branch", orchestrator.DefaultHeadBranch,
		"Source branch of the release PR. The CI wrapper pushes the speculative-version "+
			"commits here before invoking --upsert.")
	cmd.Flags().String("base-branch", "",
		"Merge target branch. Empty queries the forge for the repo's default branch.")
	return cmd
}

func runPreview(cmd *cobra.Command, _ []string) error {
	rt, err := loadRuntime(cmd)
	if err != nil {
		return err
	}
	p, err := plan.Plan(rt.Config, rt.Changesets, rt.Tags, rt.PreState)
	if err != nil {
		return err
	}

	body := release.RenderPreview(p, changelog.Today())

	upsert, _ := cmd.Flags().GetBool("upsert")
	if !upsert {
		_, err := fmt.Fprint(cmd.OutOrStdout(), body)
		return err
	}

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	provider := forge.ResolveProvider(rt.Config.Forge.Provider)
	token := forge.TokenFromEnv(provider)
	if token == "" {
		envVars := forge.TokenEnvVars(provider)
		return fmt.Errorf("--upsert: provider %q requires %v in the environment", provider, envVars)
	}
	client, err := factory.New(ctx, rt.Config.Forge, token)
	if err != nil {
		return fmt.Errorf("forge client: %w", err)
	}
	headBranch, _ := cmd.Flags().GetString("head-branch")
	baseBranch, _ := cmd.Flags().GetString("base-branch")

	res, err := orchestrator.Run(ctx, orchestrator.Options{
		Plan:       p,
		Forge:      client,
		HeadBranch: headBranch,
		BaseBranch: baseBranch,
		Today:      changelog.Today(),
	})
	if err != nil {
		return err
	}

	out := cmd.OutOrStdout()
	switch res.Action {
	case "noop":
		fmt.Fprintln(out, "Plan is empty and no release PR is open. Nothing to do.")
	case "closed":
		fmt.Fprintf(out, "Plan is empty; closed release PR #%d.\n", res.PR.Number)
	case "created":
		fmt.Fprintf(out, "Created release PR #%d: %s\n", res.PR.Number, res.PR.HTMLURL)
	case "updated":
		fmt.Fprintf(out, "Updated release PR #%d: %s\n", res.PR.Number, res.PR.HTMLURL)
	}
	return nil
}
