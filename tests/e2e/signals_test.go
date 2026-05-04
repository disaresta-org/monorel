//go:build e2e

package e2e_test

import (
	"strings"
	"testing"
)

// TestSignals_MergeStrategies: a release PR
// merged with each of squash / rebase / merge-commit produces a
// successfully-tagged release. Detection sources differ:
//   - squash:       trailer dropped → API signal fires
//   - rebase:       source body preserved → trailer signal fires
//   - merge-commit: sparse merge body → API signal fires
//
// The release outcome is the same across all three; only the
// "(detected by ...)" log line differs.
func TestSignals_MergeStrategies(t *testing.T) {
	for _, strategy := range []string{"squash", "rebase", "merge"} {
		t.Run(strategy, func(t *testing.T) {
			r := newScenarioRepo(t, "merge-"+strategy)
			r.ScaffoldSinglePackage(t, "pkg-a", "pkg-a", "pkg-a")
			r.WriteChangeset(t, "feat", map[string]string{"pkg-a": "minor"}, "feat.")
			r.CommitAll(t, "init+feat")
			r.PushMain(t)

			r.MonorelOK(t, "auto")
			prs := r.PRs(t, "open")
			if len(prs) != 1 {
				t.Fatalf("expected 1 open PR, got %d", len(prs))
			}
			r.MergePR(t, prs[0].Number, strategy)
			r.CheckoutMain(t)

			stdout := r.MonorelOK(t, "auto")
			if !strings.Contains(stdout, "pkg-a/v0.1.0") {
				t.Errorf("missing tag in output:\n%s", stdout)
			}
			tags := r.LocalTags(t)
			if len(tags) != 1 || tags[0] != "pkg-a/v0.1.0" {
				t.Errorf("got tags %v, want [pkg-a/v0.1.0]", tags)
			}
		})
	}
}

// TestSignals_DirectPushNoPR: a release commit
// pushed directly to main without going through the always-open PR
// pattern. The trailer signal fires because the commit body is
// preserved verbatim; FindPRByMergeCommit returns nil because no PR
// matches the SHA.
func TestSignals_DirectPushNoPR(t *testing.T) {
	r := newScenarioRepo(t, "direct")
	r.ScaffoldSinglePackage(t, "pkg-a", "pkg-a", "pkg-a")
	r.WriteChangeset(t, "feat", map[string]string{"pkg-a": "minor"}, "Direct push.")
	r.CommitAll(t, "init+feat")
	r.PushMain(t)

	// monorel apply directly on main (skips the release-PR pattern).
	stdout := r.MonorelOK(t, "apply")
	if !strings.Contains(stdout, "Applied 1 release(s)") {
		t.Errorf("apply didn't mention release count:\n%s", stdout)
	}
	r.run(t, "git", "push", "-q", "origin", "main")

	// Verify HEAD has the trailer.
	if !strings.Contains(r.HeadMessage(t), "monorel-Release:") {
		t.Errorf("HEAD missing trailer after apply:\n%s", r.HeadMessage(t))
	}

	// auto should detect via trailer (no PR exists).
	stdout = r.MonorelOK(t, "auto")
	if !strings.Contains(stdout, "detected by trailer") {
		t.Errorf("expected 'detected by trailer', got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "pkg-a/v0.1.0") {
		t.Errorf("missing tag:\n%s", stdout)
	}
}
