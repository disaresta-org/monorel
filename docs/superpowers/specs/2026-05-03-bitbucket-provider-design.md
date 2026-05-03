# Bitbucket Cloud provider + universal PR-body trailers fallback

Closes the "Bitbucket support" item on the v1.0 readiness list, plus a defensive recovery mechanism that benefits every provider.

## Goals

1. Add a fourth provider (`bitbucket`) so monorel covers the canonical Cloud SCMs: GitHub, Gitea / Forgejo, GitLab, Bitbucket.
2. Defend the release pipeline against squash-merge stripping `monorel-Release:` trailers from the commit body. `monorel preview --upsert` writes a redundant trailers block to the PR body as an HTML comment; `monorel tag` falls back to it when the commit body is empty.

## Non-goals

- Bitbucket Data Center / Server. Different REST API; out of scope. Layout reserves room for a future `bitbucket/datacenter/` sibling.
- Bitbucket Cloud's no-native-releases gap. `CreateRelease` is a no-op with a log line; the per-package `CHANGELOG.md` carries release notes.
- Switching the existing providers' SDK choices. github / gitea / gitlab keep their official SDKs.

## Decisions

| # | Decision | Picked |
|---|---|---|
| 1 | Cloud-only with layout-ready directory structure for future Data Center | Approved |
| 2 | `CreateRelease` policy on Bitbucket | No-op + log line; CHANGELOG.md is the canonical source |
| 3 | Merge-strategy guidance | Allow fast-forward and merge-commit; reject squash. Plus universal trailers fallback (squash safety net) |
| 4 | Auth surface | Two env vars: `BITBUCKET_EMAIL` + `BITBUCKET_TOKEN`. Username probed via `GET /2.0/user` and cached |
| 5 | API client | Hand-rolled `net/http`; zero new direct deps |
| 6 | Scope | Approach 2: Bitbucket provider + universal trailers fallback in one PR |

## Architecture

```
internal/provider/bitbucket/
├── bitbucket.go              Public package facade. New(ctx, Options) (*Client, error).
├── client.go                 net/http transport + auth header construction + JSON marshal helpers.
├── pulls.go                  PR ops: FindOpenReleasePR, CreatePR, UpdatePR, ClosePR.
├── repo.go                   GetDefaultBranch.
├── release.go                CreateRelease: no-op returning a synthetic *Release pointing at /src/<tag>.
├── identity.go               /2.0/user probe; caches email + username on the Client.
├── trailers.go               FindPRByMergeCommit (used by the universal fallback).
├── errors.go                 Sentinel errors (ErrPlanGate, ErrRateLimited).
├── bitbucket_test.go         Unit tests against httptest fakes.
├── integration_test.go       //go:build integration; gated end-to-end against a real Bitbucket workspace.
└── doc.go                    Package overview.
```

Cross-package additions (universal trailers fallback):

- `internal/provider/provider.go`: new `Client` method `FindPRByMergeCommit(ctx, sha) (*PullRequest, error)`. Implemented by all four providers.
- `internal/release/render.go` (or equivalent): append the trailers HTML comment block to the rendered PR body in `monorel preview`.
- `internal/release/release.go`'s `Tag`: on `ErrNoReleaseCommit`, fall back to looking up the merged PR via `client.FindPRByMergeCommit(headSHA)`, parse trailers from the PR body's comment block.

Wiring:

- `config/provider.go`: add `ProviderBitbucket = "bitbucket"` constant; add to `KnownProviders`.
- `internal/provider/factory/factory.go`: add `case config.ProviderBitbucket` arm.
- `internal/provider/provider.go`'s `TokenEnvVars(provider)` returns `["BITBUCKET_TOKEN"]`. New parallel `EmailEnvVars(provider)` returns `["BITBUCKET_EMAIL"]` (or empty for providers that don't need a separate email).

## Components and contracts

### `bitbucket.Options`

```go
type Options struct {
    Workspace string  // owner field from monorel.toml
    Repo      string
    Host      string  // unused for now (Cloud only); validated empty
    Email     string  // from BITBUCKET_EMAIL env
    Token     string  // from BITBUCKET_TOKEN env
}
```

Constructor errors when `Email` or `Token` is empty. `Host` non-empty is rejected with a clear error pointing at the future Data Center extension path.

### REST endpoint map

