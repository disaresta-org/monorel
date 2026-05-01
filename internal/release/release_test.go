package release_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"monorel.disaresta.com/changeset"
	"monorel.disaresta.com/config"
	"monorel.disaresta.com/internal/git/testutil"
	"monorel.disaresta.com/internal/release"
	"monorel.disaresta.com/plan"
	"monorel.disaresta.com/semver"
)

const oneFooTOML = `
[provider]
owner = "x"
repo = "y"

[packages.foo]
tag_prefix = "transports/foo"
path = "transports/foo"
changelog = "transports/foo/CHANGELOG.md"
`

// setupRepo builds an on-disk repo with monorel.toml and a single
// changeset, returns the resolved config + plan.
func setupRepo(t *testing.T, changesetName, changesetBody string, level semver.BumpLevel, tags []string) (*testutil.TestRepo, *plan.ReleasePlan, *config.Config) {
	t.Helper()
	r := testutil.NewRepo(t)
	r.WriteFile("monorel.toml", oneFooTOML)
	r.WriteFile("transports/foo/CHANGELOG.md", "# Changelog\n")
	r.WriteFile(filepath.Join(".changeset", changesetName+".md"),
		"---\n\"foo\": "+level.String()+"\n---\n\n"+changesetBody+"\n")
	r.AddCommit("seed",
		"monorel.toml",
		"transports/foo/CHANGELOG.md",
		filepath.Join(".changeset", changesetName+".md"),
	)
	for _, tag := range tags {
		r.Tag(tag, "")
	}

	cfg, err := config.Load(filepath.Join(r.Dir, "monorel.toml"))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	changesets, err := changeset.LoadAll(filepath.Join(r.Dir, ".changeset"))
	if err != nil {
		t.Fatalf("load changesets: %v", err)
	}
	repoTags, err := r.Repo.ListTags("")
	if err != nil {
		t.Fatalf("list tags: %v", err)
	}
	p, err := plan.Plan(cfg, changesets, repoTags, nil)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	return r, p, cfg
}

