# Tidy sub-module `go.sum` at release-apply time

Closes [monorel#46](https://github.com/disaresta-org/monorel/issues/46).

## Goal

After `monorel apply` runs, every released sub-module's `go.mod` and `go.sum` are exactly what `go mod tidy` would produce against the published-tag state. The release commit is `go mod tidy`-clean across the repo, including transitive deps that change because a sibling was bumped. No proxy roundtrip; no follow-up commit; no manual housekeeping after pulling a release.

## Non-goals

- Pre-release mode (`monorel pre`). `applyPrerelease` doesn't rewrite `go.mod`, so `go.sum` doesn't drift. The new code only fires on the stable path.
- The root module. Already canonically tidy because the maintainer ran `go test ./...` before releasing; its `go.mod` / `go.sum` are part of the source of truth, not derived.

## Approach

Path B-fat (local hash compute + module-cache injection + offline `go mod tidy`).

### Why offline tidy and not local-only hash compute

Local hash compute (Path B-thin) handles only the direct sibling-pin case: it adds the two `h1:` lines for each in-plan sibling a sub-module's `go.mod` requires. It cannot handle transitive drift — if the released sibling's `go.mod` adds a new transitive dep `X`, the dependent sub-module's `go.mod` would also need a new `// indirect` line and `go.sum` would need entries for `X` (and possibly `X`'s own transitive closure). Reimplementing Go's MVS to compute these locally is high-risk; the algorithm is load-bearing for the toolchain and any subtle disagreement produces silently-broken modules.

Path B-fat sidesteps this by letting Go itself do the resolution offline. We seed the local module cache with the freshly-built release artifacts for each in-plan sibling, then shell out to `go mod tidy` with `GOPROXY=off GOSUMDB=off`. Tidy resolves the seeded versions from the cache, walks the full transitive closure using the developer's already-cached deps (populated by `go test ./...` before the release), and writes canonically-correct `go.mod` and `go.sum`. We inherit zero MVS risk, full transitive coverage, and no proxy lag.

### Why not the proxy-poll fallback (Path A)

We considered a post-publish step: wait for proxy to recognize the new tag, then run online `go mod tidy`. Rejected: the proxy's negative cache can stick for 10–30 minutes after a tag push if a premature lookup poisons it (we hit this exact pathology recovering loglayer-go's v2.0.0 cascade). Cache injection eliminates that risk entirely.

## Architecture

The work splits into three small units, each with a single responsibility:

### `internal/release/gosum.go`

```
tidySubmoduleGoSums(opts Options) error
```

Top-level orchestrator called from `applyStable`, immediately after `rewriteSubmoduleGoMods` and before the consumed-changesets deletion. Returns an error on the first failure; the caller's `defer` cleans up the seeded cache entries.

Responsibilities:

1. Skip the entire pass if `len(opts.Plan.Releases) == 0` or the plan is pre-release-mode.
2. Build the seeded cache entries for every in-plan released module (delegate to `seedModuleCache`).
3. For each released sub-module that has a `go.mod` AND requires at least one in-plan sibling:
   - Run offline `go mod tidy` (delegate to `runOfflineTidy`).
   - Stage any resulting `go.mod` / `go.sum` changes.
4. On any failure, return a wrapped error naming the affected sub-module.

### `internal/release/cacheseed.go`

```
seedModuleCache(opts Options, releases []plan.PackageRelease) (cleanup func() error, err error)
```

For each released sibling at version `vX.Y.Z`, write three files to `$(go env GOMODCACHE)/cache/download/<module>/@v/`:

- `vX.Y.Z.info` — JSON `{"Version":"vX.Y.Z","Time":"<commit-iso8601>"}`. Time defaults to the current commit's timestamp.
- `vX.Y.Z.mod` — exact bytes of the rewritten `go.mod`.
- `vX.Y.Z.zip` — built via `golang.org/x/mod/zip.CreateFromDir(zipFile, module.Version{Path, Version}, modDir)`.
- `vX.Y.Z.ziphash` — `h1:` line written via `dirhash.HashZip` on the zip file. Tidy verifies module zips against this; without it, tidy refuses to load.

