//go:build e2e

package e2e_test

import (
	"sort"
	"strings"
	"testing"
)

// TestBasic_MultiModuleRelease drives a 3-module monorepo (root +
// transports/foo + plugins/bar) with two changesets producing 3
// packages at mixed bumps, end-to-end through feature → squash →
// release. Verifies sub-module go.mod rewriting and non-empty
// Release bodies.
func TestBasic_MultiModuleRelease(t *testing.T) {
	r := newScenarioRepo(t, "multi")
	rootKey, fooKey, barKey := r.ScaffoldMultiModule(t)
	r.CommitAll(t, "init")
	r.PushMain(t)

	r.WriteChangeset(t, "feat", map[string]string{
		rootKey: "minor",
		fooKey:  "minor",
	}, "Add foo transport.")
	r.WriteChangeset(t, "fix", map[string]string{
		barKey: "patch",
	}, "Fix bar.")
	r.CommitAll(t, "feat+fix")
	r.PushMain(t)

	// Feature path: opens release PR.
	r.MonorelOK(t, "auto")

	prs := r.PRs(t, "open")
	if len(prs) != 1 {
		t.Fatalf("expected 1 open PR, got %d", len(prs))
	}
	if !strings.Contains(prs[0].Body, "<!-- monorel-trailers") {
		t.Errorf("PR body missing trailers comment block")
	}

	// Squash-merge.
	r.MergePR(t, prs[0].Number, "squash")
	r.CheckoutMain(t)

	// Release path.
	stdout := r.MonorelOK(t, "auto")
	if !strings.Contains(stdout, "Released 3 package(s)") {
		t.Errorf("auto stdout doesn't claim 3 packages:\n%s", stdout)
	}
	if !strings.Contains(stdout, "detected by api") {
		// Squash-merge drops the trailer; API signal should fire.
		t.Errorf("expected detection by api, got:\n%s", stdout)
	}

	// Verify the three expected tags landed locally + remotely.
	wantTags := []string{"v0.1.0", "transports/foo/v0.1.0", "plugins/bar/v0.0.1"}
	gotLocal := r.LocalTags(t)
	gotRemote := r.RemoteTags(t)
	sort.Strings(gotLocal)
	sort.Strings(gotRemote)
	sort.Strings(wantTags)
	if !sliceEq(gotLocal, wantTags) {
		t.Errorf("local tags: got %v, want %v", gotLocal, wantTags)
	}
	if !sliceEq(gotRemote, wantTags) {
		t.Errorf("remote tags: got %v, want %v", gotRemote, wantTags)
	}

	// Verify Releases have non-empty bodies (regression for the bug
	// found during manual e2e: auto used to call PublishReleases with
	// release.Tag's empty-body Result).
	rels := r.Releases(t)
	if len(rels) != 3 {
		t.Fatalf("expected 3 Releases, got %d", len(rels))
	}
	for _, rel := range rels {
		if strings.TrimSpace(rel.Body) == "" {
			t.Errorf("Release %q has empty body", rel.TagName)
		}
	}

	// Verify go.mod rewriting — `replace` stripped, sibling pinned.
	for _, sub := range []string{"transports/foo", "plugins/bar"} {
		got := readFile(t, r.Local+"/"+sub+"/go.mod")
		if strings.Contains(got, "replace go.example.com") {
			t.Errorf("%s/go.mod still contains replace directive:\n%s", sub, got)
		}
		if !strings.Contains(got, "require go.example.com v0.1.0") {
			t.Errorf("%s/go.mod doesn't pin sibling to v0.1.0:\n%s", sub, got)
		}
	}
}

// TestBasic_EmptyPlanNoop: a docs-only commit with
// no changeset → auto runs but does nothing.
func TestBasic_EmptyPlanNoop(t *testing.T) {
	r := newScenarioRepo(t, "empty")
	r.ScaffoldSinglePackage(t, "pkg-a", "pkg-a", "pkg-a")
	r.CommitAll(t, "init")
	r.PushMain(t)

	stdout := r.MonorelOK(t, "auto")
	if !strings.Contains(stdout, "Plan is empty") && !strings.Contains(stdout, "Nothing to do") {
		// The CLI logs the noop message via rt.Log.Info; if a future
		// refactor changes the wording, this test will catch it.
		t.Errorf("expected empty-plan noop message, got:\n%s", stdout)
	}

	// No PR should be open.
	if prs := r.PRs(t, "open"); len(prs) != 0 {
		t.Errorf("expected 0 open PRs, got %d", len(prs))
	}
}

// TestBasic_InFlightRefresh: open release PR with
// one changeset, push a SECOND changeset, expect the PR to refresh
// with the merged plan.
func TestBasic_InFlightRefresh(t *testing.T) {
	r := newScenarioRepo(t, "refresh")
	r.ScaffoldSinglePackage(t, "pkg-a", "pkg-a", "pkg-a")
	r.WriteChangeset(t, "feat1", map[string]string{"pkg-a": "minor"}, "First feature.")
	r.CommitAll(t, "init+feat1")
	r.PushMain(t)

	r.MonorelOK(t, "auto")
	prs := r.PRs(t, "open")
	if len(prs) != 1 {
		t.Fatalf("expected 1 open PR after first auto, got %d", len(prs))
	}
	first := prs[0]

	// Add a second changeset on the SAME package on main, push, run
	// auto — it should UPDATE the existing PR.
	r.CheckoutMain(t)
	r.WriteChangeset(t, "feat2", map[string]string{"pkg-a": "minor"}, "Second feature.")
	r.CommitAll(t, "feat2")
	r.PushMain(t)
	stdout := r.MonorelOK(t, "auto")

	if !strings.Contains(stdout, "Updated") {
		t.Errorf("expected 'Updated release PR' message, got:\n%s", stdout)
	}

	prs = r.PRs(t, "open")
	if len(prs) != 1 {
		t.Fatalf("expected 1 open PR after refresh, got %d", len(prs))
	}
	if prs[0].Number != first.Number {
		t.Errorf("expected refresh of PR #%d, got new PR #%d", first.Number, prs[0].Number)
	}
}
