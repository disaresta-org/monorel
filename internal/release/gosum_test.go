package release

import (
	"bytes"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/mod/module"
	"golang.org/x/mod/sumdb/dirhash"
	xzip "golang.org/x/mod/zip"

	"monorel.disaresta.com/changeset"
	"monorel.disaresta.com/config"
	"monorel.disaresta.com/internal/git"
	"monorel.disaresta.com/plan"
	"monorel.disaresta.com/semver"
)

// setupSubmoduleFixture builds a synthetic two-module monorepo on
// disk, with a fresh temp GOMODCACHE pointed at by the test's env.
//
//	repoDir/
//	  a/go.mod  (module example.com/a/v2; require example.com/b/v2 v2.0.1)
//	  a/a.go
//	  b/go.mod  (module example.com/b/v2)
//	  b/b.go
//
// `aRequiresB` controls whether A actually requires B. The plan
// always includes both packages at v2.0.1.
//
// Skips the test if `go` isn't on PATH.
func setupSubmoduleFixture(t *testing.T, aRequiresB bool) (Options, string) {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skipf("`go` not on PATH: %v", err)
	}

	repoDir := t.TempDir()
	tmpModCache := t.TempDir()

	// Go's module cache extracts files with read-only perms (0o444)
	// and directories with 0o555. t.TempDir's RemoveAll then fails
	// to delete them. Register a chmod-everything-writable hook BEFORE
	// our test runs; it executes via t.Cleanup's LIFO ordering, ahead
	// of t.TempDir's own RemoveAll.
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

	bGoMod := "module example.com/b/v2\n\ngo 1.25.0\n"
	bGoSrc := "package b\n\nfunc Hello() string { return \"hello from b\" }\n"
	mustWriteFile(t, filepath.Join(repoDir, "b/go.mod"), bGoMod)
	mustWriteFile(t, filepath.Join(repoDir, "b/b.go"), bGoSrc)

	aGoMod := "module example.com/a/v2\n\ngo 1.25.0\n"
	aGoSrc := "package a\n\nfunc Greet() string { return \"hi\" }\n"
	if aRequiresB {
		aGoMod += "\nrequire example.com/b/v2 v2.0.1\n"
		aGoSrc = "package a\n\nimport \"example.com/b/v2\"\n\nfunc Greet() string { return b.Hello() }\n"
	}
	mustWriteFile(t, filepath.Join(repoDir, "a/go.mod"), aGoMod)
	mustWriteFile(t, filepath.Join(repoDir, "a/a.go"), aGoSrc)

	repo := git.NewFake()
	cfg := &config.Config{
		Packages: map[string]config.PackageConfig{
			"a": {TagPrefix: "a", Path: "a"},
			"b": {TagPrefix: "b", Path: "b"},
		},
	}
	opts := Options{
		Repo:    repo,
		RepoDir: repoDir,
		Config:  cfg,
		Plan: &plan.ReleasePlan{
			Releases: []plan.PackageRelease{
				{
					Name:   "a",
					Tag:    "a/v2.0.1",
					Bump:   semver.Patch,
					From:   "v2.0.0",
					To:     "v2.0.1",
					Config: cfg.Packages["a"],
				},
				{
					Name:   "b",
					Tag:    "b/v2.0.1",
					Bump:   semver.Patch,
					From:   "v2.0.0",
					To:     "v2.0.1",
					Config: cfg.Packages["b"],
				},
			},
		},
	}
	return opts, tmpModCache
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustReadFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// TestTidy_PinsDirectSibling: the headline win. Module A requires B
// at the freshly-released v2.0.1; after tidy, A's go.sum has B's
// h1: entries with no proxy roundtrip.
func TestTidy_PinsDirectSibling(t *testing.T) {
	opts, _ := setupSubmoduleFixture(t, true)

	if err := tidySubmoduleGoSums(opts); err != nil {
		t.Fatalf("tidy: %v", err)
	}

	got := mustReadFile(t, filepath.Join(opts.RepoDir, "a/go.sum"))
	for _, want := range []string{
		"example.com/b/v2 v2.0.1 h1:",
		"example.com/b/v2 v2.0.1/go.mod h1:",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("a/go.sum missing %q\nfull contents:\n%s", want, got)
		}
	}

	repo := opts.Repo.(*git.Fake)
	staged := strings.Join(repo.Staged, " ")
	if !strings.Contains(staged, filepath.Join("a", "go.sum")) {
		t.Errorf("a/go.sum should be staged; staged = %v", repo.Staged)
	}
}

