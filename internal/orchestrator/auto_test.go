package orchestrator

import (
	"context"
	"errors"
	"testing"

	"monorel.disaresta.com/changeset"
	"monorel.disaresta.com/config"
	"monorel.disaresta.com/internal/git"
	"monorel.disaresta.com/internal/provider"
	"monorel.disaresta.com/semver"
)

// stageAndCommit gives the fake a HEAD with the supplied commit
// message. The fake's [git.Fake.Commit] errors when nothing has been
// staged, so we stage a placeholder path; the contents are irrelevant
// to detect / Auto, which only read HEAD's commit message back.
func stageAndCommit(t *testing.T, repo *git.Fake, message string) {
	t.Helper()
	if err := repo.Add("dummy"); err != nil {
		t.Fatal(err)
	}
	if err := repo.Commit(message); err != nil {
		t.Fatal(err)
	}
}

func TestAuto_ReleaseBranch(t *testing.T) {
	repo := git.NewFake()
	// Trailer-bearing HEAD; detection short-circuits via SourceTrailer.
	stageAndCommit(t, repo, "chore(release): pkg-a v1.0.0\n\nmonorel-Release: pkg-a v1.0.0\n")

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
	stageAndCommit(t, repo, "docs: typo fix\n")

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
	stageAndCommit(t, repo, "feat: add login\n")

	pf := provider.NewFake()
	pf.DefaultBranch = "main"

	cfg := &config.Config{
		Packages: map[string]config.PackageConfig{
			// Changelog must be set: release.Apply writes the per-package
			// CHANGELOG.md to filepath.Join(RepoDir, Changelog) and would
			// fail on an empty path (writes the dir itself).
			"pkg-a": {Path: "pkg-a", TagPrefix: "pkg-a", Changelog: "pkg-a/CHANGELOG.md"},
		},
	}
	cs := []*changeset.Changeset{{
		Name: "fresh",
		Bumps: map[string]semver.BumpLevel{
			"pkg-a": semver.Minor,
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
	stageAndCommit(t, repo, "Merge pull request #5\n")
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
