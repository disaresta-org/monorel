// Package factory constructs a [forge.Client] from [config.ForgeConfig].
//
// Adding a new provider:
//  1. implement [forge.Client] in internal/forge/<name>.
//  2. add a case to [New].
//  3. add the provider name to forge.KnownProviders.
package factory

import (
	"context"
	"fmt"

	"monorel.disaresta.com/internal/config"
	"monorel.disaresta.com/internal/forge"
	"monorel.disaresta.com/internal/forge/github"
)

// New constructs a [forge.Client] for the configured provider. Pass an
// empty token to build an unauthenticated client (read-only on public
// repos; useful in tests).
func New(ctx context.Context, cfg config.ForgeConfig, token string) (forge.Client, error) {
	switch forge.ResolveProvider(cfg.Provider) {
	case forge.ProviderGitHub:
		return github.New(ctx, github.Options{
			Owner: cfg.Owner,
			Repo:  cfg.Repo,
			Host:  cfg.Host,
			Token: token,
		})
	default:
		return nil, fmt.Errorf("forge: unknown provider %q", cfg.Provider)
	}
}