Returns a `cleanup` closure that removes only the entries this seed wrote (tracked by absolute path). Designed for `defer cleanup()` in the caller; the cleanup is best-effort and logs but doesn't return an error if a remove fails (the leftover entries are content-addressed and inert).

If `go env GOMODCACHE` shells out to a missing `go` binary or returns a non-existent path, return a clear error before any seeding happens.

### `internal/release/tidy.go`

```
runOfflineTidy(modDir string) error
```

Exec `go mod tidy` in `modDir` with explicit env hygiene. Specifically:

- **Inherited from the parent process**: `PATH`, `HOME`, `GOMODCACHE`, `USER`, `TMPDIR`, `LANG`, `LC_*`, system-shape vars only.
- **Explicitly set**: `GOPROXY=off`, `GOSUMDB=off`, `GOWORK=off`, `GOFLAGS=`, `GOTOOLCHAIN=local` (last one prevents tidy from trying to download a different toolchain).
- **NOT inherited**: any caller-set `GOPROXY`, `GOSUMDB`, `GOWORK`, `GOFLAGS`, `GOPRIVATE`. `GOFLAGS=-mod=vendor` set globally would otherwise break tidy; clearing the slot guarantees consistent behavior.

The implementation builds the env from scratch as a slice of `KEY=value` entries rather than `os.Environ()` + appends. This makes the env hygiene visible at a glance and prevents future regressions from accidentally re-introducing inherited variables.

Capture combined stdout/stderr; on non-zero exit, return an error wrapping the captured output. The error includes the offending module path and a hint:

> tidy failed in transports/foo: <go's error>
>
> Hint: this typically means the local Go module cache is missing a transitive dependency. Re-run after `go test ./...` (which populates the cache), or run `go mod download all` from the repo root.

### Pre-flight cache check (out-of-plan siblings)

Before running tidy, walk each affected sub-module's `go.mod` for managed-but-not-in-plan sibling requires. For each, verify that `<GOMODCACHE>/cache/download/<module>/@v/<version>.info` exists; if not, surface a precise error:

> apply: pre-flight check failed for transports/foo
>
> The release would resolve transports/bar v1.5.0 (a monorel-managed package not in the current release plan), but its cache entry is missing. Run `go mod download go.loglayer.dev/transports/bar/v2@v1.5.0` to populate the cache, or run `go mod download all` from the repo root, then retry.

Saves the maintainer from a generic "missing module" error inside tidy's output.

## Data flow

```
applyStable
  ├── (existing) write CHANGELOGs
  ├── (existing) rewriteSubmoduleGoMods       -> sub-module go.mod files updated
  ├── tidySubmoduleGoSums                      -> NEW
  │     ├── seedModuleCache (for in-plan)     -> writes <cache>/<module>/@v/vX.Y.Z.{info,mod,zip,ziphash}
  │     ├── for each affected sub-module:
  │     │     └── runOfflineTidy               -> sub-module go.mod & go.sum updated
  │     ├── stage go.mod/go.sum changes via opts.Repo.Add
  │     └── defer cleanup()                    -> remove seeded cache entries
  └── (existing) delete consumed changesets
```

The result: a single release commit containing CHANGELOG writes, rewritten go.mod files, tidied go.mod/go.sum files, and the consumed-changesets removal — all coherent, all in one atomic mutation.

## Error handling

Hard-fail across the board. If any single sub-module's tidy fails, the whole apply aborts with a wrapped error naming the affected sub-module. Rationale:

