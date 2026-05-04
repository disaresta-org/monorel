//go:build e2e

package e2e_test

import (
	"strings"
	"testing"
)

// TestLifecycle_PreReleaseRcCycle covers Scenario 4: the rc cycle.
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

// TestLifecycle_ManuallyClosedReleasePR covers Scenario 13: an
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

// TestLifecycle_DoctorRevival covers Scenario 14 (constructed
// state): the doctor command flags a `.changeset/*.md` that was
// previously deleted by a chore(release) commit but is back on
// disk. Constructing the state directly avoids Gitea's
// modify/delete merge-conflict refusal that blocks the
// natural-merge variant.
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

func contains(slice []string, s string) bool {
	for _, e := range slice {
		if e == s {
			return true
		}
	}
	return false
}
