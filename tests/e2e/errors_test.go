//go:build e2e

package e2e_test

import (
	"strings"
	"testing"
)

// TestErrors_TagConflict covers Scenario 10: re-running auto on a
// commit whose derived tags already exist must error. The error
// message references release.ErrTagExists and is wrapped with the
// auto: prefix.
func TestErrors_TagConflict(t *testing.T) {
	r := newScenarioRepo(t, "tagconflict")
	r.ScaffoldSinglePackage(t, "pkg-a", "pkg-a", "pkg-a")
	r.WriteChangeset(t, "feat", map[string]string{"pkg-a": "minor"}, "feat.")
	r.CommitAll(t, "init+feat")
	r.PushMain(t)

	r.MonorelOK(t, "auto")
	prs := r.PRs(t, "open")
	r.MergePR(t, prs[0].Number, "squash")
	r.CheckoutMain(t)
	r.MonorelOK(t, "auto")

	// Tag exists locally + remotely. Run tag again — should error.
	res := r.Monorel(t, "tag")
	if res.ExitCode == 0 {
		t.Errorf("expected non-zero exit, got 0\nstdout: %s\nstderr: %s", res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stderr, "tag already exists") {
		t.Errorf("stderr missing 'tag already exists':\n%s", res.Stderr)
	}

	// auto's release path should propagate the error wrapped with
	// the auto: prefix.
	res = r.Monorel(t, "auto")
	if res.ExitCode == 0 {
		t.Errorf("expected non-zero exit from auto, got 0")
	}
	if !strings.Contains(res.Stderr, "auto:") || !strings.Contains(res.Stderr, "tag already exists") {
		t.Errorf("stderr should include 'auto: ... tag already exists':\n%s", res.Stderr)
	}
}

// TestErrors_TrailersFallbackFailure covers Scenario 11: when
// neither signal can fire (HEAD body has no trailer AND no PR
// matches the SHA), monorel tag returns ErrNoReleaseCommit and
// detect-release returns exit 1.
func TestErrors_TrailersFallbackFailure(t *testing.T) {
	r := newScenarioRepo(t, "fallback-fail")
	r.ScaffoldSinglePackage(t, "pkg-a", "pkg-a", "pkg-a")
	// Empty initial commit — no changeset, no release.
	r.CommitAll(t, "init")
	r.PushMain(t)

	// detect-release should exit 1 (not 0, not 2): HEAD has no
	// trailer and no PR matches the SHA.
	res := r.Monorel(t, "detect-release")
	if res.ExitCode != 1 {
		t.Errorf("detect-release exit = %d, want 1\nstdout: %s\nstderr: %s",
			res.ExitCode, res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stderr, "not a release-PR merge") {
		t.Errorf("stderr should mention 'not a release-PR merge':\n%s", res.Stderr)
	}

	// monorel tag should error with ErrNoReleaseCommit.
	res = r.Monorel(t, "tag")
	if res.ExitCode == 0 {
		t.Errorf("expected non-zero exit from tag, got 0")
	}
	if !strings.Contains(res.Stderr, "no monorel-Release: trailer") {
		t.Errorf("stderr missing ErrNoReleaseCommit phrasing:\n%s", res.Stderr)
	}
}

// TestErrors_ValidateConfigErrors covers Scenario 12: monorel
// validate detects:
//   - missing package path
//   - missing changelog parent directory
//   - changeset referencing an unknown package
func TestErrors_ValidateConfigErrors(t *testing.T) {
	r := newScenarioRepo(t, "validate")
	// Don't call ScaffoldSinglePackage — write a deliberately
	// broken monorel.toml.
	r.WriteFile(t, "monorel.toml", `[provider]
name = "github"
owner = "test"
repo = "test"

[packages."pkg-a"]
tag_prefix = "pkg-a"
path = "pkg-a"
changelog = "pkg-a/CHANGELOG.md"
`)
	// Note: pkg-a/ directory does NOT exist; that's the trap.
	r.WriteChangeset(t, "bad", map[string]string{"pkg-z-does-not-exist": "minor"}, "Bug.")
	r.CommitAll(t, "init+broken")
	r.PushMain(t)

	res := r.Monorel(t, "validate")
	if res.ExitCode == 0 {
		t.Errorf("validate should have exited non-zero on broken config\nstdout: %s\nstderr: %s",
			res.Stdout, res.Stderr)
	}
	wantSubstrings := []string{
		"path_missing",
		"changelog_dir_missing",
		"changeset_unknown_package",
	}
	combined := res.Stdout + "\n" + res.Stderr
	for _, want := range wantSubstrings {
		if !strings.Contains(combined, want) {
			t.Errorf("expected validate output to mention %q\nfull output:\n%s", want, combined)
		}
	}
}
