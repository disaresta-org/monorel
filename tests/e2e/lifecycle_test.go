//go:build e2e

package e2e_test

import (
	"strings"
	"testing"
)

// TestLifecycle_PreReleaseRcCycle: the rc cycle.
//
//	pre enter rc → release rc.0 → release rc.1 → pre exit → release stable
//
// Verifies tags are formatted correctly across the transition. The
// rc.0 -> rc.1 step is fragile against Gitea's async-mergeable
// computation; the test polls and retries.
func TestLifecycle_PreReleaseRcCycle(t *testing.T) {
	r := newScenarioRepo(t, "pre")
	r.ScaffoldSinglePackage(t, "pkg-a", "pkg-a", "pkg-a")
	r.WriteChangeset(t, "feat", map[string]string{"pkg-a": "minor"}, "Initial feature.")
	r.CommitAll(t, "init+feat")
	r.PushMain(t)

	// Enter pre-release mode.
	r.MonorelOK(t, "pre", "enter", "rc")
	r.CommitAll(t, "chore: enter rc pre-release mode")
	r.PushMain(t)

	// rc.0 release.
	r.MonorelOK(t, "auto")
	prs := r.PRs(t, "open")
	if len(prs) != 1 {
		t.Fatalf("expected 1 open PR for rc.0, got %d", len(prs))
	}
	r.MergePR(t, prs[0].Number, "squash")
	r.CheckoutMain(t)
	r.MonorelOK(t, "auto")

	tags := r.LocalTags(t)
	if !contains(tags, "pkg-a/v0.1.0-rc.0") {
		t.Errorf("missing rc.0 tag in %v", tags)
	}

	// Add another changeset; expect rc.1.
	r.WriteChangeset(t, "more", map[string]string{"pkg-a": "patch"}, "More polish.")
	r.CommitAll(t, "chore: more polish")
	r.PushMain(t)

	r.MonorelOK(t, "auto")
	prs = r.PRs(t, "open")
	if len(prs) != 1 {
		t.Fatalf("expected 1 open PR for rc.1, got %d", len(prs))
	}
	r.MergePR(t, prs[0].Number, "squash")
	r.CheckoutMain(t)
	r.MonorelOK(t, "auto")

	tags = r.LocalTags(t)
	if !contains(tags, "pkg-a/v0.1.0-rc.1") {
		t.Errorf("missing rc.1 tag in %v", tags)
	}

	// Exit pre mode and cut a stable.
	r.MonorelOK(t, "pre", "exit")
	r.CommitAll(t, "chore: exit pre-release mode")
	r.PushMain(t)

	r.MonorelOK(t, "auto")
	prs = r.PRs(t, "open")
	if len(prs) != 1 {
		t.Fatalf("expected 1 open PR for stable, got %d", len(prs))
	}
	r.MergePR(t, prs[0].Number, "squash")
	r.CheckoutMain(t)
	r.MonorelOK(t, "auto")

	tags = r.LocalTags(t)
	if !contains(tags, "pkg-a/v0.1.0") {
		t.Errorf("missing stable v0.1.0 tag in %v", tags)
	}
}

// TestLifecycle_ManuallyClosedReleasePR: an
// open release PR that the user manually closes (without merging).
// The next auto run should open a fresh PR.
func TestLifecycle_ManuallyClosedReleasePR(t *testing.T) {
	r := newScenarioRepo(t, "closed-pr")
	r.ScaffoldSinglePackage(t, "pkg-a", "pkg-a", "pkg-a")
	r.WriteChangeset(t, "feat", map[string]string{"pkg-a": "minor"}, "Feature.")
	r.CommitAll(t, "init+feat")
	r.PushMain(t)

	r.MonorelOK(t, "auto")
	prs := r.PRs(t, "open")
	if len(prs) != 1 {
		t.Fatalf("expected 1 open PR, got %d", len(prs))
	}
	first := prs[0].Number
	r.ClosePR(t, first)

	// Switch back to main so the next auto run reads the still-pending
	// changesets from the working tree (after the first auto, the
	// local tree is on monorel/release with the changesets consumed).
	r.CheckoutMain(t)

	// auto with no new commits — the changeset is still pending on
	// main, the previous PR is closed, so auto should open a fresh PR.
	r.MonorelOK(t, "auto")
	prs = r.PRs(t, "open")
	if len(prs) != 1 {
		t.Fatalf("expected 1 open PR after closed-PR refresh, got %d", len(prs))
	}
	if prs[0].Number == first {
		t.Errorf("expected a NEW PR; got the same PR #%d", first)
	}
}

