# `monorel detect-release` and `monorel auto` Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace text-pattern release detection with provider-API detection. Ship two new monorel subcommands (`detect-release`, `auto`), simplify the action wrapper to a single `command: auto`, and collapse every example workflow / pipeline file to one stage with one command.

**Architecture:** New `internal/detect` package wraps the trailer-or-API decision. New `internal/orchestrator/auto.go` is the dispatch layer; it calls `detect`, then either runs the tag/publish flow or the apply/preview flow. `monorel auto` now owns its own git push (the action wrapper used to do this) via two new `internal/git.Repo` methods (`Fetch`, `CheckoutNewBranch`). The action wrapper at `ci/github/action.yml` is rewritten to drop `inputs.command` entirely; it just downloads the binary and runs `monorel auto`. Every example workflow file collapses to one job / stage / step.

**Tech Stack:** Go (existing), cobra CLI, GitHub Actions composite action, GitLab CI YAML, Bitbucket Pipelines bash, Gitea Actions YAML.

**Spec:** [`docs/superpowers/specs/2026-05-03-detect-release-and-auto-design.md`](../specs/2026-05-03-detect-release-and-auto-design.md)

**Branch:** `spec/detect-release-auto` is the spec branch. Implementation lands on a child branch off main; suggested name `feat/detect-release-auto`.

---

## Pre-implementation: branch hygiene

Spec is committed at `c96600f` on `spec/detect-release-auto`. The implementation should land as a separate branch so the spec-only commit is reviewable in isolation if needed. Suggested:

```bash
cd /home/theo/projects/monorel
git checkout main
git pull --ff-only
git checkout -b feat/detect-release-auto
git cherry-pick c96600f   # carries the spec doc forward
```

The pre-existing stashed work (`fix/release-guard-merge-strategies`) is obsoleted by this design and stays stashed; do NOT cherry-pick it.

---

## File Structure

