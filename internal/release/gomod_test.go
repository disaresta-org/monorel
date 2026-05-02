package release

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"monorel.disaresta.com/changeset"
	"monorel.disaresta.com/config"
	"monorel.disaresta.com/internal/git"
	"monorel.disaresta.com/plan"
	"monorel.disaresta.com/semver"
)

// TestRewriteSubmoduleGoMods_StripsDevReplacesAndPinsRequires
// constructs a synthetic two-sub-module monorepo on disk:
//
//	root/
//	  transports/foo/go.mod   (module = example.com/transports/foo/v2,
//	                            replace example.com/transports/bar/v2 => ../bar,
//	                            require example.com/transports/bar/v2 v2.0.0-placeholder)
//	  transports/bar/go.mod   (module = example.com/transports/bar/v2)
//
// and verifies that after rewriteSubmoduleGoMods runs against a
// release plan that includes both packages at v2.0.1:
//
//   - transports/foo/go.mod no longer contains the `replace ../bar`
//     directive.
//   - transports/foo/go.mod's require for transports/bar/v2 reads
//     v2.0.1 instead of the placeholder.
//   - both go.mod files are staged via repo.Add.
func TestRewriteSubmoduleGoMods_StripsDevReplacesAndPinsRequires(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "transports/foo/go.mod"), `module example.com/transports/foo/v2

go 1.25.0

replace example.com/transports/bar/v2 => ../bar

require example.com/transports/bar/v2 v2.0.0-00010101000000-000000000000
`)
	mustWrite(t, filepath.Join(dir, "transports/bar/go.mod"), `module example.com/transports/bar/v2

go 1.25.0
`)

	repo := git.NewFake()
	opts := Options{
		Repo:    repo,
		RepoDir: dir,
		Plan: &plan.ReleasePlan{
			Releases: []plan.PackageRelease{
				{
					Name:   "transports/foo",
					Tag:    "transports/foo/v2.0.1",
					Bump:   semver.Patch,
					From:   "v2.0.0",
					To:     "v2.0.1",
					Config: config.PackageConfig{TagPrefix: "transports/foo", Path: "transports/foo"},
				},
				{
					Name:   "transports/bar",
					Tag:    "transports/bar/v2.0.1",
					Bump:   semver.Patch,
					From:   "v2.0.0",
					To:     "v2.0.1",
					Config: config.PackageConfig{TagPrefix: "transports/bar", Path: "transports/bar"},
				},
			},
		},
	}

	if err := rewriteSubmoduleGoMods(opts); err != nil {
		t.Fatalf("rewriteSubmoduleGoMods: %v", err)
	}

	got := mustRead(t, filepath.Join(dir, "transports/foo/go.mod"))
	if strings.Contains(got, "replace") {
		t.Errorf("dev replace directive should be stripped:\n%s", got)
	}
	if !strings.Contains(got, "v2.0.1") {
		t.Errorf("require version should be pinned to v2.0.1:\n%s", got)
	}
	if strings.Contains(got, "v2.0.0-00010101000000-000000000000") {
		t.Errorf("placeholder require version should be gone:\n%s", got)
	}

	// Both go.mod files should be staged. transports/foo got
	// edited; transports/bar didn't change (no sibling deps), so
	// only foo should be staged.
	staged := strings.Join(repo.Staged, " ")
	if !strings.Contains(staged, filepath.Join("transports", "foo", "go.mod")) {
		t.Errorf("foo/go.mod should be staged; staged = %v", repo.Staged)
	}
	if strings.Contains(staged, filepath.Join("transports", "bar", "go.mod")) {
		t.Errorf("bar/go.mod was not modified, should not be staged; staged = %v", repo.Staged)
	}
}

// TestRewriteSubmoduleGoMods_KeepsExternalReplaces verifies that a
// replace directive whose target ISN'T a sibling package is left
// alone.
func TestRewriteSubmoduleGoMods_KeepsExternalReplaces(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "transports/foo/go.mod"), `module example.com/transports/foo/v2

go 1.25.0

replace github.com/some/dep => github.com/forks/dep v1.2.3

require github.com/some/dep v1.0.0
`)

	repo := git.NewFake()
	opts := Options{
		Repo:    repo,
		RepoDir: dir,
		Plan: &plan.ReleasePlan{
			Releases: []plan.PackageRelease{{
				Name:   "transports/foo",
				Tag:    "transports/foo/v2.0.1",
				To:     "v2.0.1",
				Config: config.PackageConfig{TagPrefix: "transports/foo", Path: "transports/foo"},
			}},
		},
	}

	if err := rewriteSubmoduleGoMods(opts); err != nil {
		t.Fatalf("rewriteSubmoduleGoMods: %v", err)
	}

	got := mustRead(t, filepath.Join(dir, "transports/foo/go.mod"))
	if !strings.Contains(got, "replace github.com/some/dep => github.com/forks/dep v1.2.3") {
		t.Errorf("external replace should be preserved:\n%s", got)
	}
	// No sibling rewrite happened, no changes staged.
	if len(repo.Staged) != 0 {
		t.Errorf("nothing should be staged when no rewrite occurs; staged = %v", repo.Staged)
	}
}

// TestRewriteSubmoduleGoMods_NoGoModSkipsSilently confirms a
// release whose Path doesn't contain a go.mod (e.g. a pure-changelog
// package) doesn't error out.
func TestRewriteSubmoduleGoMods_NoGoModSkipsSilently(t *testing.T) {
	dir := t.TempDir()

	repo := git.NewFake()
	opts := Options{
		Repo:    repo,
		RepoDir: dir,
		Plan: &plan.ReleasePlan{
			Releases: []plan.PackageRelease{{
				Name:   "no-gomod",
				Tag:    "no-gomod/v1.0.0",
				To:     "v1.0.0",
				Config: config.PackageConfig{TagPrefix: "no-gomod", Path: "no-gomod"},
			}},
		},
	}

	if err := rewriteSubmoduleGoMods(opts); err != nil {
		t.Fatalf("rewriteSubmoduleGoMods on no-go.mod path should succeed: %v", err)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// Ensure changeset import is exercised so the linter is happy.
var _ = changeset.Changeset{}