| Interface method | HTTP |
|---|---|
| `GetDefaultBranch` | `GET /2.0/repositories/{ws}/{repo}` -> `mainbranch.name` |
| `FindOpenReleasePR(headBranch)` | `GET /2.0/repositories/{ws}/{repo}/pullrequests?q=state="OPEN" AND source.branch.name="<branch>"` |
| `CreatePR` | `POST /2.0/repositories/{ws}/{repo}/pullrequests` (JSON body) |
| `UpdatePR` | `PUT /2.0/repositories/{ws}/{repo}/pullrequests/{id}` (JSON body) |
| `ClosePR` | `POST /2.0/repositories/{ws}/{repo}/pullrequests/{id}/decline` |
| `CreateRelease` | no-op; returns `&Release{Tag, HTMLURL: ".../src/{tag}"}` plus an INFO log per call |
| `FindPRByMergeCommit(sha)` | `GET /2.0/repositories/{ws}/{repo}/pullrequests?q=state="MERGED" AND merge_commit.hash="<sha>"` |

### State mapping

Bitbucket PR states are `OPEN` / `MERGED` / `DECLINED` / `SUPERSEDED`. Provider-interface `PullRequest.State` is `"open"` / `"closed"`. Map `OPEN` to `"open"`; everything else to `"closed"`.

### Error sentinels (`errors.go`)

```go
var ErrPlanGate    = errors.New("bitbucket: workspace plan not configured (visit bitbucket.org/<workspace>/workspace/settings/plans)")
var ErrRateLimited = errors.New("bitbucket: rate limited (HTTP 429); retry after a short delay")
```

402 surfaces from the orchestrator's `git push` step (not the REST client) but is covered for completeness on REST too.

## Data flow

### Auth on the wire

- REST: `Authorization: Basic <base64(email:token)>` on every call.
- Git over HTTPS: the orchestrator builds clone/push URLs. For Bitbucket, the URL needs `<username>:<token>@bitbucket.org/...`. Provider clients gain a `GitCredentials() (username, token string, err error)` method:
  - github / gitea / gitlab return `(token, "")` (single bearer-equivalent).
  - bitbucket returns the probed username + token.

The orchestrator queries the provider client for git credentials rather than re-implementing per-provider URL construction.

### Username probe

1. Constructor stores email, token, workspace, repo. No network calls.
2. On the first call to any method that needs the username (`GitCredentials`, or any REST call), the client calls `GET /2.0/user`, parses, caches `username`. A `sync.Once` guards against concurrent first-call double-probes.
3. If the probe fails, the original method returns the probe error.
4. Subsequent calls reuse the cached value.

### Universal trailers fallback (Approach 2)

Write side - `monorel preview --upsert` appends to the rendered PR body:

```
<!-- monorel-trailers (do not edit; required for tag recovery if the merge commit body is rewritten)
monorel-Release: transports/foo v1.7.0
monorel-Release: go.example.com/widget v2.0.1
monorel-PreRelease: false
-->
```

HTML comments are invisible in the rendered PR body but persist in the source. All four providers preserve them on PR-body fetches.

Read side - `monorel tag`:

1. Read HEAD's commit message; parse `monorel-Release:` trailers (existing behavior).
2. If trailers found: proceed as today.
3. If no trailers found: compute HEAD's SHA. Call `client.FindPRByMergeCommit(sha)`. Parse trailers from the returned PR body's `monorel-trailers` comment block.
4. If neither HEAD body nor PR body has trailers: return a wrapped `ErrNoReleaseCommit` with an actionable message.

## Error handling

| Source | Status | Sentinel / wrap |
|---|---|---|
| Auth bad | 401 | `bitbucket: auth failed (check BITBUCKET_EMAIL + BITBUCKET_TOKEN); verify the token has Bitbucket scopes` |
| Auth scope insufficient | 403 | `bitbucket: forbidden; the token is missing a scope. Required: read/write repository, read/write pullrequest` |
| Repo not found | 404 | `bitbucket: repo %q not found in workspace %q (or you lack access)` |
| Workspace plan gate | 402 | `ErrPlanGate` (sentinel; message includes settings URL) |
| Validation | 400 | wrap response body's `error.message` |
| Rate limit | 429 | `ErrRateLimited` |
| Server | 5xx | `bitbucket: server error %d: %s` |
| Network | n/a | passthrough from `net/http`'s error |

