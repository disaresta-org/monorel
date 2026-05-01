// Package factory wires concrete forge implementations to the generic
// [forge.Client] interface based on the configured provider.
//
// Lives in its own subpackage so that internal/forge stays free of
// implementation dependencies (no cycle: forge does not depend on
// its providers; the factory imports both forge and a provider
// subpackage, but neither imports the factory).
//
// Adding a new provider:
//  1. implement [forge.Client] in internal/forge/<name>.
//  2. add a case to [New].
//  3. add the provider name to the validator in internal/config.
package factory

import (
	"context"
	"fmt"

	"github.com/disaresta-org/monorel/internal/config"
	"github.com/disaresta-org/monorel/internal/forge"
	"github.com/disaresta-org/monorel/internal/forge/github"
)

// New constructs a [forge.Client] for the configured provider. token
// is the access token used for authenticated calls; pass "" to build
// an unauthenticated client (read-only access; useful in tests).
//
// An empty cfg.Provider defaults to "github".
func New(ctx context.Context, cfg config.ForgeConfig, token string) (forge.Client, error) {
	provider := cfg.Provider
	if provider == "" {
		provider = "github"
	}
	switch provider {
	case "github":
		return github.New(ctx, github.Options{
			Owner: cfg.Owner,
			Repo:  cfg.Repo,
			Host:  cfg.Host,
			Token: token,
		})
	default:
		return nil, fmt.Errorf("forge: unknown provider %q", provider)
	}
}
