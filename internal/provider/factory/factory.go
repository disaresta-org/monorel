// Package factory constructs a [provider.Client] from [config.ProviderConfig].
//
// Adding a new provider:
//  1. implement [provider.Client] in internal/forge/<name>.
//  2. add a case to [New].
//  3. add the provider name to config.KnownProviders.
package factory

import (
	"context"
	"fmt"

	"monorel.disaresta.com/config"
	"monorel.disaresta.com/internal/provider"
	"monorel.disaresta.com/internal/provider/github"
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
	default:
		return nil, fmt.Errorf("forge: unknown provider %q", cfg.Name)
	}
}
