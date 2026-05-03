package bitbucket

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"
)

// Options configures a new Bitbucket Cloud-backed [provider.Client].
type Options struct {
	// Workspace is the bitbucket workspace slug (the part of the URL
	// like bitbucket.org/<workspace>/<repo>). Mirrors the `owner`
	// field from monorel.toml.
	Workspace string

	// Repo is the repository slug.
	Repo string

	// Host must be empty (Cloud-only). Non-empty is rejected with an
	// error pointing at the future Data Center extension path.
	Host string

	// Email is the Atlassian-account email. Required for REST auth
	// (HTTP Basic with email + token).
	Email string

	// Token is an Atlassian API token with Bitbucket scopes
	// (read/write:repository:bitbucket and
	// read/write:pullrequest:bitbucket).
	Token string
}

// ErrMissingWorkspaceRepo is returned when Options doesn't carry both
// a workspace and a repo.
var ErrMissingWorkspaceRepo = errors.New("bitbucket: Workspace and Repo are required")

// ErrMissingEmail is returned when Options.Email is empty.
var ErrMissingEmail = errors.New("bitbucket: Email is required (REST auth uses HTTP Basic with email + token)")

// ErrMissingToken is returned when Options.Token is empty.
var ErrMissingToken = errors.New("bitbucket: Token is required")

// ErrHostNotSupported is returned when Options.Host is non-empty.
// Bitbucket Data Center / Server is not implemented; only Cloud is
// supported.
var ErrHostNotSupported = errors.New("bitbucket: Host must be empty (Cloud-only); Data Center support is not implemented")

// New returns a Bitbucket Cloud REST API v2 client. Does NOT make a
// network call: the first request fires when one of the Client
// methods is invoked, including the lazy /2.0/user probe that
// resolves the auth username for git credentials.
//
// The return type is the unexported *client during Phase 2 of the
// build-out. It will be widened to [provider.Client] once the full
// interface (GetDefaultBranch, FindOpenReleasePR, CreatePR,
// UpdatePR, ClosePR, CreateRelease, FindPRByMergeCommit) is
// implemented in Phase 3.
func New(_ context.Context, opts Options) (*client, error) {
	if opts.Workspace == "" || opts.Repo == "" {
		return nil, ErrMissingWorkspaceRepo
	}
	if opts.Email == "" {
		return nil, ErrMissingEmail
	}
	if opts.Token == "" {
		return nil, ErrMissingToken
	}
	if opts.Host != "" {
		return nil, ErrHostNotSupported
	}
	return &client{
		workspace: opts.Workspace,
		repo:      opts.Repo,
		email:     opts.Email,
		token:     opts.Token,
		baseURL:   "https://api.bitbucket.org/2.0",
		http: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

// client is the unexported provider.Client implementation. Public
// surface is the Options + New constructor only.
type client struct {
	workspace string
	repo      string
	email     string
	token     string

	baseURL string

	http *http.Client

	// Identity-probe state. Lazily populated on first call needing
	// the username. Wired in Phase 3 (identity.go); declared here so
	// Phase 2 ships the full client shape.
	identityOnce sync.Once //lint:ignore U1000 wired by identity.go in Phase 3
	username     string    //lint:ignore U1000 wired by identity.go in Phase 3
	identityErr  error     //lint:ignore U1000 wired by identity.go in Phase 3
}