| Path | Status | Responsibility |
|---|---|---|
| `internal/detect/detect.go` | NEW | `IsReleaseMerge(ctx, repo, prov, sha) (*Result, error)` + types |
| `internal/detect/detect_test.go` | NEW | Unit tests against `git.Fake` + `provider.Fake` |
| `internal/orchestrator/auto.go` | NEW | `Auto(ctx, opts) (*AutoResult, error)` — dispatch on detect |
| `internal/orchestrator/auto_test.go` | NEW | Unit tests for the dispatch matrix |
| `internal/git/repo.go` | MODIFY | Extend `Repo` interface with `Fetch` + `CheckoutNewBranch` |
| `internal/git/exec.go` | MODIFY | Implement `Fetch` + `CheckoutNewBranch` on `*Exec` |
| `internal/git/fake.go` | MODIFY | Implement `Fetch` + `CheckoutNewBranch` on `*Fake` |
| `internal/git/repo_test.go` | MODIFY | Add tests for `Fetch` + `CheckoutNewBranch` on `*Exec` and `*Fake` |
| `internal/cli/auto.go` | NEW | `newAutoCmd()` cobra command |
| `internal/cli/auto_test.go` | NEW | Cobra-level smoke test |
| `internal/cli/detect_release.go` | NEW | `newDetectReleaseCmd()` cobra command |
| `internal/cli/detect_release_test.go` | NEW | Cobra-level smoke test |
| `internal/cli/root.go` | MODIFY | Register both new commands |
| `internal/cli/runtime.go` | MODIFY (small) | Add a `LoadRuntimeForAuto` variant that doesn't fail on missing `.changeset/` (`monorel auto` must work on the post-merge commit when the changeset directory was already consumed) |
| `ci/github/action.yml` | REWRITE | Drop `inputs.command`, drop the case-statement; just download binary + run `monorel auto` |
| `examples/github/.github/workflows/release.yml` | REWRITE | Single workflow (the existing `release-pr.yml` deletes) |
| `examples/github/.github/workflows/release-pr.yml` | DELETE | Subsumed by the merged workflow |
| `examples/gitea/.gitea/workflows/release.yml` | REWRITE | Same shape as github |
| `examples/gitea/.gitea/workflows/release-pr.yml` | DELETE | Subsumed |
| `examples/gitlab/.gitlab-ci.yml` | REWRITE | One stage with one rule |
| `examples/bitbucket/bitbucket-pipelines.yml` | REWRITE | One step, one bash line |
| `docs/src/_partials/github-release-yml.md` | REWRITE | Mirror the example file (single workflow) |
| `docs/src/_partials/github-release-pr-yml.md` | DELETE | Subsumed |
| `docs/src/_partials/gitea-release-yml.md` | REWRITE | Mirror the example |
| `docs/src/_partials/gitea-release-pr-yml.md` | DELETE | Subsumed |
| `docs/src/_partials/gitlab-ci-yml.md` | REWRITE | Mirror the example |
| `docs/src/_partials/bitbucket-pipelines-yml.md` | REWRITE | Mirror the example |
| `docs/src/integrations/github.md` | MODIFY | Replace two-workflow narrative with one-workflow narrative; drop the `release-pr.yml` include |
| `docs/src/integrations/gitea.md` | MODIFY | Same |
| `docs/src/integrations/gitlab.md` | MODIFY | Same (single-stage narrative) |
| `docs/src/integrations/bitbucket.md` | MODIFY | Update to reflect API-based detection (the bash if/else goes away) |
| `docs/src/cli-reference.md` | MODIFY | Document `monorel auto` and `monorel detect-release` |
| `.github/workflows/release.yml` | MODIFY | monorel's OWN release workflow gets the same single-workflow treatment (drop release-pr.yml, drop the if-guard, run `monorel auto` via go run) |
| `.github/workflows/release-pr.yml` | DELETE | monorel's own release-pr.yml is subsumed by the merged release.yml |
| `.changeset/detect-release-auto.md` | NEW | `:major` bump for monorel.disaresta.com (this PR replaces the v1.0.0 `:major` changeset that's already on main; we bundle them by adding to the unmerged release PR #63) |

**Out of scope, do not touch:**

- `provider.Client.FindPRByMergeCommit` and `provider.PullRequest.HeadRef`: shipped in v0.14, stable.
- `release.Tag`, `release.Apply`, `release.PublishReleases`: unchanged contracts.
- The doctor package, plan package, semver package, validate package, changelog package: unchanged.

---

## Phase 1: New `internal/detect` package

### Task 1: Create the detect package skeleton + types

**Files:**
- Create: `internal/detect/detect.go`

- [ ] **Step 1: Create the file with types only (no behavior yet)**

```go
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
```

- [ ] **Step 2: Verify it compiles**

Run: `cd /home/theo/projects/monorel && go build ./internal/detect/`
Expected: build succeeds with no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/detect/detect.go
git commit -m "feat(detect): add internal/detect package skeleton

Types and constants for the detect-release work. IsReleaseMerge
implementation lands in the next commit.

Part of the v1.0 release-detection refactor (spec at
docs/superpowers/specs/2026-05-03-detect-release-and-auto-design.md)."
```

### Task 2: Implement `IsReleaseMerge` (TDD)

**Files:**
- Create: `internal/detect/detect_test.go`
- Modify: `internal/detect/detect.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/detect/detect_test.go
package detect

import (
	"context"
	"errors"
	"testing"

	"monorel.disaresta.com/internal/git"
	"monorel.disaresta.com/internal/provider"
)

func TestIsReleaseMerge_TrailerHits(t *testing.T) {
	repo := git.NewFake()
	if err := repo.Commit("chore(release): pkg-a v1.0.0\n\nmonorel-Release: pkg-a v1.0.0\n"); err != nil {
		t.Fatal(err)
	}
	pf := provider.NewFake()

	res, err := IsReleaseMerge(context.Background(), repo, pf, "")
	if err != nil {
		t.Fatalf("IsReleaseMerge: %v", err)
	}
	if !res.IsRelease {
		t.Errorf("IsRelease = false, want true")
	}
	if res.Source != SourceTrailer {
		t.Errorf("Source = %q, want %q", res.Source, SourceTrailer)
	}
	// Trailer signal short-circuits: API was not called.
	// (provider.Fake records Calls; verify no FindPRByMergeCommit calls.)
}

func TestIsReleaseMerge_APIHits(t *testing.T) {
	repo := git.NewFake()
	// HEAD has no trailer.
	if err := repo.Commit("Merge pull request #5 from monorel/release\n"); err != nil {
		t.Fatal(err)
	}
	sha, _ := repo.CurrentSHA()

	pf := provider.NewFake()
	pr := &provider.PullRequest{
		Number:    5,
		State:     "closed",
		HeadRef:   "monorel/release",
		MergedSHA: sha,
	}
	pf.PRs[5] = pr

	res, err := IsReleaseMerge(context.Background(), repo, pf, sha)
	if err != nil {
		t.Fatalf("IsReleaseMerge: %v", err)
	}
	if !res.IsRelease {
		t.Errorf("IsRelease = false, want true")
	}
	if res.Source != SourceAPI {
		t.Errorf("Source = %q, want %q", res.Source, SourceAPI)
	}
	if res.PR == nil || res.PR.Number != 5 {
		t.Errorf("PR = %+v, want PR #5", res.PR)
	}
}

func TestIsReleaseMerge_APIReturnsWrongHeadRef(t *testing.T) {
	repo := git.NewFake()
	if err := repo.Commit("Merge pull request #5\n"); err != nil {
		t.Fatal(err)
	}
	sha, _ := repo.CurrentSHA()

	pf := provider.NewFake()
	pf.PRs[5] = &provider.PullRequest{
		Number:    5,
		State:     "closed",
		HeadRef:   "feature/something-else", // not monorel/release
		MergedSHA: sha,
	}

	res, err := IsReleaseMerge(context.Background(), repo, pf, sha)
	if err != nil {
		t.Fatal(err)
	}
	if res.IsRelease {
		t.Errorf("IsRelease = true, want false")
	}
	if res.Source != SourceNone {
		t.Errorf("Source = %q, want %q", res.Source, SourceNone)
	}
}

func TestIsReleaseMerge_APIReturnsNoPR(t *testing.T) {
	repo := git.NewFake()
	if err := repo.Commit("Some unrelated commit\n"); err != nil {
		t.Fatal(err)
	}
	pf := provider.NewFake() // no PRs seeded

	res, err := IsReleaseMerge(context.Background(), repo, pf, "deadbeef")
	if err != nil {
		t.Fatal(err)
	}
	if res.IsRelease {
		t.Errorf("IsRelease = true, want false")
	}
}

func TestIsReleaseMerge_APIErrorPropagates(t *testing.T) {
	repo := git.NewFake()
	if err := repo.Commit("Some commit\n"); err != nil {
		t.Fatal(err)
	}
	pf := provider.NewFake()
	pf.FailNext = provider.FailOnce(errors.New("network down"))

	_, err := IsReleaseMerge(context.Background(), repo, pf, "abcd")
	if err == nil {
		t.Fatal("expected wrapped network error")
	}
	// The error is wrapped, not the same value, so check substring.
	if !contains(err.Error(), "network down") {
		t.Errorf("err = %v; should wrap underlying network error", err)
	}
}

func TestIsReleaseMerge_NilProvider(t *testing.T) {
	repo := git.NewFake()
	_, err := IsReleaseMerge(context.Background(), repo, nil, "abcd")
	if !errors.Is(err, ErrProviderRequired) {
		t.Errorf("err = %v, want ErrProviderRequired", err)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || strings.Contains(s, substr))
}
```

(Note: this test file imports `strings` for the `contains` helper. Adjust the import block accordingly.)

- [ ] **Step 2: Run the tests — verify they fail**

Run: `cd /home/theo/projects/monorel && go test ./internal/detect/ -v`
Expected: build failure (`undefined: IsReleaseMerge`).

- [ ] **Step 3: Implement `IsReleaseMerge` in `internal/detect/detect.go`**

Append to `internal/detect/detect.go`:

```go
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
```

- [ ] **Step 4: Run the tests — verify they pass**

Run: `cd /home/theo/projects/monorel && go test ./internal/detect/ -v`
Expected: all six tests pass.

- [ ] **Step 5: Verify the trailer fast path doesn't call the API**

Add at the end of `TestIsReleaseMerge_TrailerHits` (after the existing assertions):

```go
	if pf.Calls.FindPRByMergeCommit != 0 {
		t.Errorf("FindPRByMergeCommit was called %d times; trailer signal should short-circuit",
			pf.Calls.FindPRByMergeCommit)
	}
```

If `provider.Fake.Calls.FindPRByMergeCommit` doesn't exist as a counter, check the actual `provider/fake.go` for the call-tracking mechanism (it might be a slice of calls, a map, or different field name). Adapt the assertion to whatever shape the fake exposes; the spirit is "API not called when trailer hit." If the fake has no such tracking, skip this assertion (the existing tests still cover correctness).

Run: `cd /home/theo/projects/monorel && go test ./internal/detect/ -v`
Expected: all tests still pass; the new assertion confirms short-circuit behavior.

- [ ] **Step 6: Commit**

```bash
git add internal/detect/detect.go internal/detect/detect_test.go
git commit -m "feat(detect): IsReleaseMerge with trailer + API signals

Two-signal release-merge detection:
  - Trailer fast path: strings.Contains(HEAD, 'monorel-Release:').
    No network call. Hits when squash/rebase propagated the source
    body.
  - API signal: provider.FindPRByMergeCommit + HeadRef ==
    'monorel/release'. Hits when the trailer was lost (Bitbucket
    squash, merge-commit on any provider).

OR-combined: either signal alone is sufficient. Provider-required
is enforced via ErrProviderRequired (even when the trailer would
have sufficed; the fast path is an optimization, not a contract).

Tests cover all six cases from the spec: trailer hit, API hit
with matching HeadRef, API hit with wrong HeadRef, API returns
no PR, API errors, nil provider."
```

---

## Phase 2: Extend `internal/git.Repo` with `Fetch` + `CheckoutNewBranch`

`monorel auto`'s feature-branch path needs to fetch the base branch and check out `monorel/release` from it (replacing the bash that today lives in `ci/github/action.yml`). The existing `Repo.CheckoutBranch(name)` only creates from current HEAD, so a new method is needed.

### Task 3: Add `Fetch` to the `Repo` interface

**Files:**
- Modify: `internal/git/repo.go`
- Modify: `internal/git/exec.go`
- Modify: `internal/git/fake.go`
- Modify: `internal/git/repo_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/git/repo_test.go`:

```go
func TestExec_Fetch(t *testing.T) {
	r := newTestRepo(t)
	// Newly-init'd repos have no remote; assert the call returns a
	// meaningful error rather than a panic.
	err := r.Repo.Fetch("origin", "main")
	if err == nil {
		t.Fatal("expected error fetching from a repo with no origin")
	}
}

func TestFake_Fetch(t *testing.T) {
	f := git.NewFake()
	if err := f.Fetch("origin", "main"); err != nil {
		t.Errorf("Fake.Fetch should be a no-op success; got %v", err)
	}
	// FailNext should still propagate.
	f.FailNext = func() error { return errors.New("boom") }
	if err := f.Fetch("origin", "main"); err == nil {
		t.Errorf("expected FailNext error to propagate")
	}
}
```

(`newTestRepo` is the existing helper in `repo_test.go` that constructs an `*Exec` against a tmpdir-backed git repo. Read the file before this task to confirm the helper name; if it's called something else like `setupRepo` or `newRepo`, use that.)

- [ ] **Step 2: Run the tests — verify they fail**

Run: `cd /home/theo/projects/monorel && go test ./internal/git/ -run "Fetch" -v`
Expected: build failure (`undefined: r.Repo.Fetch`).

- [ ] **Step 3: Add `Fetch` to the `Repo` interface in `internal/git/repo.go`**

Add to the `Repo` interface (after `PushTags`, before `CheckoutBranch`):

```go
	// Fetch updates remote-tracking refs for ref from remote
	// (`git fetch <remote> <ref>`). Used by the auto orchestrator to
	// refresh origin/<base-branch> before creating monorel/release
	// from it. Empty ref fetches every ref the remote advertises.
	Fetch(remote, ref string) error

```

- [ ] **Step 4: Implement on `*Exec` in `internal/git/exec.go`**

Add (next to the existing `Push` / `PushTags` methods):

```go
// Fetch implements [Repo.Fetch].
func (e *Exec) Fetch(remote, ref string) error {
	args := []string{"fetch", "--no-tags"}
	if remote != "" {
		args = append(args, remote)
	}
	if ref != "" {
		args = append(args, ref)
	}
	_, err := e.run(args...)
	return err
}
```

- [ ] **Step 5: Implement on `*Fake` in `internal/git/fake.go`**

Add (next to the existing fake methods):

```go
// Fetch implements [Repo.Fetch]. The fake records the call and runs
// the FailNext hook; it does not simulate any state change.
func (f *Fake) Fetch(_ /*remote*/, _ /*ref*/ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.take()
}
```

- [ ] **Step 6: Run the tests — verify they pass**

Run: `cd /home/theo/projects/monorel && go test ./internal/git/ -run "Fetch" -v`
Expected: both tests pass.

- [ ] **Step 7: Don't commit yet — Task 4 lands `CheckoutNewBranch` in the same commit**

### Task 4: Add `CheckoutNewBranch` to the `Repo` interface

**Files:**
- Modify: `internal/git/repo.go`
- Modify: `internal/git/exec.go`
- Modify: `internal/git/fake.go`
- Modify: `internal/git/repo_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/git/repo_test.go`:

```go
func TestExec_CheckoutNewBranch(t *testing.T) {
	r := newTestRepo(t)
	// Make a commit so HEAD is well-defined.
	mustCommit(t, r, "init.txt", "init")
	if err := r.Repo.CheckoutNewBranch("feat/x", "HEAD"); err != nil {
		t.Fatalf("CheckoutNewBranch: %v", err)
	}
	// The new branch should now be HEAD.
	out, err := r.Run("rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(out); got != "feat/x" {
		t.Errorf("HEAD ref = %q, want feat/x", got)
	}
}

func TestFake_CheckoutNewBranch(t *testing.T) {
	f := git.NewFake()
	if err := f.CheckoutNewBranch("feat/x", "HEAD"); err != nil {
		t.Errorf("Fake.CheckoutNewBranch: %v", err)
	}
	if got := f.CurrentBranch; got != "feat/x" {
		t.Errorf("CurrentBranch = %q, want feat/x", got)
	}
}
```

(`mustCommit` is presumed-existing in the test file, similar to other tests in this directory. If it doesn't exist or has a different signature, replace with the inline equivalent — `r.Run("config", ...)`, write a file, `r.Repo.Add`, `r.Repo.Commit`. Read the file first.)

(`f.CurrentBranch` is the field where the Fake records the active branch; read `internal/git/fake.go` first to confirm its name. If it's named differently — `f.Branch`, `f.HEAD` — use that.)

- [ ] **Step 2: Run the tests — verify they fail**

Run: `cd /home/theo/projects/monorel && go test ./internal/git/ -run "CheckoutNewBranch" -v`
Expected: build failure.

- [ ] **Step 3: Add `CheckoutNewBranch` to the `Repo` interface**

In `internal/git/repo.go`, modify the `CheckoutBranch` doc to mention the sibling, and add the new method below it:

```go
	// CheckoutBranch checks out branch, creating it from the
	// current HEAD if it doesn't exist (`git checkout -B`). For
	// creating a branch from a non-HEAD start point (e.g. a remote
	// tracking ref), use [Repo.CheckoutNewBranch].
	CheckoutBranch(branch string) error

	// CheckoutNewBranch creates branch at startPoint and checks it
	// out (`git checkout -B <branch> <startPoint>`). Used by the
	// auto orchestrator to create monorel/release from the freshly
	// fetched origin/<base>.
	CheckoutNewBranch(branch, startPoint string) error

```

- [ ] **Step 4: Implement on `*Exec`**

```go
// CheckoutNewBranch implements [Repo.CheckoutNewBranch].
func (e *Exec) CheckoutNewBranch(branch, startPoint string) error {
	if branch == "" {
		return errors.New("branch is empty")
	}
	if startPoint == "" {
		return errors.New("startPoint is empty")
	}
	_, err := e.run("checkout", "-B", branch, startPoint)
	return err
}
```

- [ ] **Step 5: Implement on `*Fake`**

```go
// CheckoutNewBranch implements [Repo.CheckoutNewBranch]. The fake
// records the new branch as the current branch; startPoint is
// observed but not simulated.
func (f *Fake) CheckoutNewBranch(branch, startPoint string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.take(); err != nil {
		return err
	}
	if branch == "" {
		return errors.New("branch is empty")
	}
	if startPoint == "" {
		return errors.New("startPoint is empty")
	}
	f.CurrentBranch = branch
	return nil
}
```

(Field name on `*Fake` may be different; adapt to existing convention.)

- [ ] **Step 6: Run all `internal/git` tests**

Run: `cd /home/theo/projects/monorel && go test ./internal/git/ -v`
Expected: all tests pass.

- [ ] **Step 7: Commit Phase 2 (Tasks 3 + 4 together)**

```bash
git add internal/git/
git commit -m "feat(git): add Fetch and CheckoutNewBranch to Repo interface

Two new git operations needed by the upcoming monorel auto
orchestrator:

  - Fetch(remote, ref): wraps 'git fetch <remote> <ref>'. Used to
    refresh origin/<base-branch> before creating monorel/release
    from it.
  - CheckoutNewBranch(branch, startPoint): wraps 'git checkout -B
    <branch> <startPoint>'. The existing CheckoutBranch only
    creates from current HEAD; CheckoutNewBranch supports a
    non-HEAD start point (typically a remote tracking ref).

Both implemented on Exec (shells out) and Fake (records call;
no state simulation beyond CurrentBranch tracking)."
```

---

## Phase 3: New `internal/orchestrator/auto.go`

The `Auto` function is the dispatch layer. Calls `detect.IsReleaseMerge`, then either runs the tag/publish flow or the apply/preview flow.

### Task 5: Implement `Auto` (TDD)

**Files:**
- Create: `internal/orchestrator/auto.go`
- Create: `internal/orchestrator/auto_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/orchestrator/auto_test.go`:

```go
package orchestrator

import (
	"context"
	"testing"

	"monorel.disaresta.com/changeset"
	"monorel.disaresta.com/config"
	"monorel.disaresta.com/internal/git"
	"monorel.disaresta.com/internal/provider"
	"monorel.disaresta.com/plan"
)

func TestAuto_ReleaseBranch(t *testing.T) {
	repo := git.NewFake()
	// Trailer-bearing HEAD; detection short-circuits via SourceTrailer.
	if err := repo.Commit("chore(release): pkg-a v1.0.0\n\nmonorel-Release: pkg-a v1.0.0\n"); err != nil {
		t.Fatal(err)
	}

	pf := provider.NewFake()

	cfg := &config.Config{
		Packages: map[string]config.PackageConfig{
			"pkg-a": {Path: "pkg-a", TagPrefix: "pkg-a"},
		},
	}

	res, err := Auto(context.Background(), AutoOptions{
		Config:       cfg,
		Repo:         repo,
		Provider:     pf,
		RepoDir:      ".",
		ChangesetDir: ".changeset",
	})
	if err != nil {
		t.Fatalf("Auto: %v", err)
	}
	if res.Branch != AutoBranchRelease {
		t.Errorf("Branch = %q, want %q", res.Branch, AutoBranchRelease)
	}
	if len(res.Tags) != 1 || res.Tags[0] != "pkg-a/v1.0.0" {
		t.Errorf("Tags = %v, want [pkg-a/v1.0.0]", res.Tags)
	}
}

func TestAuto_FeatureBranch_EmptyPlan(t *testing.T) {
	repo := git.NewFake()
	if err := repo.Commit("docs: typo fix\n"); err != nil {
		t.Fatal(err)
	}

	pf := provider.NewFake()
	// Set the default branch the orchestrator will fetch.
	pf.DefaultBranch = "main"
	// No open release PR, so UpsertPreview returns ActionNoop.

	cfg := &config.Config{
		Packages: map[string]config.PackageConfig{},
	}

	res, err := Auto(context.Background(), AutoOptions{
		Config:       cfg,
		Repo:         repo,
		Provider:     pf,
		RepoDir:      ".",
		ChangesetDir: ".changeset",
	})
	if err != nil {
		t.Fatalf("Auto: %v", err)
	}
	if res.Branch != AutoBranchFeature {
		t.Errorf("Branch = %q, want %q", res.Branch, AutoBranchFeature)
	}
	if res.Action != ActionNoop {
		t.Errorf("Action = %q, want %q", res.Action, ActionNoop)
	}
}

func TestAuto_FeatureBranch_NonEmptyPlan(t *testing.T) {
	// A plan with one pending changeset.
	repo := git.NewFake()
	if err := repo.Commit("feat: add login\n"); err != nil {
		t.Fatal(err)
	}

	pf := provider.NewFake()
	pf.DefaultBranch = "main"

	cfg := &config.Config{
		Packages: map[string]config.PackageConfig{
			"pkg-a": {Path: "pkg-a", TagPrefix: "pkg-a"},
		},
	}
	cs := []*changeset.Changeset{{
		Name: "fresh",
		Bumps: map[string]changeset.BumpLevel{
			"pkg-a": changeset.BumpMinor,
		},
		Body: "Add login.",
	}}

	res, err := Auto(context.Background(), AutoOptions{
		Config:       cfg,
		Repo:         repo,
		Provider:     pf,
		RepoDir:      t.TempDir(), // apply writes to disk
		ChangesetDir: t.TempDir(),
		Changesets:   cs,
		Tags:         nil,
	})
	if err != nil {
		t.Fatalf("Auto: %v", err)
	}
	if res.Branch != AutoBranchFeature {
		t.Errorf("Branch = %q, want %q", res.Branch, AutoBranchFeature)
	}
	if res.Action != ActionCreated {
		t.Errorf("Action = %q, want %q (a release PR should have been opened)", res.Action, ActionCreated)
	}
}

func TestAuto_DetectError(t *testing.T) {
	repo := git.NewFake()
	if err := repo.Commit("Merge pull request #5\n"); err != nil {
		t.Fatal(err)
	}
	pf := provider.NewFake()
	pf.FailNext = provider.FailOnce(errFakeBoom)

	cfg := &config.Config{Packages: map[string]config.PackageConfig{}}

	_, err := Auto(context.Background(), AutoOptions{
		Config:       cfg,
		Repo:         repo,
		Provider:     pf,
		RepoDir:      ".",
		ChangesetDir: ".changeset",
	})
	if err == nil {
		t.Fatal("expected detect error to propagate")
	}
}

var errFakeBoom = errors.New("fake boom")
```

(Note: this test file references `errors` for `errFakeBoom`; add the import. Also `changeset.BumpMinor` may have a different exported name; adapt to existing API. The plan/changeset/release packages' actual interfaces are stable, but field names differ from these placeholders; consult the source before writing.)

- [ ] **Step 2: Run tests — verify they fail**

Run: `cd /home/theo/projects/monorel && go test ./internal/orchestrator/ -run "TestAuto" -v`
Expected: build failure (`undefined: Auto`, `undefined: AutoOptions`, etc.).

- [ ] **Step 3: Implement `Auto` in `internal/orchestrator/auto.go`**

```go
package orchestrator

import (
	"context"
	"errors"
	"fmt"

	"monorel.disaresta.com/changeset"
	"monorel.disaresta.com/changelog"
	"monorel.disaresta.com/config"
	"monorel.disaresta.com/internal/detect"
	"monorel.disaresta.com/internal/git"
	"monorel.disaresta.com/internal/provider"
	"monorel.disaresta.com/internal/release"
	"monorel.disaresta.com/plan"
)

// AutoBranch describes which path Auto took. Stringly-typed at the
// wire level; constants let callers switch exhaustively.
type AutoBranch string

const (
	// AutoBranchRelease means the post-merge release flow ran:
	// monorel tag, push tags, monorel publish.
	AutoBranchRelease AutoBranch = "release"

	// AutoBranchFeature means the upstream-of-merge feature flow ran:
	// monorel apply on the speculative branch (when a plan is
	// pending), force-push the branch (likewise), then upsert the
	// release PR via the orchestrator.
	AutoBranchFeature AutoBranch = "feature"
)

// AutoOptions bundles the inputs to [Auto]. Mirrors the existing
// [Options] but adds the read-only state Auto needs to dispatch
// (changesets, tags, pre-state) plus the two paths' file-system
// dependencies (RepoDir, ChangesetDir).
type AutoOptions struct {
	// Config is the parsed monorel.toml.
	Config *config.Config

	// Repo is the git interface for the working repository.
	Repo git.Repo

	// Provider is the provider client (required; nil errors).
	Provider provider.Client

	// RepoDir is the repository root.
	RepoDir string

	// ChangesetDir is .changeset/ under RepoDir.
	ChangesetDir string

	// Changesets are the pending changesets loaded from
	// ChangesetDir. May be empty.
	Changesets []*changeset.Changeset

	// Tags is the existing tag list from Repo.ListTags("").
	Tags []string

	// PreState is the pre-release state. nil for stable mode.
	PreState *changeset.PreState

	// HeadBranch overrides the source branch for the release PR.
	// Empty defaults to [DefaultHeadBranch].
	HeadBranch string

	// BaseBranch overrides the merge target. Empty queries the
	// provider for the default branch.
	BaseBranch string

	// Today overrides the date used in CHANGELOG entries
	// (YYYY-MM-DD). Empty defaults to today's UTC date.
	Today string

	// Remote is the git remote name for fetch / push (typically
	// "origin"). Empty defaults to "origin".
	Remote string
}

// AutoResult reports what [Auto] did.
type AutoResult struct {
	// Branch is which path Auto took.
	Branch AutoBranch

	// CommitSHA is HEAD's SHA at the moment Auto started.
	CommitSHA string

	// DetectionSource is the signal that decided Branch when
	// Branch == AutoBranchRelease. Empty for AutoBranchFeature.
	DetectionSource detect.Source

	// Tags lists the tags created (release branch only).
	Tags []string

	// Releases lists the provider-side releases created (release
	// branch only).
	Releases []*provider.Release

	// Action is the orchestrator's PR-side decision (feature branch
	// only). One of ActionNoop / ActionCreated / ActionUpdated /
	// ActionClosed.
	Action Action

	// PR is the upserted (or closed) PR (feature branch, when
	// applicable).
	PR *provider.PullRequest
}

// Auto detects whether HEAD is a release-PR merge, then dispatches:
//
//   - Release: monorel tag, push tags, monorel publish.
//   - Feature, plan empty: orchestrator.Run with empty plan
//     (closes any stale release PR; otherwise no-op).
//   - Feature, plan non-empty: fetch base, checkout monorel/release
//     from origin/<base>, monorel apply, force-push monorel/release,
//     orchestrator.Run with the plan.
//
// Auto always exits with a populated AutoResult on success.
func Auto(ctx context.Context, opts AutoOptions) (*AutoResult, error) {
	if opts.Config == nil {
		return nil, errors.New("auto: nil Config")
	}
	if opts.Repo == nil {
		return nil, errors.New("auto: nil Repo")
	}
	if opts.Provider == nil {
		return nil, errors.New("auto: nil Provider")
	}

	remote := opts.Remote
	if remote == "" {
		remote = "origin"
	}

	headSHA, err := opts.Repo.CurrentSHA()
	if err != nil {
		return nil, fmt.Errorf("auto: read HEAD SHA: %w", err)
	}

	det, err := detect.IsReleaseMerge(ctx, opts.Repo, opts.Provider, headSHA)
	if err != nil {
		return nil, fmt.Errorf("auto: detect release merge: %w", err)
	}

	if det.IsRelease {
		return autoRelease(ctx, opts, headSHA, det.Source, remote)
	}
	return autoFeature(ctx, opts, headSHA, remote)
}

func autoRelease(ctx context.Context, opts AutoOptions, headSHA string, src detect.Source, remote string) (*AutoResult, error) {
	tagRes, err := release.Tag(release.TagOptions{
		Config:   opts.Config,
		Repo:     opts.Repo,
		Provider: opts.Provider,
	})
	if err != nil {
		return nil, fmt.Errorf("auto: tag: %w", err)
	}

	if err := opts.Repo.PushTags(remote); err != nil {
		return nil, fmt.Errorf("auto: push tags: %w", err)
	}

	releases, err := release.PublishReleases(ctx, opts.Provider, tagRes)
	if err != nil {
		return nil, fmt.Errorf("auto: publish: %w", err)
	}

	return &AutoResult{
		Branch:          AutoBranchRelease,
		CommitSHA:       headSHA,
		DetectionSource: src,
		Tags:            tagRes.Tags(),
		Releases:        releases,
	}, nil
}

func autoFeature(ctx context.Context, opts AutoOptions, headSHA, remote string) (*AutoResult, error) {
	p, err := plan.Plan(opts.Config, opts.Changesets, opts.Tags, opts.PreState)
	if err != nil {
		return nil, fmt.Errorf("auto: plan: %w", err)
	}

	headBranch := opts.HeadBranch
	if headBranch == "" {
		headBranch = DefaultHeadBranch
	}

	// Non-empty plan: stage on monorel/release, push, then upsert.
	if !p.IsEmpty() {
		baseBranch := opts.BaseBranch
		if baseBranch == "" {
			baseBranch, err = opts.Provider.GetDefaultBranch(ctx)
			if err != nil {
				return nil, fmt.Errorf("auto: get default branch: %w", err)
			}
		}

		if err := opts.Repo.Fetch(remote, baseBranch); err != nil {
			return nil, fmt.Errorf("auto: fetch %s/%s: %w", remote, baseBranch, err)
		}
		if err := opts.Repo.CheckoutNewBranch(headBranch, remote+"/"+baseBranch); err != nil {
			return nil, fmt.Errorf("auto: checkout %s from %s/%s: %w", headBranch, remote, baseBranch, err)
		}

		if _, err := release.Apply(release.Options{
			Plan:         p,
			Config:       opts.Config,
			Repo:         opts.Repo,
			RepoDir:      opts.RepoDir,
			ChangesetDir: opts.ChangesetDir,
			PreState:     opts.PreState,
			Today:        opts.Today,
		}); err != nil {
			return nil, fmt.Errorf("auto: apply: %w", err)
		}

		if err := opts.Repo.Push(remote, headBranch, true); err != nil {
			return nil, fmt.Errorf("auto: push %s: %w", headBranch, err)
		}
	}

	today := opts.Today
	if today == "" {
		today = changelog.Today()
	}

	res, err := Run(ctx, Options{
		Plan:       p,
		Provider:   opts.Provider,
		HeadBranch: headBranch,
		BaseBranch: opts.BaseBranch,
		Today:      today,
	})
	if err != nil {
		return nil, fmt.Errorf("auto: orchestrator: %w", err)
	}

	return &AutoResult{
		Branch:    AutoBranchFeature,
		CommitSHA: headSHA,
		Action:    res.Action,
		PR:        res.PR,
	}, nil
}
```

- [ ] **Step 4: Run tests — verify they pass**

Run: `cd /home/theo/projects/monorel && go test ./internal/orchestrator/ -run "TestAuto" -v`
Expected: all four tests pass.

- [ ] **Step 5: Verify nothing else regressed**

Run: `cd /home/theo/projects/monorel && go test ./...`
Expected: all packages pass.

- [ ] **Step 6: Commit**

```bash
git add internal/orchestrator/auto.go internal/orchestrator/auto_test.go
git commit -m "feat(orchestrator): Auto dispatches release vs feature flow

New Auto function wires detect.IsReleaseMerge to one of two flows:

  - Release: monorel tag, push tags, monorel publish.
  - Feature, empty plan: orchestrator.Run with empty plan
    (closes any stale release PR; otherwise no-op).
  - Feature, non-empty plan: fetch base, checkout monorel/release
    from origin/<base>, monorel apply, force-push the branch,
    orchestrator.Run with the plan.

Auto owns the git push that the action wrapper used to do; this
makes monorel auto a drop-in for the entire post-checkout half of
the existing ci/github/action.yml release-pr / release commands."
```

---

## Phase 4: New CLI commands

### Task 6: `monorel detect-release`

**Files:**
- Create: `internal/cli/detect_release.go`
- Create: `internal/cli/detect_release_test.go`
- Modify: `internal/cli/root.go`

- [ ] **Step 1: Write the failing test**

Create `internal/cli/detect_release_test.go`:

```go
package cli

import (
	"strings"
	"testing"
)

func TestDetectRelease_Help(t *testing.T) {
	// Smoke test: the command registers and its help text mentions
	// the exit-code contract.
	out, err := runCLI(t, "detect-release", "--help")
	if err != nil {
		t.Fatalf("detect-release --help: %v", err)
	}
	for _, want := range []string{"detect-release", "exit", "release"} {
		if !strings.Contains(out, want) {
			t.Errorf("help missing %q\n%s", want, out)
		}
	}
}
```

(`runCLI` is the existing helper for cobra-level tests. Confirm by reading `internal/cli/helpers_test.go`. If absent, mirror the pattern from `init_test.go` or `tag_test.go` which exercise commands end-to-end.)

- [ ] **Step 2: Run — fail**

Run: `cd /home/theo/projects/monorel && go test ./internal/cli/ -run "TestDetectRelease" -v`
Expected: failure (`unknown command "detect-release"`).

- [ ] **Step 3: Implement the command**

Create `internal/cli/detect_release.go`:

```go
package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"monorel.disaresta.com/config"
	"monorel.disaresta.com/internal/detect"
	"monorel.disaresta.com/internal/provider"
	"monorel.disaresta.com/internal/provider/factory"
)

func newDetectReleaseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "detect-release",
		Short: "Report whether HEAD is the merge of monorel's release PR.",
		Long: `Inspects HEAD using two signals:

  1. Trailer: HEAD's commit body contains a "monorel-Release:" trailer.
     Hits when squash-merge or rebase-merge propagated the source body.
  2. API: provider.FindPRByMergeCommit returns a PR whose source
     branch is "monorel/release". Hits when the trailer was lost
     (Bitbucket squash, merge-commit on any provider).

Either signal alone is sufficient. The trailer is checked first
(no network); the API call only fires when the trailer is missing.

Exit codes:

  0  HEAD is a release-PR merge. Caller should run monorel tag /
     publish.
  1  HEAD is NOT a release-PR merge. Caller should run monorel apply
     / preview --upsert.
  2  Detection failed (network, auth, or repo-state error). Caller
     should retry or surface the error.

Used internally by monorel auto. Useful as a standalone gate for
custom CI scripts.`,
		RunE: runDetectRelease,
	}
}

func runDetectRelease(cmd *cobra.Command, _ []string) error {
	rt, err := loadRuntime(cmd)
	if err != nil {
		return err
	}

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	providerName := config.ResolveProvider(rt.Config.Provider.Name)
	token := provider.TokenFromEnv(providerName)
	if token == "" {
		envVars := provider.TokenEnvVars(providerName)
		fmt.Fprintf(cmd.ErrOrStderr(), "detect-release: provider %q requires %s in the environment\n", providerName, envVars)
		return ErrExit(2)
	}
	client, err := factory.New(ctx, rt.Config.Provider, token)
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "detect-release: provider client: %v\n", err)
		return ErrExit(2)
	}

	headSHA, err := rt.Repo.CurrentSHA()
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "detect-release: read HEAD SHA: %v\n", err)
		return ErrExit(2)
	}

	res, err := detect.IsReleaseMerge(ctx, rt.Repo, client, headSHA)
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "detect-release: %v\n", err)
		return ErrExit(2)
	}

	if res.IsRelease {
		fmt.Fprintf(cmd.OutOrStdout(), "release commit detected (source: %s)\n", res.Source)
		return nil
	}
	// Exit code 1 with a hint on stderr; main() suppresses the
	// trailing "Error: ..." print because ErrExit is silent.
	fmt.Fprintln(cmd.ErrOrStderr(), "HEAD is not a release-PR merge")
	return ErrExit(1)
}
```

(`ErrExit` is defined in `internal/cli/validate.go`: `type ErrExit int` with `Error() string` returning `fmt.Sprintf("exit %d", int(e))`. main() uses `cli.IsSilentExit(err)` to skip the default error print and `cli.ExitCode(err)` to set the process exit code. Pattern: print diagnostic to `cmd.ErrOrStderr()` first, then `return ErrExit(N)`. The wrapped exit code propagates without an "Error: ..." line on stderr.)

- [ ] **Step 4: Register the command**

In `internal/cli/root.go`, modify `cmd.AddCommand(...)` to include `newDetectReleaseCmd()`:

```go
	cmd.AddCommand(
		newAddCmd(),
		newStatusCmd(),
		newPlanCmd(),
		newValidateCmd(),
		newApplyCmd(),
		newTagCmd(),
		newReleaseCmd(),
		newPublishCmd(),
		newPreviewCmd(),
		newPreCmd(),
		newInitCmd(),
		newDoctorCmd(),
		newDetectReleaseCmd(),  // ADDED
	)