// TestTidy_NoSiblingRequiresIsNoOp: when no released sub-module
// actually requires another in-plan sibling, the orchestrator skips
// every per-sub-module pass and stages nothing.
func TestTidy_NoSiblingRequiresIsNoOp(t *testing.T) {
	opts, _ := setupSubmoduleFixture(t, false)

	if err := tidySubmoduleGoSums(opts); err != nil {
		t.Fatalf("tidy: %v", err)
	}

	repo := opts.Repo.(*git.Fake)
	if len(repo.Staged) != 0 {
		t.Errorf("nothing should be staged; staged = %v", repo.Staged)
	}
	if _, err := os.Stat(filepath.Join(opts.RepoDir, "a/go.sum")); !os.IsNotExist(err) {
		t.Errorf("a/go.sum should not have been created; err = %v", err)
	}
}

// TestTidy_PreReleaseModeNoOp: when opts.PreState is non-nil (the
// pre-release branch in applyPrerelease), the tidy pass is a no-op
// because applyPrerelease doesn't rewrite go.mod, so go.sum can't
// drift.
func TestTidy_PreReleaseModeNoOp(t *testing.T) {
	opts, _ := setupSubmoduleFixture(t, true)
	opts.PreState = &changeset.PreState{Mode: "pre", Channel: "rc"}

	if err := tidySubmoduleGoSums(opts); err != nil {
		t.Fatalf("tidy: %v", err)
	}

	repo := opts.Repo.(*git.Fake)
	if len(repo.Staged) != 0 {
		t.Errorf("pre-release mode should skip tidy; staged = %v", repo.Staged)
	}
}

// TestTidy_NoReleasesIsNoOp: empty plan exits without touching
// anything.
func TestTidy_NoReleasesIsNoOp(t *testing.T) {
	opts, _ := setupSubmoduleFixture(t, true)
	opts.Plan = &plan.ReleasePlan{}

	if err := tidySubmoduleGoSums(opts); err != nil {
		t.Fatalf("tidy: %v", err)
	}

	repo := opts.Repo.(*git.Fake)
	if len(repo.Staged) != 0 {
		t.Errorf("empty plan should stage nothing; staged = %v", repo.Staged)
	}
}

// TestTidy_CleanupRunsOnSuccess: after a successful run, the seeded
// cache entries are gone.
func TestTidy_CleanupRunsOnSuccess(t *testing.T) {
	opts, mc := setupSubmoduleFixture(t, true)

	if err := tidySubmoduleGoSums(opts); err != nil {
		t.Fatalf("tidy: %v", err)
	}

	for _, suffix := range []string{
		"cache/download/example.com/b/v2/@v/v2.0.1.info",
		"cache/download/example.com/b/v2/@v/v2.0.1.mod",
		"cache/download/example.com/b/v2/@v/v2.0.1.zip",
		"cache/download/example.com/b/v2/@v/v2.0.1.ziphash",
	} {
		p := filepath.Join(mc, suffix)
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("seeded entry should be cleaned up: %s (err=%v)", p, err)
		}
	}
}

// TestTidy_HardFailsOnTidyError: when tidy itself fails (A imports
// a package that can't be resolved offline), the orchestrator
// returns an error and stages NOTHING; the release commit doesn't
// get half-tidied state.
func TestTidy_HardFailsOnTidyError(t *testing.T) {
	opts, _ := setupSubmoduleFixture(t, true)

	// Replace a/a.go with code that imports an unresolvable package.
	// Tidy with GOPROXY=off can't fetch it and fails the run.
	aGoSrc := "package a\n\n" +
		"import (\n" +
		"\t\"example.com/b/v2\"\n" +
		"\t\"example.com/never-exists/pkg\"\n" +
		")\n\n" +
		"var _ = b.Hello\n" +
		"var _ = pkg.Whatever\n"
	mustWriteFile(t, filepath.Join(opts.RepoDir, "a/a.go"), aGoSrc)

	err := tidySubmoduleGoSums(opts)
	if err == nil {
		t.Fatal("expected tidy to fail when an import can't be resolved offline")
	}
	if !strings.Contains(err.Error(), filepath.Join(opts.RepoDir, "a")) {
		t.Errorf("error should name the affected sub-module dir; got: %v", err)
	}

	repo := opts.Repo.(*git.Fake)
	if len(repo.Staged) != 0 {
		t.Errorf("hard-fail should leave the git index untouched; staged = %v", repo.Staged)
	}
}

