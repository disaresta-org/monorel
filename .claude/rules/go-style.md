# Go Style Rules

Project-specific Go conventions. General Go style (effective Go, gofmt, golint) is assumed; this file only records patterns that came up in this codebase.

## Naming

### Avoid stdlib name collisions

Don't pick exported identifiers that shadow widely-used stdlib types or packages. Watch for: `Context`, `Error` (use `Err` for fields), `Request`, `Response`, `Reader`, `Writer`, `Time`, `Map`, `String`.

### Package name vs import path

Where a subpackage's directory name collides with a stdlib or widely-known third-party package (e.g. `internal/provider/github`), the package name follows the directory name. Disambiguation happens at import sites with an alias (`gh "github.com/disaresta-org/monorel/internal/provider/github"`) or by the implementation type being unexported (callers see only `provider.Client`).

## API Surface

### Pure-function core

The planner (`internal/plan.Plan`) is a pure function. New planning logic goes there, NOT in the CLI or release-application layers. Tests are table-driven over the planner; CLI and release tests exercise integration but should not duplicate the per-bump-level matrix.

### Constructor pair: `New` panics, `Build` returns error

When a constructor can fail at runtime (missing env, invalid arg from config), pair `New` with a `Build`-style sibling, not just one or the other. monorel's surface is small; today only providers use this (`github.New(ctx, opts) (provider.Client, error)`). When you add another constructor that can fail, follow the same pattern.

### Concurrency-safety classes

Every method on a public type falls into one of:

1. **Pure**: no shared state. Always safe.
2. **Returns-new**: builds and returns a fresh value; receiver untouched.
3. **Atomic-mutate**: backed by atomics or a mutex, safe under contention.
4. **Setup-only mutate-in-place**: NOT safe concurrently with reads. GoDoc must say `// Setup-only:` explicitly.

Default to (1) or (2). The `provider.Fake` is (3) (mutex-backed). `git.Exec` is (4) — one working tree per Exec instance, no concurrent invocations.

## Code Reuse

### Lift duplicated boilerplate to a shared package

When N implementations share the same setup pattern (config defaulting, conversion, validation), put the helper next to the interface it services. Threshold heuristic: **3+ verbatim copies** of the same 4+ line pattern is the moment to lift.

Don't generalize across providers past the shared helpers. Each provider has its own auth flow, error shapes, and pagination quirks; the repetition between provider impls is honest and abstraction would just move it.

### Don't fight the compiler

Recent Go (1.22+) inlines aggressively and stack-allocates small structs. Before adding `sync.Pool`, manual escape-analysis tricks, or other allocation-elimination strategies:

1. Measure baseline with `-benchmem`.
2. Apply the change.
3. Measure again with `benchstat` (10 runs).
4. If allocs/op didn't decrease, revert.

monorel doesn't have a measured hot path today; it's a CLI invoked once per release. Don't optimize speculatively.

## File Organization

### Split by responsibility, not by line count alone

A 400+ LOC file that does one thing is fine. A 200 LOC file that mixes two unrelated concerns is not. Soft signal to look for a split: a file approaches ~300 LOC AND you can name two distinct responsibilities AND the helper functions cluster cleanly into two non-overlapping sets. If any of those three is missing, leave it alone.

### Co-locate by package, split by file

Don't fragment a single concept across packages just because the file got large. Within a package, splitting into multiple files is cheap and doesn't affect callers. Across packages it's a public-API decision.

## Errors

### Sentinel errors over typed errors

Public errors are sentinels declared at package scope: `var ErrPlanEmpty = errors.New(...)`. Callers compare with `errors.Is`. Don't introduce error types unless the caller genuinely needs to extract structured data from the error.

### Don't validate at internal boundaries

Validate at the public API boundary (`config.Load`, `provider.New`, CLI flag parsing), then trust the input through internal code. Don't re-check `nil` on every internal hop.

## Comments

### Setup-only methods get a one-line marker

```go
// IncrementCounter raises pkg's counter and returns the value before
// the increment.
//
// Setup-only: caller must own the PreState (no concurrent reads).
func (s *PreState) IncrementCounter(pkg string) int { ... }
```

### Don't narrate the change

No comments referring to "added for X", "used by Y", "renamed from Z", or "see PR #123". Those belong in commit messages and CHANGELOG.md, not source.

### Justify non-obvious choices

A comment like `// changesets stay during pre-mode; counters in pre.json record progress` is keeper because the why is non-obvious. A comment like `// loop over releases` is noise.

## Testing

### Three layers

- **Unit (the bulk)**: pure-function tests on `internal/plan`, `internal/changeset`, `internal/changelog`, `internal/semver`, `internal/config`. Table-driven, no I/O.
- **Integration**: `internal/git/testutil.NewRepo(t)` creates a real on-disk git repo per test. End-to-end CLI tests exercise commands as a user would.
- **Fakes**: `provider.NewFake()` and `git.NewFake()` for higher-layer tests that don't need a real working tree or live API.

### testutil hermeticity

`testutil.NewRepo` writes `commit.gpgsign=false` and `tag.gpgsign=false` to the repo's local config and sets `GIT_CONFIG_GLOBAL=/dev/null` / `GIT_CONFIG_SYSTEM=/dev/null` in the env. This matters because contributor environments (1Password SSH agent, GPG signing, hooks) can otherwise leak into hermetic tests. Don't remove this hardening.

### Fake fault injection

`provider.Fake.FailNext` is a `func() error` consulted before every API call. Use `provider.FailOnce(err)` for a one-shot failure; use `provider.FailOnNth(n, err)` to test partial-success paths.
