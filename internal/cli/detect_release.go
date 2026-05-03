package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"monorel.disaresta.com/config"
	"monorel.disaresta.com/internal/detect"
	"monorel.disaresta.com/internal/provider"
	"monorel.disaresta.com/internal/provider/factory"
)

func newDetectReleaseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "detect-release",
		Short: "Report whether HEAD is the merge of monorel's release PR.",
		Long: `Inspects HEAD using two signals:

  1. Trailer: HEAD's commit body contains a "monorel-Release:" trailer.
     Hits when squash-merge or rebase-merge propagated the source body.
  2. API: provider.FindPRByMergeCommit returns a PR whose source
     branch is "monorel/release". Hits when the trailer was lost
     (Bitbucket squash, merge-commit on any provider).

Either signal alone is sufficient. The trailer is checked first
(no network); the API call only fires when the trailer is missing.

Exit codes:

  0  HEAD is a release-PR merge. Caller should run monorel tag /
     publish.
  1  HEAD is NOT a release-PR merge. Caller should run monorel apply
     / preview --upsert.
  2  Detection failed (network, auth, or repo-state error). Caller
     should retry or surface the error.

Used internally by monorel auto. Useful as a standalone gate for
custom CI scripts.`,
		RunE: runDetectRelease,
	}
}

func runDetectRelease(cmd *cobra.Command, _ []string) error {
	rt, err := loadRuntime(cmd)
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "detect-release: %v\n", err)
		return ErrExit(2)
	}

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	providerName := config.ResolveProvider(rt.Config.Provider.Name)
	token := provider.TokenFromEnv(providerName)
	if token == "" {
		envVars := strings.Join(provider.TokenEnvVars(providerName), " or ")
		fmt.Fprintf(cmd.ErrOrStderr(), "detect-release: provider %q requires %s in the environment\n", providerName, envVars)
		return ErrExit(2)
	}
	client, err := factory.New(ctx, rt.Config.Provider, token)
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "detect-release: provider client: %v\n", err)
		return ErrExit(2)
	}

	headSHA, err := rt.Repo.CurrentSHA()
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "detect-release: read HEAD SHA: %v\n", err)
		return ErrExit(2)
	}

	res, err := detect.IsReleaseMerge(ctx, rt.Repo, client, headSHA)
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "detect-release: %v\n", err)
		return ErrExit(2)
	}

	if res.IsRelease {
		fmt.Fprintf(cmd.OutOrStdout(), "release commit detected (source: %s)\n", res.Source)
		return nil
	}
	// Exit code 1 with a hint on stderr; main() suppresses the
	// trailing "Error: ..." print because ErrExit is silent.
	fmt.Fprintln(cmd.ErrOrStderr(), "HEAD is not a release-PR merge")
	return ErrExit(1)
}
