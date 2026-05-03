# cacheseed-fixpoint and release-pipeline fixes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Address [issue #54](https://github.com/disaresta-org/monorel/issues/54) end-to-end. Fix the `cacheseed.go` wrong-`h1:`-hash bug that breaks every monorel-driven release. Add a cache-priming step so offline tidy works on fresh CI runners. Document the toolchain prerequisite and the chore(release) CI race in the right doc layers.

**Architecture:** Three coordinated changes in one PR:

1. Reorder `applyStable` so all working-tree mutations happen before the cache seed, and replace the single-pass seed-and-tidy inside `tidySubmoduleGoSums` with iterate-to-fixpoint.
2. Add a `primeModuleCache` step before the seed so third-party deps land in `GOMODCACHE` before offline tidy runs with `GOPROXY=off`.
3. Inline-comment the existing `actions/setup-go` step in `ci/github/README.md`'s example workflows, add a Recipes section with the chore(release) skip filter, and add CI-agnostic universal sections to `docs/src/workflows.md`.

**Tech Stack:** Go 1.26 (per the existing `go.mod`). Test infra: standard `testing` + the existing `setupSubmoduleFixture` helper in `internal/release/gosum_test.go`. Hash computation via `golang.org/x/mod/sumdb/dirhash` and `golang.org/x/mod/zip`. Spec at `docs/superpowers/specs/2026-05-03-cacheseed-fixpoint-and-release-pipeline-fixes-design.md`.

---

## File structure

**Modified:**

