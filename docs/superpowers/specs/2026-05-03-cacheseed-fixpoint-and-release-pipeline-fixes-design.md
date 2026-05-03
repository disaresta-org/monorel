# cacheseed fixpoint + release-pipeline fix package

**Date:** 2026-05-03
**Status:** Approved (ready for implementation plan)
**Scope:** Four coordinated changes to address [issue #54](https://github.com/disaresta-org/monorel/issues/54). Single PR, three commits.

## Motivation

Issue #54 surfaced four distinct problems in monorel's release pipeline, all of which bit `loglayer/loglayer-go`'s v2.1.0 release in sequence:

1. **`cacheseed.go` writes the wrong `h1:` hash to released sub-modules' `go.sum` files.** The seed zips the working tree before all working-tree mutations have been applied; the chore(release) commit then includes mutations the seed didn't see, so the proxy's hash for the published version differs from the seeded hash. Every fresh-cache install of a monorel-driven release fails with `SECURITY ERROR: checksum mismatch`. **Critical.**

2. **Offline tidy assumes a primed module cache** but only seeds in-plan siblings and pre-flight-checks managed-but-not-in-plan siblings. Third-party deps (anything not in `monorel.toml`) must already be in `GOMODCACHE`. On fresh CI runners, they aren't, and `GOPROXY=off` blocks fetching. Tidy fails with "module lookup disabled by GOPROXY=off."

3. **`GOTOOLCHAIN=local`** during offline tidy blocks Go's auto-toolchain-download. Consumers whose sub-modules require a Go version newer than the runner's pre-installed Go hit a `go.mod requires go >= X.Y` error. The action's README doesn't document the prerequisite.

4. **CI race on `chore(release):` commits.** The release commit updates module `go.mod` files to require new versions; the corresponding tags are pushed by `release.yml` on the same push. Any other workflow that fires on the same push and resolves Go modules races the tag push and surfaces a phantom CI failure. No upstream documentation explains the workflow filter consumers need.

Concern #1 is the only true code bug in monorel's released code (it produces invalid releases). #2 is also a code bug but with a narrower failure surface (fresh CI runners). #3 and #4 are documentation gaps that surface as flaky CI for adopters.

The previous spec at `docs/superpowers/specs/2026-05-03-tidy-gosum-design.md` introduced the offline-tidy + cache-seeding flow; this spec extends it to handle (1) the working-tree mutation race and (2) the third-party cache priming, plus the documentation additions for (3) and (4).

## Architecture overview

Three commits, all in one PR:

1. **Commit 1, cacheseed fixpoint** (concern #1): reorder `applyStable` so all working-tree mutations happen before `tidySubmoduleGoSums`, and replace the single-pass seed-and-tidy inside it with iterate-to-fixpoint.
2. **Commit 2, cache priming** (concern #2): add a `primeModuleCache` step before the seed, populating third-party deps via `go mod download` with the inherited `GOPROXY`.
3. **Commit 3, docs** (concerns #3 + #4): inline-comment the existing example workflows in `ci/github/README.md` and strengthen its "Requirements" section; add a new "Recipes" section there for the chore(release) skip filter; add CI-agnostic "CI environment requirements" and "Avoiding the chore(release) CI race" sections to `docs/src/workflows.md`.

Each commit ships independently. The order is significant only because (2) layers on the call site (1) introduces; the reverse order would require an interim state.

## Concern #1: cacheseed wrong-hash bug

### Root cause

`internal/release/release.go:382-424` (`applyStable`) orders the chore(release)-commit composition steps as:

1. CHANGELOG writes (lines 383-398).
2. `rewriteSubmoduleGoMods` (lines 403-405): rewrites sub-modules' `go.mod` files.
3. `tidySubmoduleGoSums` (lines 411-413): seeds in-plan modules into the local cache, then runs offline tidy in each affected sub-module.
4. Consumed-changesets deletion (lines 416-422): `opts.Repo.Remove(".changeset/<consumed>.md")` for each consumed changeset.

Step 3's seed (`internal/release/cacheseed.go:144`) runs `xzip.CreateFromDir(modDir)` against the working tree. At that moment, the consumed `.changeset/*.md` files are still on disk (step 4 hasn't run). They live inside the **root module's source tree** (the repo root is the root module's `modDir`), so they're part of the root module's zip per Go's module-zip rules. The hash monorel writes to the seeded cache is `hash(working-tree-with-consumed-changesets)`.

After step 4, the working tree no longer has those files. The chore(release) commit lands without them. The release tag points at the chore(release) commit. `git archive` of the tag (= what the proxy serves) computes `hash(working-tree-without-consumed-changesets)` ≠ the seeded hash.

Step 3's offline tidy in each affected sub-module reads the seeded `.ziphash` to populate the sub-module's own `go.sum` `h1:` lines for in-plan siblings. So every affected sub-module's `go.sum` records the wrong hash. Downstream consumers fetching the in-plan sibling at the published version hit `SECURITY ERROR: checksum mismatch`.

The bug surfaces only when an in-plan module's source tree contains files that mutate between seed and commit. The canonical case is the root module + repo-level `.changeset/`, but the same class of bug exists for any working-tree mutation between seed and commit.

A second, latent variant: tidy in step 3 mutates affected sub-modules' own `go.sum` files. Each affected sub-module's working tree thus differs from what the seed captured. If any sibling requires another in-plan sibling, that sibling's tidy would record the pre-mutation hash. None of loglayer-go's release exposes this case (the dep graph is "everything depends on root, no sibling depends on a sibling"), but a future cross-sibling release would. The fixpoint loop addresses both variants.

### Fix part 1: reorder `applyStable`

Move the consumed-changesets deletion (currently lines 416-422) to run between `rewriteSubmoduleGoMods` and `tidySubmoduleGoSums`. The block move is mechanical; no other changes to `applyStable`.

```go
// Before:                              // After:
1. CHANGELOG writes                     1. CHANGELOG writes
2. rewriteSubmoduleGoMods               2. rewriteSubmoduleGoMods
3. tidySubmoduleGoSums                  3. consumed-changesets deletion  ← MOVED
4. consumed-changesets deletion         4. tidySubmoduleGoSums
```

Update the GoDoc comments on `tidySubmoduleGoSums` (`gosum.go:30-31`) and the move-source comment in `applyStable` to reflect the new order.

### Fix part 2: iterate-to-fixpoint inside `tidySubmoduleGoSums`

Replace the single-pass seed-and-tidy with a fixpoint loop. The convergence story:

- Each iteration re-seeds in-plan modules from the **current** working tree (which captures any mutations the previous iteration's tidy made).
- Each iteration re-runs offline tidy in every affected sub-module.
- Convergence: when no affected sub-module's `go.sum` changed during this iteration's tidy, every recorded `h1:` matches the seeded hash, which matches the working-tree hash, which is what the chore(release) commit will publish.
- Bound: ≤ depth of the in-plan sibling dep graph. Hard cap at 10 iterations catches genuine bugs (cycles, non-determinism) before they runaway-loop.

```go
// internal/release/gosum.go (replacing the body of tidySubmoduleGoSums
// from the seed/tidy pass downward):

const maxTidyIterations = 10

for i := 0; i < maxTidyIterations; i++ {
    if err := clearSeededEntries(seeded); err != nil {
        return err
    }
    cleanup, err := seedModuleCache(opts)
    if err != nil {
        cleanup()
        return err
    }
    seeded = cleanup // capture for next iteration's clear

    before, err := readGoSums(opts.RepoDir, affected)
    if err != nil {
        return err
    }

    for _, sub := range affected {
        modDir := filepath.Join(opts.RepoDir, sub)
        if err := runOfflineTidy(modDir); err != nil {
            return err
        }
    }

    after, err := readGoSums(opts.RepoDir, affected)
    if err != nil {
        return err
    }

    if !goSumsChanged(before, after) {
        // Fixpoint reached. Stage and return.
        return stageAffected(opts, affected)
    }
}

return errFixpointNotReached(opts.RepoDir, affected, /* per-iteration diffs */)
```

The exact API for `seedModuleCache`'s cleanup will be reshaped: instead of returning `func()`, it returns `[]seededEntry` so the loop can clear before re-seeding (rather than accumulating LIFO-deferred cleanups).

### New helpers (all in `gosum.go` or `cacheseed.go`)

- `clearSeededEntries(entries []seededEntry) error` (cacheseed.go): explicit cleanup of previous iteration's seed cache files. Replaces the `cleanup` closure pattern.
- `readGoSums(repoDir string, paths []string) (map[string][]byte, error)` (gosum.go): snapshot every affected sub-module's `go.sum` file bytes.
- `goSumsChanged(before, after map[string][]byte) bool` (gosum.go): byte-equal comparison of two snapshots.
- `stageAffected(opts Options, paths []string) error` (gosum.go): staging step, factored out of the existing inline loop at lines 80-94.
- `errFixpointNotReached` type (gosum.go): error carrying per-iteration `goSumsChanged` diffs as a diagnostic payload. The orchestrator surfaces it with a "this is likely a monorel bug; please file an issue with the diff below" hint, matching the existing error UX in `preflightOutOfPlanCache` (lines 195-201).

### `seedModuleCache` API change

Change the return type from `(func(), error)` to `([]seededEntry, error)`. The fixpoint loop passes the previous iteration's `[]seededEntry` to `clearSeededEntries` before the next `seedModuleCache` call. Callers outside `tidySubmoduleGoSums` (test code, possibly future callers) get a `clearSeededEntries(seeded)` call to invoke explicitly.

The existing single test caller pattern (`cleanup, err := seedModuleCache(opts); defer cleanup()`) becomes `seeded, err := seedModuleCache(opts); defer clearSeededEntries(seeded)`.

## Concern #2: cache priming for offline tidy

### Root cause

`internal/release/tidy.go:74` sets `GOPROXY=off` for the offline-tidy subprocess. The doc comment at lines 15-18 explains the rationale: "rely on the seeded cache for in-plan siblings; out-of-plan managed siblings are confirmed by `preflightOutOfPlanCache`." But this only covers monorel-managed modules. Third-party deps (anything not declared in `monorel.toml`'s `[packages]` table) must already be present in `GOMODCACHE` from prior `go build` / `go test` work.

On a fresh GitHub runner, `GOMODCACHE` is empty (or has only Go's pre-installed standard-library tooling). Tidy fails with:

```
go: <sub-module> imports
    github.com/some/third-party-dep: module lookup disabled by GOPROXY=off
```

The fix needs to ensure third-party deps are in `GOMODCACHE` before offline tidy runs.

### Fix

Add a `primeModuleCache` step inside `tidySubmoduleGoSums`, between the existing pre-flight check (`preflightOutOfPlanCache`) and the seed step (`seedModuleCache`):

```go
// internal/release/gosum.go:

// 3. Pre-flight: confirm out-of-plan managed siblings are cached.
if err := preflightOutOfPlanCache(opts, affected, inPlan); err != nil {
    return err
}

// NEW. 4. Prime the module cache with third-party deps.
for _, sub := range affected {
    modDir := filepath.Join(opts.RepoDir, sub)
    if err := primeModuleCache(modDir); err != nil {
        return err
    }
}

// 5. Seed the cache with in-plan releases.
// ... (fixpoint loop from concern #1)
```

The new helper lives in `tidy.go` next to `runOfflineTidy`:

```go
// primeModuleCache populates the local module cache with the
// third-party deps modDir's go.mod transitively requires. Subsequent
// offline tidy with GOPROXY=off can resolve those deps from the cache
// without reaching out to the network.
//
// Uses the inherited GOPROXY (typically https://proxy.golang.org,direct)
// and GOSUMDB so go.sum hashes are verified during download. Does NOT
// mutate go.sum: `go mod download` reads go.sum, downloads the listed
// modules, and writes nothing.
//
// PATH, HOME, USER, TMPDIR, LANG, LC_*, GOMODCACHE, GOCACHE, GOPROXY,
// GOSUMDB pass through.
func primeModuleCache(modDir string) error {
    cmd := exec.Command("go", "mod", "download")
    cmd.Dir = modDir
    cmd.Env = primeCacheEnv()

    out, err := cmd.CombinedOutput()
    if err != nil {
        return fmt.Errorf("prime module cache in %s: %w\n%s", modDir, err, out)
    }
    return nil
}

func primeCacheEnv() []string {
    inherit := []string{
        "PATH", "HOME", "USER", "TMPDIR", "LANG",
        "GOMODCACHE", "GOCACHE",
        "GOPROXY", "GOSUMDB",  // NEW vs offlineTidyEnv
    }
    env := make([]string, 0, len(inherit)+4)
    for _, k := range inherit {
        if v, ok := os.LookupEnv(k); ok {
            env = append(env, k+"="+v)
        }
    }
    for _, e := range os.Environ() {
        if strings.HasPrefix(e, "LC_") {
            env = append(env, e)
        }
    }
    env = append(env,
        "GOWORK=off",         // ignore workspace, mirroring offlineTidyEnv
        "GOFLAGS=",           // clear caller's flags, mirroring offlineTidyEnv
        "GOTOOLCHAIN=local",  // same toolchain policy as offlineTidyEnv
    )
    return env
}
```

The key differences vs `offlineTidyEnv`:
- `GOPROXY` is inherited (not forced to `off`).
- `GOSUMDB` is inherited (so `go mod download` still verifies against the public checksum database for newly-fetched modules; `go.sum` provides the project-level guarantee).

### Why not just loosen `GOPROXY=off` during tidy?

That alternative is simpler (one-line removal) but loses the documented determinism guarantee at `tidy.go:15-18`. Tidy with a network-enabled `GOPROXY` could resolve modules differently across re-runs (e.g., if a sibling tag is published mid-run). Keeping `GOPROXY=off` during tidy preserves "tidy reads only from the explicit cache, never from the network" as a hard guarantee. The cost is one extra subprocess per affected sub-module during release, which is negligible (≤ a few seconds total for typical releases).

### Tests

New test in `gosum_test.go`:

- `TestTidySubmoduleGoSums_PrimesThirdPartyDeps`: build a fixture where an affected sub-module requires a third-party dep (e.g., `github.com/davecgh/go-spew` from `golang.org/x/mod`'s test fixtures or a real public module). Run with a fresh `GOMODCACHE`. Confirm `tidySubmoduleGoSums` succeeds.

## Concern #3: Toolchain documentation

### Root cause

`internal/release/tidy.go:78` sets `GOTOOLCHAIN=local` for the offline-tidy subprocess. The rationale (line 27): "don't auto-download a different toolchain." This is a deliberate, defensible choice: catch toolchain mismatches loudly rather than silently inflate cache size with auto-downloads.

The action's existing example workflows in `ci/github/README.md` (lines 26-49 and 51-74) already include `actions/setup-go@v5 with go-version-file: go.mod`. The existing "Requirements" section (line 78) already explains that the runner needs a Go binary satisfying every released module's `go` directive. The loglayer-go failure was a workflow that didn't follow the example: it had `actions/checkout@v4` followed directly by `disaresta-org/monorel/ci/github@v0.11.0` with no `setup-go` between them. Consumers who skip the existing setup-go step or who use `go-version-file: go.mod` against a root with a lower `go` directive than its sub-modules will hit the same failure.

The doc gap is therefore narrower than concerns #1 and #2: the existing example is correct but its rationale isn't visible enough at the point where someone copy-pastes from it. And the requirement is universal (it bites any CI system, not just GitHub Actions) but is currently documented only in the GitHub-specific README.

### Fix

Documentation only. Two layers.

**`ci/github/README.md`: inline comments on the existing examples.** Add a comment above `actions/setup-go@v5` in each example workflow explaining the rationale, so a casual reader doesn't strip the step. Clarify the `go-version-file: go.mod` choice:

```yaml
- uses: actions/checkout@v4
  with:
    fetch-depth: 0

# monorel runs `go mod tidy` with GOTOOLCHAIN=local during release,
# so Go must already be installed at a version satisfying every
# released sub-module's `go` directive (the highest one wins).
# `go-version-file: go.mod` reads the root module's go.mod; if a
# sub-module declares a higher floor, pin `go-version` explicitly.
- uses: actions/setup-go@v5
  with:
    go-version-file: go.mod

- uses: disaresta-org/monorel/ci/github@<version>
  with:
    command: pr
```

Strengthen the existing "Requirements" → "go on PATH" bullet (line 78) to call out the failure mode explicitly: "If the runner's Go is older than the highest sub-module's `go` directive, tidy fails with `go.mod requires go >= X.Y; running Z; GOTOOLCHAIN=local`. The fix is always to bump the runner's Go, not to remove `GOTOOLCHAIN=local` from monorel's tidy step (the env var is part of monorel's offline-tidy determinism guarantee)."

**`docs/src/workflows.md`: new CI-agnostic section.** monorel ships a GitHub-specific CI integration today, but the underlying requirement applies to any CI invoking `monorel release` (GitLab CI, Gitea Actions, CircleCI, Drone, self-hosted runners, etc.). Add a "## CI environment requirements" section listing the universal requirements:

- Go installed at a version ≥ the highest `go` directive across released sub-modules. The `GOTOOLCHAIN=local` policy during offline tidy means the runner's Go must satisfy every sub-module; auto-download is intentionally blocked. Pin explicitly when sub-modules raise the floor.
- `GOPROXY` set to a real proxy or `direct` so the prime-cache step (concern #2) can fetch third-party deps. The offline-tidy that follows uses `GOPROXY=off` regardless of this setting; only the priming step honors `GOPROXY`.
- Push permissions for tags (the `release` command runs `git push --follow-tags`). Provider API token with `contents: write` and `pull-requests: write` for the `pr` command.

Cross-link to `ci/github/README.md` for the GitHub-specific implementation.

No code change.

## Concern #4: CI race recipe on `chore(release):` commits

### Root cause

The `chore(release):` merge commit (created when the always-open release PR is merged to main) carries the new module versions in its `go.mod` files. The matching tags are created and pushed by `release.yml` (using `monorel/ci/github` with `command: release`) running on that same push event. Any other workflow that fires on the same push event and runs `go mod tidy` (or any operation that resolves Go modules) races the tag-push and may transiently fail with:

```
go: <module>: reading <module>/go.mod at revision v2.1.0: unknown revision v2.1.0
```

The CI failure is a phantom: the release succeeds, the tags get pushed, and the next non-release commit's CI run is clean. But the chore(release) commit carries a red mark in the GitHub UI on every release.

### Fix

Documentation only. Two layers.

**`ci/github/README.md`: new "Recipes" section** with the GitHub-specific syntax:

```markdown
## Recipes

### Skipping CI on chore(release) commits

The release commit `chore(release): ...` is created by the always-open
release PR's merge. On the same push event, `release.yml` (using this
action with `command: release`) creates and pushes per-package tags.
Any *other* workflow that runs on the same push and resolves Go module
versions will race the tag-push and may transiently fail with:

    go: example.com/foo/v2: reading example.com/foo/go.mod at revision
        v2.1.0: unknown revision v2.1.0

To avoid the phantom failure, skip the workflow on `chore(release):`
commits. The skip filter:

```yaml
jobs:
  test:
    if: github.event_name == 'pull_request' || !startsWith(github.event.head_commit.message, 'chore(release):')
    # ... rest of job ...
```

Apply the filter to every job (`test`, `staticcheck`, `govulncheck`,
etc.) that runs `go mod tidy` or anything else that resolves the new
versions. Pull-request triggers stay always-on; only push-to-main runs
are skipped, and only when the head commit is the release-PR merge.

The `release.yml` workflow that runs the actual release pipeline does
NOT need this filter; its own `if:` clause is the *opposite* shape
(only run on `chore(release):` commits), so it's already mutually
exclusive with the skip pattern above.
```

**`docs/src/workflows.md`: new CI-agnostic section "Avoiding the chore(release) CI race"** describing the principle and the per-CI filter syntax. Cover the three CI systems whose providers monorel supports today:

- **GitHub Actions / Gitea Actions** (same syntax): `if: ${{ !startsWith(github.event.head_commit.message, 'chore(release):') }}` on each job. (Cross-link the GitHub README's Recipes section for the full snippet.)
- **GitLab CI**: `rules:` clause on each job:
  ```yaml
  rules:
    - if: '$CI_COMMIT_TITLE =~ /^chore\(release\):/'
      when: never
    - when: on_success
  ```
- **Generic / other CI systems**: skip the workflow when the head commit subject starts with `chore(release):`. The exact syntax varies; the principle is universal.

Don't ship full alternative workflow files for GitLab/Gitea (out of scope; monorel doesn't ship reference CI integrations for those today). The filter snippets are enough to unblock anyone applying the principle to their own setup.

No code change.

## Tests (full coverage across concerns #1 and #2)

All in `internal/release/gosum_test.go`, using the existing `setupSubmoduleFixture` helper (lines 31+).

### Concern #1 regression tests

1. **`TestApplyStable_RootChangesetDeletion_HashesMatchPublished`** (or in `release_test.go` as it exercises `applyStable`'s reorder): build a fixture with the root module having a `.changeset/<name>.md` consumed by the plan and one sub-module requiring root. Run `applyStable`. After the simulated chore(release) commit lands on the fake repo, compute the hash via `dirhash.HashZip` against `git archive` of the simulated tag, compare against the `h1:` entry in the sub-module's `go.sum`. They must match.

2. **`TestTidySubmoduleGoSums_CrossSiblingCascade_ConvergesQuickly`**: three modules: A (root), B (depends on A), C (depends on B). All in-plan. Run `tidySubmoduleGoSums`. Verify it converges. Verify every `h1:` recorded in B's and C's `go.sum` matches what the corresponding tagged version's bytes hash to. Optionally instrument the iteration count via a test hook and assert ≤ 3 iterations.

3. **`TestTidySubmoduleGoSums_FixpointNotReached_SurfacesDiagnosticError`**: pin the error UX. Mock the seed step or tidy step to introduce non-determinism (e.g., a test-only environment variable that makes seed produce a different hash each call). Confirm `tidySubmoduleGoSums` returns `errFixpointNotReached` with the per-iteration diff payload populated and the error message hints at filing an issue.

### Concern #2 regression test

4. **`TestTidySubmoduleGoSums_PrimesThirdPartyDeps_FromColdCache`**: build a fixture where an affected sub-module's `go.mod` requires a real third-party dep (e.g., `gopkg.in/yaml.v3`, which monorel itself uses). Run with a fresh `GOMODCACHE` (already what `setupSubmoduleFixture` sets up via `t.Setenv("GOMODCACHE", tmpModCache)`). Confirm `tidySubmoduleGoSums` succeeds. Without `primeModuleCache`, this would fail with the "module lookup disabled by GOPROXY=off" error.

### Existing test coverage

The existing tests in `gosum_test.go` (lines 131-330) all exercise `tidySubmoduleGoSums` with single-pass scenarios. After the fixpoint loop lands, those tests still produce the same end-state because the loop converges in 1 iteration when there's no cross-sibling dep to propagate. They should pass without modification.

## Documentation surface

Two layers of documentation across the four concerns. The detailed shapes are in the per-concern sections above; the summary:

**`ci/github/README.md`** (GitHub-specific):
- Inline comments on the existing `actions/setup-go` step in both example workflows, explaining the `GOTOOLCHAIN=local` rationale and the `go-version-file: go.mod` choice (concern #3).
- Strengthened "Requirements" → "go on PATH" bullet calling out the failure mode (concern #3).
- New "## Recipes" → "### Skipping CI on chore(release) commits" section with the GitHub-Actions if-clause snippet (concern #4).

**`docs/src/workflows.md`** (CI-agnostic):
- New "## CI environment requirements" section listing universal requirements: Go version ≥ highest sub-module floor, `GOPROXY` set to a real proxy or `direct`, push permissions for tags + provider API token (concern #3 universal version).
- New "## Avoiding the chore(release) CI race" section describing the race and the per-CI filter syntax for GitHub Actions, GitLab CI, and Gitea Actions (concern #4 universal version).

Cross-link `ci/github/README.md` from `docs/src/workflows.md` for the GitHub-specific implementation, and `docs/src/workflows.md` from `ci/github/README.md` for the universal principles.

Add an entry to monorel's CHANGELOG (`CHANGELOG.md`) under the next release version covering all four concerns, with cross-references to the loglayer-go incident PRs (#77 / #78 / #80 / #81) as the empirical example.

## Out of scope

- **Action-side prerequisites enforcement.** The action could fail loudly at startup if `setup-go` hasn't run with a sufficient Go version, instead of letting tidy fail with the cryptic `GOTOOLCHAIN=local` error. Worth doing later; the README addition is the right level of intervention for now.
- **Pushing tags before the chore(release) commit.** That'd eliminate concern #4 entirely (no race possible) but would require a redesign of monorel's release pipeline. Concern #4 is a docs gap; pre-tagging is a separate design problem.
- **Yanking and republishing v2.1.0 / cli v2.2.0 / pretty v2.1.0 / console v2.1.0 of `loglayer/loglayer-go`.** loglayer-go applied a manual recovery (PR #81: strip and retidy) that fixed the bad `go.sum` hashes on `main`, so consumers fetching from `main`'s HEAD are fine. The published tags themselves still have the wrong recorded hashes embedded in their sub-modules' go.sum, but those entries are recorded against the *seeded* hash, not against the proxy hash; downstream consumers re-tidy and overwrite them with correct values. Already-published tags don't need to be re-cut for monorel's fix to take effect on future releases.

## File-level summary

**Modified:**
- `internal/release/release.go`: reorder consumed-changesets deletion in `applyStable`. Update GoDoc on `applyStable` reflecting the new order.
- `internal/release/gosum.go`: replace single-pass body of `tidySubmoduleGoSums` with fixpoint loop. Add `readGoSums`, `goSumsChanged`, `stageAffected` helpers. Add `errFixpointNotReached` type. Insert `primeModuleCache` call before the seed step. Update GoDoc reflecting both changes.
- `internal/release/tidy.go`: add `primeModuleCache` and `primeCacheEnv` next to existing `runOfflineTidy` and `offlineTidyEnv`. No changes to the existing helpers.
- `internal/release/cacheseed.go`: change `seedModuleCache` return type from `(func(), error)` to `([]seededEntry, error)`. Add exported helper `clearSeededEntries(entries []seededEntry) error`. No changes to the hash computation itself.
- `internal/release/gosum_test.go`: add the four new tests above. Update existing tests' calls to `seedModuleCache` for the new return signature. Existing tests' calls of `tidySubmoduleGoSums` are unchanged.
- `ci/github/README.md`: inline-comment the existing example workflows' `actions/setup-go` step with the rationale (concern #3). Strengthen the existing "Requirements" → "go on PATH" bullet to call out the `GOTOOLCHAIN=local` failure mode (concern #3). Add a new "## Recipes" section with the chore(release) skip-CI snippet (concern #4).
- `docs/src/workflows.md`: add two new CI-agnostic sections: "## CI environment requirements" (concern #3 universal version) and "## Avoiding the chore(release) CI race" (concern #4 universal version with GitHub / GitLab / Gitea filter snippets). Cross-link from the GitHub-specific README for the implementation details.
- `CHANGELOG.md`: entry covering all four concerns under the next release.

**New:**
- None. All changes layer onto existing files.

## Changeset

Single root-package changeset, `:minor` bump:

```
.changeset/cacheseed-fixpoint-and-pipeline-fixes.md
---
"monorel.disaresta.com": minor
---

Fix `cacheseed` writing the wrong h1: hash for released sub-modules
(would silently produce broken go.sum entries on every release; see
loglayer-go's v2.1.0 incident). Reorder applyStable so all working-tree
mutations happen before the seed step, and replace the single-pass
seed-and-tidy with iterate-to-fixpoint to handle cross-sibling dep
chains.

Add a `go mod download` priming step before offline tidy so fresh CI
runners (with empty GOMODCACHE) can resolve third-party deps. The
`GOPROXY=off` invariant during tidy is preserved.

Document the `actions/setup-go` prerequisite (sub-modules with
`go 1.25.0` directives need a 1.25+ runner since GOTOOLCHAIN=local
during tidy blocks auto-download) and the `chore(release):`-commit
skip filter recipe.

See https://github.com/disaresta-org/monorel/issues/54.
```
