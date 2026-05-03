# `monorel detect-release` and `monorel auto`: provider-API release detection

**Status**: approved by the user during brainstorming on 2026-05-03.

## Problem

monorel's release pipeline today decides "is this commit the merge of the always-open release PR?" by matching commit-message text. The current guard requires a `chore(release):` subject prefix plus a `monorel-Release:` body trailer.

Two failure modes follow from this approach:

1. **Merge-strategy fragility.** Squash-merge and rebase-merge propagate the source commit body, so the trailer reaches the merge commit and the guard fires. Merge-commit (`--no-ff`) produces a sparse merge commit whose subject is `Merge pull request #N from monorel/release` and whose body is empty by default. The trailer is in the second-parent commit, not in HEAD. The guard never fires; the release silently doesn't ship.

2. **Bitbucket squash drops the trailer.** Bitbucket's squash strategy auto-generates a subject of `Merged in monorel/release (pull request #N)` and rewrites the body to include only the source commit's subject, not its body. The trailer is lost. After the v0.14 work, Bitbucket's pipeline guard substituted `monorel/release` (the source-branch name) for the trailer, but the asymmetry between the four providers' guards is itself a maintenance risk.

The user's stated principle: "we can't assume they'll always use release commit syntax." Text-pattern detection is fundamentally an approximation of "did the merge come from `monorel/release`?" The provider API answers that question directly.

## Solution

Replace text-pattern detection with provider-API detection.

Two new monorel subcommands:

- `monorel detect-release` exits 0 when HEAD is a release-PR merge, 1 when not, 2 on error.
- `monorel auto` is the orchestrator: detects, then dispatches to the tag/publish pipeline (yes branch) or the apply/preview pipeline (no branch). Includes its own `git push` step so CI wrappers stay trivial.

The action wrapper at `ci/github/` collapses to a single `command: auto` entry. The four example workflows / pipeline files (GitHub, Gitea, GitLab, Bitbucket) collapse to one file per provider with one command in the script.

## Architecture

### Detection logic

`internal/detect/IsReleaseMerge(ctx, repo, provider, sha)` evaluates two signals in order:

1. **Trailer**: `strings.Contains(repo.HeadCommitMessage(), "monorel-Release:")`. Fast path; no network call. Hits when squash and rebase have propagated the source body.
2. **API**: `provider.FindPRByMergeCommit(ctx, sha)`. The returned PR's `HeadRef` is checked against the hardcoded `"monorel/release"`. Hits when the trailer was lost (Bitbucket squash, merge-commit on any provider) but the merge metadata is intact.

If either signal returns yes, the result is `IsRelease=true`. The two signals are OR'd, not AND'd, so a deleted PR doesn't break recovery as long as the trailer is still in the commit body.

If both signals say no, the result is `IsRelease=false`.

If the API call returns an error (network, auth), the error propagates to the caller. The CLI surfaces this as exit code 2.

### Source branch is hardcoded

The release branch name is `"monorel/release"`. monorel hardcodes this everywhere already (the orchestrator, the example workflows, the docs). Making it configurable is YAGNI for v1.0; can be added as a `:minor` bump later.

### `monorel auto` flow

```
1. Load config; build provider client; open repo.
2. result, err := detect.IsReleaseMerge(ctx, repo, provider, repo.HeadSHA())
3. if err != nil → exit 2
4. if result.IsRelease:
     a. tag := release.Tag(opts)            // existing
     b. repo.Push("--follow-tags")           // new method on internal/git.Repo
     c. provider.CreateRelease(...) per tag  // existing publish path
5. else:
     a. plan := plan.Plan(...)
     b. if plan.Empty:
          - orchestrator.UpsertPreview(empty plan)  // closes stale PR
     c. else:
          - release.ApplyOnBranch("monorel/release")
          - repo.Push("+monorel/release")           // force-push
          - orchestrator.UpsertPreview(plan)
6. Return summary; exit 0.
```

### Components

```
cmd/monorel/
  detect_release.go       NEW
  auto.go                 NEW

internal/
  detect/
    detect.go             NEW: IsReleaseMerge, Result, Source enum
    detect_test.go        NEW
  orchestrator/
    auto.go               NEW: Auto(ctx, opts) wires detect → dispatch
  git/
    repo.go               EXTEND: Repo.Push(args ...string) method
                                   (promote existing internal use to a documented method)

ci/github/
  action.yml              REWRITE: drop `inputs.command`, drop the case-statement;
                                   download binary + run `monorel auto`
```

The detection logic is internal. The doctor package stays the only diagnostic-shaped public-library surface for v1.0; promoting `detect` to public can happen later if a use case emerges.

## Cross-provider parity

The single `monorel auto` command runs on every provider's runner. Each example collapses to one file:

- **GitHub Actions** (`examples/github/.github/workflows/release.yml`): `on: push: branches: [main]`, one job, one step that calls the action wrapper with `command: auto`.
- **Gitea Actions** (`examples/gitea/.gitea/workflows/release.yml`): same shape.
- **GitLab CI** (`examples/gitlab/.gitlab-ci.yml`): one stage with one rule (`if: $CI_COMMIT_BRANCH == $CI_DEFAULT_BRANCH`), one script step `monorel auto`.
- **Bitbucket Pipelines** (`examples/bitbucket/bitbucket-pipelines.yml`): one step, one bash line `monorel auto`.

