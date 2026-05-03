---
"monorel.disaresta.com": minor
---

monorel now supports Bitbucket Cloud (`provider.name = "bitbucket"`) alongside GitHub, Gitea / Forgejo, and GitLab. The `internal/provider/bitbucket/` package implements the `provider.Client` interface against Bitbucket's REST API v2 (hand-rolled `net/http`; no new direct deps).

Auth uses two environment variables: `BITBUCKET_EMAIL` (Atlassian account email) and `BITBUCKET_TOKEN` (Atlassian API token with Bitbucket scopes). The Bitbucket username for git over HTTPS is probed from `/2.0/user` and cached on the client.

Bitbucket Cloud has no first-class Release concept, so `monorel publish` is a no-op on Bitbucket; per-package `CHANGELOG.md` is the canonical release-notes source.

Plus a defensive recovery mechanism that benefits every provider: `monorel preview` now appends a `<!-- monorel-trailers ... -->` HTML comment to the PR body. `monorel tag` falls back to that block when the merge commit body lacks `monorel-Release:` trailers (e.g. because of a squash-merge that rewrote the body). The fallback uses the new `provider.Client.FindPRByMergeCommit` method, implemented by every provider.

See [Bitbucket integration](/integrations/bitbucket).