func TestApply_Stable_HappyPath(t *testing.T) {
	r, p, _ := setupRepo(t, "first-feature", "First feature line.", semver.Minor,
		[]string{"transports/foo/v1.5.0"})

	res, err := release.ApplyAndTag(release.Options{
		Plan:         p,
		Repo:         r.Repo,
		RepoDir:      r.Dir,
		ChangesetDir: filepath.Join(r.Dir, ".changeset"),
		Today:        "2026-04-30",
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(res.Releases) != 1 || res.Tags()[0] != "transports/foo/v1.6.0" {
		t.Errorf("Tags = %v, want [transports/foo/v1.6.0]", res.Tags())
	}
	if res.CommitSHA == "" {
		t.Error("CommitSHA empty")
	}

	// Tag exists.
	tags, err := r.Repo.ListTags("transports/foo/v1.6")
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 1 || tags[0] != "transports/foo/v1.6.0" {
		t.Errorf("tags after release: %v", tags)
	}

	// CHANGELOG written.
	data, err := os.ReadFile(filepath.Join(r.Dir, "transports/foo/CHANGELOG.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"## [1.6.0] - 2026-04-30", "### Minor Changes", "- First feature line."} {
		if !strings.Contains(string(data), want) {
			t.Errorf("CHANGELOG missing %q\nfull:\n%s", want, data)
		}
	}

	// Changeset file deleted.
	if _, err := os.Stat(filepath.Join(r.Dir, ".changeset", "first-feature.md")); !os.IsNotExist(err) {
		t.Errorf("changeset still exists: %v", err)
	}

	// Working tree clean.
	clean, err := r.Repo.IsClean()
	if err != nil {
		t.Fatal(err)
	}
	if !clean {
		t.Error("tree not clean after release")
	}
}

func TestApply_Stable_InitialRelease(t *testing.T) {
	r, p, _ := setupRepo(t, "first", "First feature.", semver.Minor, nil)
	res, err := release.ApplyAndTag(release.Options{
		Plan:         p,
		Repo:         r.Repo,
		RepoDir:      r.Dir,
		ChangesetDir: filepath.Join(r.Dir, ".changeset"),
		Today:        "2026-04-30",
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(res.Releases) != 1 || res.Tags()[0] != "transports/foo/v0.1.0" {
		t.Errorf("Tags = %v, want [transports/foo/v0.1.0]", res.Tags())
	}
}

func TestApply_TagAlreadyExistsAborts(t *testing.T) {
	// Plan and tags can drift if the user added a tag manually
	// between `monorel plan` and `monorel release`. The applier
	// must catch that and abort BEFORE any filesystem mutation.
	r, p, _ := setupRepo(t, "first", "Feature.", semver.Minor,
		[]string{"transports/foo/v1.5.0"})

	// Manually create the tag the planner is about to produce, so
	// the preflight check triggers without depending on planner
	// behavior.
	r.Tag(p.Releases[0].Tag, "")

	_, err := release.ApplyAndTag(release.Options{
		Plan:         p,
		Repo:         r.Repo,
		RepoDir:      r.Dir,
		ChangesetDir: filepath.Join(r.Dir, ".changeset"),
		Today:        "2026-04-30",
	})
	if err == nil {
		t.Fatal("expected error; got nil")
	}
	if !errors.Is(err, release.ErrTagExists) {
		t.Errorf("error = %v, want wrapping ErrTagExists", err)
	}

	// No mutation: changeset file still exists.
	if _, statErr := os.Stat(filepath.Join(r.Dir, ".changeset", "first.md")); statErr != nil {
		t.Errorf("changeset deleted on aborted Apply: %v", statErr)
	}
}

func TestApply_EmptyPlan(t *testing.T) {
	r := testutil.NewRepo(t)
	_, err := release.ApplyAndTag(release.Options{
		Plan:         &plan.ReleasePlan{},
		Repo:         r.Repo,
		RepoDir:      r.Dir,
		ChangesetDir: filepath.Join(r.Dir, ".changeset"),
	})
	if !errors.Is(err, release.ErrPlanEmpty) {
		t.Errorf("err = %v, want ErrPlanEmpty", err)
	}
}

func TestApply_PreRelease(t *testing.T) {
	// Setup repo + plan with pre-state.
	r := testutil.NewRepo(t)
	r.WriteFile("monorel.toml", oneFooTOML)
	r.WriteFile("transports/foo/CHANGELOG.md", "# Changelog\n")
	r.WriteFile(filepath.Join(".changeset", "first.md"),
		"---\n\"foo\": minor\n---\n\nFeature.\n")
	pre := &changeset.PreState{Mode: "pre", Channel: "rc", Counters: map[string]int{}}
	if err := pre.Write(filepath.Join(r.Dir, ".changeset")); err != nil {
		t.Fatal(err)
	}
	r.AddCommit("seed",
		"monorel.toml",
		"transports/foo/CHANGELOG.md",
		filepath.Join(".changeset", "first.md"),
		filepath.Join(".changeset", "pre.json"),
	)
	r.Tag("transports/foo/v1.5.0", "")

	cfg, err := config.Load(filepath.Join(r.Dir, "monorel.toml"))
	if err != nil {
		t.Fatal(err)
	}
	changesets, err := changeset.LoadAll(filepath.Join(r.Dir, ".changeset"))
	if err != nil {
		t.Fatal(err)
	}
	tags, _ := r.Repo.ListTags("")
	p, err := plan.Plan(cfg, changesets, tags, pre)
	if err != nil {
		t.Fatal(err)
	}

	res, err := release.ApplyAndTag(release.Options{
		Plan:         p,
		Repo:         r.Repo,
		RepoDir:      r.Dir,
		ChangesetDir: filepath.Join(r.Dir, ".changeset"),
		PreState:     pre,
		Today:        "2026-04-30",
	})
	if err != nil {
		t.Fatalf("Apply pre: %v", err)
	}
	if res.Tags()[0] != "transports/foo/v1.6.0-rc.0" {
		t.Errorf("Tags = %v, want suffixed", res.Tags())
	}

	// CHANGELOG should be UNTOUCHED (pre mode skips CHANGELOG writes).
	data, err := os.ReadFile(filepath.Join(r.Dir, "transports/foo/CHANGELOG.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "## [1.6.0-rc.0]") {
		t.Errorf("pre release should not write CHANGELOG entry; got:\n%s", data)
	}

	// Changeset file should STILL EXIST.
	if _, err := os.Stat(filepath.Join(r.Dir, ".changeset", "first.md")); err != nil {
		t.Errorf("pre release should keep changeset file: %v", err)
	}

	// pre.json counter incremented.
	updated, err := changeset.LoadPreState(filepath.Join(r.Dir, ".changeset"))
	if err != nil {
		t.Fatal(err)
	}
	if updated.Counters["foo"] != 1 {
		t.Errorf("counter[foo] = %d, want 1", updated.Counters["foo"])
	}
}

func TestApply_MultiPackage(t *testing.T) {
	r := testutil.NewRepo(t)
	r.WriteFile("monorel.toml", `
[provider]
owner = "x"
repo = "y"

[packages.foo]
tag_prefix = "transports/foo"
path = "transports/foo"
changelog = "transports/foo/CHANGELOG.md"

[packages.bar]
tag_prefix = "transports/bar"
path = "transports/bar"
changelog = "transports/bar/CHANGELOG.md"
`)
	r.WriteFile("transports/foo/CHANGELOG.md", "# Changelog\n")
	r.WriteFile("transports/bar/CHANGELOG.md", "# Changelog\n")
	// One changeset bumps both.
	r.WriteFile(filepath.Join(".changeset", "multi.md"),
		"---\n\"foo\": minor\n\"bar\": patch\n---\n\nMulti-package change.\n")
	r.AddCommit("seed",
		"monorel.toml",
		"transports/foo/CHANGELOG.md",
		"transports/bar/CHANGELOG.md",
		filepath.Join(".changeset", "multi.md"),
	)
	r.Tag("transports/foo/v1.5.0", "")
	r.Tag("transports/bar/v0.1.0", "")

	cfg, err := config.Load(filepath.Join(r.Dir, "monorel.toml"))
	if err != nil {
		t.Fatal(err)
	}
	changesets, _ := changeset.LoadAll(filepath.Join(r.Dir, ".changeset"))
	tags, _ := r.Repo.ListTags("")
	p, err := plan.Plan(cfg, changesets, tags, nil)
	if err != nil {
		t.Fatal(err)
	}

	res, err := release.ApplyAndTag(release.Options{
		Plan:         p,
		Repo:         r.Repo,
		RepoDir:      r.Dir,
		ChangesetDir: filepath.Join(r.Dir, ".changeset"),
		Today:        "2026-04-30",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Releases) != 2 {
		t.Errorf("Tags = %v, want 2 entries", res.Tags())
	}
	// Both CHANGELOGs bucket the change at their respective level.
	foo, _ := os.ReadFile(filepath.Join(r.Dir, "transports/foo/CHANGELOG.md"))
	if !strings.Contains(string(foo), "### Minor Changes") {
		t.Errorf("foo CHANGELOG should have Minor Changes:\n%s", foo)
	}
	bar, _ := os.ReadFile(filepath.Join(r.Dir, "transports/bar/CHANGELOG.md"))
	if !strings.Contains(string(bar), "### Patch Changes") {
		t.Errorf("bar CHANGELOG should have Patch Changes:\n%s", bar)
	}
	// Single changeset deleted exactly once.
	if _, err := os.Stat(filepath.Join(r.Dir, ".changeset", "multi.md")); !os.IsNotExist(err) {
		t.Errorf("multi.md not deleted: %v", err)
	}
}

func TestApply_ReRunIsNoMutation(t *testing.T) {
	// After a successful Apply, the same plan tag now exists. A
	// second Apply with the same plan (same changesets gone) is
	// not a real scenario, but if a caller forces it we want a
	// clean abort, not corruption.
	r, p, _ := setupRepo(t, "first", "Feature.", semver.Minor,
		[]string{"transports/foo/v1.5.0"})

	if _, err := release.ApplyAndTag(release.Options{
		Plan:         p,
		Repo:         r.Repo,
		RepoDir:      r.Dir,
		ChangesetDir: filepath.Join(r.Dir, ".changeset"),
		Today:        "2026-04-30",
	}); err != nil {
		t.Fatal(err)
	}

	// Re-run with the same plan. The tag is now in the repo, so
	// preflightTags should reject.
	_, err := release.ApplyAndTag(release.Options{
		Plan:         p,
		Repo:         r.Repo,
		RepoDir:      r.Dir,
		ChangesetDir: filepath.Join(r.Dir, ".changeset"),
		Today:        "2026-04-30",
	})
	if !errors.Is(err, release.ErrTagExists) {
		t.Errorf("re-run err = %v, want ErrTagExists", err)
	}
}
