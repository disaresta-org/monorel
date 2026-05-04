// Package factory constructs a [provider.Client] from [config.ProviderConfig].
//
// Adding a new provider:
//  1. implement [provider.Client] in internal/provider/<name>.
//  2. add a case to [New].
//  3. add the provider name to config.KnownProviders.
//
// internal/provider/bitbucket is in-tree but not currently dispatched
// from [New]: the maintainer hasn't been able to verify the workflow
// end-to-end against a live Bitbucket Pipelines runner. Re-enabling
// is a two-line change (uncomment the case below and add
// ProviderBitbucket to config.KnownProviders).
package factory

import (
	"context"
	"fmt"

	"monorel.disaresta.com/config"
	"monorel.disaresta.com/internal/provider"
	"monorel.disaresta.com/internal/provider/gitea"
	"monorel.disaresta.com/internal/provider/github"
	"monorel.disaresta.com/internal/provider/gitlab"
)

// New constructs a [provider.Client] for the configured provider. Pass an
// empty token to build an unauthenticated client (read-only on public
// repos; useful in tests).
func New(ctx context.Context, cfg config.ProviderConfig, token string) (provider.Client, error) {
	switch config.ResolveProvider(cfg.Name) {
	case config.ProviderGitHub:
		return github.New(ctx, github.Options{
			Owner: cfg.Owner,
			Repo:  cfg.Repo,
			Host:  cfg.Host,
			Token: token,
		})
	case config.ProviderGitea:
		return gitea.New(ctx, gitea.Options{
			Owner: cfg.Owner,
			Repo:  cfg.Repo,
			Host:  cfg.Host,
			Token: token,
		})
	case config.ProviderGitLab:
		return gitlab.New(ctx, gitlab.Options{
			Owner: cfg.Owner,
			Repo:  cfg.Repo,
			Host:  cfg.Host,
			Token: token,
		})
	// case config.ProviderBitbucket: see package doc; not dispatched
	// until end-to-end Pipelines verification is complete.
	default:
		return nil, fmt.Errorf("provider: unknown provider %q", cfg.Name)
	}
}
