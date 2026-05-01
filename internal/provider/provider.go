// Package provider is the version-control-host seam for monorel.
//
// [Client] models the slice of host operations monorel needs:
//   - upsert an open change request (PR / merge request)
//   - create a release pointing at a tag with markdown notes
//   - read repository metadata (default branch)
//
// Implementations live in subpackages under internal/provider.
//
// The provider name catalog (ProviderGitHub, KnownProviders,
// IsKnownProvider, ResolveProvider) lives in the public config
// package — those values are part of monorel.toml's schema and
// external consumers may want to reference them.
package provider

import (
	"context"
	"os"

	"monorel.disaresta.com/config"
)

// TokenEnvVars returns the environment variable names to consult for
// a given provider's auth token, in priority order. Unknown providers
// (or providers that don't use a static env-var token) yield nil.
func TokenEnvVars(provider string) []string {
	switch config.ResolveProvider(provider) {
	case config.ProviderGitHub:
		return []string{"GITHUB_TOKEN", "GH_TOKEN"}
	case config.ProviderGitea:
		return []string{"GITEA_TOKEN"}
	}
	return nil
}

// TokenFromEnv returns the first non-empty env var value listed by
// [TokenEnvVars] for the given provider, or "" if none is set.
func TokenFromEnv(provider string) string {
	for _, name := range TokenEnvVars(provider) {
		if v := os.Getenv(name); v != "" {
			return v
		}
	}
	return ""
}

// Client is the slice of provider operations monorel needs. The methods
// are intentionally narrow: a provider implementation only has to model
// PR lifecycle for the always-open release-PR pattern, plus per-tag
// release creation, plus default-branch lookup.
type Client interface {
	// GetDefaultBranch returns the repo's default branch (e.g.
	// "main"). Used by the orchestrator to compute the base branch
	// for the release PR.
	GetDefaultBranch(ctx context.Context) (string, error)

	// FindOpenReleasePR returns the open PR/MR whose head ref is
	// the given branch, if one exists. (nil, nil) means "no such
	// open PR" (NOT an error). The orchestrator upserts: create
	// when missing, update when present.
	FindOpenReleasePR(ctx context.Context, headBranch string) (*PullRequest, error)

	// CreatePR opens a new PR/MR and returns it.
	CreatePR(ctx context.Context, opts CreatePROptions) (*PullRequest, error)

	// UpdatePR edits the title and/or body of an existing PR/MR.
	// nil fields in opts mean "leave unchanged."
	UpdatePR(ctx context.Context, number int, opts UpdatePROptions) (*PullRequest, error)

	// ClosePR closes a PR/MR without merging. Used when the planner
	// produces an empty plan and the orchestrator wants to retire
	// the previously-open release PR.
	ClosePR(ctx context.Context, number int) error

	// CreateRelease creates a release pointed at an already-existing
	// git tag. monorel pushes tags first, then calls this once per
	// tag with the matching CHANGELOG entry as the body.
	//
	// Providers without a first-class "release" concept (e.g. plain
	// Bitbucket) may return an "unsupported" error; callers should
	// treat that as advisory, not fatal.
	CreateRelease(ctx context.Context, opts CreateReleaseOptions) (*Release, error)
}

// PullRequest is the minimal shape monorel cares about. The name uses
// the GitHub convention because it's the most widely-recognized term
// across the ecosystem; on GitLab this represents a merge request.
type PullRequest struct {
	Number  int
	State   string // "open" or "closed"
	Title   string
	Body    string
	HeadRef string
	HTMLURL string
}

// CreatePROptions is the input to [Client.CreatePR].
type CreatePROptions struct {
	Title      string
	Body       string
	HeadBranch string
	BaseBranch string
}

// UpdatePROptions is the input to [Client.UpdatePR]. Pointer-valued
// fields so callers can express "leave unchanged" (nil) vs "set to
// empty string" (a non-nil empty string).
type UpdatePROptions struct {
	Title *string
	Body  *string
}

// CreateReleaseOptions is the input to [Client.CreateRelease].
type CreateReleaseOptions struct {
	// Tag is the existing git tag this release points at, e.g.
	// "transports/foo/v1.6.2". Must be already pushed to the
	// remote before CreateRelease is called.
	Tag string

	// Name is the human-readable title shown on the release page.
	// Conventionally the same as Tag.
	Name string

	// Body is the markdown release notes (typically the package's
	// CHANGELOG entry for this version).
	Body string

	// Prerelease toggles the provider's "Pre-release" flag. monorel
	// sets it to true for tags carrying a SemVer pre-release suffix
	// (-rc.N, -beta.N, etc.).
	Prerelease bool
}

// Release is the minimal Release shape monorel cares about.
type Release struct {
	ID      int64
	Tag     string
	HTMLURL string
}
