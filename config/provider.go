package config

// Provider names recognized by [IsKnownProvider]. These are the
// values that `provider.name` in monorel.toml can take. Adding a
// new provider: append to KnownProviders and wire up a provider
// implementation under internal/provider/<name>/.
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
	ProviderBitbucket,
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