```

- [ ] **Step 5: Run tests — verify they pass**

Run: `cd /home/theo/projects/monorel && go test ./internal/cli/ -run "TestDetectRelease" -v`
Expected: pass.

- [ ] **Step 6: Don't commit yet — Task 7 ships `monorel auto` in the same commit**

### Task 7: `monorel auto`

**Files:**
- Create: `internal/cli/auto.go`
- Create: `internal/cli/auto_test.go`
- Modify: `internal/cli/root.go`

- [ ] **Step 1: Write the failing test**

```go
package cli

import (
	"strings"
	"testing"
)

func TestAuto_Help(t *testing.T) {
	out, err := runCLI(t, "auto", "--help")
	if err != nil {
		t.Fatalf("auto --help: %v", err)
	}
	for _, want := range []string{"auto", "release", "preview"} {
		if !strings.Contains(out, want) {
			t.Errorf("help missing %q\n%s", want, out)
		}
	}
}
```

- [ ] **Step 2: Run — fail**

Run: `cd /home/theo/projects/monorel && go test ./internal/cli/ -run "TestAuto" -v`
Expected: failure.

- [ ] **Step 3: Implement**

Create `internal/cli/auto.go`:

```go
package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"monorel.disaresta.com/changelog"
	"monorel.disaresta.com/config"
	"monorel.disaresta.com/internal/orchestrator"
	"monorel.disaresta.com/internal/provider"
	"monorel.disaresta.com/internal/provider/factory"
)

func newAutoCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auto",
		Short: "Auto-dispatched post-push pipeline (detect, then release or preview).",
		Long: `One-stop CI command. Runs in this order:

  1. Detect: is HEAD the merge of monorel's release PR? Uses the
     trailer-or-API logic of monorel detect-release.

  2. If yes (release branch):
       - monorel tag         (creates per-package tags from trailers)
       - git push --tags     (pushes tags to origin)
       - monorel publish     (creates provider Releases per tag)

  3. If no (feature branch):
       - plan := plan.Plan(...)
       - if plan empty: monorel preview --upsert (closes any open
         release PR if there is no longer anything to release).
       - else: fetch origin/<base>, checkout monorel/release from
         it, monorel apply, force-push monorel/release, monorel
         preview --upsert (open or update the release PR).

The single entry point makes the per-provider CI workflow trivial:
configure git author, then run "monorel auto". No "if commit
matches X then run command Y" branching in YAML or bash — that
logic lives inside monorel.`,
		RunE: runAuto,
	}
	cmd.Flags().String("base-branch", "",
		"Merge target branch. Empty queries the provider for the repo's default branch.")
	cmd.Flags().String("remote", "origin",
		"Git remote name for fetch and push.")
	return cmd
}

func runAuto(cmd *cobra.Command, _ []string) error {
	rt, err := loadRuntime(cmd)
	if err != nil {
		return err
	}

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	providerName := config.ResolveProvider(rt.Config.Provider.Name)
	token := provider.TokenFromEnv(providerName)
	if token == "" {
		envVars := provider.TokenEnvVars(providerName)
		return fmt.Errorf("auto: provider %q requires %s in the environment", providerName, envVars)
	}
	client, err := factory.New(ctx, rt.Config.Provider, token)
	if err != nil {
		return fmt.Errorf("provider client: %w", err)
	}

	baseBranch, _ := cmd.Flags().GetString("base-branch")
	remote, _ := cmd.Flags().GetString("remote")

	res, err := orchestrator.Auto(ctx, orchestrator.AutoOptions{
		Config:       rt.Config,
		Repo:         rt.Repo,
		Provider:     client,
		RepoDir:      rt.RepoDir,
		ChangesetDir: rt.ChangesetDir,
		Changesets:   rt.Changesets,
		Tags:         rt.Tags,
		PreState:     rt.PreState,
		BaseBranch:   baseBranch,
		Remote:       remote,
		Today:        changelog.Today(),
	})
	if err != nil {
		return err
	}

	out := cmd.OutOrStdout()
	switch res.Branch {
	case orchestrator.AutoBranchRelease:
		fmt.Fprintf(out, "Released %d package(s) at %s (detection source: %s):\n",
			len(res.Tags), short(res.CommitSHA), res.DetectionSource)
		for _, tag := range res.Tags {
			fmt.Fprintf(out, "  %s\n", tag)
		}
	case orchestrator.AutoBranchFeature:
		switch res.Action {
		case orchestrator.ActionNoop:
			rt.Log.Info("Plan is empty and no release PR is open. Nothing to do.")
		case orchestrator.ActionClosed:
			rt.Log.Info("Plan is empty; closed release PR #%d.", res.PR.Number)
		case orchestrator.ActionCreated:
			rt.Log.Info("Created release PR #%d: %s", res.PR.Number, res.PR.HTMLURL)
		case orchestrator.ActionUpdated:
			rt.Log.Info("Updated release PR #%d: %s", res.PR.Number, res.PR.HTMLURL)
		}
	}
	return nil
}
```

(`short(sha)` is an existing helper in `internal/cli/`; verify by `grep -n 'func short' internal/cli/*.go` before pasting.)

- [ ] **Step 4: Register the command**

In `internal/cli/root.go`:

```go
	cmd.AddCommand(
		newAddCmd(),
		newStatusCmd(),
		newPlanCmd(),
		newValidateCmd(),
		newApplyCmd(),
		newTagCmd(),
		newReleaseCmd(),
		newPublishCmd(),
		newPreviewCmd(),
		newPreCmd(),
		newInitCmd(),
		newDoctorCmd(),
		newDetectReleaseCmd(),
		newAutoCmd(),  // ADDED
	)
```

- [ ] **Step 5: Run tests — verify they pass**

Run: `cd /home/theo/projects/monorel && go test ./internal/cli/ -v`
Expected: all CLI tests pass (existing ones + the two new help-smoke tests).

- [ ] **Step 6: Run the full suite**

Run: `cd /home/theo/projects/monorel && go test ./... && go vet ./... && staticcheck ./... && gofmt -l .`
Expected: all clean.

- [ ] **Step 7: Commit Phase 4 (Tasks 6 + 7 together)**

```bash
git add internal/cli/auto.go internal/cli/auto_test.go internal/cli/detect_release.go internal/cli/detect_release_test.go internal/cli/root.go
git commit -m "feat(cli): add monorel detect-release and monorel auto

Two new top-level subcommands:

  - detect-release: pure detection. Exits 0 when HEAD is a
    release-PR merge, 1 when not, 2 on error. Useful as a standalone
    gate for custom CI scripts.
  - auto: detect + dispatch. Runs the release pipeline (tag, push,
    publish) when HEAD is a release-PR merge, otherwise the feature
    pipeline (apply on monorel/release, push, preview --upsert).
    Owns its own git push so the action wrapper stays trivial.

Both require the provider's auth token in the environment."
```

---

## Phase 5: Action wrapper rewrite

### Task 8: Rewrite `ci/github/action.yml`

**Files:**
- Modify: `ci/github/action.yml`
- Modify: `ci/github/README.md`

- [ ] **Step 1: Rewrite the action.yml**

Replace `ci/github/action.yml` with:

```yaml
name: monorel
description: "Run monorel auto against the current repo. Detects whether HEAD is a release-PR merge and dispatches accordingly."
author: disaresta-org
branding:
  icon: tag
  color: blue

inputs:
  version:
    description: "monorel version to run, e.g. v1.2.3. Defaults to 'latest'."
    required: false
    default: latest
  config:
    description: "Path to monorel.toml relative to the repo root."
    required: false
    default: monorel.toml
  token:
    description: "Token used for provider API calls. Needs contents:write and pull-requests:write permissions on the workflow."
    required: false
    default: ${{ github.token }}

runs:
  using: composite
  steps:
    - name: Configure git author
      shell: bash
      run: |
        set -euo pipefail
        git config user.name  "monorel-bot[automation]"
        git config user.email "monorel-bot@users.noreply.github.com"

    - name: Resolve binary asset
      id: resolve
      shell: bash
      env:
        VERSION: ${{ inputs.version }}
      run: |
        set -euo pipefail
        case "$RUNNER_OS" in
          Linux)   os=linux  ;;
          macOS)   os=darwin ;;
          Windows) os=windows ;;
          *) echo "::error::unsupported runner OS: $RUNNER_OS"; exit 1 ;;
        esac
        case "$RUNNER_ARCH" in
          X64)   arch=amd64 ;;
          ARM64) arch=arm64 ;;
          *) echo "::error::unsupported runner arch: $RUNNER_ARCH"; exit 1 ;;
        esac
        if [ "$VERSION" = "latest" ]; then
          tag=$(curl -fsSL -H "Accept: application/vnd.github+json" \
            "https://api.github.com/repos/disaresta-org/monorel/releases/latest" \
            | jq -r '.tag_name')
        else
          tag="$VERSION"
        fi
        if [ -z "$tag" ] || [ "$tag" = "null" ]; then
          echo "::error::could not resolve monorel version"
          exit 1
        fi
        ext=tar.gz
        [ "$os" = "windows" ] && ext=zip
        url="https://github.com/disaresta-org/monorel/releases/download/${tag}/monorel_${tag#v}_${os}_${arch}.${ext}"
        {
          echo "tag=$tag"
          echo "os=$os"
          echo "arch=$arch"
          echo "ext=$ext"
          echo "url=$url"
        } >> "$GITHUB_OUTPUT"

    - name: Download monorel
      shell: bash
      env:
        URL: ${{ steps.resolve.outputs.url }}
        EXT: ${{ steps.resolve.outputs.ext }}
      run: |
        set -euo pipefail
        tmp=$(mktemp -d)
        echo "downloading $URL"
        curl -fsSL "$URL" -o "$tmp/monorel.$EXT"
        case "$EXT" in
          tar.gz) tar -xzf "$tmp/monorel.$EXT" -C "$tmp" ;;
          zip)    unzip -q "$tmp/monorel.$EXT" -d "$tmp" ;;
        esac
        bin=$(find "$tmp" -maxdepth 2 -type f \( -name 'monorel' -o -name 'monorel.exe' \) | head -n1)
        if [ -z "$bin" ]; then
          echo "::error::extracted archive does not contain a monorel binary"
          ls -R "$tmp"
          exit 1
        fi
        case "$RUNNER_OS" in
          Windows)
            install -m 0755 "$bin" "$RUNNER_TEMP/monorel.exe"
            echo "$RUNNER_TEMP" >> "$GITHUB_PATH"
            "$RUNNER_TEMP/monorel.exe" --version
            ;;
          *)
            install -m 0755 "$bin" /usr/local/bin/monorel
            monorel --version
            ;;
        esac

    - name: Run monorel auto
      shell: bash
      env:
        CONFIG: ${{ inputs.config }}
        GITHUB_TOKEN: ${{ inputs.token }}
      run: |
        set -euo pipefail
        monorel auto --config "$CONFIG"
```

- [ ] **Step 2: Update `ci/github/README.md`**

Read it first, then update the prose to:
- Drop the `command: pr` / `command: release` / `command: doctor` documentation.
- Replace with a single example showing `command: auto` is no longer required (the action takes no `command` input).
- Mention that `monorel doctor` should run as a separate step (or its own job) in workflows that want pre-merge diagnostics; the action wrapper is now release-only.

The shape should match the new action.yml: `version`, `config`, `token` inputs only.

- [ ] **Step 3: Verify the YAML parses**

Run: `cd /home/theo/projects/monorel && python3 -c "import yaml; yaml.safe_load(open('ci/github/action.yml'))"`
Expected: no error.

- [ ] **Step 4: Commit**

```bash
git add ci/github/action.yml ci/github/README.md
git commit -m "feat(ci): rewrite GitHub action wrapper around monorel auto

The wrapper now takes only version, config, and token inputs. The
command input is removed; the wrapper just runs 'monorel auto',
which internally dispatches between release and feature flows
based on detection.

doctor remains a separate command for users who want pre-merge
diagnostics; they invoke it as a separate job step rather than
through this action wrapper."
```

---

## Phase 6: Example workflow + partial collapses

The four providers need their workflow / pipeline files updated. monorel's own `.github/workflows/release.yml` and `release-pr.yml` also collapse.

### Task 9: Collapse the GitHub examples

**Files:**
- Modify: `examples/github/.github/workflows/release.yml`
- Delete: `examples/github/.github/workflows/release-pr.yml`
- Modify: `docs/src/_partials/github-release-yml.md`
- Delete: `docs/src/_partials/github-release-pr-yml.md`

- [ ] **Step 1: Replace `examples/github/.github/workflows/release.yml`**

```yaml
name: monorel
on:
  push:
    branches: [main]

permissions:
  contents: write
  pull-requests: write

jobs:
  monorel:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
      - uses: disaresta-org/monorel/ci/github@v1.0.0
```

- [ ] **Step 2: Delete the obsolete release-pr.yml**

```bash
git rm examples/github/.github/workflows/release-pr.yml
```

- [ ] **Step 3: Update the partial to mirror the example**

`docs/src/_partials/github-release-yml.md`:

````markdown
```yaml
name: monorel
on:
  push:
    branches: [main]

permissions:
  contents: write
  pull-requests: write

jobs:
  monorel:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
      - uses: disaresta-org/monorel/ci/github@v1.0.0
```
````

- [ ] **Step 4: Delete the obsolete release-pr partial**

```bash
git rm docs/src/_partials/github-release-pr-yml.md
```

- [ ] **Step 5: Don't commit yet (Tasks 9-12 collapse together)**

### Task 10: Collapse the Gitea examples

**Files:**
- Modify: `examples/gitea/.gitea/workflows/release.yml`
- Delete: `examples/gitea/.gitea/workflows/release-pr.yml`
- Modify: `docs/src/_partials/gitea-release-yml.md`
- Delete: `docs/src/_partials/gitea-release-pr-yml.md`

- [ ] **Step 1: Replace `examples/gitea/.gitea/workflows/release.yml`**

```yaml
name: monorel
on:
  push:
    branches: [main]

permissions:
  contents: write
  pull-requests: write

jobs:
  monorel:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
      - uses: disaresta-org/monorel/ci/github@v1.0.0
        with:
          token: ${{ secrets.GITEA_TOKEN }}
```

- [ ] **Step 2: Delete the obsolete release-pr.yml**

```bash
git rm examples/gitea/.gitea/workflows/release-pr.yml
```

- [ ] **Step 3: Update the partial**

`docs/src/_partials/gitea-release-yml.md`:

````markdown
```yaml
name: monorel
on:
  push:
    branches: [main]

permissions:
  contents: write
  pull-requests: write

jobs:
  monorel:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
      - uses: disaresta-org/monorel/ci/github@v1.0.0
        with:
          token: ${{ secrets.GITEA_TOKEN }}
```
````

- [ ] **Step 4: Delete the obsolete partial**

```bash
git rm docs/src/_partials/gitea-release-pr-yml.md
```

### Task 11: Collapse the GitLab example

**Files:**
- Modify: `examples/gitlab/.gitlab-ci.yml`
- Modify: `docs/src/_partials/gitlab-ci-yml.md`

- [ ] **Step 1: Replace `examples/gitlab/.gitlab-ci.yml`**

```yaml
default:
  image: ghcr.io/disaresta-org/monorel:1.0.0

monorel:
  rules:
    - if: $CI_COMMIT_BRANCH == $CI_DEFAULT_BRANCH
  variables:
    GITLAB_TOKEN: $MONOREL_GITLAB_TOKEN
  script:
    - git config user.name  "monorel-bot[automation]"
    - git config user.email "monorel-bot@users.noreply.example.com"
    - monorel auto
    # Push tags / branches uses GITLAB_TOKEN. monorel auto handles
    # both directions internally; no explicit git push step needed
    # here.
```

- [ ] **Step 2: Update the partial to match**

`docs/src/_partials/gitlab-ci-yml.md`:

````markdown
```yaml
default:
  image: ghcr.io/disaresta-org/monorel:1.0.0

monorel:
  rules:
    - if: $CI_COMMIT_BRANCH == $CI_DEFAULT_BRANCH
  variables:
    GITLAB_TOKEN: $MONOREL_GITLAB_TOKEN
  script:
    - git config user.name  "monorel-bot[automation]"
    - git config user.email "monorel-bot@users.noreply.example.com"
    - monorel auto
```
````

### Task 12: Collapse the Bitbucket example

**Files:**
- Modify: `examples/bitbucket/bitbucket-pipelines.yml`
- Modify: `docs/src/_partials/bitbucket-pipelines-yml.md`

- [ ] **Step 1: Replace `examples/bitbucket/bitbucket-pipelines.yml`**

```yaml
# bitbucket-pipelines.yml
image: ghcr.io/disaresta-org/monorel:1.0.0

pipelines:
  branches:
    main:
      - step:
          name: monorel
          script:
            - git config user.name  "monorel-bot[automation]"
            - git config user.email "monorel-bot@users.noreply.example.com"
            # monorel auto detects via the Bitbucket REST API whether
            # HEAD is a release-PR merge and dispatches accordingly.
            # No bash conditional, no merge-strategy fragility.
            - monorel auto
```

- [ ] **Step 2: Update the partial**

`docs/src/_partials/bitbucket-pipelines-yml.md`:

````markdown
```yaml
# bitbucket-pipelines.yml
image: ghcr.io/disaresta-org/monorel:1.0.0

pipelines:
  branches:
    main:
      - step:
          name: monorel
          script:
            - git config user.name  "monorel-bot[automation]"
            - git config user.email "monorel-bot@users.noreply.example.com"
            - monorel auto
```
````

- [ ] **Step 3: Verify the docs build**

```bash
cd /home/theo/projects/monorel/docs && bun run docs:build
```

Expected: clean (no broken includes from the deleted partials).

- [ ] **Step 4: Commit Phase 6 (Tasks 9-12 together)**

```bash
git add examples/ docs/src/_partials/
git commit -m "feat(ci): collapse example workflows and partials to single file per provider

monorel auto now owns the detect-and-dispatch logic, so each
provider's release pipeline reduces to one workflow / stage / step
that just runs 'monorel auto' on push to the default branch.

Removed:
  - examples/{github,gitea}/.../release-pr.yml (subsumed into
    release.yml; auto handles both branches).
  - docs/src/_partials/{github,gitea}-release-pr-yml.md.

Rewrote:
  - examples/{github,gitea,gitlab,bitbucket}/... and the four
    matching partials. Each is now a one-step config that runs
    'monorel auto'.

The two-workflow / two-stage / two-conditional shape is gone."
```

---

## Phase 7: monorel's own workflow

monorel itself uses a slightly different shape (no action wrapper; uses `go run` instead) but the same principle: collapse to one workflow file with `monorel auto`.

### Task 13: Collapse monorel's own release.yml

**Files:**
- Modify: `.github/workflows/release.yml`
- Delete: `.github/workflows/release-pr.yml`

- [ ] **Step 1: Read the existing files**

```bash
cat .github/workflows/release.yml
cat .github/workflows/release-pr.yml
```

Both currently have an if-guard plus a multi-step body. Note any monorel-specific specialness (no `monorel publish`, the `Capture root tag` step that downstream `build-binaries` / `build-image` consume, the `deploy-docs` job).

- [ ] **Step 2: Rewrite `.github/workflows/release.yml` to collapse the two**

Preserve the chained jobs (deploy-docs, build-binaries, build-image) but consolidate the trigger-side guard. The `monorel auto` step replaces both the `monorel apply / preview --upsert` flow (formerly in release-pr.yml) AND the `monorel tag / git push --follow-tags` flow (formerly in release.yml).

Key adjustments:
- The new top-level job runs unconditionally on push to main; `monorel auto` decides what to do.
- The `Capture root tag` step still runs but only produces a non-empty output when monorel auto's release path actually tagged.
- The `monorel publish` omission in monorel's own pipeline (because goreleaser owns release creation; documented at the top of the file) is preserved — replace `monorel auto` with a custom invocation that ONLY does the detect-and-tag-and-push half. Specifically:

```yaml
      - name: monorel auto
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
        run: |
          set -euo pipefail
          # monorel itself omits the publish step (goreleaser owns
          # GitHub Release creation; see the v0.2 incident note at
          # the top of this file). detect-release + tag + push is
          # the same thing without the trailing publish call.
          if go run ./cmd/monorel detect-release; then
            go run ./cmd/monorel tag
            git push --follow-tags
          else
            go run ./cmd/monorel preview --upsert
          fi
```

(The full `monorel auto` Go-level orchestration is still the right answer for OTHER repos, but monorel's own pipeline has a goreleaser-specific exception. The bash here is the smallest expression of "auto, but skip publish.")

- [ ] **Step 3: Delete release-pr.yml**

```bash
git rm .github/workflows/release-pr.yml
```

- [ ] **Step 4: Verify the YAML parses**

```bash
python3 -c "import yaml; yaml.safe_load(open('.github/workflows/release.yml'))"
```

- [ ] **Step 5: Commit**

```bash
git add .github/workflows/release.yml
git rm .github/workflows/release-pr.yml
git commit -m "feat(ci): collapse monorel's own release-pr / release into one workflow

monorel auto absorbs both halves. The workflow runs unconditionally
on push to main; auto's internal detect-then-dispatch handles which
side runs.

monorel's own pipeline still skips the trailing publish step
(goreleaser owns Release creation; see the file's header note for
the v0.2 incident background), so the auto invocation is split
into 'detect-release && tag && push --follow-tags' rather than
calling monorel auto verbatim. Other repos use the action wrapper
which calls monorel auto in full."
```

---

## Phase 8: Documentation updates

### Task 14: Rewrite the integration pages

**Files:**
- Modify: `docs/src/integrations/github.md`
- Modify: `docs/src/integrations/gitea.md`
- Modify: `docs/src/integrations/gitlab.md`
- Modify: `docs/src/integrations/bitbucket.md`

For each integration page:

- [ ] **Step 1: Drop the two-workflow narrative**

Each page currently has a section like "release-pr.yml" + "release.yml" with two `<!--@include: ../_partials/<provider>-release-pr-yml.md-->` and `<!--@include: ../_partials/<provider>-release-yml.md-->` directives. Collapse to one section, one include of `<provider>-release-yml.md`.

- [ ] **Step 2: Update prose around merge strategies**

The previous text-pattern guards were merge-strategy-fragile; the new API-based detection works across squash, rebase, merge-commit. Update each page's "Merge strategy" section to reflect this:
- GitHub / Gitea: any of squash, rebase, or merge-commit work.
- GitLab: same.
- Bitbucket: squash, fast-forward, and merge-commit all work via the API signal. The Bitbucket-specific section about the `monorel-Release:` trailer being lost on squash is still true but no longer relevant to detection (monorel tag handles it via the universal trailers fallback).

- [ ] **Step 3: Verify the docs build**

```bash
cd /home/theo/projects/monorel/docs && bun run docs:build
```

- [ ] **Step 4: Commit**

```bash
git add docs/src/integrations/
git commit -m "docs(integrations): single-workflow narrative for all four providers

Each integration page now has one workflow / pipeline section with
one partial include. The two-workflow shape and the merge-strategy
fragility callouts are gone.

Bitbucket page still mentions that squash drops the trailer and
the universal trailers fallback recovers, but it's no longer the
load-bearing detection signal: API-based detection works
regardless of merge strategy."
```

### Task 15: Update the CLI reference

**Files:**
- Modify: `docs/src/cli-reference.md`

- [ ] **Step 1: Add `monorel auto` and `monorel detect-release` sections**

Read the existing structure first:

```bash
head -30 docs/src/cli-reference.md
grep '^##' docs/src/cli-reference.md
```

Add a section for `auto` and a section for `detect-release` matching the existing per-command sections' shape. Each section: brief intro, common flags, exit codes (for detect-release), example invocation, link to the integration pages for usage in CI.

- [ ] **Step 2: Verify the docs build**

```bash
cd /home/theo/projects/monorel/docs && bun run docs:build
```

- [ ] **Step 3: Commit**

```bash
git add docs/src/cli-reference.md
git commit -m "docs(cli-reference): document monorel auto and detect-release"
```

---

## Phase 9: Changeset + PR

### Task 16: Add the changeset

**Files:**
- Create: `.changeset/detect-release-auto.md`

- [ ] **Step 1: Write the changeset**

```markdown
---
"monorel.disaresta.com": major
---

**Provider-API release detection.**

Two new monorel subcommands replace the previous text-pattern release-detection:

- `monorel detect-release` reports whether HEAD is the merge of monorel's release PR. Exit 0 yes, 1 no, 2 error.
- `monorel auto` is the one-stop CI command. It detects, then runs the release pipeline (tag + push + publish) or the feature pipeline (apply + push + preview --upsert) accordingly.

The action wrapper at `disaresta-org/monorel/ci/github` simplifies to a single auto step. The `command: pr`, `command: release`, and `command: doctor` inputs are removed. Each provider's example workflow / pipeline file collapses to one file with one step that runs `monorel auto`.

Detection uses two signals OR'd together: the `monorel-Release:` trailer in HEAD's commit body (fast path; squash + rebase) and the provider's `FindPRByMergeCommit` returning a PR whose source branch is `monorel/release` (network signal; covers merge-commit and Bitbucket squash). Either signal alone is sufficient.

Migration from the previous pre-1.0 surface:

- Replace `command: pr` and `command: release` workflow steps with a single step (no `command:` input) that runs the action wrapper. The wrapper runs `monorel auto` internally.
- `command: doctor` users invoke `monorel doctor` as their own step instead.
- Custom CI scripts that text-grep `chore(release):` or `monorel-Release:` from commit messages should switch to running `monorel detect-release` and branching on its exit code.
```

- [ ] **Step 2: Verify monorel apply would consume it cleanly**

```bash
cd /home/theo/projects/monorel && go run ./cmd/monorel plan
```

Expected: the planner shows `monorel.disaresta.com` going from v0.14.0 (or v1.0.0 if PR #63 has merged) to a major bump.

- [ ] **Step 3: Commit**

```bash
git add .changeset/detect-release-auto.md
git commit -m "chore(changeset): major bump for detect-release + auto"
```

### Task 17: Open the PR

- [ ] **Step 1: Push the branch**

```bash
git push -u origin feat/detect-release-auto
```

- [ ] **Step 2: Open the PR**

```bash
gh pr create --title "feat: provider-API release detection (monorel auto + detect-release)" --body "..."
```

PR body covers: spec link, summary, the v0.14 → v1.0 graduation, test plan, and the migration path from the old `command: pr` / `command: release` action inputs.

- [ ] **Step 3: Coordinate with the v1.0 release PR (#63)**

If PR #63 (the v1.0.0 release PR) is still open when this PR lands, the v1.0.0 release plan needs to absorb this `:major` changeset. Either:
- Land THIS PR first; PR #63 auto-regenerates with the merged content.
- Close PR #63, merge this, then re-open / re-cut v1.0 from the new state.

Coordinate manually; this plan doesn't prescribe.

---

## Self-Review

**Spec coverage:**

- ✅ Solution: detect-release + auto subcommands. Tasks 1-7 cover.
- ✅ Architecture: `internal/detect`, `internal/orchestrator/auto.go`, `internal/cli/{auto,detect_release}.go`. Tasks 1, 5, 6, 7.
- ✅ Detection logic (trailer + API). Task 2.
- ✅ Source branch hardcoded. Constant `releaseHeadBranch` in detect package.
- ✅ Auto flow (release vs feature dispatch). Task 5.
- ✅ Cross-provider parity. Tasks 9-12.
- ✅ Failure modes table. Implicit in Task 5's autoRelease/autoFeature error wrapping.
- ✅ Non-goals respected. No public detect package; no configurable source branch; no `command: pr` etc.
- ✅ Testing: unit + integration + cross-provider. Tasks 2, 5, 6, 7.
- ✅ Migration: changeset doc explains. Task 16.
- ✅ Effort estimate. Phases 1-2 (Tasks 1-4) ≈ 0.5 day. Phases 3-4 (Tasks 5-7) ≈ 0.75 day. Phases 5-9 (Tasks 8-17) ≈ 0.5 day. Total within the 1.5-day spec estimate.

**Placeholder scan:**

- ❌ Task 5 test references `changeset.BumpMinor` and `Changesets` field shapes that need verification before paste. Mitigation: each test step says "adapt to existing API" with reference to the actual source.
- ❌ Task 6 references `silentExit` without defining it. Mitigation: cross-reference to existing `IsSilentExit` / `ExitCode` helpers in `internal/cli/`. Implementer should grep for the type before writing.

These are conscious adapt-to-existing-code instructions, not unsolved blockers. The test-step "adapt to actual API" framing matches how every other plan in the repo handles minor naming uncertainty.

**Type consistency:**

- `Result` type in `internal/detect`: defined in Task 1, consumed in Task 2 tests, used in Task 5's `autoRelease`. Field `Source` of type `Source` enum, consumer reads via switch. Consistent.
- `AutoOptions` and `AutoResult` types in `internal/orchestrator`: defined in Task 5, consumed in Task 7 (`runAuto`). Fields match.
- `AutoBranch` enum (`AutoBranchRelease`, `AutoBranchFeature`): defined in Task 5, switched on in Task 7. Consistent.
- `releaseHeadBranch` constant (`"monorel/release"`): in detect package only. Mirrors `orchestrator.DefaultHeadBranch` value but doesn't import it (avoiding the circular dep). Documented inline.
- `git.Repo.Fetch` and `git.Repo.CheckoutNewBranch`: signatures defined in Task 3 / Task 4, consumed in Task 5. Match.

No drift between definitions and consumers.
