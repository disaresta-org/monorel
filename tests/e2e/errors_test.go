//go:build e2e

package e2e_test

import (
	"strings"
	"testing"
)

// TestErrors_TagConflict: re-running auto on a
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

// TestErrors_TrailersFallbackFailure: when
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

// TestErrors_AutoIdempotentTagged: re-running
// auto on a HEAD that's already been tagged should error with the
// tag-conflict path. (Auto's release branch always tries to create
// the derived tags; if they already exist locally, ErrTagExists
// fires.) This pins the contract that auto is NOT a silent no-op
// when the release was already cut.
func TestErrors_AutoIdempotentTagged(t *testing.T) {
	r := newScenarioRepo(t, "idempotent")
	r.ScaffoldSinglePackage(t, "pkg-a", "pkg-a", "pkg-a")
	r.WriteChangeset(t, "feat", map[string]string{"pkg-a": "minor"}, "feat.")
	r.CommitAll(t, "init+feat")
	r.PushMain(t)

	r.MonorelOK(t, "auto")
	prs := r.PRs(t, "open")
	r.MergePR(t, prs[0].Number, "squash")
	r.CheckoutMain(t)
	r.MonorelOK(t, "auto") // creates the tag, runs publish

	// Second auto call on the SAME commit. Detection still says yes
	// (HEAD is the merge of the release PR; both signals would still
	// fire). release.Tag tries to create the tag, errors because it
	// already exists.
	res := r.Monorel(t, "auto")
	if res.ExitCode == 0 {
		t.Errorf("expected non-zero exit on idempotent re-run, got 0\nstdout: %s", res.Stdout)
	}
	if !strings.Contains(res.Stderr, "tag already exists") {
		t.Errorf("stderr missing 'tag already exists':\n%s", res.Stderr)
	}
}

// TestErrors_ValidateStrict: validate --strict
// exits non-zero when only warnings are present. The plain validate
// run on the same state would exit 0.
func TestErrors_ValidateStrict(t *testing.T) {
	r := newScenarioRepo(t, "validate-strict")
	r.ScaffoldSinglePackage(t, "pkg-a", "pkg-a", "pkg-a")
	r.CommitAll(t, "init")

	// Construct a warning-only state by tagging a non-semver tag.
	// validate --check-tags surfaces that as a warning. We use the
	// combination: --check-tags --strict together, since plain
	// validate doesn't walk tags by default.
	r.run(t, "git", "tag", "pkg-a/garbage-not-semver")

	// validate --check-tags alone: warnings, exits 0.
	res := r.Monorel(t, "validate", "--check-tags")
	if res.ExitCode != 0 {
		t.Errorf("validate --check-tags should exit 0 on warnings; got %d\nstdout: %s\nstderr: %s",
			res.ExitCode, res.Stdout, res.Stderr)
	}

	// validate --check-tags --strict: warnings → exit 2.
	res = r.Monorel(t, "validate", "--check-tags", "--strict")
	if res.ExitCode != 2 {
		t.Errorf("validate --check-tags --strict should exit 2 on warnings; got %d\nstdout: %s\nstderr: %s",
			res.ExitCode, res.Stdout, res.Stderr)
	}
}

// TestErrors_ValidateCheckTags pins three contracts: --check-tags
// exits 0 on a warning-only state (--strict is required to escalate),
// the invalid tag names appear in the report, and a regression that
// flags the valid tag as a warning fails the test.
func TestErrors_ValidateCheckTags(t *testing.T) {
	r := newScenarioRepo(t, "check-tags")
	r.ScaffoldSinglePackage(t, "pkg-a", "pkg-a", "pkg-a")
	r.CommitAll(t, "init")
	r.run(t, "git", "tag", "pkg-a/v1.2.3")       // valid
	r.run(t, "git", "tag", "pkg-a/garbage")      // invalid; warning expected
	r.run(t, "git", "tag", "pkg-a/v-not-semver") // also invalid

	res := r.Monorel(t, "validate", "--check-tags")
	if res.ExitCode != 0 {
		t.Errorf("validate --check-tags on warning-only state should exit 0; got %d\nstdout: %s\nstderr: %s",
			res.ExitCode, res.Stdout, res.Stderr)
	}
	combined := res.Stdout + "\n" + res.Stderr
	for _, want := range []string{"pkg-a/garbage", "pkg-a/v-not-semver"} {
		if !strings.Contains(combined, want) {
			t.Errorf("--check-tags should mention %q:\n%s", want, combined)
		}
	}
	// Regression guard: valid tag must not appear on a warning line.
	for _, line := range strings.Split(combined, "\n") {
		if strings.Contains(line, "pkg-a/v1.2.3") &&
			(strings.Contains(line, "warning") || strings.Contains(line, "tag_not_semver")) {
			t.Errorf("valid tag pkg-a/v1.2.3 should not be flagged:\n%s", line)
		}
	}
}

// TestErrors_ValidateConfigErrors: monorel
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
