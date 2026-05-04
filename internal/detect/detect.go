// Package detect answers a single question: is HEAD the merge commit
// of monorel's always-open release PR? It checks two signals and
// returns the first match.
//
//  1. Trailer signal. HEAD's commit body contains a `monorel-Release:`
//     trailer. Hits when squash-merge or rebase-merge propagated the
//     source body to HEAD.
//  2. API signal. The provider's [provider.Client.FindPRByMergeCommit]
//     returns a PR whose source branch is `monorel/release`. Hits
//     when the trailer was lost (Bitbucket squash, merge-commit on
//     any provider) but the merge metadata is intact.
//
// If either signal returns yes, the result is IsRelease=true. If both
// return no, the result is IsRelease=false. If the API call errors
// (network, auth, rate limit), the error propagates.
//
// The trailer signal does NOT require a network call; it is checked
// first as a fast path. Callers with an unauthenticated repo (no
// provider token) still get correct results when squash or rebase
// preserved the trailer.
package detect

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"monorel.disaresta.com/internal/git"
	"monorel.disaresta.com/internal/provider"
)

// releaseHeadBranch is the source branch monorel always uses for the
// always-open release PR. Hardcoded to match
// [orchestrator.DefaultHeadBranch] without taking a dependency on the
// orchestrator package (which would create an import cycle once
// orchestrator imports detect for the Auto flow).
const releaseHeadBranch = "monorel/release"

// trailerMarker is the literal substring detect looks for in HEAD's
// commit body to confirm a release commit. monorel apply writes one
// `monorel-Release:` line per released package; checking for the
// prefix is sufficient because the marker is monorel-distinctive.
const trailerMarker = "monorel-Release:"

// Source describes which signal told detect "yes, this is a release
// commit." Populated for diagnostic logging in the CLI.
type Source string

const (
	// SourceTrailer means the trailer was found in HEAD's commit body.
	SourceTrailer Source = "trailer"

	// SourceAPI means the provider's FindPRByMergeCommit returned a
	// PR with the expected source branch.
	SourceAPI Source = "api"

	// SourceNone means no signal matched. Result.IsRelease is false.
	SourceNone Source = ""
)

// Result reports the outcome of [IsReleaseMerge].
type Result struct {
	// IsRelease is true when at least one signal matched.
	IsRelease bool

	// Source is which signal matched. Populated when IsRelease is true;
	// SourceNone otherwise.
	Source Source

	// PR is the merged release PR. Non-nil only when Source ==
	// SourceAPI (the trailer signal doesn't fetch the PR).
	PR *provider.PullRequest
}

// ErrProviderRequired is returned when [IsReleaseMerge] is called with
// a nil provider. The provider is required even when the trailer
// signal would have sufficed: the trailer fast path is an
// optimization, not a contract guarantee.
var ErrProviderRequired = errors.New("detect: Provider is required")

// ErrRepoRequired is returned when [IsReleaseMerge] is called with a
// nil repo. The repo is required for both signals: the trailer fast
// path reads HEAD's commit message, and the API path needs HEAD's SHA.
var ErrRepoRequired = errors.New("detect: Repo is required")

// IsReleaseMerge reports whether HEAD is the merge commit of monorel's
// always-open release PR. See package doc for the signal contract.
//
// Required arguments:
//   - ctx is forwarded to the provider's FindPRByMergeCommit call.
//   - repo reads HEAD's commit message; pass nil and the function
//     returns an error.
//   - prov is the configured provider client; nil returns
//     [ErrProviderRequired].
//   - sha is HEAD's commit SHA. Empty is allowed but only the trailer
//     signal can match (the API call needs a SHA).
//
// On success, callers should branch on Result.IsRelease.
//
// On error, the caller can choose between propagating (the CLI exits 2
// in that case) and falling back to a different policy. Errors are
// always provider-side; the trailer check itself only fails if reading
// HEAD's commit message fails.
func IsReleaseMerge(ctx context.Context, repo git.Repo, prov provider.Client, sha string) (*Result, error) {
	if repo == nil {
		return nil, ErrRepoRequired
	}
	if prov == nil {
		return nil, ErrProviderRequired
	}

	// Trailer signal (no network).
	msg, err := repo.HeadCommitMessage()
	if err != nil {
		return nil, fmt.Errorf("detect: read HEAD commit message: %w", err)
	}
	if strings.Contains(msg, trailerMarker) {
		return &Result{IsRelease: true, Source: SourceTrailer}, nil
	}

	// API signal. Empty SHA is a programmer error in practice, but
	// the provider implementations all return (nil, nil) for unknown
	// SHAs, so we forward without an extra guard.
	pr, err := prov.FindPRByMergeCommit(ctx, sha)
	if err != nil {
		return nil, fmt.Errorf("detect: find PR for SHA %q: %w", sha, err)
	}
	if pr != nil && pr.HeadRef == releaseHeadBranch {
		return &Result{IsRelease: true, Source: SourceAPI, PR: pr}, nil
	}

	return &Result{IsRelease: false, Source: SourceNone}, nil
}