The two-workflow / two-stage / two-conditional shape goes away entirely. Each example partial in `docs/src/_partials/` follows the same collapse.

## Failure modes

| Scenario | Detection result | `auto` behavior |
|---|---|---|
| Squash-merge of release PR | trailer ✓ → yes | tag + push + publish |
| Rebase-merge of release PR | trailer ✓ → yes | tag + push + publish |
| Merge-commit of release PR | API ✓ → yes | tag + push + publish |
| Bitbucket squash of release PR | API ✓ → yes | tag + push + publish |
| Direct push of `chore(release):` commit (no PR) | trailer ✓ → yes (recovery path) | tag + push + publish |
| Random feature commit | trailer ✗, API ✗ → no | apply + push + preview --upsert (or preview-only when plan empty) |
| API outage during release | trailer ✓ on squash/rebase/direct → yes; merge-commit fails (no fallback signal) | tag pipeline runs OR exit 2 |
| Provider rejects auth | exit 2 | n/a: auto bails |
| Git push fails | n/a: auto bails after tag/apply | exit 2 |

The "API outage during release" row is the one degradation we accept: merge-commit users hit a hard failure when the API is unreachable. They retry. This is a tractable failure mode (loud, not silent).

## Non-goals

- Configurable source-branch name. Hardcoded `"monorel/release"` for v1.0.
- Public Go library access to `internal/detect`. Stays internal; promote if a use case appears.
- Backward compatibility with the v0.14 action-wrapper `command: pr` / `command: release` inputs. Pre-1.0 means no users to break; the wrapper rewrite drops them.
- Walking to second parent locally (without an API call). The API check is the canonical signal; walking parents would re-introduce a text-pattern dependency on the second parent.
- A separate `monorel detect-feature` or symmetrically-named command. `detect-release` exit code 1 is the "feature commit" signal.

## Testing

### Unit

`internal/detect/detect_test.go` against fakes:

- HEAD body has trailer → yes (Source: TrailerSignal). API not called.
- HEAD body lacks trailer; `FindPRByMergeCommit` returns matching PR → yes (Source: APISignal).
- HEAD body lacks trailer; `FindPRByMergeCommit` returns PR with wrong HeadRef → no.
- HEAD body lacks trailer; `FindPRByMergeCommit` returns nil → no.
- HEAD body lacks trailer; `FindPRByMergeCommit` returns error → propagated err.
- Provider == nil → `ErrProviderRequired`.

`internal/orchestrator/auto_test.go` against the same fakes, asserting dispatch:

- Detection returns yes → tag was called, push was called, publish was called. Preview was NOT called.
- Detection returns no, plan empty → preview called with empty plan (closes stale PR). Tag NOT called.
- Detection returns no, plan non-empty → apply called, push called, preview called. Tag NOT called.
- Detection errors → Auto returns wrapped error before any side effects.

`cmd/monorel/auto_test.go`, `detect_release_test.go`: cobra-level smoke tests verifying flag parsing and that the right runtime is constructed. Deep behavior is in the orchestrator/detect tests.

### Integration

A new `internal/release/auto_integration_test.go` (or wherever existing integration-style tests live) using `testutil.NewRepo(t)` plus the in-memory provider fake to walk the full auto pipeline against a real on-disk git repo:

1. Add a changeset, commit, push.
2. Run `auto` → assert preview-PR-was-upserted.
3. Simulate merging the release PR (write the trailer-bearing commit to main; squash-merge style).
4. Run `auto` → assert tag was created, push happened, release was created.

The build-tag-gated `internal/provider/bitbucket/integration_test.go` gets extended to cover `auto` end-to-end against live Bitbucket Cloud (the same temp-repo flow done manually during v0.14, formalized).

### Cross-provider correctness

Each provider's `FindPRByMergeCommit` already has unit tests. The new contract being relied on is `pr.HeadRef == "monorel/release"`. Each provider's existing test file gets a 3-line addition asserting `HeadRef` is populated correctly from the upstream API response.

### Removed coverage

The text-pattern guards in `release.yml` / `release-pr.yml` / partials / examples disappear in this design. Their YAML expressions had no Go tests, so no test deletions; the YAML changes themselves are the deletion.

## Migration

Pre-1.0 means no users on the v0.14 action-wrapper inputs. Internally:

1. The current `feat/...` branches with text-pattern guard tightening (PR #62, the stashed `fix/release-guard-merge-strategies` work) are obsoleted by this design. They were stopgaps for the same problem.
2. The v1.0 release PR (#63) absorbs this work. v1.0 ships with `monorel auto`, `monorel detect-release`, and the simplified action wrapper. The legacy `command: pr` and `command: release` are removed.
3. Documentation is regenerated for the single-workflow shape on every integration page.

## Effort estimate

~1.5 days:

- ~0.5 day: `internal/detect` package + tests.
- ~0.5 day: `internal/orchestrator/auto.go` + tests + `git.Repo.Push` extension.
- ~0.25 day: `cmd/monorel/{auto,detect_release}.go` + cobra wiring.
- ~0.25 day: action wrapper rewrite + example workflow collapses + docs partial collapses + integration page rewrites.

The provider-side `FindPRByMergeCommit` was implemented in v0.14; nothing new there.

## Open questions

None. Brainstorm closed; ready for writing-plans.
