// Package bitbucket is the [provider.Client] implementation for
// Bitbucket Cloud (bitbucket.org).
//
// Bitbucket Data Center / Server is out of scope; that variant uses
// a different REST API (/rest/api/1.0/...) and a different auth
// model. If support is added later, it should land under
// internal/provider/bitbucket/datacenter/ as a sibling of the
// current Cloud implementation.
//
// # Auth
//
// REST API: HTTP Basic with `email:token`. Both BITBUCKET_EMAIL and
// BITBUCKET_TOKEN are required. The token must be an Atlassian API
// token created with Bitbucket scopes:
// read/write:repository:bitbucket and read/write:pullrequest:bitbucket.
//
// Git over HTTPS uses a different identifier: <bitbucket-username>:<token>
// (NOT email). The provider client probes /2.0/user on first call to
// learn the username and caches it. Callers that need to construct
// an HTTPS git URL ask for the username via the cached state.
//
// # Releases
//
// Bitbucket Cloud has no first-class Release concept. CreateRelease
// is a no-op that returns a synthetic *Release pointing at the tag's
// /src/ URL. Per-package CHANGELOG.md is the canonical release-notes
// source.
//
// # State mapping
//
// Bitbucket PR states are OPEN / MERGED / DECLINED / SUPERSEDED.
// Provider-interface State is "open" / "closed". Map OPEN -> "open";
// everything else -> "closed".
package bitbucket
