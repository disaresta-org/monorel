package config

// Provider names recognized by [IsKnownProvider]. These are the
// values that `provider.name` in monorel.toml can take. Adding a
// new provider: append to KnownProviders and wire up a provider
// implementation under internal/provider/<name>/.
//
// ProviderBitbucket names a constant retained for the in-tree
// `internal/provider/bitbucket` implementation, which is intentionally
// NOT in [KnownProviders]: end-to-end verification against a live
// Bitbucket Pipelines runner has not been completed (the workspace
// 2FA enforcement on the maintainer's account blocks the API enable
// path). The constant and the provider package are kept on disk so a
// future re-enablement is a one-line change in [KnownProviders] and
// the factory dispatch; until then the validator rejects
// `name = "bitbucket"` with "is not recognized."
const (
	ProviderGitHub    = "github"
	ProviderGitea     = "gitea"
	ProviderGitLab    = "gitlab"
	ProviderBitbucket = "bitbucket"
)

// DefaultProvider is the value [ResolveProvider] returns for an
// empty input. monorel.toml's `provider.name` defaults to "github"
// when omitted.
const DefaultProvider = ProviderGitHub

// KnownProviders is every provider name recognized in this build,
// in alphabetical order. The slice is shared and read-only; callers
// should not mutate it.
var KnownProviders = []string{
	ProviderGitea,
	ProviderGitHub,
	ProviderGitLab,
}

// ResolveProvider returns name, or [DefaultProvider] if name is empty.
// Callers reading config should pass through ResolveProvider so the
// empty-string default is applied uniformly.
func ResolveProvider(name string) string {
	if name == "" {
		return DefaultProvider
	}
	return name
}

// IsKnownProvider reports whether name is a recognized provider.
// Empty name is rejected; resolve to the default first if you mean
// "accept empty as github."
func IsKnownProvider(name string) bool {
	for _, p := range KnownProviders {
		if p == name {
			return true
		}
	}
	return false
}