// TestLifecycle_DoctorRevival drives the doctor command against a
// constructed-state revival: a `.changeset/*.md` that was previously
// deleted by a chore(release) commit reappears on disk. Constructing
// the state directly avoids Gitea's modify/delete merge-conflict
// refusal that blocks the natural-merge variant.
func TestLifecycle_DoctorRevival(t *testing.T) {
	r := newScenarioRepo(t, "doctor")
	r.ScaffoldSinglePackage(t, "pkg-a", "pkg-a", "pkg-a")
	r.WriteChangeset(t, "feat", map[string]string{"pkg-a": "minor"}, "Initial feature.")
	r.CommitAll(t, "init+feat")
	r.PushMain(t)

	// Cut a release.
	r.MonorelOK(t, "auto")
	prs := r.PRs(t, "open")
	r.MergePR(t, prs[0].Number, "squash")
	r.CheckoutMain(t)
	r.MonorelOK(t, "auto")

	// Re-introduce the previously-consumed changeset directly on
	// main. This simulates a stale-branch + squash-merge revival.
	r.WriteChangeset(t, "feat", map[string]string{"pkg-a": "minor"}, "Initial feature.")
	r.CommitAll(t, "feat: revive feat.md (simulated stale-branch revival)")
	r.PushMain(t)

	// doctor should flag it.
	res := r.Monorel(t, "doctor")
	combined := res.Stdout + "\n" + res.Stderr
	if !strings.Contains(combined, "revived-changeset") {
		t.Errorf("doctor didn't surface revived-changeset finding\nstdout: %s\nstderr: %s",
			res.Stdout, res.Stderr)
	}
	if !strings.Contains(combined, "feat.md") {
		t.Errorf("doctor finding doesn't name the file\nfull output:\n%s", combined)
	}
}

// TestLifecycle_NoopAfterRelease: a release was
// already cut; a docs-only commit lands on top of the chore(release)
// merge; running auto again should be a no-op (no new PR opened, no
// re-tag attempt). The detection path returns "feature" because HEAD
// is past the release commit, and the planner sees no pending
// changesets.
func TestLifecycle_NoopAfterRelease(t *testing.T) {
	r := newScenarioRepo(t, "noop-after")
	r.ScaffoldSinglePackage(t, "pkg-a", "pkg-a", "pkg-a")
	r.WriteChangeset(t, "feat", map[string]string{"pkg-a": "minor"}, "feat.")
	r.CommitAll(t, "init+feat")
	r.PushMain(t)

	r.MonorelOK(t, "auto")
	prs := r.PRs(t, "open")
	r.MergePR(t, prs[0].Number, "squash")
	r.CheckoutMain(t)
	r.MonorelOK(t, "auto") // tags + publishes

	// Push a docs commit (no changeset). HEAD is now past the
	// chore(release) commit.
	r.WriteFile(t, "pkg-a/README.md", "# pkg-a\n\nUpdated docs.\n")
	r.CommitAll(t, "docs: small update")
	r.PushMain(t)

	stdout := r.MonorelOK(t, "auto")
	if !strings.Contains(stdout, "Plan is empty") && !strings.Contains(stdout, "Nothing to do") {
		t.Errorf("expected 'Plan is empty' noop after release; got:\n%s", stdout)
	}
	if open := r.PRs(t, "open"); len(open) != 0 {
		t.Errorf("expected 0 open PRs after noop, got %d", len(open))
	}
}

