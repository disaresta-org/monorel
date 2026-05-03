package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"monorel.disaresta.com/changelog"
	"monorel.disaresta.com/config"
	"monorel.disaresta.com/internal/orchestrator"
	"monorel.disaresta.com/internal/provider"
	"monorel.disaresta.com/internal/provider/factory"
)

func newAutoCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auto",
		Short: "Auto-dispatched post-push pipeline (detect, then release or preview).",
		Long: `One-stop CI command. Runs in this order:

  1. Detect: is HEAD the merge of monorel's release PR? Uses the
     trailer-or-API logic of monorel detect-release.

  2. If yes (release branch):
       - monorel tag         (creates per-package tags from trailers)
       - git push --tags     (pushes tags to origin)
       - monorel publish     (creates provider Releases per tag)

  3. If no (feature branch):
       - plan := plan.Plan(...)
       - if plan empty: monorel preview --upsert (closes any open
         release PR if there is no longer anything to release).
       - else: fetch origin/<base>, checkout monorel/release from
         it, monorel apply, force-push monorel/release, monorel
         preview --upsert (open or update the release PR).

The single entry point makes the per-provider CI workflow trivial:
configure git author, then run "monorel auto". No "if commit
matches X then run command Y" branching in YAML or bash; that
logic lives inside monorel.`,
		RunE: runAuto,
	}
	cmd.Flags().String("base-branch", "",
		"Merge target branch. Empty queries the provider for the repo's default branch.")
	cmd.Flags().String("remote", "origin",
		"Git remote name for fetch and push.")
	return cmd
}

func runAuto(cmd *cobra.Command, _ []string) error {
	rt, err := loadRuntime(cmd)
	if err != nil {
		return err
	}

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	providerName := config.ResolveProvider(rt.Config.Provider.Name)
	token := provider.TokenFromEnv(providerName)
	if token == "" {
		envVars := strings.Join(provider.TokenEnvVars(providerName), " or ")
		return fmt.Errorf("auto: provider %q requires %s in the environment", providerName, envVars)
	}
	client, err := factory.New(ctx, rt.Config.Provider, token)
	if err != nil {
		return fmt.Errorf("provider client: %w", err)
	}

	baseBranch, _ := cmd.Flags().GetString("base-branch")
	remote, _ := cmd.Flags().GetString("remote")

	res, err := orchestrator.Auto(ctx, orchestrator.AutoOptions{
		Config:       rt.Config,
		Repo:         rt.Repo,
		Provider:     client,
		RepoDir:      rt.RepoDir,
		ChangesetDir: rt.ChangesetDir,
		Changesets:   rt.Changesets,
		Tags:         rt.Tags,
		PreState:     rt.PreState,
		BaseBranch:   baseBranch,
		Remote:       remote,
		Today:        changelog.Today(),
	})
	if err != nil {
		return err
	}

	out := cmd.OutOrStdout()
	switch res.Branch {
	case orchestrator.AutoBranchRelease:
		fmt.Fprintf(out, "Released %d package(s) at %s (detection source: %s):\n",
			len(res.Tags), short(res.CommitSHA), res.DetectionSource)
		for _, tag := range res.Tags {
			fmt.Fprintf(out, "  %s\n", tag)
		}
	case orchestrator.AutoBranchFeature:
		switch res.Action {
		case orchestrator.ActionNoop:
			rt.Log.Info("Plan is empty and no release PR is open. Nothing to do.")
		case orchestrator.ActionClosed:
			rt.Log.Info("Plan is empty; closed release PR #%d.", res.PR.Number)
		case orchestrator.ActionCreated:
			rt.Log.Info("Created release PR #%d: %s", res.PR.Number, res.PR.HTMLURL)
		case orchestrator.ActionUpdated:
			rt.Log.Info("Updated release PR #%d: %s", res.PR.Number, res.PR.HTMLURL)
		}
	}
	return nil
}
