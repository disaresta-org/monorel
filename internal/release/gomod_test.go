package release

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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

// TestRewriteSubmoduleGoMods_MultiSiblingRequirePin verifies that a
// go.mod with multiple sibling require lines gets every sibling
// pinned, not just the first one.
func TestRewriteSubmoduleGoMods_MultiSiblingRequirePin(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "transports/baz/go.mod"), `module example.com/transports/baz/v2

go 1.25.0

replace example.com/transports/foo/v2 => ../foo
replace example.com/transports/bar/v2 => ../bar

require (
	example.com/transports/bar/v2 v2.0.0-00010101000000-000000000000
	example.com/transports/foo/v2 v2.0.0-00010101000000-000000000000
)
`)
	mustWrite(t, filepath.Join(dir, "transports/foo/go.mod"), `module example.com/transports/foo/v2

go 1.25.0
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
					Name: "transports/baz", Tag: "transports/baz/v2.0.1", To: "v2.0.1", Bump: semver.Patch,
					Config: config.PackageConfig{TagPrefix: "transports/baz", Path: "transports/baz"},
				},
				{
					Name: "transports/foo", Tag: "transports/foo/v2.0.1", To: "v2.0.1", Bump: semver.Patch,
					Config: config.PackageConfig{TagPrefix: "transports/foo", Path: "transports/foo"},
				},
				{
					Name: "transports/bar", Tag: "transports/bar/v2.0.1", To: "v2.0.1", Bump: semver.Patch,
					Config: config.PackageConfig{TagPrefix: "transports/bar", Path: "transports/bar"},
				},
			},
		},
	}

	if err := rewriteSubmoduleGoMods(opts); err != nil {
		t.Fatalf("rewriteSubmoduleGoMods: %v", err)
	}

	got := mustRead(t, filepath.Join(dir, "transports/baz/go.mod"))
	for _, want := range []string{
		"example.com/transports/foo/v2 v2.0.1",
		"example.com/transports/bar/v2 v2.0.1",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing pinned require %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "v2.0.0-00010101000000-000000000000") {
		t.Errorf("placeholder version still present:\n%s", got)
	}
	if strings.Contains(got, "replace example.com/transports/") {
		t.Errorf("dev replace directives should be stripped:\n%s", got)
	}
}

// TestRewriteSubmoduleGoMods_OutOfPlanSiblingLeftAlone verifies
// that a require for a package that ISN'T in the current release
// plan (even if it shares the same naming pattern) is not touched.
// The contract is "rewrite siblings IN the plan"; out-of-plan
// siblings stay at whatever version the go.mod already specifies.
func TestRewriteSubmoduleGoMods_OutOfPlanSiblingLeftAlone(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "transports/foo/go.mod"), `module example.com/transports/foo/v2

go 1.25.0

require example.com/transports/external/v2 v2.5.0
`)

	repo := git.NewFake()
	opts := Options{
		Repo:    repo,
		RepoDir: dir,
		Plan: &plan.ReleasePlan{
			Releases: []plan.PackageRelease{{
				Name: "transports/foo", Tag: "transports/foo/v2.0.1", To: "v2.0.1", Bump: semver.Patch,
				Config: config.PackageConfig{TagPrefix: "transports/foo", Path: "transports/foo"},
			}},
		},
	}

	if err := rewriteSubmoduleGoMods(opts); err != nil {
		t.Fatalf("rewriteSubmoduleGoMods: %v", err)
	}

	got := mustRead(t, filepath.Join(dir, "transports/foo/go.mod"))
	if !strings.Contains(got, "example.com/transports/external/v2 v2.5.0") {
		t.Errorf("out-of-plan sibling require should be preserved:\n%s", got)
	}
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