`FindOpenReleasePR` and `FindPRByMergeCommit` return `(nil, nil)` for "no such PR." `CreateRelease` returns `(*Release, nil)` always (no-op success).

`monorel tag`'s fallback failure message:

> release: HEAD has no monorel-Release: trailers and the merged PR (#42) body also lacks the monorel-trailers comment block. The squash-merge probably rewrote both, OR a contributor edited the PR body. Recover by:
>
> - re-running monorel preview against this PR's source state, or
> - hand-creating tags: `git tag -a <prefix>/v<X.Y.Z> <merge-sha> -m "..."` for each package the PR was supposed to bump.

## Testing

### Unit tests

- `internal/provider/bitbucket/bitbucket_test.go`: each REST method against an `httptest.NewServer` fake. Verifies request URL, query, body JSON, and parsed response.
- Auth probe tests: success path (cache populated), 401 surfacing, concurrent-first-call locking under `t.Parallel`.
- Error-mapping tests per status code (401 / 403 / 402 / 429 / 500).

### Integration tests (build-tag gated)

- `internal/provider/bitbucket/integration_test.go` with `//go:build integration`.
- Runs only when `BITBUCKET_INTEGRATION=1` + `BITBUCKET_EMAIL` + `BITBUCKET_TOKEN` are set, plus `BITBUCKET_WORKSPACE` for the target workspace.
- Walks the API: create repo, push initial branch + a feature branch + a tag, list PRs, create PR, find by head ref, update PR title, decline PR, delete repo.

### Cross-provider tests for the trailers fallback

- `internal/release/tag_test.go` (or equivalent): cases for read-from-PR-body fallback success, both-missing failure path.
- Each existing provider (github, gitea, gitlab) gets a `FindPRByMergeCommit` test against an httptest fake to confirm correct query construction.

## Documentation

- `docs/src/integrations/bitbucket.md` (new): setup walkthrough, auth (two env vars), merge-strategy requirement, plan-gate troubleshooting, no-native-releases note.
- `examples/bitbucket/`: `monorel.toml` + `bitbucket-pipelines.yml` (their CI format).
- `docs/src/_partials/bitbucket-pipelines-yml.md` (new): the pipelines snippet.
- `docs/.vitepress/config.ts`: add Bitbucket to the Integrations sidebar.
- `docs/src/cheat-sheet.md`: extend the env-vars table with `BITBUCKET_EMAIL` + `BITBUCKET_TOKEN`.
- `docs/src/public/llms.txt` and `docs/src/public/llms-full.txt`: add Bitbucket to provider tables.
- `docs/src/introduction.md`'s "At a glance" comparison table: extend if Bitbucket adds a row-distinguishing capability (probably none).
- `README.md`: list Bitbucket alongside the other providers.
- `docs/src/faq.md`: amend the squash-merge / tag-recovery entry to mention the trailers fallback.
- `.changeset/bitbucket-provider.md`: `:minor` bump.

## Risks and mitigation

- **Bitbucket changes the `/2.0/user` endpoint or its scope requirements.** Probe failure surfaces immediately with a clear error pointing at scopes. Doc note in the integration page lists the required scopes verbatim. Hand-rolled client makes the probe call easy to update.
- **HTML comment in PR body gets edited away by a contributor.** `monorel tag` falls back to a clear error message naming the PR and listing recovery steps. The damage is bounded: only when commit body trailers are ALSO missing (i.e. squash-merged AND PR body edited).
- **402 plan-gate confuses users.** The integration page walks through the one-time plan-acceptance step explicitly. The provider's wrapped error includes the URL to visit.
- **Atlassian eventually launches first-class Bitbucket Cloud releases.** When that happens, `CreateRelease`'s no-op gets replaced with a real implementation. Backward-compatible (the existing `Release{Tag, HTMLURL}` shape works for both).
- **The trailers fallback breaks an existing provider's tag flow.** The fallback only fires on `ErrNoReleaseCommit` from the existing path; today the existing path covers fast-forward / merge-commit / no-rebase merges fine. Defensive only.

## Effort

- Bitbucket package: ~1 day (hand-rolled HTTP client, 7 endpoints, error mapping, identity probe).
- Universal trailers fallback: ~0.5 day (4 provider implementations of `FindPRByMergeCommit`, render side, `Tag` fallback path, tests).
- Documentation sweep: ~0.5 day.
- Total: ~2 days focused work.