- A partial tidy means some sub-modules are clean and some aren't. The release commit, if allowed to proceed, would be inconsistent across the repo — worse than no commit at all.
- Tidy failures are almost always operator errors (cache not populated, the developer didn't run tests first). Surfacing the failure early lets the maintainer fix the root cause and retry; per-sub-module isolation would silently propagate the drift problem onto post-merge.
- The caller (cobra's `RunE`) already returns errors to `cmd/monorel/main.go`'s top-level printer; no special handling needed.

To minimize partial state, `tidySubmoduleGoSums` runs in two passes: first, every affected sub-module's tidy runs (working tree is mutated as tidy writes its output); only if every tidy succeeds does the orchestrator stage the changes via `opts.Repo.Add`. On failure, the working tree shows dirty `go.mod` / `go.sum` files but the git index is untouched. The maintainer recovers with `git checkout -- '*/go.mod' '*/go.sum'` and retries.

### Cleanup contract

`seedModuleCache` returns a cleanup closure. The orchestrator calls it via `defer` so it runs whether tidy succeeds or fails. Cleanup attempts to remove every entry it wrote (tracked by absolute path); a per-entry remove failure is logged via `opts.Log` (when present) but does NOT cause cleanup itself to fail. The contract for callers is:

- **Happy path**: every seeded entry is removed before the orchestrator returns.
- **Cleanup-failure path**: leftover entries remain in the developer's `GOMODCACHE`. Cache entries are content-addressed; the leftovers are inert (a future `go get <module>@<version>` overwrites them with byte-identical content).

Tests assert the happy-path behavior (post-run, the seeded paths don't exist). The cleanup-failure path is not unit-tested because the only realistic trigger is a filesystem error during `os.Remove`, which we model in tests by stubbing.

### Existing cache entries

If the developer's `GOMODCACHE` already has an entry at `<module>/@v/vX.Y.Z` (e.g. from a prior `go get module@vX.Y.Z` against an existing tag), the seed overwrites it. The overwrite is benign in practice: `zip.CreateFromDir`'s output matches `proxy.golang.org`'s archive bit-for-bit, so the new entry is byte-identical. Cleanup removes the entry on exit; the developer's next `go get module@vX.Y.Z` re-fetches normally.

## Out-of-plan managed siblings (#44 interaction)

The smarter rewriter (#44) pins sibling requires for managed packages outside the current release plan to their latest existing tag. Those versions already exist on the proxy; their cache entries are normally populated from prior dev work. Tidy with `GOPROXY=off` resolves them from the developer's existing cache.

The pre-flight cache check surfaces the missing-cache case before tidy runs (see `tidy.go`'s pre-flight section above), giving the maintainer a precise fix instead of generic "missing module" output from tidy.

We do not pre-fetch out-of-plan siblings via the live proxy. Pre-fetching would re-introduce a proxy roundtrip — small, but conceptually inconsistent with "fully offline at apply time."

## Tidy mutation surprise

`go mod tidy` is semi-active: when MVS picks a higher version of a transitive dep than what's currently in `go.sum`, tidy writes the higher version's hashes. For "republish to clean go.mod" releases (loglayer-go's v2.0.1 cascade is the canonical example), the maintainer expects byte-identical re-publish. If a sibling's rewritten `go.mod` newly resolves a transitive dep at a higher version than dependents currently track, that bump shows up in the release commit's diff.

We accept this. Reasons:

1. The bump is canonically correct — it's what the maintainer would get by manually running `go mod tidy` after the release. Pre-empting it produces a commit that's *less* tidy by the toolchain's own definition.
2. `go mod tidy -e` doesn't actually suppress version bumps; `-e` only changes how *errors* are reported. There's no flag to lock transitive versions.
3. Maintainers who want a strictly byte-identical re-publish can revert any unwanted bump in a separate commit before merging the release PR. The maintainer-facing changelog (`monorel preview`'s rendered plan) makes the bump visible.

The release PR's diff includes the changes; the maintainer reviews them before merging. Surprise is bounded.

## Testing

Synthetic two-sub-module monorepo on disk per test, mirroring `gomod_test.go`'s pattern. Tests live in `internal/release/gosum_test.go`.

| Test | Setup | Asserts |
|------|-------|---------|
| `TestTidy_PinsDirectSibling` | A requires B; B is in plan at v2.0.1 | A's go.sum has B's `h1:` lines after apply |
| `TestTidy_AddsTransitiveIndirect` | A requires B; B's rewritten go.mod adds new dep `X v1.2.3` | A's go.mod has `X v1.2.3 // indirect`; A's go.sum has X's `h1:` lines |
| `TestTidy_PreservesUnrelatedExistingEntries` | A's go.sum has third-party deps unrelated to B | Those entries are sorted alongside the new ones, no losses |
| `TestTidy_IdempotentOnRerun` | run twice on the same input | Second run produces no `Repo.Add` calls |
| `TestTidy_PreReleaseModeNoOp` | `opts.PreState != nil` | tidy pass is skipped entirely |
| `TestTidy_HardFailsOnTidyError` | seeded cache deliberately corrupted (mismatched ziphash) | apply returns an error naming the affected sub-module; `repo.Staged` is empty (no files staged from the failed run) |
| `TestTidy_CleanupRunsOnSuccess` | normal happy path | the seeded cache entries are gone after apply |
| `TestTidy_CleanupRunsOnFailure` | tidy fails | the seeded cache entries are still gone after apply (deferred cleanup) |
| `TestSeedModuleCache_ProducesValidZip` | unit test; no tidy involved | the seeded zip can be opened by `archive/zip` and round-trips through `dirhash.HashZip` |

The `TestTidy_*` integration tests require `go` on PATH (consistent with the existing test suite that runs `go mod tidy` in `internal/cli` tests). Skip with a clear message if `exec.LookPath("go")` fails.

## Concurrency

The release-apply pipeline is single-threaded. No concurrency primitives needed in this code. The cleanup closure isn't required to be re-entrant.

## Configuration surface

None. The new behavior is unconditional on stable releases. Skipping is implicit (pre-release mode, no released sub-modules with go.mod, no in-plan-sibling requires).

If a future operator needs to disable the tidy pass (e.g. a CI runner without `go` on PATH), we'd add an `Options.SkipGoSumTidy bool` field and document it. Not in scope today; ship the unconditional version first.

## Dependency surface

Adds:

- `golang.org/x/mod/zip` — direct.
- `golang.org/x/mod/sumdb/dirhash` — direct.
- `golang.org/x/mod/module` — direct.

All three are already indirect deps via `golang.org/x/mod/modfile` (used by the rewriter). Promotion only.

## File layout

```
internal/release/
├── gomod.go              (existing; rewriter, unchanged)
├── gomod_test.go         (existing; rewriter tests, unchanged)
├── gosum.go              (NEW; tidySubmoduleGoSums, ~80 LOC)
├── gosum_test.go         (NEW; integration tests, ~250 LOC)
├── cacheseed.go          (NEW; seedModuleCache + helpers, ~100 LOC)
├── tidy.go               (NEW; runOfflineTidy, ~30 LOC)
├── release.go            (existing; one-line addition in applyStable)
└── ...
```

Splitting into three files keeps each unit at a comfortable size and makes the seeding and tidy-exec layers independently testable. `gosum.go` is the orchestrator everyone reads; `cacheseed.go` and `tidy.go` are implementation details.

## Documentation

- `docs/src/whats-new.md`: bullet under today's date, scope `` `monorel`: ``, naming the new behavior + the `golang.org/x/mod` dep promotion.
- `docs/src/introduction.md` "What monorel does" section: a sub-bullet about clean go.sum (or merge into the existing "Clean go.mod at release time" bullet, since they describe the same UX outcome from the user's perspective).
- The "At a glance" comparison table at the top: extend the existing `Cleans go.mod for proxy publish` row's wording, or add a sibling row `Cleans go.sum (incl. transitive)`.
- `.changeset/tidy-gosum-on-release.md`: `:minor` bump. Body explains the new behavior, references #46.

## Risk and mitigation

- **Operator without `go` on PATH at release time.** Today's release pipelines run `go test` first, so `go` is always on PATH; any future runner that doesn't have it would need to add it. Surface a clear error if `exec.LookPath("go")` fails: "tidy step requires `go` on PATH; install Go or set `Options.SkipGoSumTidy`" (forward-compatible language even if the option doesn't exist yet).
- **Cache pollution if cleanup fails.** Cache entries are content-addressed; leftover files are inert. The cleanup logs but doesn't fail; if a future user reports stale cache entries, we have logs to chase.
- **Tidy mutates files we didn't expect.** Tidy is the source of truth — if it touches a file other than `go.mod` / `go.sum`, that's a Go behavior change we'd want to surface. The integration tests catch this by comparing the staged file set against the expected set.
- **Subtle hash mismatch with the proxy.** Zero MVS reimplementation, so subtle disagreement is extremely unlikely. The `golang.org/x/mod/zip` and `dirhash` packages are owned by the Go team and used by the toolchain itself; any divergence would surface in the toolchain first.
