package release_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"monorel.disaresta.com/changeset"
	"monorel.disaresta.com/config"
	"monorel.disaresta.com/internal/git"
	"monorel.disaresta.com/internal/git/testutil"
	"monorel.disaresta.com/internal/provider"
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

func TestApply_WritesParseableTrailers(t *testing.T) {
	// Apply -> Tag is bridged by the commit-body trailer format. This
	// test pins the format that Apply emits so a future change to
	// commitMessage's output can't silently break Tag's parser.
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
	r.WriteFile(filepath.Join(".changeset", "multi.md"),
		"---\n\"foo\": minor\n\"bar\": patch\n---\n\nMulti.\n")
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

	// Run Apply (NOT ApplyAndTag) so the commit lands without tags.
	if _, err := release.Apply(release.Options{
		Plan:         p,
		Repo:         r.Repo,
		RepoDir:      r.Dir,
		ChangesetDir: filepath.Join(r.Dir, ".changeset"),
		Today:        "2026-04-30",
	}); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	msg, err := r.Repo.HeadCommitMessage()
	if err != nil {
		t.Fatal(err)
	}
	// Each package contributes one trailer; PreRelease line is set.
	for _, want := range []string{
		"monorel-Release: foo v1.6.0",
		"monorel-Release: bar v0.1.1",
		"monorel-PreRelease: false",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("commit message missing %q\nfull:\n%s", want, msg)
		}
	}
	// Trailer order must match plan order (Tag relies on this for
	// stable ReleaseInfo ordering downstream).
	if len(p.Releases) != 2 {
		t.Fatalf("plan has %d releases, want 2", len(p.Releases))
	}
	first := strings.Index(msg, "monorel-Release: "+p.Releases[0].Name)
	second := strings.Index(msg, "monorel-Release: "+p.Releases[1].Name)
	if first < 0 || second < 0 || first > second {
		t.Errorf("trailer order doesn't match plan order %v: %q at %d, %q at %d\nfull:\n%s",
			[]string{p.Releases[0].Name, p.Releases[1].Name},
			p.Releases[0].Name, first, p.Releases[1].Name, second, msg)
	}
}

func TestApply_PreRelease_TrailerFlagsTrue(t *testing.T) {
	// Pre-release Apply must set monorel-PreRelease: true so Tag
	// propagates the flag into ReleaseInfo.Prerelease (which the
	// publish step reads to flag the GitHub Release).
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

	cfg, _ := config.Load(filepath.Join(r.Dir, "monorel.toml"))
	changesets, _ := changeset.LoadAll(filepath.Join(r.Dir, ".changeset"))
	tags, _ := r.Repo.ListTags("")
	p, err := plan.Plan(cfg, changesets, tags, pre)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := release.Apply(release.Options{
		Plan:         p,
		Repo:         r.Repo,
		RepoDir:      r.Dir,
		ChangesetDir: filepath.Join(r.Dir, ".changeset"),
		PreState:     pre,
		Today:        "2026-04-30",
	}); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	msg, err := r.Repo.HeadCommitMessage()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"monorel-Release: foo v1.6.0-rc.0",
		"monorel-PreRelease: true",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("commit message missing %q\nfull:\n%s", want, msg)
		}
	}
}

func TestTag_FallbackToPRBody(t *testing.T) {
	// Squash-merge has rewritten HEAD's commit body, removing the
	// monorel-Release: trailers. Tag with a Provider set should fall
	// back to fetching the merged PR's body and parsing trailers from
	// the embedded HTML comment block.
	f := git.NewFake()
	f.HeadSHA = "abc123"
	if err := f.Add("dummy"); err != nil {
		t.Fatal(err)
	}
	if err := f.Commit("chore(release): foo v1.7.0\n\n(no trailers; squash-merge)"); err != nil {
		t.Fatal(err)
	}

	pf := provider.NewFake()
	pf.PRs[42] = &provider.PullRequest{
		Number: 42,
		State:  "closed",
		Title:  "release",
		Body: `Plan body
<!-- monorel-trailers (do not edit; required for tag recovery if the merge commit body is rewritten)
monorel-Release: foo v1.7.0
monorel-PreRelease: false
-->`,
		MergedSHA: "abc123",
	}

	cfg := &config.Config{
		Packages: map[string]config.PackageConfig{
			"foo": pkgConfig("transports/foo", "transports/foo"),
		},
	}
	res, err := release.Tag(release.TagOptions{Config: cfg, Repo: f, Provider: pf})
	if err != nil {
		t.Fatalf("Tag with fallback: %v", err)
	}
	if len(res.Releases) != 1 || res.Releases[0].Tag != "transports/foo/v1.7.0" {
		t.Errorf("got %+v, want one tag transports/foo/v1.7.0", res.Releases)
	}
}

func TestTag_FallbackBothMissing(t *testing.T) {
	// HEAD has no trailers AND the provider has no PR matching the
	// SHA. Tag should still return ErrNoReleaseCommit (fallback isn't
	// a free pass; both sources have to be empty for a no-op).
	f := git.NewFake()
	f.HeadSHA = "abc123"
	if err := f.Add("dummy"); err != nil {
		t.Fatal(err)
	}
	if err := f.Commit("chore(release): foo v1.7.0\n\n(no trailers)"); err != nil {
		t.Fatal(err)
	}

	pf := provider.NewFake()
	// No PR seeded.

	cfg := &config.Config{Packages: map[string]config.PackageConfig{}}
	_, err := release.Tag(release.TagOptions{Config: cfg, Repo: f, Provider: pf})
	if err == nil {
		t.Fatal("expected ErrNoReleaseCommit; got nil")
	}
	if !errors.Is(err, release.ErrNoReleaseCommit) {
		t.Errorf("err = %v, want ErrNoReleaseCommit", err)
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
