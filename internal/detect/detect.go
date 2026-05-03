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
	"errors"

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

// Skeleton-commit shims so staticcheck doesn't flag the unexported
// constants as unused before [IsReleaseMerge] (the only consumer)
// lands in the next commit. Removed there.
var (
	_ = releaseHeadBranch
	_ = trailerMarker
)
