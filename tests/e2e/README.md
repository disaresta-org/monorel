# monorel e2e suite

End-to-end tests for the full `monorel` lifecycle against a live Forgejo / Gitea instance. The suite uses [testcontainers-go's Forgejo module](https://golang.testcontainers.org/modules/forgejo/) to spin up a fresh container per test run; no `docker run` is required by the operator.

Forgejo is API-compatible with Gitea (identical `/api/v1/...` shape), so these tests validate monorel against both. The image is pinned to a specific Forgejo minor version in `main_test.go`'s `forgejoImage` constant.

## Run locally

```sh
# from the repo root
go test -tags=e2e -count=1 -v ./tests/e2e/...
```

The build tag `e2e` keeps the suite out of the default `go test ./...` run. CI is wired up via `.github/workflows/e2e-gitea.yml`.

A working Docker daemon is required. On Linux, the default `unix:///var/run/docker.sock` is fine; on macOS, Docker Desktop or Colima both work.

### Run against an existing Gitea / Forgejo instance

If you'd rather iterate against a long-running instance instead of a fresh container per run, set `GITEA_TOKEN` (and optionally `GITEA_HOST` and `GITEA_USERNAME`):

```sh
GITEA_HOST=http://localhost:3000 \
GITEA_TOKEN=$YOUR_TOKEN \
GITEA_USERNAME=monorel \
go test -tags=e2e -count=1 -v -run TestBasic ./tests/e2e/
```

testcontainers is skipped; the suite uses your existing instance and per-test repos.

## Coverage

| File | Tests |
|------|-------|
| `basic_test.go` | Multi-module mixed-bump release; empty-plan noop; in-flight release-PR refresh |
| `signals_test.go` | Squash / rebase / merge-commit detection matrix; direct-push (no PR) |
| `versioning_test.go` | Initial-release matrix (`:major` / `:minor` / `:patch`); new package mid-life; sub-module-only release; max-bump precedence; overlapping-changeset bump merge |
| `errors_test.go` | Tag conflict; trailers fallback failure (both signals absent); `monorel validate` config errors; auto idempotency on already-tagged HEAD; `validate --strict`; `validate --check-tags` non-semver warning |
| `lifecycle_test.go` | Pre-release rc cycle (rc.0 → rc.1 → stable); manually-closed release PR; doctor revival detection; noop-after-release; pre-release counter increments; stale `monorel/release` overwrite; concurrent contributors; pre-mode error paths; doctor on clean state |
| `content_test.go` | Markdown (backticks, fenced blocks, links) survives in PR body and Release body |

Each scenario runs against a fresh Gitea repo (cleaned up at test end unless `MONOREL_E2E_KEEP=1`). The per-test repo create + clone overhead dominates runtime; the suite finishes in well under two minutes on a modern laptop.

## Helpers

`helpers_test.go` exposes a `*ScenarioRepo` handle with the verbs each scenario needs:

```go
r := newScenarioRepo(t, "my-scenario")
r.ScaffoldMultiModule(t)             // 3-module loglayer-go-style scaffold
r.WriteChangeset(t, "feat", map[string]string{"pkg-a": "minor"}, "Body.")
r.CommitAll(t, "feat: x")
r.PushMain(t)
r.MonorelOK(t, "auto")               // run monorel binary, assert exit 0
r.MergePR(t, prNum, "squash")        // poll mergeable, then merge via API
r.CheckoutMain(t)                    // reset local tree to origin/main
tags := r.LocalTags(t)
rels := r.Releases(t)                // Gitea API objects
```

`MergePR` polls Gitea's `mergeable` field — it's computed asynchronously after PR creation. Without polling, fresh PRs return HTTP 405 on merge attempts. The poll caps at 30s.

## Cleanup

By default each test deletes its Gitea repo at end via `t.Cleanup`. Set `MONOREL_E2E_KEEP=1` to leave repos in place for inspection (useful for debugging a single failing scenario). The container goes away regardless of this flag — testcontainers tracks containers per test process and reaps them with `ryuk`.

## Adding a new scenario

1. Pick the test file matching the scenario family, or create a new one (build tag `//go:build e2e`).
2. Use `newScenarioRepo(t, "<descriptive-suffix>")` to get a clean handle.
3. Drive monorel via `r.Monorel*` and Gitea via `r.MergePR` / `r.PRs` / `r.Releases` etc.
4. Assert on tags, releases, PR titles/bodies, and `monorel auto` stdout (the headline lines are stable).

The helpers expose just enough to keep scenarios short. If a new scenario needs a verb that doesn't exist yet, add it to `helpers_test.go` — but err on the side of small, focused helpers over a god-object.