// TestLifecycle_PreReleaseCounter: three
// successive rc releases produce rc.0, rc.1, rc.2 (the per-package
// counter increments by 1 each time, never resetting).
func TestLifecycle_PreReleaseCounter(t *testing.T) {
	r := newScenarioRepo(t, "pre-counter")
	r.ScaffoldSinglePackage(t, "pkg-a", "pkg-a", "pkg-a")
	r.WriteChangeset(t, "feat", map[string]string{"pkg-a": "minor"}, "Initial.")
	r.CommitAll(t, "init+feat")
	r.PushMain(t)

	r.MonorelOK(t, "pre", "enter", "rc")
	r.CommitAll(t, "chore: enter rc")
	r.PushMain(t)

	cutOne := func(label string, idx int) {
		t.Helper()
		r.WriteChangeset(t, "rc-"+label, map[string]string{"pkg-a": "patch"}, "rc "+label+".")
		r.CommitAll(t, "chore: rc "+label)
		r.PushMain(t)

		r.MonorelOK(t, "auto")
		prs := r.PRs(t, "open")
		if len(prs) != 1 {
			t.Fatalf("rc.%d: expected 1 open PR, got %d", idx, len(prs))
		}
		r.MergePR(t, prs[0].Number, "squash")
		r.CheckoutMain(t)
		r.MonorelOK(t, "auto")
	}
	// rc.0 reuses the initial `feat` changeset committed above; rc.1
	// and rc.2 each drop a fresh sentinel changeset via cutOne so the
	// planner has work to do.
	r.MonorelOK(t, "auto")
	prs := r.PRs(t, "open")
	r.MergePR(t, prs[0].Number, "squash")
	r.CheckoutMain(t)
	r.MonorelOK(t, "auto")
	if !contains(r.LocalTags(t), "pkg-a/v0.1.0-rc.0") {
		t.Fatalf("rc.0 missing: %v", r.LocalTags(t))
	}

	cutOne("a", 1)
	if !contains(r.LocalTags(t), "pkg-a/v0.1.0-rc.1") {
		t.Errorf("rc.1 missing: %v", r.LocalTags(t))
	}
	cutOne("b", 2)
	if !contains(r.LocalTags(t), "pkg-a/v0.1.0-rc.2") {
		t.Errorf("rc.2 missing: %v", r.LocalTags(t))
	}
}

// TestLifecycle_StaleReleaseBranchOverwrite proves that a stale
// monorel/release branch (from a previous run with a different
// package selection) is force-pushed cleanly on the next auto run,
// not cherry-picked on top of.
func TestLifecycle_StaleReleaseBranchOverwrite(t *testing.T) {
	r := newScenarioRepo(t, "stale-release")
	rootKey, fooKey, _ := r.ScaffoldMultiModule(t)
	r.WriteChangeset(t, "old", map[string]string{rootKey: "minor"}, "old.")
	r.CommitAll(t, "init+old changeset")
	r.PushMain(t)

	// First auto: pushes monorel/release with the root-only plan.
	r.MonorelOK(t, "auto")
	r.CheckoutMain(t)

	// Replace the changeset set: drop old, add a new one for foo.
	// This simulates a contributor who reverted the original feature
	// and added a different one.
	r.run(t, "git", "rm", "-q", ".changeset/old.md")
	r.WriteChangeset(t, "new", map[string]string{fooKey: "minor"}, "new.")
	r.CommitAll(t, "swap: drop old, add new")
	r.PushMain(t)

	// Second auto: must overwrite monorel/release with the new plan
	// rather than carry old's state.
	r.MonorelOK(t, "auto")

	prs := r.PRs(t, "open")
	if len(prs) != 1 {
		t.Fatalf("expected 1 open PR after overwrite, got %d", len(prs))
	}
	body := prs[0].Body
	// Independent assertions: stale state must be GONE, fresh state
	// must be PRESENT. A merged-state regression (both visible) must
	// fail the test, which the previous compound condition allowed.
	if strings.Contains(body, "go.example.com v") {
		t.Errorf("PR body still references old root plan:\n%s", body)
	}
	if !strings.Contains(body, "transports/foo") {
		t.Errorf("PR body should mention transports/foo (the new plan):\n%s", body)
	}
}

