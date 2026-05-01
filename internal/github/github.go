// Package github is the GitHub-API seam for the always-open release
// PR pattern and the per-tag GitHub Release creation.
//
// Higher layers (the action orchestrator and the release CLI command)
// take a Client and never touch the go-github API directly. The
// default implementation in this package wraps go-github + oauth2;
// tests use Fake.
package github

import "context"

// Client is the slice of GitHub-API operations monorel needs. The
// methods are intentionally narrow:
//
//   - PR lifecycle for the always-open release-PR pattern.
//   - GitHub Release creation, one per tag, body sourced from the
//     package's CHANGELOG entry.
//   - Read-only metadata (default branch).
type Client interface {
	// GetDefaultBranch returns the repo's default branch (e.g.
	// "main"). Used by the action orchestrator to compute the base
	// branch for the release PR.
	GetDefaultBranch(ctx context.Context) (string, error)

	// FindOpenReleasePR returns the open PR whose head ref is the
	// given branch, if one exists. (nil, nil) means "no such open
	// PR" (NOT an error). The action upserts: create when missing,
	// update when present.
	FindOpenReleasePR(ctx context.Context, headBranch string) (*PullRequest, error)

	// CreatePR opens a new PR and returns it.
	CreatePR(ctx context.Context, opts CreatePROptions) (*PullRequest, error)

	// UpdatePR edits the title and/or body of an existing PR. nil
	// fields in opts mean "leave unchanged."
	UpdatePR(ctx context.Context, number int, opts UpdatePROptions) (*PullRequest, error)

	// ClosePR closes a PR without merging. Used when the planner
	// produces an empty plan and the action wants to retire the
	// previously-open release PR.
	ClosePR(ctx context.Context, number int) error

	// CreateRelease creates a GitHub Release pointed at an
	// already-existing git tag. monorel pushes tags first, then
	// calls this once per tag with the matching CHANGELOG entry as
	// the body.
	CreateRelease(ctx context.Context, opts CreateReleaseOptions) (*Release, error)
}

// PullRequest is the minimal PR shape monorel cares about.
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
	// Tag is the existing git tag this Release points at, e.g.
	// "transports/foo/v1.6.2". Must be already pushed to the
	// remote before CreateRelease is called.
	Tag string

	// Name is the human-readable title shown on the Release page.
	// Conventionally the same as Tag.
	Name string

	// Body is the markdown release notes (typically the package's
	// CHANGELOG entry for this version).
	Body string

	// Prerelease toggles GitHub's "Pre-release" flag. monorel sets
	// it to true for tags carrying a SemVer pre-release suffix
	// (-rc.N, -beta.N, etc.).
	Prerelease bool
}

// Release is the minimal Release shape monorel cares about.
type Release struct {
	ID      int64
	Tag     string
	HTMLURL string
}