- `internal/release/release.go`: reorder consumed-changesets deletion in `applyStable` to run before `tidySubmoduleGoSums` (concern #1, fix part 1).
- `internal/release/cacheseed.go`: change `seedModuleCache` return type from `(func(), error)` to `([]seededEntry, error)`. Add exported helper `clearSeededEntries`.
- `internal/release/gosum.go`: replace single-pass body of `tidySubmoduleGoSums` with iterate-to-fixpoint loop. Add `readGoSums`, `goSumsChanged`, `stageAffected` helpers. Add `errFixpointNotReached` error type (concern #1, fix part 2). Insert `primeModuleCache` call before the seed step (concern #2).
- `internal/release/tidy.go`: add `primeModuleCache` and `primeCacheEnv` next to existing `runOfflineTidy` and `offlineTidyEnv` (concern #2).
- `internal/release/gosum_test.go`: regression tests for concerns #1 and #2.
- `ci/github/README.md`: inline comments on `actions/setup-go` in both example workflows; strengthen "Requirements" → "go on PATH" bullet; add new "## Recipes" section (concerns #3 and #4 GitHub-specific).
- `docs/src/workflows.md`: add "## CI environment requirements" and "## Avoiding the chore(release) CI race" sections (concerns #3 and #4 universal).
- `CHANGELOG.md`: entry under the next release covering all four concerns.

**New:**

- `.changeset/cacheseed-fixpoint-and-pipeline-fixes.md`: changeset declaring a `:minor` bump.

---

## Working agreements

- **Branch:** `fix/cacheseed-fixpoint` in `/tmp/monorel-src` (already cut from `main`). All commits land on this branch.
- **Per-task verify command:** `go test ./...` from the repo root, unless the task explicitly says otherwise.
- **Commit format:** Conventional Commits with appropriate scope. Body explains the why; trailers include `Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>`.
- **No `--no-verify`:** monorel's repo doesn't have a lefthook pre-commit hook today (verified by `ls .lefthook* lefthook.yml` returning empty). If commits fail for unexpected reasons, debug rather than bypass.
- **TDD discipline:** every code change is preceded by a test that demonstrates the bug. Don't commit "implementation + test" together unless the spec calls for the implementation to be the test.
- **Each task ends in a green commit.** No half-finished states across task boundaries.
- **Self-review before commit:** read the diff. Confirm only the intended files / lines changed. Run `go test ./...` from the repo root.

---

## Task 1: regression test for the changeset-deletion mutation race

**Files:**
- Modify: `internal/release/gosum_test.go`

This task adds a failing test that demonstrates the bug. The fix lands in Task 2.

- [ ] **Step 1: Read the existing test fixture.**

Open `/tmp/monorel-src/internal/release/gosum_test.go` and skim `setupSubmoduleFixture` (around line 31). Note its shape: it builds two modules `a` and `b` under `repoDir`, with optional `aRequiresB`, and sets `GOMODCACHE` to a temp dir. It does NOT create a `.changeset/` directory or any consumed changesets. The new test will need to.

- [ ] **Step 2: Add a new test that exercises `Apply` end-to-end with a consumed changeset.**

Append to `internal/release/gosum_test.go`:

```go
// TestApply_RootChangesetDeletion_HashesMatchPublished pins the
// regression for the loglayer-go v2.1.0 incident. The chore(release)
// commit deletes consumed `.changeset/*.md` files. Those files live
// inside the root module's source tree, so the root module's zip
// hash differs between (a) the working tree at seed time (with the
// changesets present) and (b) the chore(release) commit (without
// them). Before the fix in Task 2, the affected sub-module's go.sum
// would record (a)'s hash, which doesn't match what `git archive`
// of the published tag produces.
//
// This test runs the full Apply pipeline and asserts that the
// `h1:` for the in-plan root recorded in the affected sub-module's
// go.sum equals the hash of `git archive` against the chore(release)
// commit.
func TestApply_RootChangesetDeletion_HashesMatchPublished(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skipf("`go` not on PATH: %v", err)
	}

	repoDir := t.TempDir()
	tmpModCache := t.TempDir()
	t.Cleanup(func() {
		filepath.WalkDir(tmpModCache, func(path string, _ fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			os.Chmod(path, 0o755)
			return nil
		})
	})
	t.Setenv("GOMODCACHE", tmpModCache)

	// Layout:
	//   <repoDir>/
	//     go.mod              (module example.com/root, go 1.26)
	//     root.go
	//     .changeset/foo.md   (will be consumed by the plan)
	//     sub/go.mod          (requires example.com/root)
	//     sub/sub.go
	mustWriteFile(t, filepath.Join(repoDir, "go.mod"),
		"module example.com/root\n\ngo 1.26\n")
	mustWriteFile(t, filepath.Join(repoDir, "root.go"),
		"package root\n\nfunc Hello() string { return \"hi\" }\n")
	mustWriteFile(t, filepath.Join(repoDir, ".changeset", "foo.md"),
		"---\n\"root\": minor\n---\n\nbump\n")
	mustWriteFile(t, filepath.Join(repoDir, "sub", "go.mod"),
		"module example.com/sub\n\ngo 1.26\n\nrequire example.com/root v0.1.0\n")
	mustWriteFile(t, filepath.Join(repoDir, "sub", "sub.go"),
		"package sub\n\nimport \"example.com/root\"\n\nfunc Greet() string { return root.Hello() }\n")

	repo := git.NewFake()
	cfg := &config.Config{
		Packages: map[string]config.PackageConfig{
			"root": {TagPrefix: "", Path: "."},
			"sub":  {TagPrefix: "sub", Path: "sub"},
		},
	}
	rp := &plan.ReleasePlan{
		Releases: []plan.PackageRelease{
			{Config: cfg.Packages["root"], Tag: "v0.1.0", Bump: semver.Minor},
			{Config: cfg.Packages["sub"], Tag: "sub/v0.1.0", Bump: semver.Minor},
		},
		Consumed: []changeset.Consumed{{Name: "foo"}},
	}

	opts := Options{
		Plan:         rp,
		Config:       cfg,
		Repo:         repo,
		RepoDir:      repoDir,
		ChangesetDir: filepath.Join(repoDir, ".changeset"),
		Today:        "2026-05-03",
	}

	if _, err := Apply(opts); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	// Read back the recorded hash for example.com/root in sub/go.sum.
	subGoSum, err := os.ReadFile(filepath.Join(repoDir, "sub", "go.sum"))
	if err != nil {
		t.Fatalf("read sub/go.sum: %v", err)
	}
	recordedH1 := extractH1(t, subGoSum, "example.com/root v0.1.0")
	if recordedH1 == "" {
		t.Fatalf("sub/go.sum: missing h1: line for example.com/root v0.1.0\n%s", subGoSum)
	}

	// Compute what `git archive`'s hash would be against the working
	// tree as of the chore(release) commit. Apply staged the deletion
	// of .changeset/foo.md into the FakeRepo, so on disk it's still
	// there. For the test to compare against the post-deletion shape,
	// remove the file from the working tree manually here. (FakeRepo
	// stages but doesn't commit; this is the test's stand-in for the
	// real commit.)
	if err := os.Remove(filepath.Join(repoDir, ".changeset", "foo.md")); err != nil {
		t.Fatalf("remove staged-deleted changeset: %v", err)
	}

	expectedH1, err := dirhashOfRoot(repoDir)
	if err != nil {
		t.Fatalf("compute expected hash: %v", err)
	}
	if recordedH1 != expectedH1 {
		t.Errorf("sub/go.sum recorded wrong h1: for example.com/root v0.1.0\n  recorded: %s\n  expected: %s",
			recordedH1, expectedH1)
	}
}

// extractH1 pulls the `h1:HASH=` value for the given "<module> <version>"
// prefix from a go.sum file's bytes. Returns the empty string if the
// line is absent.
func extractH1(t *testing.T, goSum []byte, prefix string) string {
	t.Helper()
	want := []byte(prefix + " ")
	for _, line := range bytes.Split(goSum, []byte("\n")) {
		if !bytes.HasPrefix(line, want) {
			continue
		}
		// Skip the /go.mod sub-hash; we want the full-zip hash.
		if bytes.Contains(line, []byte("/go.mod ")) {
			continue
		}
		// line looks like: example.com/root v0.1.0 h1:HASH=
		parts := bytes.Fields(line)
		if len(parts) < 3 {
			continue
		}
		return string(parts[2])
	}
	return ""
}

// dirhashOfRoot computes Hash1 of the root module zip built from
// repoDir, matching what `git archive` would produce for a published
// version pointing at the working tree's current state. Used to
// verify that monorel's recorded hash matches what consumers will
// fetch.
func dirhashOfRoot(repoDir string) (string, error) {
	mv := module.Version{Path: "example.com/root", Version: "v0.1.0"}
	tmpZip, err := os.CreateTemp("", "monorel-test-zip-*.zip")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmpZip.Name())
	defer tmpZip.Close()
	if err := xzip.CreateFromDir(tmpZip, mv, repoDir); err != nil {
		return "", err
	}
	if err := tmpZip.Close(); err != nil {
		return "", err
	}
	return dirhash.HashZip(tmpZip.Name(), dirhash.Hash1)
}
```

Add the imports to the existing import block at the top of `gosum_test.go`:

```go
import (
	"bytes"                                  // NEW: for extractH1
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/mod/module"                // NEW: for dirhashOfRoot
	"golang.org/x/mod/sumdb/dirhash"         // NEW
	xzip "golang.org/x/mod/zip"              // NEW

	"monorel.disaresta.com/changeset"
	"monorel.disaresta.com/config"
	"monorel.disaresta.com/internal/git"
	"monorel.disaresta.com/plan"
	"monorel.disaresta.com/semver"
)
```

- [ ] **Step 3: Run the test, confirm it fails.**

Run: `cd /tmp/monorel-src && go test ./internal/release -run TestApply_RootChangesetDeletion_HashesMatchPublished -v`

Expected: FAIL with a hash mismatch like "recorded: h1:... ; expected: h1:..." (different hashes).

If it errors instead of failing on the assertion (e.g., because of an Apply error), debug the fixture before proceeding. The test should reach the final hash comparison and fail there.

- [ ] **Step 4: Commit.**

```bash
cd /tmp/monorel-src
git add internal/release/gosum_test.go
git commit -m "$(cat <<'EOF'
test(release): regression for chore(release) changeset-deletion hash race

The chore(release) commit deletes consumed `.changeset/*.md` files.
Those files live inside the root module's source tree, so the root
module's zip hash differs between (a) the working tree at seed time
(with the changesets present) and (b) the chore(release) commit
(without them). The affected sub-module's go.sum then records the
wrong `h1:` for the in-plan root.

Pin this regression with an end-to-end test that runs Apply and
compares the recorded hash against `dirhash.HashZip` over the
post-deletion working tree. The test fails on this commit; the next
commit (apply-step reorder) makes it pass.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: reorder `applyStable` so consumed-changesets delete before tidy

**Files:**
- Modify: `internal/release/release.go:382-424`

- [ ] **Step 1: Read `applyStable` to confirm the current order.**

Open `/tmp/monorel-src/internal/release/release.go` and find `applyStable` (line ~379). Confirm the steps in order:

1. CHANGELOG writes (lines 383-398).
2. `rewriteSubmoduleGoMods` call (lines 403-405).
3. `tidySubmoduleGoSums` call (lines 411-413).
4. Consumed-changesets deletion (lines 416-422).

- [ ] **Step 2: Move the consumed-changesets deletion ahead of `tidySubmoduleGoSums`.**

Edit `internal/release/release.go`. The current shape is:

```go
	// Strip dev-only sibling replaces and pin sibling require
	// versions in each released package's go.mod, so the published
	// modules are clean for downstream consumers.
	if err := rewriteSubmoduleGoMods(opts); err != nil {
		return err
	}

	// Run `go mod tidy` (offline, against a seeded local cache) in
	// every released sub-module that requires an in-plan sibling, so
	// the release commit's go.sum entries match what consumers will
	// hash from the proxy after the tag pushes.
	if err := tidySubmoduleGoSums(opts); err != nil {
		return err
	}

	// Delete the consumed changesets. The planner's Consumed list
	// dedupes multi-package changesets, so we hit each file once.
	for _, cs := range opts.Plan.Consumed {
		rel := filepath.Join(".changeset", cs.Name+".md")
		if err := opts.Repo.Remove(rel); err != nil {
			return fmt.Errorf("release: remove %s: %w", rel, err)
		}
	}
	return nil
}
```

Replace with (the deletion block moves up; the tidy call moves down):

```go
	// Strip dev-only sibling replaces and pin sibling require
	// versions in each released package's go.mod, so the published
	// modules are clean for downstream consumers.
	if err := rewriteSubmoduleGoMods(opts); err != nil {
		return err
	}

	// Delete the consumed changesets BEFORE seeding/tidy. The
	// `.changeset/*.md` files live inside the root module's source
	// tree; running tidy's seed step against a working tree that
	// still has them produces a hash that doesn't match the
	// chore(release) commit's hash (which lands without them). See
	// docs/superpowers/specs/2026-05-03-cacheseed-fixpoint-and-release-pipeline-fixes-design.md.
	for _, cs := range opts.Plan.Consumed {
		rel := filepath.Join(".changeset", cs.Name+".md")
		if err := opts.Repo.Remove(rel); err != nil {
			return fmt.Errorf("release: remove %s: %w", rel, err)
		}
	}

	// Run `go mod tidy` (offline, against a seeded local cache) in
	// every released sub-module that requires an in-plan sibling, so
	// the release commit's go.sum entries match what consumers will
	// hash from the proxy after the tag pushes.
	if err := tidySubmoduleGoSums(opts); err != nil {
		return err
	}
	return nil
}
```

Also update the GoDoc on `applyStable` (line ~379) so the comment reflects the new order. Find:

```go
// applyStable writes CHANGELOG entries, rewrites go.mod files for
// the released sub-modules, deletes consumed changesets, and stages
// everything. Caller does the commit.
```

Replace with:

```go
// applyStable writes CHANGELOG entries, rewrites go.mod files for
// the released sub-modules, deletes consumed changesets, then runs
// offline tidy in every affected sub-module, and stages everything.
// The deletion happens before tidy so the seed step inside tidy
// hashes the working tree in its final commit shape (not a
// transient state with changesets still present); see
// docs/superpowers/specs/2026-05-03-cacheseed-fixpoint-and-release-pipeline-fixes-design.md.
// Caller does the commit.
```

Also update `tidySubmoduleGoSums`'s GoDoc in `gosum.go` line ~30:

```go
// Called from applyStable AFTER rewriteSubmoduleGoMods and BEFORE
// the consumed-changesets deletion so all the file changes land in
// the same release commit.
```

Replace with:

```go
// Called from applyStable AFTER rewriteSubmoduleGoMods AND AFTER
// the consumed-changesets deletion. The deletion is upstream of the
// seed step so the working tree already has its final commit shape;
// otherwise the seed's hash wouldn't match what `git archive` of
// the published tag produces. See
// docs/superpowers/specs/2026-05-03-cacheseed-fixpoint-and-release-pipeline-fixes-design.md.
```

- [ ] **Step 3: Run the regression test from Task 1.**

Run: `cd /tmp/monorel-src && go test ./internal/release -run TestApply_RootChangesetDeletion_HashesMatchPublished -v`

Expected: PASS.

- [ ] **Step 4: Run the full release-package test suite.**

Run: `cd /tmp/monorel-src && go test ./internal/release/...`

Expected: PASS for every test, including the existing ones. The reorder is a behavior-preserving refactor for the canonical "no consumed-changeset, no working-tree-mutation-after-seed" case (every existing test).

- [ ] **Step 5: Run the full repo test suite.**

Run: `cd /tmp/monorel-src && go test ./...`

Expected: PASS.

- [ ] **Step 6: Commit.**

```bash
cd /tmp/monorel-src
git add internal/release/release.go internal/release/gosum.go
git commit -m "$(cat <<'EOF'
fix(release): delete consumed changesets before tidy seed step

applyStable's order was: CHANGELOG writes -> rewriteSubmoduleGoMods
-> tidySubmoduleGoSums (seed + tidy) -> consumed-changesets
deletion -> commit. The seed step inside tidy zipped the working
tree while consumed `.changeset/*.md` files were still on disk;
the chore(release) commit landed without them; `git archive` of
the published tag computed a different hash than the seed produced.
Affected sub-modules' go.sum recorded the wrong `h1:` for the
in-plan root, breaking every fresh-cache install with SECURITY
ERROR.

Move the consumed-changesets deletion ahead of tidy. The seed now
zips the working tree in its final commit shape; recorded hashes
match what `git archive` of the published tag produces.

The previous commit's regression test now passes.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: change `seedModuleCache` return type to `[]seededEntry`

**Files:**
- Modify: `internal/release/cacheseed.go`
- Modify: `internal/release/gosum.go:65-69`

This task changes the API in preparation for the fixpoint loop in Task 5. The fixpoint loop needs to clear and re-seed across iterations; the closure-based cleanup pattern doesn't compose well across iterations.

- [ ] **Step 1: Edit `seedModuleCache` to return `[]seededEntry` instead of a closure.**

Open `internal/release/cacheseed.go`. Find the function signature (line 41):

```go
func seedModuleCache(opts Options) (cleanup func(), err error) {
```

Replace with:

```go
func seedModuleCache(opts Options) (seeded []seededEntry, err error) {
```

Then replace the function body. The current body is:

```go
func seedModuleCache(opts Options) (cleanup func(), err error) {
	mc, err := goModCache()
	if err != nil {
		return func() {}, fmt.Errorf("seed module cache: %w", err)
	}

	var seeded []seededEntry
	cleanup = func() {
		for _, e := range seeded {
			_ = os.Remove(e.infoPath)
			_ = os.Remove(e.modPath)
			_ = os.Remove(e.zipPath)
			_ = os.Remove(e.zipHashPath)
		}
	}

	now := time.Now().UTC().Format(time.RFC3339)

	for _, r := range opts.Plan.Releases {
		modDir := filepath.Join(opts.RepoDir, r.Config.Path)
		modFilePath := filepath.Join(modDir, "go.mod")

		modBytes, err := os.ReadFile(modFilePath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return cleanup, fmt.Errorf("seed module cache: read %s: %w", modFilePath, err)
		}

		mf, err := readModFile(modFilePath)
		if err != nil {
			return cleanup, fmt.Errorf("seed module cache: %w", err)
		}
		if mf == nil {
			continue
		}

		mv := module.Version{
			Path:    mf.Module.Mod.Path,
			Version: tagVersion(r.Tag),
		}

		entry, err := writeCacheEntry(mc, mv, modDir, modBytes, now)
		seeded = append(seeded, entry)
		if err != nil {
			return cleanup, fmt.Errorf("seed module cache for %s@%s: %w", mv.Path, mv.Version, err)
		}
	}

	return cleanup, nil
}
```

Replace with the slice-returning version:

```go
func seedModuleCache(opts Options) (seeded []seededEntry, err error) {
	mc, err := goModCache()
	if err != nil {
		return nil, fmt.Errorf("seed module cache: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)

	for _, r := range opts.Plan.Releases {
		modDir := filepath.Join(opts.RepoDir, r.Config.Path)
		modFilePath := filepath.Join(modDir, "go.mod")

		modBytes, err := os.ReadFile(modFilePath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return seeded, fmt.Errorf("seed module cache: read %s: %w", modFilePath, err)
		}

		mf, err := readModFile(modFilePath)
		if err != nil {
			return seeded, fmt.Errorf("seed module cache: %w", err)
		}
		if mf == nil {
			continue
		}

		mv := module.Version{
			Path:    mf.Module.Mod.Path,
			Version: tagVersion(r.Tag),
		}

		entry, err := writeCacheEntry(mc, mv, modDir, modBytes, now)
		// Append unconditionally: writeCacheEntry returns its
		// (partially-populated) entry on error too, and the caller
		// uses the slice to clean up whatever was written before
		// failure. Empty fields in entry round-trip safely through
		// os.Remove via clearSeededEntries.
		seeded = append(seeded, entry)
		if err != nil {
			return seeded, fmt.Errorf("seed module cache for %s@%s: %w", mv.Path, mv.Version, err)
		}
	}

	return seeded, nil
}
```

Update the GoDoc just above the function (lines 27-40) to reflect the new return type:

```go
// seedModuleCache writes the four cache files (.info, .mod, .zip,
// .ziphash) for every in-plan released package into the developer's
// Go module cache, in the layout `go mod tidy` expects when running
// offline. Returns a slice of [seededEntry] tracking every file
// written; callers pass it to [clearSeededEntries] to remove the
// entries when done. The slice is populated even on error (whatever
// was written before the failure) so cleanup can still run.
//
// The cleanup is best-effort: per-entry remove failures are not
// returned, since cache entries are content-addressed and a stale
// leftover is inert (the next normal fetch overwrites it).
//
// Skips packages whose Path doesn't contain a go.mod (e.g. a
// pure-changelog package), and packages whose go.mod parse fails
// (those bubble out of the rewriter earlier).
```

- [ ] **Step 2: Add `clearSeededEntries` helper.**

Append to `internal/release/cacheseed.go` (after `seedModuleCache`, before `writeCacheEntry`):

```go
// clearSeededEntries removes every cache file in entries. Called by
// the orchestrator at the end of [tidySubmoduleGoSums] (and between
// iterations of the fixpoint loop). Best-effort: per-file remove
// failures are silently ignored, since cache entries are
// content-addressed and a stale leftover is inert.
func clearSeededEntries(entries []seededEntry) {
	for _, e := range entries {
		_ = os.Remove(e.infoPath)
		_ = os.Remove(e.modPath)
		_ = os.Remove(e.zipPath)
		_ = os.Remove(e.zipHashPath)
	}
}
```

- [ ] **Step 3: Update the single caller in `gosum.go`.**

Open `internal/release/gosum.go` and find the call (line 65-69):

```go
	// 4. Seed the cache with in-plan releases.
	cleanup, err := seedModuleCache(opts)
	defer cleanup()
	if err != nil {
		return err
	}
```

Replace with:

```go
	// 4. Seed the cache with in-plan releases.
	seeded, err := seedModuleCache(opts)
	defer clearSeededEntries(seeded)
	if err != nil {
		return err
	}
```

- [ ] **Step 4: Run the full repo test suite.**

Run: `cd /tmp/monorel-src && go test ./...`

Expected: PASS. The change is a refactor; behavior is unchanged.

- [ ] **Step 5: Commit.**

```bash
cd /tmp/monorel-src
git add internal/release/cacheseed.go internal/release/gosum.go
git commit -m "$(cat <<'EOF'
refactor(release): seedModuleCache returns slice instead of cleanup closure

Preparation for the fixpoint loop in tidySubmoduleGoSums. The loop
needs to clear and re-seed the cache across iterations; a cleanup
closure doesn't compose well with that. Return the []seededEntry
directly so the orchestrator can pass it to clearSeededEntries
explicitly, and clear-then-re-seed inside an iteration is a single
function-call pair rather than a closure dance.

Behavior unchanged. The single caller (tidySubmoduleGoSums) gets a
two-line update; no other call sites exist (verified via grep).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: add `readGoSums`, `goSumsChanged`, `stageAffected`, `errFixpointNotReached`

**Files:**
- Modify: `internal/release/gosum.go`

This task adds the helpers and error type the fixpoint loop will use. Loop replacement comes in Task 5.

- [ ] **Step 1: Append the helpers to `internal/release/gosum.go`.**

Add to the end of the file (after `managedImportPaths`):

```go
// readGoSums reads the go.sum bytes for every sub-module path in
// affected. Missing files are recorded as nil byte slices (a
// sub-module that hasn't yet been tidied may not have a go.sum
// file). Used by [tidySubmoduleGoSums]'s fixpoint loop to detect
// whether the most recent tidy iteration mutated any go.sum.
func readGoSums(repoDir string, affected []string) (map[string][]byte, error) {
	out := make(map[string][]byte, len(affected))
	for _, sub := range affected {
		path := filepath.Join(repoDir, sub, "go.sum")
		b, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				out[sub] = nil
				continue
			}
			return nil, fmt.Errorf("readGoSums: %s: %w", path, err)
		}
		out[sub] = b
	}
	return out, nil
}

// goSumsChanged reports whether before and after differ on any key.
// Caller passes snapshots from [readGoSums]; both maps must have the
// same key set.
func goSumsChanged(before, after map[string][]byte) bool {
	for k, b := range before {
		if !bytes.Equal(b, after[k]) {
			return true
		}
	}
	return false
}

// stageAffected stages each affected sub-module's go.mod and go.sum
// in the repo. Idempotent: missing files are skipped; non-Stat
// failures bubble up. Used by [tidySubmoduleGoSums] after the
// fixpoint loop converges.
func stageAffected(opts Options, affected []string) error {
	for _, sub := range affected {
		for _, name := range []string{"go.mod", "go.sum"} {
			rel := filepath.Join(sub, name)
			abs := filepath.Join(opts.RepoDir, rel)
			if _, err := os.Stat(abs); os.IsNotExist(err) {
				continue
			} else if err != nil {
				return fmt.Errorf("tidy: stat %s: %w", abs, err)
			}
			if err := opts.Repo.Add(rel); err != nil {
				return fmt.Errorf("tidy: stage %s: %w", rel, err)
			}
		}
	}
	return nil
}

// errFixpointNotReached is returned by [tidySubmoduleGoSums] when
// the seed-and-tidy loop fails to converge within maxTidyIterations.
// The error carries the per-iteration go.sum diffs as a diagnostic
// payload so the maintainer can see exactly which sub-module's
// go.sum kept changing across iterations. This typically indicates
// a monorel bug (cycle, non-determinism); the message hints the
// maintainer to file an issue.
type errFixpointNotReached struct {
	iterations int
	finalDiffs map[string][2][]byte // per sub-module: [0]=before, [1]=after for the last iteration
}

func (e *errFixpointNotReached) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "tidy did not converge after %d iterations; this indicates a monorel bug.\n",
		e.iterations)
	fmt.Fprintf(&b, "Please file an issue at https://github.com/disaresta-org/monorel/issues with the following diffs:\n\n")
	for sub, ba := range e.finalDiffs {
		if bytes.Equal(ba[0], ba[1]) {
			continue
		}
		fmt.Fprintf(&b, "==> %s/go.sum (last iteration's diff):\n", sub)
		fmt.Fprintf(&b, "BEFORE:\n%s\n", ba[0])
		fmt.Fprintf(&b, "AFTER:\n%s\n", ba[1])
	}
	return b.String()
}
```

Add `bytes` to the import block at the top of `gosum.go`. The current imports are:

```go
import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/mod/module"
)
```

Replace with:

```go
import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/mod/module"
)
```

- [ ] **Step 2: Run the full repo test suite.**

Run: `cd /tmp/monorel-src && go test ./...`

Expected: PASS. The helpers aren't called yet but they compile cleanly; existing tests are unaffected.

- [ ] **Step 3: Commit.**

```bash
cd /tmp/monorel-src
git add internal/release/gosum.go
git commit -m "$(cat <<'EOF'
refactor(release): add helpers for tidy fixpoint loop

Adds readGoSums, goSumsChanged, stageAffected, and the
errFixpointNotReached error type. None of them is called yet; the
fixpoint loop in the next commit wires them up.

Helpers extracted now (rather than inlined inside the loop) so the
loop body stays focused on orchestration and each helper has its own
test surface in the gosum_test.go suite.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: replace `tidySubmoduleGoSums` body with iterate-to-fixpoint

**Files:**
- Modify: `internal/release/gosum.go`

- [ ] **Step 1: Replace the seed/tidy/stage section with the fixpoint loop.**

Open `internal/release/gosum.go` and find `tidySubmoduleGoSums`. The current shape after Task 3 is:

```go
func tidySubmoduleGoSums(opts Options) error {
	if opts.PreState != nil {
		return nil
	}
	if opts.Plan == nil || len(opts.Plan.Releases) == 0 {
		return nil
	}

	// 1. Build the in-plan module set so we can detect sibling
	//    requires in each sub-module's go.mod.
	inPlan, err := inPlanSiblings(opts)
	if err != nil {
		return err
	}

	// 2. Determine which sub-modules to tidy (skip ones with no
	//    in-plan sibling requires; nothing for tidy to add).
	affected, err := affectedSubmodules(opts, inPlan)
	if err != nil {
		return err
	}
	if len(affected) == 0 {
		return nil
	}

	// 3. Pre-flight: confirm out-of-plan managed siblings used by
	//    the affected sub-modules are in the developer's cache.
	if err := preflightOutOfPlanCache(opts, affected, inPlan); err != nil {
		return err
	}

	// 4. Seed the cache with in-plan releases.
	seeded, err := seedModuleCache(opts)
	defer clearSeededEntries(seeded)
	if err != nil {
		return err
	}

	// 5. Run tidy in each affected sub-module. Hard-fail on the
	//    first failure; staging happens only after all succeed.
	for _, sub := range affected {
		modDir := filepath.Join(opts.RepoDir, sub)
		if err := runOfflineTidy(modDir); err != nil {
			return err
		}
	}

	// 6. Stage every (potentially) modified go.mod / go.sum.
	for _, sub := range affected {
		for _, name := range []string{"go.mod", "go.sum"} {
			rel := filepath.Join(sub, name)
			abs := filepath.Join(opts.RepoDir, rel)
			if _, err := os.Stat(abs); os.IsNotExist(err) {
				continue
			} else if err != nil {
				return fmt.Errorf("tidy: stat %s: %w", abs, err)
			}
			if err := opts.Repo.Add(rel); err != nil {
				return fmt.Errorf("tidy: stage %s: %w", rel, err)
			}
		}
	}
	return nil
}
```

Replace steps 4-6 (everything from the seed call through the return) with the fixpoint loop:

```go
	// 4. Iterate seed-and-tidy to fixpoint. Each iteration zips
	//    every in-plan module's working tree, seeds the cache with
	//    the resulting hashes, and runs offline tidy in each
	//    affected sub-module. If any sub-module's go.sum was
	//    mutated by the iteration's tidy, the working tree changed
	//    too, so the next iteration re-seeds and re-tidies. The
	//    loop converges when no go.sum is mutated; at that point
	//    every recorded h1: matches the seeded hash, which matches
	//    the working-tree hash, which is what `git archive` of the
	//    published tag will produce.
	//
	//    Iteration cap defends against cycles/non-determinism. The
	//    practical bound is the depth of the in-plan sibling dep
	//    graph; a typical monorepo converges in 1-3 iterations.
	const maxTidyIterations = 10
	var seeded []seededEntry
	defer func() { clearSeededEntries(seeded) }()

	var lastDiffs map[string][2][]byte
	for i := 0; i < maxTidyIterations; i++ {
		clearSeededEntries(seeded)
		seeded, err = seedModuleCache(opts)
		if err != nil {
			return err
		}

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
			// Fixpoint reached.
			return stageAffected(opts, affected)
		}

		// Capture the diff for diagnostic in case fixpoint never
		// converges. Overwritten each iteration; we only emit the
		// final iteration's diff if we exit via the cap.
		lastDiffs = make(map[string][2][]byte, len(before))
		for k := range before {
			lastDiffs[k] = [2][]byte{before[k], after[k]}
		}
	}

	return &errFixpointNotReached{
		iterations: maxTidyIterations,
		finalDiffs: lastDiffs,
	}
}
```

- [ ] **Step 2: Run the existing tests.**

Run: `cd /tmp/monorel-src && go test ./internal/release -v`

Expected: PASS for every existing test. The fixpoint loop converges in 1 iteration when no cross-sibling dep mutation cascades, which is what every existing test fixture exercises. The new regression test from Task 1 also continues to pass (Task 2's reorder fixed the canonical case; the loop adds the cross-sibling case for free).

- [ ] **Step 3: Run the full repo test suite.**

Run: `cd /tmp/monorel-src && go test ./...`

Expected: PASS.

- [ ] **Step 4: Commit.**

```bash
cd /tmp/monorel-src
git add internal/release/gosum.go
git commit -m "$(cat <<'EOF'
fix(release): iterate seed-and-tidy to fixpoint

Replaces the single-pass seed-and-tidy in tidySubmoduleGoSums with
an iterate-to-fixpoint loop. Each iteration re-seeds in-plan modules
from the current working tree (so any go.sum mutations from the
previous iteration's tidy are reflected in the seeded hash), then
re-runs tidy. Convergence: when no affected sub-module's go.sum
changed during the iteration, every recorded h1: matches what
`git archive` of the published tag will produce.

Closes the cross-sibling cascade variant of the wrong-h1: bug. The
canonical changeset-deletion variant was fixed by the previous
commit (applyStable reorder); this commit handles cases where a
sub-module's own tidy mutates its own go.sum and another in-plan
sibling depends on it.

Iteration cap: 10. Practical bound is dep-graph depth; typical
monorepos converge in 1-3. Cap defends against cycles or
non-determinism with a clear errFixpointNotReached error.

Existing tests pass unchanged (they all converge in 1 iteration).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: cross-sibling cascade regression test

**Files:**
- Modify: `internal/release/gosum_test.go`

This task adds a test for the second variant of the wrong-h1: bug: an in-plan sub-module that other in-plan siblings depend on. Single-pass tidy would record the pre-tidy hash for the dependent sibling; the fixpoint loop converges to the post-tidy hash.

- [ ] **Step 1: Append the test to `internal/release/gosum_test.go`.**

```go
// TestApply_CrossSiblingCascade_HashesConvergeToPublished pins the
// regression for the cross-sibling cascade variant of the wrong-h1:
// bug. Three in-plan modules A, B, C: A is the root; B requires A
// and is required by C; C requires both A and B. Single-pass
// seed-and-tidy would record B's pre-tidy hash in C's go.sum, since
// B's own tidy modifies B's go.sum after C's tidy ran against the
// seeded (pre-modification) hash.
//
// The fixpoint loop converges: every recorded h1: equals the hash
// of `git archive` against the published commit.
func TestApply_CrossSiblingCascade_HashesConvergeToPublished(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skipf("`go` not on PATH: %v", err)
	}

	repoDir := t.TempDir()
	tmpModCache := t.TempDir()
	t.Cleanup(func() {
		filepath.WalkDir(tmpModCache, func(path string, _ fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			os.Chmod(path, 0o755)
			return nil
		})
	})
	t.Setenv("GOMODCACHE", tmpModCache)

	// Modules:
	//   a/  (example.com/a, no deps)
	//   b/  (example.com/b, requires a)
	//   c/  (example.com/c, requires a AND b)
	mustWriteFile(t, filepath.Join(repoDir, "a/go.mod"),
		"module example.com/a\n\ngo 1.26\n")
	mustWriteFile(t, filepath.Join(repoDir, "a/a.go"),
		"package a\n\nfunc Hello() string { return \"a\" }\n")

	mustWriteFile(t, filepath.Join(repoDir, "b/go.mod"),
		"module example.com/b\n\ngo 1.26\n\nrequire example.com/a v0.1.0\n")
	mustWriteFile(t, filepath.Join(repoDir, "b/b.go"),
		"package b\n\nimport \"example.com/a\"\n\nfunc Hello() string { return a.Hello() + \"-b\" }\n")

	mustWriteFile(t, filepath.Join(repoDir, "c/go.mod"),
		"module example.com/c\n\ngo 1.26\n\nrequire (\n\texample.com/a v0.1.0\n\texample.com/b v0.1.0\n)\n")
	mustWriteFile(t, filepath.Join(repoDir, "c/c.go"),
		"package c\n\nimport (\n\t\"example.com/a\"\n\t\"example.com/b\"\n)\n\nfunc Hello() string { return a.Hello() + \"-\" + b.Hello() }\n")

	repo := git.NewFake()
	cfg := &config.Config{
		Packages: map[string]config.PackageConfig{
			"a": {TagPrefix: "a", Path: "a"},
			"b": {TagPrefix: "b", Path: "b"},
			"c": {TagPrefix: "c", Path: "c"},
		},
	}
	rp := &plan.ReleasePlan{
		Releases: []plan.PackageRelease{
			{Config: cfg.Packages["a"], Tag: "a/v0.1.0", Bump: semver.Minor},
			{Config: cfg.Packages["b"], Tag: "b/v0.1.0", Bump: semver.Minor},
			{Config: cfg.Packages["c"], Tag: "c/v0.1.0", Bump: semver.Minor},
		},
	}

	opts := Options{
		Plan:         rp,
		Config:       cfg,
		Repo:         repo,
		RepoDir:      repoDir,
		ChangesetDir: filepath.Join(repoDir, ".changeset"),
		Today:        "2026-05-03",
	}

	if _, err := Apply(opts); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	// Compute expected hashes after Apply has staged everything
	// (including b's go.sum mutations and c's go.sum mutations).
	// `dirhash` over each module's directory matches what
	// `git archive` of the eventual tag will produce.
	expectedA, err := dirhashOfModule(repoDir, "a", "example.com/a", "v0.1.0")
	if err != nil {
		t.Fatalf("hash a: %v", err)
	}
	expectedB, err := dirhashOfModule(repoDir, "b", "example.com/b", "v0.1.0")
	if err != nil {
		t.Fatalf("hash b: %v", err)
	}

	// c/go.sum should record A's actual hash AND B's actual hash.
	cGoSum, err := os.ReadFile(filepath.Join(repoDir, "c", "go.sum"))
	if err != nil {
		t.Fatalf("read c/go.sum: %v", err)
	}
	if got := extractH1(t, cGoSum, "example.com/a v0.1.0"); got != expectedA {
		t.Errorf("c/go.sum recorded wrong h1: for example.com/a v0.1.0\n  got:  %s\n  want: %s\n  full go.sum:\n%s",
			got, expectedA, cGoSum)
	}
	if got := extractH1(t, cGoSum, "example.com/b v0.1.0"); got != expectedB {
		t.Errorf("c/go.sum recorded wrong h1: for example.com/b v0.1.0\n  got:  %s\n  want: %s\n  full go.sum:\n%s",
			got, expectedB, cGoSum)
	}

	// b/go.sum should record A's actual hash.
	bGoSum, err := os.ReadFile(filepath.Join(repoDir, "b", "go.sum"))
	if err != nil {
		t.Fatalf("read b/go.sum: %v", err)
	}
	if got := extractH1(t, bGoSum, "example.com/a v0.1.0"); got != expectedA {
		t.Errorf("b/go.sum recorded wrong h1: for example.com/a v0.1.0\n  got:  %s\n  want: %s",
			got, expectedA)
	}
}

// dirhashOfModule computes Hash1 of a sub-module's zip built from
// repoDir/sub. Like dirhashOfRoot but for an arbitrary sub-module.
func dirhashOfModule(repoDir, sub, importPath, version string) (string, error) {
	mv := module.Version{Path: importPath, Version: version}
	tmpZip, err := os.CreateTemp("", "monorel-test-zip-*.zip")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmpZip.Name())
	defer tmpZip.Close()
	if err := xzip.CreateFromDir(tmpZip, mv, filepath.Join(repoDir, sub)); err != nil {
		return "", err
	}
	if err := tmpZip.Close(); err != nil {
		return "", err
	}
	return dirhash.HashZip(tmpZip.Name(), dirhash.Hash1)
}
```

- [ ] **Step 2: Run the new test.**

Run: `cd /tmp/monorel-src && go test ./internal/release -run TestApply_CrossSiblingCascade_HashesConvergeToPublished -v`

Expected: PASS. The fixpoint loop from Task 5 handles the cascade.

- [ ] **Step 3: Run the full repo test suite.**

Run: `cd /tmp/monorel-src && go test ./...`

Expected: PASS.

- [ ] **Step 4: Commit.**

```bash
cd /tmp/monorel-src
git add internal/release/gosum_test.go
git commit -m "$(cat <<'EOF'
test(release): regression for cross-sibling cascade variant

Three-module fixture: a (root), b (requires a), c (requires a and
b). Without the fixpoint loop, c/go.sum would record b's pre-tidy
hash (since b's own tidy mutates b/go.sum after c's tidy ran with
the seeded pre-tidy hash). With the loop, every recorded h1: equals
the hash of `git archive` against the published commit.

This complements Task 1's regression test: that test covers the
canonical "root .changeset deletion" variant; this one covers the
cross-sibling cascade variant. Both are needed because the fix has
two parts: applyStable reorder (canonical) + fixpoint loop
(cascade).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: fixpoint-not-reached diagnostic test

**Files:**
- Modify: `internal/release/gosum_test.go`
- Modify: `internal/release/gosum.go` (add a test hook)

The fixpoint-not-reached error is hard to trigger with real tidy because monorel's tidy is deterministic. To test the diagnostic path, expose a small test hook that lets the test inject non-determinism.

- [ ] **Step 1: Add the hook to `gosum.go`.**

Append to `internal/release/gosum.go` after the helpers added in Task 4:

```go
// tidyHook is a per-iteration test hook used by the fixpoint loop
// to inject non-determinism (or any other behavior) for testing
// errFixpointNotReached. Production code never sets it; the only
// caller is the gosum_test.go suite. nil means "no-op."
var tidyHook func(iteration int) error
```

Then modify the fixpoint loop in `tidySubmoduleGoSums` to call the hook. The current loop body is:

```go
	for i := 0; i < maxTidyIterations; i++ {
		clearSeededEntries(seeded)
		seeded, err = seedModuleCache(opts)
		if err != nil {
			return err
		}

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
        // ...
```

After the inner `runOfflineTidy` loop and before the `after` snapshot, add:

```go
		if tidyHook != nil {
			if err := tidyHook(i); err != nil {
				return err
			}
		}
```

So the loop becomes:

```go
	for i := 0; i < maxTidyIterations; i++ {
		clearSeededEntries(seeded)
		seeded, err = seedModuleCache(opts)
		if err != nil {
			return err
		}

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

		if tidyHook != nil {
			if err := tidyHook(i); err != nil {
				return err
			}
		}

		after, err := readGoSums(opts.RepoDir, affected)
		if err != nil {
			return err
		}

		if !goSumsChanged(before, after) {
			return stageAffected(opts, affected)
		}

		lastDiffs = make(map[string][2][]byte, len(before))
		for k := range before {
			lastDiffs[k] = [2][]byte{before[k], after[k]}
		}
	}
```

- [ ] **Step 2: Add the test.**

Append to `internal/release/gosum_test.go`:

```go
// TestTidySubmoduleGoSums_FixpointNotReached_SurfacesDiagnosticError
// pins the diagnostic path. Inject a hook that mutates an affected
// sub-module's go.sum on every iteration so the loop never
// converges; assert it returns errFixpointNotReached after
// maxTidyIterations and the message includes the per-iteration
// diff payload.
func TestTidySubmoduleGoSums_FixpointNotReached_SurfacesDiagnosticError(t *testing.T) {
	opts, _ := setupSubmoduleFixture(t, true /* aRequiresB */)

	// Inject non-determinism: append a unique line to a/go.sum
	// after each tidy iteration. The next iteration's "before"
	// snapshot will see the appended line; tidy may or may not
	// remove it (depending on whether it parses as a valid sum
	// line); either way, before != after, so the loop never
	// converges.
	originalHook := tidyHook
	t.Cleanup(func() { tidyHook = originalHook })
	tidyHook = func(iteration int) error {
		path := filepath.Join(opts.RepoDir, "a", "go.sum")
		f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = fmt.Fprintf(f, "# fixpoint-test marker iteration=%d\n", iteration)
		return err
	}

	err := tidySubmoduleGoSums(opts)
	if err == nil {
		t.Fatal("tidySubmoduleGoSums: want errFixpointNotReached, got nil")
	}
	var fpErr *errFixpointNotReached
	if !errors.As(err, &fpErr) {
		t.Fatalf("tidySubmoduleGoSums: want *errFixpointNotReached, got %T: %v", err, err)
	}
	if fpErr.iterations != 10 {
		t.Errorf("iterations: got %d, want 10", fpErr.iterations)
	}
	msg := fpErr.Error()
	if !strings.Contains(msg, "did not converge after 10 iterations") {
		t.Errorf("error message missing convergence header: %q", msg)
	}
	if !strings.Contains(msg, "monorel/issues") {
		t.Errorf("error message missing issue-filing hint: %q", msg)
	}
	if !strings.Contains(msg, "BEFORE:") || !strings.Contains(msg, "AFTER:") {
		t.Errorf("error message missing diff payload: %q", msg)
	}
}
```

Add the `errors` import to `gosum_test.go` if not already present.

- [ ] **Step 3: Run the new test.**

Run: `cd /tmp/monorel-src && go test ./internal/release -run TestTidySubmoduleGoSums_FixpointNotReached_SurfacesDiagnosticError -v`

Expected: PASS.

- [ ] **Step 4: Run the full repo test suite.**

Run: `cd /tmp/monorel-src && go test ./...`

Expected: PASS.

- [ ] **Step 5: Commit.**

```bash
cd /tmp/monorel-src
git add internal/release/gosum.go internal/release/gosum_test.go
git commit -m "$(cat <<'EOF'
test(release): pin errFixpointNotReached diagnostic path

Adds a test-only tidyHook that lets the test inject non-determinism
(by appending unique markers to an affected sub-module's go.sum each
iteration). Confirms that tidySubmoduleGoSums returns
*errFixpointNotReached with the iteration count, the issue-filing
hint, and the per-iteration diff payload.

Production code never sets the hook (it's nil by default and only
mutated from gosum_test.go).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 8: add `primeModuleCache` and `primeCacheEnv`

**Files:**
- Modify: `internal/release/tidy.go`

This task adds the cache-priming helper but doesn't wire it into `tidySubmoduleGoSums` yet. Wiring is Task 9.

- [ ] **Step 1: Append `primeModuleCache` and `primeCacheEnv` to `tidy.go`.**

Open `internal/release/tidy.go`. After the existing `offlineTidyEnv` (line 81), append:

```go
// primeModuleCache populates the local module cache with the
// third-party deps modDir's go.mod transitively requires. Subsequent
// offline tidy with GOPROXY=off can resolve those deps from the
// cache without reaching out to the network.
//
// Uses the inherited GOPROXY (typically https://proxy.golang.org,direct)
// and GOSUMDB so go.sum hashes are verified during download. Does
// NOT mutate go.sum: `go mod download` reads go.sum, downloads the
// listed modules, and writes nothing. The download is bounded by the
// existing entries in go.sum: any module not already pinned wouldn't
// be downloaded (tidy adds pins later, after the in-plan siblings
// are seeded).
//
// PATH, HOME, USER, TMPDIR, LANG, LC_*, GOMODCACHE, GOCACHE, GOPROXY,
// GOSUMDB pass through.
func primeModuleCache(modDir string) error {
	cmd := exec.Command("go", "mod", "download")
	cmd.Dir = modDir
	cmd.Env = primeCacheEnv()

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("prime module cache in %s: %w\n%s\n\n"+
			"Hint: this typically means GOPROXY isn't reachable from this environment. "+
			"Confirm `GOPROXY` is a real proxy URL (e.g. https://proxy.golang.org,direct), "+
			"or set `GOPROXY=direct` to fetch straight from the source repo",
			modDir, err, out)
	}
	return nil
}

// primeCacheEnv builds the env slice for the prime-cache subprocess.
// Mirrors offlineTidyEnv's "scratch env, no caller GOFLAGS leak"
// shape, but inherits GOPROXY and GOSUMDB so download can fetch from
// the network.
func primeCacheEnv() []string {
	inherit := []string{
		"PATH",
		"HOME",
		"USER",
		"TMPDIR",
		"LANG",
		"GOMODCACHE",
		"GOCACHE",
		"GOPROXY",
		"GOSUMDB",
	}
	env := make([]string, 0, len(inherit)+8)
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
		"GOWORK=off",
		"GOFLAGS=",
		"GOTOOLCHAIN=local",
	)
	return env
}
```

- [ ] **Step 2: Run the full repo test suite.**

Run: `cd /tmp/monorel-src && go test ./...`

Expected: PASS. The new helper isn't called yet; existing tests are unaffected.

- [ ] **Step 3: Commit.**

```bash
cd /tmp/monorel-src
git add internal/release/tidy.go
git commit -m "$(cat <<'EOF'
feat(release): add primeModuleCache helper

monorel's offline tidy runs with GOPROXY=off, which means it can
only resolve modules already in the local cache. In-plan siblings
are seeded explicitly; managed-but-not-in-plan siblings are
pre-flight-checked. Third-party deps (anything not in
monorel.toml) must already be in GOMODCACHE from prior dev work.

On a fresh CI runner, GOMODCACHE is empty and tidy fails with
"module lookup disabled by GOPROXY=off." Add a primeModuleCache
helper that runs `go mod download` (with the inherited GOPROXY)
to populate the cache before the offline tidy runs. The
GOPROXY=off-during-tidy invariant is preserved.

Helper not called yet; the next commit wires it into
tidySubmoduleGoSums.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 9: wire `primeModuleCache` into `tidySubmoduleGoSums`

**Files:**
- Modify: `internal/release/gosum.go`

- [ ] **Step 1: Add the prime-cache step before the seed.**

Open `internal/release/gosum.go` and find `tidySubmoduleGoSums`. After the pre-flight check (the `preflightOutOfPlanCache` call, around line 60-62) and before the fixpoint loop's `var seeded []seededEntry` declaration, insert:

```go
	// 4. Prime the module cache with third-party deps each affected
	//    sub-module transitively requires. Tidy under GOPROXY=off
	//    can only resolve from the cache; managed siblings are
	//    seeded by the loop below, but third-party deps depend on
	//    the cache being warm. Dev cache is usually warm from prior
	//    builds; fresh CI runner cache isn't. Populate the cache
	//    explicitly so the offline tidy that follows resolves
	//    every transitive dep from the cache.
	for _, sub := range affected {
		modDir := filepath.Join(opts.RepoDir, sub)
		if err := primeModuleCache(modDir); err != nil {
			return err
		}
	}

```

The new call slots between step 3 (`preflightOutOfPlanCache`) and step 4 (the fixpoint loop). Renumber the comment markers in the function: the existing "// 4." comment on the seed-and-tidy loop becomes "// 5.".

- [ ] **Step 2: Run the existing tests.**

Run: `cd /tmp/monorel-src && go test ./internal/release -v`

Expected: PASS. The existing tests use a fixture where third-party deps aren't required, so `go mod download` is a no-op for them. (If any existing test fails at this step because its fixture *does* implicitly depend on a third-party module that isn't cached, that test was already running with a pre-warmed dev cache; we'll catch and address it here.)

- [ ] **Step 3: Run the full repo test suite.**

Run: `cd /tmp/monorel-src && go test ./...`

Expected: PASS.

- [ ] **Step 4: Commit.**

```bash
cd /tmp/monorel-src
git add internal/release/gosum.go
git commit -m "$(cat <<'EOF'
fix(release): prime module cache before offline tidy

Calls primeModuleCache for each affected sub-module before the
seed-and-tidy fixpoint loop. Third-party deps land in GOMODCACHE
via `go mod download` with the inherited GOPROXY, so the offline
tidy that follows (with GOPROXY=off) can resolve them from the
cache.

Fixes "module lookup disabled by GOPROXY=off" failures on fresh CI
runners.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 10: cold-cache regression test for cache priming

**Files:**
- Modify: `internal/release/gosum_test.go`

- [ ] **Step 1: Append the test.**

```go
// TestTidySubmoduleGoSums_PrimesThirdPartyDeps_FromColdCache pins
// the regression for the fresh-CI-runner case. An affected
// sub-module requires a third-party module not in monorel.toml's
// managed set. With a fresh GOMODCACHE (default for
// setupSubmoduleFixture), offline tidy with GOPROXY=off would fail
// without the prime-cache step. With it, tidy succeeds because the
// cache is populated before tidy runs.
func TestTidySubmoduleGoSums_PrimesThirdPartyDeps_FromColdCache(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skipf("`go` not on PATH: %v", err)
	}
	if testing.Short() {
		t.Skip("network-bound: requires GOPROXY reachability for go mod download")
	}

	repoDir := t.TempDir()
	tmpModCache := t.TempDir()
	t.Cleanup(func() {
		filepath.WalkDir(tmpModCache, func(path string, _ fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			os.Chmod(path, 0o755)
			return nil
		})
	})
	t.Setenv("GOMODCACHE", tmpModCache)

	// a/ requires example.com/b (in-plan, will be seeded) AND
	// gopkg.in/yaml.v3 (third-party). Without primeModuleCache,
	// tidy under GOPROXY=off would fail on yaml.v3 since it's not
	// in the fresh GOMODCACHE.
	mustWriteFile(t, filepath.Join(repoDir, "a/go.mod"),
		"module example.com/a\n\ngo 1.26\n\nrequire (\n\texample.com/b v0.1.0\n\tgopkg.in/yaml.v3 v3.0.1\n)\n")
	mustWriteFile(t, filepath.Join(repoDir, "a/a.go"),
		"package a\n\nimport (\n\t\"example.com/b\"\n\t_ \"gopkg.in/yaml.v3\"\n)\n\nfunc Greet() string { return b.Hello() }\n")
	// Pre-populate a's go.sum with the yaml.v3 hash so tidy doesn't
	// have to write it (which would require sum verification). The
	// hash is the public proxy hash for yaml.v3 v3.0.1.
	mustWriteFile(t, filepath.Join(repoDir, "a/go.sum"),
		"gopkg.in/yaml.v3 v3.0.1 h1:fxVm/GzAzEWqLHuvctI91KS9hhNmmWOoWu0XTYJS7CA=\n"+
			"gopkg.in/yaml.v3 v3.0.1/go.mod h1:K4uyk7z7BCEPqu6E+C64Yfv1cQ7kz7rIZviUmN+EgEM=\n")

	mustWriteFile(t, filepath.Join(repoDir, "b/go.mod"),
		"module example.com/b\n\ngo 1.26\n")
	mustWriteFile(t, filepath.Join(repoDir, "b/b.go"),
		"package b\n\nfunc Hello() string { return \"hi\" }\n")

	repo := git.NewFake()
	cfg := &config.Config{
		Packages: map[string]config.PackageConfig{
			"a": {TagPrefix: "a", Path: "a"},
			"b": {TagPrefix: "b", Path: "b"},
		},
	}
	rp := &plan.ReleasePlan{
		Releases: []plan.PackageRelease{
			{Config: cfg.Packages["a"], Tag: "a/v0.1.0", Bump: semver.Minor},
			{Config: cfg.Packages["b"], Tag: "b/v0.1.0", Bump: semver.Minor},
		},
	}

	opts := Options{
		Plan:         rp,
		Config:       cfg,
		Repo:         repo,
		RepoDir:      repoDir,
		ChangesetDir: filepath.Join(repoDir, ".changeset"),
		Today:        "2026-05-03",
	}

	if err := tidySubmoduleGoSums(opts); err != nil {
		t.Fatalf("tidySubmoduleGoSums: %v", err)
	}

	// Sanity: a/go.sum now has both example.com/b and yaml.v3 entries.
	aGoSum, err := os.ReadFile(filepath.Join(repoDir, "a", "go.sum"))
	if err != nil {
		t.Fatalf("read a/go.sum: %v", err)
	}
	if !bytes.Contains(aGoSum, []byte("example.com/b v0.1.0")) {
		t.Errorf("a/go.sum: missing example.com/b v0.1.0:\n%s", aGoSum)
	}
	if !bytes.Contains(aGoSum, []byte("gopkg.in/yaml.v3 v3.0.1")) {
		t.Errorf("a/go.sum: missing gopkg.in/yaml.v3 v3.0.1:\n%s", aGoSum)
	}
}
```

- [ ] **Step 2: Run the test.**

Run: `cd /tmp/monorel-src && go test ./internal/release -run TestTidySubmoduleGoSums_PrimesThirdPartyDeps_FromColdCache -v`

Expected: PASS. The test downloads `gopkg.in/yaml.v3` from the public proxy via the prime-cache step, then offline tidy resolves it from the cache.

If the test runs in an environment without network access, the test skips via `testing.Short()` (run with `go test -short`). For the standard `go test`, network access is assumed.

- [ ] **Step 3: Run the full repo test suite.**

Run: `cd /tmp/monorel-src && go test ./...`

Expected: PASS.

- [ ] **Step 4: Commit.**

```bash
cd /tmp/monorel-src
git add internal/release/gosum_test.go
git commit -m "$(cat <<'EOF'
test(release): regression for fresh-CI-runner cold-cache case

A two-module fixture where a's go.mod requires both an in-plan
sibling (example.com/b) and a third-party module (gopkg.in/yaml.v3).
With a fresh GOMODCACHE and no priming, offline tidy under
GOPROXY=off would fail on yaml.v3.

This pins the regression so a future change to the cache-management
flow that drops the priming step would surface here rather than only
in real CI runs.

Skips under `go test -short` since the test requires GOPROXY
reachability.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 11: ci/github/README.md updates (concerns #3 and #4)

**Files:**
- Modify: `ci/github/README.md`

- [ ] **Step 1: Add inline comments above each `actions/setup-go@v5` step.**

Open `ci/github/README.md`. Find the first example workflow (`release-pr.yml`, around lines 26-49). The current setup-go step is:

```yaml
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
```

Replace with:

```yaml
      # monorel runs `go mod tidy` with GOTOOLCHAIN=local during release,
      # so Go must already be installed at a version satisfying every
      # released sub-module's `go` directive (the highest one wins).
      # `go-version-file: go.mod` reads the root module's go.mod; if a
      # sub-module declares a higher floor, pin `go-version` explicitly.
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
```

Apply the identical comment block to the second example workflow (`release.yml`, around lines 51-74).

- [ ] **Step 2: Strengthen the "Requirements" → "go on PATH" bullet.**

Find the bullet (around line 78). The current text is:

```markdown
- **`go` on `PATH`.** monorel's `apply` step runs `go mod tidy` (offline, against a seeded local cache) in every released sub-module that requires an in-plan sibling, so the release commit's `go.sum` is canonically clean for the proxy-published state. The runner needs a `go` binary whose version satisfies every released module's `go` directive; use `actions/setup-go@v5` with `go-version-file: go.mod` (or pin `go-version` explicitly) to install the right version. GitHub-hosted runners include a recent Go by default, but pinning is safer than relying on the runner's pre-installed version, especially when modules use a recent `go` directive.
```

Replace with:

```markdown
- **`go` on `PATH`.** monorel's `apply` step runs `go mod tidy` (offline, against a seeded local cache) in every released sub-module that requires an in-plan sibling, so the release commit's `go.sum` is canonically clean for the proxy-published state. The runner needs a `go` binary whose version satisfies every released module's `go` directive; use `actions/setup-go@v5` with `go-version-file: go.mod` (or pin `go-version` explicitly) to install the right version. GitHub-hosted runners include a recent Go by default, but pinning is safer than relying on the runner's pre-installed version, especially when modules use a recent `go` directive.

  If the runner's Go is older than the highest sub-module's `go` directive, tidy fails with `go.mod requires go >= X.Y; running Z; GOTOOLCHAIN=local`. The fix is always to bump the runner's Go (raise the `go-version`), not to remove `GOTOOLCHAIN=local` from monorel's tidy step (the env var is part of monorel's offline-tidy determinism guarantee).

  See [Avoiding the chore(release) CI race](../../docs/src/workflows.md#avoiding-the-chorerelease-ci-race) for the related skip-filter pattern other workflows on the same branch should apply.
```

- [ ] **Step 3: Add a "## Recipes" section.**

Append to the end of `ci/github/README.md`:

```markdown
## Recipes

### Skipping CI on chore(release) commits

The release commit `chore(release): ...` is created by the always-open release PR's merge. On the same push event, `release.yml` (using this action with `command: release`) creates and pushes per-package tags. Any *other* workflow that runs on the same push and resolves Go module versions will race the tag-push and may transiently fail with:

```
go: example.com/foo/v2: reading example.com/foo/go.mod at revision v2.1.0: unknown revision v2.1.0
```

To avoid the phantom failure, skip the workflow on `chore(release):` commits. The skip filter:

```yaml
jobs:
  test:
    if: github.event_name == 'pull_request' || !startsWith(github.event.head_commit.message, 'chore(release):')
    # ... rest of job ...
```

Apply the filter to every job (`test`, `staticcheck`, `govulncheck`, etc.) that runs `go mod tidy` or anything else that resolves the new versions. Pull-request triggers stay always-on; only push-to-main runs are skipped, and only when the head commit is the release-PR merge.

The `release.yml` workflow that runs the actual release pipeline does NOT need this filter; its own `if:` clause is the *opposite* shape (only run on `chore(release):` commits), so it's already mutually exclusive with the skip pattern above.

For non-GitHub-Actions CI systems, see the [universal recipe](../../docs/src/workflows.md#avoiding-the-chorerelease-ci-race) covering GitHub / GitLab / Gitea filter syntax.
```

- [ ] **Step 4: Build docs to confirm no link breaks.**

Run: `cd /tmp/monorel-src/docs && bun run docs:build 2>&1 | tail -5`

Expected: clean build. If links to `../../docs/src/workflows.md` 404 because that section doesn't exist yet, that's expected; Task 12 adds it. If the build fails for any other reason, debug.

For now, accept that the cross-link to `workflows.md` is a forward reference that resolves once Task 12 lands.

- [ ] **Step 5: Commit.**

```bash
cd /tmp/monorel-src
git add ci/github/README.md
git commit -m "$(cat <<'EOF'
docs(ci/github): inline comments on setup-go + Recipes section

Three additions to ci/github/README.md addressing concerns #3 and
#4 from issue #54:

- Inline comments above the `actions/setup-go@v5` step in both
  example workflows explaining the GOTOOLCHAIN=local rationale and
  the `go-version-file: go.mod` choice (so a casual reader doesn't
  strip the step).
- Strengthened "Requirements" -> "go on PATH" bullet calling out
  the failure mode and pointing at the universal recipe.
- New "## Recipes" section with the GitHub-Actions if-clause snippet
  for skipping CI on chore(release) commits, plus a forward link
  to docs/src/workflows.md for non-GitHub CI systems (added in the
  next commit).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 12: docs/src/workflows.md universal sections

**Files:**
- Modify: `docs/src/workflows.md`

- [ ] **Step 1: Add the two new sections at the end of `workflows.md`.**

Open `/tmp/monorel-src/docs/src/workflows.md`. Append (after the existing "## See also" or whatever the last section is):

```markdown
## CI environment requirements

Any CI system invoking `monorel release` (GitHub Actions, GitLab CI, Gitea Actions, CircleCI, Drone, self-hosted runners) must provide the following:

- **Go installed at a version compatible with every released sub-module's `go` directive.** monorel runs `go mod tidy` with `GOTOOLCHAIN=local` during release; the env var is intentional and part of the offline-tidy determinism guarantee. Auto-toolchain-download is blocked. Pin the runner's Go to the highest floor explicitly (or use `go-version-file: go.mod` if the root module's `go` directive matches the highest sub-module floor).
- **`GOPROXY` set to a real proxy or `direct`.** monorel's release pipeline includes a "prime cache" step that runs `go mod download` (with the inherited `GOPROXY`) to populate the local module cache for third-party deps. The offline tidy that follows uses `GOPROXY=off` regardless of this setting; only the priming step honors `GOPROXY`. If `GOPROXY` is empty or missing, `go mod download` falls back to its default (`https://proxy.golang.org,direct`), which is what most CI systems already provide implicitly.
- **Push permissions for tags.** The `release` command runs `git push --follow-tags` to publish the per-package tags created by `monorel release`. The runner's git config needs commit + push credentials.
- **Provider API token.** For the `pr` command (always-open release PR maintenance) and the `publish` step inside `release`, the provider API token needs `contents: write` and `pull-requests: write` (GitHub naming; equivalent on GitLab / Gitea).

For GitHub Actions, see [`ci/github/README.md`](../../ci/github/README.md) for the canonical workflow examples that satisfy these requirements.

For GitLab CI, the equivalent setup is a `before_script:` block that installs Go (e.g., via the `golang:1.25` image or a `gimme` install) and a `rules:` clause on each job (see "Avoiding the chore(release) CI race" below). The token requirement maps to `CI_JOB_TOKEN` for read-only access plus a project access token for push/release operations.

For Gitea Actions, the syntax matches GitHub Actions (Gitea Actions is API-compatible). Substitute `disaresta-org/monorel/ci/github@<version>` references with whatever path your Gitea instance uses for the same action.

## Avoiding the chore(release) CI race

The release commit `chore(release): ...` (created when the always-open release PR is merged) updates module `go.mod` files to require new in-plan sibling versions. The matching tags are created and pushed by the workflow running `monorel release` on the same push. Any *other* workflow that fires on the same push and resolves Go module versions will race the tag push and may transiently fail with:

```
go: example.com/foo/v2: reading example.com/foo/go.mod at revision v2.1.0: unknown revision v2.1.0
```

The release succeeds and the tags get pushed, but the racing workflow's red mark stays in the UI. The fix is to skip the racing workflow when the head commit subject begins with `chore(release):`. The principle is universal; the syntax varies per CI system.

### GitHub Actions / Gitea Actions

Same syntax: an `if:` clause on each job that runs Go module resolution.

```yaml
jobs:
  test:
    if: github.event_name == 'pull_request' || !startsWith(github.event.head_commit.message, 'chore(release):')
    # ... rest of job ...
```

See [`ci/github/README.md`](../../ci/github/README.md#skipping-ci-on-chorerelease-commits) for the full GitHub Actions snippet.

### GitLab CI

Use a `rules:` clause on each job:

```yaml
test:
  rules:
    - if: '$CI_PIPELINE_SOURCE == "merge_request_event"'
    - if: '$CI_COMMIT_TITLE =~ /^chore\(release\):/'
      when: never
    - when: on_success
  script:
    - go test ./...
```

The two-clause `rules` first allows merge-request runs through unconditionally, then explicitly drops `chore(release):` push runs on the default branch, then defaults to running.

### Other CI systems

The principle is universal: skip the workflow when the head commit subject starts with `chore(release):`. Most CI systems support a per-job filter on commit message; the exact key varies (`commit.message`, `CI_COMMIT_MESSAGE`, `BUILDKITE_MESSAGE`, etc.). Apply the same `^chore(release):` regex check.

The release-pipeline workflow itself (the one running `monorel release`) does NOT need this filter; its own filter is the *opposite* shape (only run on `chore(release):`), so it's mutually exclusive with the skip pattern above.
```

- [ ] **Step 2: Build docs to confirm clean.**

Run: `cd /tmp/monorel-src/docs && bun run docs:build 2>&1 | tail -5`

Expected: clean build. The cross-links between `ci/github/README.md` and `docs/src/workflows.md` should now both resolve.

- [ ] **Step 3: Run the full repo test suite to confirm nothing regressed (defensive: docs changes shouldn't affect tests, but a sanity check is cheap).**

Run: `cd /tmp/monorel-src && go test ./...`

Expected: PASS.

- [ ] **Step 4: Commit.**

```bash
cd /tmp/monorel-src
git add docs/src/workflows.md
git commit -m "$(cat <<'EOF'
docs(workflows): CI environment requirements + chore(release) recipe

Two new sections in docs/src/workflows.md, both CI-agnostic:

- "## CI environment requirements" lists the universal requirements
  for any CI invoking `monorel release` (Go version, GOPROXY, push
  permissions, provider API token). Cross-links to ci/github/README.md
  for the GitHub-specific implementation; gives GitLab CI / Gitea
  Actions guidance for the others.
- "## Avoiding the chore(release) CI race" describes the race and
  shows per-CI filter syntax for GitHub Actions, GitLab CI, and a
  generic principle for other systems.

Closes the loop on concerns #3 and #4 of issue #54: ci/github/README.md
covers GitHub Actions; this doc covers everyone else.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 13: CHANGELOG entry

**Files:**
- Modify: `CHANGELOG.md`

- [ ] **Step 1: Read the existing CHANGELOG to confirm the entry format.**

Open `/tmp/monorel-src/CHANGELOG.md` and look at the top entry. monorel's CHANGELOG follows Keep-a-Changelog format with sections like `### Added`, `### Changed`, `### Fixed`. Entries are appended above the most recent release.

- [ ] **Step 2: Add the entry.**

Find the heading for the current pre-release "Unreleased" section (or create one if it doesn't exist; monorel's `monorel release` command writes this). Add bullets covering the four concerns:

```markdown
## [Unreleased]

### Fixed

- `release/cacheseed`: write the correct `h1:` hash for in-plan modules. Previously, the seed step zipped the working tree before consumed `.changeset/*.md` files were deleted; the resulting hash didn't match what `git archive` of the published tag produced, breaking every fresh-cache install with `SECURITY ERROR: checksum mismatch`. The fix reorders `applyStable` so the deletion happens before the seed, and replaces the single-pass seed-and-tidy with iterate-to-fixpoint to handle the cross-sibling cascade variant of the same class of bug. Surfaced by [`loglayer/loglayer-go#76` and follow-ups](https://github.com/loglayer/loglayer-go/pull/76); see [issue #54](https://github.com/disaresta-org/monorel/issues/54).
- `release/tidy`: prime the module cache for third-party deps before offline tidy runs. Fresh CI runners with empty `GOMODCACHE` previously failed with `module lookup disabled by GOPROXY=off`; the new `primeModuleCache` step runs `go mod download` (with the inherited `GOPROXY`) before tidy.

### Documentation

- `ci/github/README.md`: inline comments on `actions/setup-go@v5` in both example workflows explaining the `GOTOOLCHAIN=local` rationale and the `go-version-file: go.mod` choice. New "## Recipes → Skipping CI on chore(release) commits" section with the if-clause snippet for consumer workflows on the same push.
- `docs/src/workflows.md`: new CI-agnostic sections "CI environment requirements" (universal version of the prerequisite story) and "Avoiding the chore(release) CI race" (filter syntax for GitHub Actions, GitLab CI, Gitea Actions, and a generic principle for others).
```

If `CHANGELOG.md` doesn't currently have an `## [Unreleased]` heading, add it above the most recent versioned heading.

- [ ] **Step 3: Build docs (CHANGELOG is sometimes embedded in the docs site).**

Run: `cd /tmp/monorel-src/docs && bun run docs:build 2>&1 | tail -5`

Expected: clean build.

- [ ] **Step 4: Commit.**

```bash
cd /tmp/monorel-src
git add CHANGELOG.md
git commit -m "$(cat <<'EOF'
docs(changelog): entry for issue #54 four-concern fix package

Three bullets covering the cacheseed wrong-hash fix, the cache
priming fix, and the docs additions for toolchain prerequisites
and the chore(release) CI race.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 14: changeset entry

**Files:**
- Create: `.changeset/cacheseed-fixpoint-and-pipeline-fixes.md`

- [ ] **Step 1: Write the changeset.**

Create `/tmp/monorel-src/.changeset/cacheseed-fixpoint-and-pipeline-fixes.md`:

```markdown
---
"monorel.disaresta.com": minor
---

Fix `cacheseed` writing the wrong h1: hash for released sub-modules
(would silently produce broken go.sum entries on every release; see
[`loglayer/loglayer-go`'s v2.1.0 incident](https://github.com/loglayer/loglayer-go/pull/76)).
Reorder `applyStable` so all working-tree mutations happen before
the seed step, and replace the single-pass seed-and-tidy with
iterate-to-fixpoint to handle cross-sibling dep chains.

Add a `go mod download` priming step before offline tidy so fresh
CI runners (with empty `GOMODCACHE`) can resolve third-party deps.
The `GOPROXY=off` invariant during tidy is preserved.

Document the `actions/setup-go` prerequisite (sub-modules with
`go 1.25.0` directives need a 1.25+ runner since `GOTOOLCHAIN=local`
during tidy blocks auto-download) and the `chore(release):`-commit
skip filter recipe. See [issue #54](https://github.com/disaresta-org/monorel/issues/54).
```

- [ ] **Step 2: Run `monorel preview` (if installed) to confirm the changeset parses.**

Run: `cd /tmp/monorel-src && go run . preview --upsert=false 2>&1 | tail -10`

Expected: monorel's preview command prints the rendered plan including the new changeset. If `go run . preview` errors, check that the changeset frontmatter package key (`monorel.disaresta.com`) matches what's in `monorel.toml`'s `[packages]` table.

If monorel isn't trivially runnable in the test environment, skip this step; the changeset format is documented and a malformed file would be caught by CI's release-pr workflow.

- [ ] **Step 3: Commit.**

```bash
cd /tmp/monorel-src
git add .changeset/cacheseed-fixpoint-and-pipeline-fixes.md
git commit -m "$(cat <<'EOF'
chore: changeset for cacheseed-fixpoint and pipeline fixes

Names monorel.disaresta.com at :minor for the four-concern fix
package addressing issue #54.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 15: final verification

**Files:** none (verification only)

- [ ] **Step 1: Run the full test suite.**

Run: `cd /tmp/monorel-src && go test ./...`

Expected: PASS for every package.

- [ ] **Step 2: Run any project-wide linters monorel uses.**

Run: `cd /tmp/monorel-src && go vet ./...`

Expected: no findings.

If monorel has staticcheck or other linters in CI, run them locally:

Run: `cd /tmp/monorel-src && command -v staticcheck >/dev/null && staticcheck ./... || echo "staticcheck not installed; skip"`

- [ ] **Step 3: Build the docs site.**

Run: `cd /tmp/monorel-src/docs && bun run docs:build 2>&1 | tail -5`

Expected: clean build with all cross-links resolving.

- [ ] **Step 4: Inspect the branch's full diff.**

Run: `cd /tmp/monorel-src && git log --oneline main..HEAD && git diff --stat main..HEAD`

Expected: 14 commits (Tasks 1-14), each focused; total diff covers `internal/release/{release,gosum,cacheseed,tidy}.go`, `internal/release/gosum_test.go`, `ci/github/README.md`, `docs/src/workflows.md`, `CHANGELOG.md`, `.changeset/cacheseed-fixpoint-and-pipeline-fixes.md`. No other files touched.

- [ ] **Step 5: Push the branch.**

Run: `cd /tmp/monorel-src && git push -u origin fix/cacheseed-fixpoint`

If a pre-push hook surfaces any drift (it shouldn't given the test runs were clean, but it's a hygiene check), address it before opening the PR.

- [ ] **Step 6: Open the PR.**

```bash
cd /tmp/monorel-src
git fetch origin main
git rebase origin/main  # ensure no conflicts; force-push if rebase produces commits
gh pr create --title "fix: address issue #54 (cacheseed wrong-hash + pipeline robustness)" --body "$(cat <<'EOF'
## Summary

Closes #54. Four-concern fix package addressing problems surfaced by
[`loglayer/loglayer-go`'s v2.1.0 release](https://github.com/loglayer/loglayer-go/pull/76):

- **Critical**: `release/cacheseed` writes the correct `h1:` hash for in-plan modules. Reorders `applyStable` so consumed-changeset deletion happens before the seed step. Replaces single-pass seed-and-tidy with iterate-to-fixpoint to handle cross-sibling dep chains.
- **Cache priming**: new `primeModuleCache` step before offline tidy so fresh CI runners can resolve third-party deps.
- **Toolchain docs**: inline comments on `actions/setup-go` in `ci/github/README.md` examples; strengthened "Requirements" bullet.
- **chore(release) race docs**: new `## Recipes` section in `ci/github/README.md` plus universal sections in `docs/src/workflows.md`.

## Test plan

- [x] `go test ./...` passes (existing + 4 new regression tests)
- [x] `go vet ./...` clean
- [x] `bun run docs:build` clean
- [x] Manually walked through `applyStable` reorder against the spec at `docs/superpowers/specs/2026-05-03-cacheseed-fixpoint-and-release-pipeline-fixes-design.md`

## Commits

14 atomic commits (TDD-style). Reviewable as commit-by-commit; can squash-merge to land as the spec's "three commits" if you prefer the bundled history.

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

---

## Self-review checklist (run before submitting the PR)

- [ ] **Spec coverage:** every concern in `docs/superpowers/specs/2026-05-03-cacheseed-fixpoint-and-release-pipeline-fixes-design.md` maps to at least one task. Spot-check:
  - Concern #1 reorder: Task 2.
  - Concern #1 fixpoint: Tasks 3-7.
  - Concern #2: Tasks 8-10.
  - Concern #3 README: Task 11.
  - Concern #3 universal: Task 12.
  - Concern #4 README: Task 11.
  - Concern #4 universal: Task 12.
  - CHANGELOG: Task 13.
  - Changeset: Task 14.
- [ ] **API consistency:** `seedModuleCache` returns `[]seededEntry` everywhere it's referenced. `clearSeededEntries`, `readGoSums`, `goSumsChanged`, `stageAffected`, `errFixpointNotReached` all match across tasks.
- [ ] **No em dashes** in any added prose (per the project's documentation rule). The plan above is the exception (it's an internal scaffolding doc); the docs the plan tells the engineer to write are em-dash-free.
- [ ] **Conventional Commits** on every commit subject.