// TestLifecycle_ConcurrentContributors: PR A
// merges first; release PR opens with A's package; THEN PR B merges
// with a different package; refresh should show both packages.
func TestLifecycle_ConcurrentContributors(t *testing.T) {
	r := newScenarioRepo(t, "concurrent")
	rootKey, fooKey, barKey := r.ScaffoldMultiModule(t)
	_, _ = rootKey, fooKey
	r.CommitAll(t, "init")
	r.PushMain(t)

	// Contributor A's changeset.
	r.WriteChangeset(t, "feat-foo", map[string]string{fooKey: "minor"}, "A: foo.")
	r.CommitAll(t, "feat: contributor A")
	r.PushMain(t)
	r.MonorelOK(t, "auto") // release PR opens with foo.

	prs := r.PRs(t, "open")
	if len(prs) != 1 {
		t.Fatalf("expected 1 open PR after A's merge, got %d", len(prs))
	}
	first := prs[0].Number

	// Now B's changeset lands (without merging the release PR).
	r.CheckoutMain(t)
	r.WriteChangeset(t, "fix-bar", map[string]string{barKey: "patch"}, "B: bar.")
	r.CommitAll(t, "fix: contributor B")
	r.PushMain(t)
	stdout := r.MonorelOK(t, "auto") // refresh PR with both A + B.

	if !strings.Contains(stdout, "Updated") {
		t.Errorf("expected 'Updated release PR' (PR refresh), got:\n%s", stdout)
	}

	prs = r.PRs(t, "open")
	if len(prs) != 1 || prs[0].Number != first {
		t.Errorf("expected 1 open PR (#%d), got %d (or different number): %+v", first, len(prs), prs)
	}
	body := prs[0].Body
	for _, want := range []string{"transports/foo", "plugins/bar"} {
		if !strings.Contains(body, want) {
			t.Errorf("refreshed PR body missing %q:\n%s", want, body)
		}
	}
}

// TestLifecycle_PreModeErrors: pre exit outside
// pre mode is a graceful no-op (exits 0, prints "nothing to exit"),
// while pre enter while already in pre mode errors.
func TestLifecycle_PreModeErrors(t *testing.T) {
	r := newScenarioRepo(t, "pre-errors")
	r.ScaffoldSinglePackage(t, "pkg-a", "pkg-a", "pkg-a")
	r.CommitAll(t, "init")
	r.PushMain(t)

	// pre exit outside pre mode → no-op (exits 0).
	res := r.Monorel(t, "pre", "exit")
	if res.ExitCode != 0 {
		t.Errorf("pre exit outside pre mode should exit 0 (no-op); got %d\nstdout: %s\nstderr: %s",
			res.ExitCode, res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stdout, "not in pre-release mode") {
		t.Errorf("pre exit no-op should mention 'not in pre-release mode'; got:\n%s",
			res.Stdout)
	}

	// Enter pre mode.
	r.MonorelOK(t, "pre", "enter", "rc")

	// pre enter again → error. Pin the message so a future rewrite
	// can't silently weaken the contract to a no-op.
	res = r.Monorel(t, "pre", "enter", "beta")
	if res.ExitCode == 0 {
		t.Errorf("pre enter while already in pre mode should error; got 0 exit\nstdout: %s\nstderr: %s",
			res.Stdout, res.Stderr)
	}
	combined := res.Stdout + "\n" + res.Stderr
	if !strings.Contains(combined, "already in pre-release mode") {
		t.Errorf("expected 'already in pre-release mode' in output; got:\n%s", combined)
	}
}

// TestLifecycle_DoctorCleanState: a fresh repo
// with no chore(release) commits ever produces no doctor findings.
// The negative complement to TestLifecycle_DoctorRevival.
func TestLifecycle_DoctorCleanState(t *testing.T) {
	r := newScenarioRepo(t, "doctor-clean")
	r.ScaffoldSinglePackage(t, "pkg-a", "pkg-a", "pkg-a")
	r.WriteChangeset(t, "feat", map[string]string{"pkg-a": "minor"}, "feat.")
	r.CommitAll(t, "init+feat")
	r.PushMain(t)

	res := r.Monorel(t, "doctor")
	if res.ExitCode != 0 {
		t.Errorf("doctor on clean state should exit 0; got %d\nstdout: %s\nstderr: %s",
			res.ExitCode, res.Stdout, res.Stderr)
	}
	combined := res.Stdout + "\n" + res.Stderr
	if !strings.Contains(combined, "No findings") && !strings.Contains(combined, "looks healthy") {
		t.Errorf("doctor on clean state should report no findings\nfull output:\n%s", combined)
	}
}