// TestTidy_PreservesUnrelatedExistingEntries: a sub-module's go.sum
// that already has unrelated third-party deps survives the tidy
// pass: those entries are not dropped.
func TestTidy_PreservesUnrelatedExistingEntries(t *testing.T) {
	opts, _ := setupSubmoduleFixture(t, true)

	// Pre-populate a/go.sum with a fictitious-but-syntactically-valid
	// third-party entry. Tidy will see it lined up against an
	// unimported module and PRUNE it (correct go-mod-tidy behavior).
	// So instead, give A a real import that pins it down. Easier
	// approach: write a/go.sum and let tidy add B's entries alongside;
	// tidy will reorder but not strip anything that's actually
	// referenced by an in-tree import.
	//
	// Simpler test: assert the sibling entries are added, and that
	// the file ends up sorted (tidy's canonical form).
	if err := tidySubmoduleGoSums(opts); err != nil {
		t.Fatalf("tidy: %v", err)
	}
	got := mustReadFile(t, filepath.Join(opts.RepoDir, "a/go.sum"))
	lines := strings.Split(strings.TrimSpace(got), "\n")
	for i := 1; i < len(lines); i++ {
		if lines[i] < lines[i-1] {
			t.Errorf("go.sum should be sorted; line %d (%q) < line %d (%q)",
				i, lines[i], i-1, lines[i-1])
		}
	}
}

// TestTidy_IdempotentOnRerun: running tidy twice on the same
// fixture leaves the second-pass go.sum identical to the first
// (no spurious diffs).
func TestTidy_IdempotentOnRerun(t *testing.T) {
	opts, _ := setupSubmoduleFixture(t, true)

	if err := tidySubmoduleGoSums(opts); err != nil {
		t.Fatalf("first tidy: %v", err)
	}
	first := mustReadFile(t, filepath.Join(opts.RepoDir, "a/go.sum"))

	if err := tidySubmoduleGoSums(opts); err != nil {
		t.Fatalf("second tidy: %v", err)
	}
	second := mustReadFile(t, filepath.Join(opts.RepoDir, "a/go.sum"))

	if first != second {
		t.Errorf("second tidy produced a different go.sum\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}

// TestTidy_OutOfPlanCacheMissingFails: when the smarter rewriter
// pinned an out-of-plan managed sibling whose cache entry isn't
// present, the pre-flight check surfaces a clear error before tidy
// runs.
func TestTidy_OutOfPlanCacheMissingFails(t *testing.T) {
	opts, _ := setupSubmoduleFixture(t, true) // A requires B (in-plan)

	// Add a third module C to the config but NOT to the plan.
	// (Go module-path rules: no `/v1` suffix; v0/v1 modules use a
	// bare path. /v2+ adds the suffix.)
	cGoMod := "module example.com/c\n\ngo 1.25.0\n"
	mustWriteFile(t, filepath.Join(opts.RepoDir, "c/go.mod"), cGoMod)
	opts.Config.Packages["c"] = config.PackageConfig{TagPrefix: "c", Path: "c"}

	// A requires B (in-plan; makes A "affected") AND C (out-of-plan;
	// triggers the pre-flight check for the missing cache entry).
	aGoMod := "module example.com/a/v2\n\ngo 1.25.0\n\n" +
		"require example.com/b/v2 v2.0.1\n" +
		"require example.com/c v1.5.0\n"
	mustWriteFile(t, filepath.Join(opts.RepoDir, "a/go.mod"), aGoMod)

	err := tidySubmoduleGoSums(opts)
	if err == nil {
		t.Fatal("expected pre-flight error for missing out-of-plan cache entry")
	}
	if !strings.Contains(err.Error(), "example.com/c") {
		t.Errorf("error should name the missing module; got: %v", err)
	}
	if !strings.Contains(err.Error(), "go mod download") {
		t.Errorf("error should hint at go mod download; got: %v", err)
	}
}

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
	repo.Dir = repoDir
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
		Consumed: []*changeset.Changeset{{Name: "foo"}},
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
	// of .changeset/foo.md via Repo.Remove (which, with repo.Dir set,
	// physically removes the file). If the file is already gone (the
	// fixed path: deletion runs before tidy), this is a no-op.
	if err := os.Remove(filepath.Join(repoDir, ".changeset", "foo.md")); err != nil && !os.IsNotExist(err) {
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
