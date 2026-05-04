package orchestrator

import (
	"context"
	"errors"
	"strings"
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

	// Verify the apply step ran by checking HEAD's commit body has
	// the monorel-Release: trailer (proves release.Apply was called
	// with the right plan, not just that orchestrator.Run was).
	msg, err := repo.HeadCommitMessage()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(msg, "monorel-Release:") {
		t.Errorf("HEAD commit message missing monorel-Release: trailer; apply was not called.\nGot: %s", msg)
	}

	// Verify the force-push happened (Push was called with force=true).
	if !pushedRefForce(repo, "monorel/release") {
		t.Errorf("Push of monorel/release with force=true not recorded; got pushes: %+v", repo.Pushes)
	}
}

// pushedRefForce reports whether the fake recorded a force-push of
// branch to any remote. git.Fake records pushes as
// "<remote>:<ref>[:force]" strings, so we substring-match both the
// branch name and the force suffix.
func pushedRefForce(f *git.Fake, branch string) bool {
	for _, p := range f.Pushes {
		if strings.Contains(p, branch) && strings.Contains(p, "force") {
			return true
		}
	}
	return false
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

// TestAuto_FeatureBranch_ErrorWraps verifies that the error wraps in
// the non-empty-plan feature path are stable. Each case forces a
// failure at a specific step and asserts the resulting error contains
// the wrap prefix.
//
// Coverage is limited by the fakes' fault models. The provider fake's
// FailNext is a closure (we can sequence with FailOnNth), so we can
// reach GetDefaultBranch (the second provider call after detect's
// FindPRByMergeCommit). The release.Apply wrap is reachable by passing
// invalid Apply inputs (empty RepoDir) so Apply's own validation
// fails before any repo op runs.
//
// The repo fake's FailNext is a one-shot error (no closure), so it
// always consumes on the very first repo op (CurrentSHA in Auto's
// preamble), which would only surface the "auto: read HEAD SHA" wrap.
// Reaching the Fetch / CheckoutNewBranch / Push wraps would require
// closure-style sequencing on git.Fake; those cases are dropped here
// rather than misrepresent what's actually being tested.
func TestAuto_FeatureBranch_ErrorWraps(t *testing.T) {
	cases := []struct {
		name        string
		emptyRepo   bool // when true, pass RepoDir="" to force Apply's validation error
		setup       func(repo *git.Fake, pf *provider.Fake)
		wantWrapped string
	}{
		{
			name: "GetDefaultBranch fails",
			setup: func(_ *git.Fake, pf *provider.Fake) {
				// 1st provider call: detect.FindPRByMergeCommit (succeed).
				// 2nd provider call: autoFeature.GetDefaultBranch (fail).
				pf.FailNext = provider.FailOnNth(2, errFakeBoom)
			},
			wantWrapped: "auto: get default branch",
		},
		{
			name:      "Apply fails",
			emptyRepo: true,
			setup: func(_ *git.Fake, _ *provider.Fake) {
				// No fake-fault arming: Apply's own validation rejects
				// the empty RepoDir, surfacing the "auto: apply:" wrap.
			},
			wantWrapped: "auto: apply",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := git.NewFake()
			stageAndCommit(t, repo, "feat: add login\n") // non-release HEAD

			pf := provider.NewFake()
			pf.DefaultBranch = "main"

			cfg := &config.Config{
				Packages: map[string]config.PackageConfig{
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

			tc.setup(repo, pf)

			repoDir := t.TempDir()
			if tc.emptyRepo {
				repoDir = ""
			}

			_, err := Auto(context.Background(), AutoOptions{
				Config:       cfg,
				Repo:         repo,
				Provider:     pf,
				RepoDir:      repoDir,
				ChangesetDir: t.TempDir(),
				Changesets:   cs,
				Tags:         nil,
			})
			if err == nil {
				t.Fatalf("expected wrapped error containing %q", tc.wantWrapped)
			}
			if !strings.Contains(err.Error(), tc.wantWrapped) {
				t.Errorf("err = %q; should contain %q", err.Error(), tc.wantWrapped)
			}
		})
	}
}
