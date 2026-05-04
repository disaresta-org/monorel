//go:build e2e

package e2e_test

import (
	"strings"
	"testing"
)

// TestVersioning_InitialReleaseMatrix covers Scenario 7: the first
// release of a package follows monorel's initial-release rules:
//
//	:major → v1.0.0
//	:minor → v0.1.0
//	:patch → v0.0.1
func TestVersioning_InitialReleaseMatrix(t *testing.T) {
	cases := []struct {
		bump string
		want string
	}{
		{"major", "v1.0.0"},
		{"minor", "v0.1.0"},
		{"patch", "v0.0.1"},
	}
	for _, tc := range cases {
		t.Run(tc.bump, func(t *testing.T) {
			r := newScenarioRepo(t, "init-"+tc.bump)
			r.ScaffoldSinglePackage(t, "pkg-a", "pkg-a", "pkg-a")
			r.WriteChangeset(t, "feat", map[string]string{"pkg-a": tc.bump}, "Initial.")
			r.CommitAll(t, "init")
			r.PushMain(t)

			r.MonorelOK(t, "auto")
			prs := r.PRs(t, "open")
			if len(prs) != 1 {
				t.Fatalf("expected 1 open PR, got %d", len(prs))
			}
			r.MergePR(t, prs[0].Number, "squash")
			r.CheckoutMain(t)
			r.MonorelOK(t, "auto")

			tags := r.LocalTags(t)
			want := "pkg-a/" + tc.want
			if len(tags) != 1 || tags[0] != want {
				t.Errorf("tags = %v, want [%s]", tags, want)
			}
		})
	}
}

// TestVersioning_NewPackageMidLife covers Scenario 8: an existing
// package has been released; a new package is registered in
// monorel.toml and gets its initial release while the existing
// package's history is untouched.
func TestVersioning_NewPackageMidLife(t *testing.T) {
	r := newScenarioRepo(t, "newpkg")
	r.ScaffoldSinglePackage(t, "pkg-a", "pkg-a", "pkg-a")
	r.WriteChangeset(t, "feat-a", map[string]string{"pkg-a": "minor"}, "Initial pkg-a.")
	r.CommitAll(t, "init")
	r.PushMain(t)

	// Release pkg-a v0.1.0.
	r.MonorelOK(t, "auto")
	prs := r.PRs(t, "open")
	r.MergePR(t, prs[0].Number, "squash")
	r.CheckoutMain(t)
	r.MonorelOK(t, "auto")

	// Now register pkg-b alongside pkg-a, plus a :major changeset
	// for pkg-b. Expected: pkg-b/v1.0.0 (initial) + pkg-a unchanged.
	r.WriteFile(t, "monorel.toml", strings.ReplaceAll(readFile(t, r.Local+"/monorel.toml"),
		`[packages."pkg-a"]
tag_prefix = "pkg-a"
path = "pkg-a"
changelog = "pkg-a/CHANGELOG.md"
`,
		`[packages."pkg-a"]
tag_prefix = "pkg-a"
path = "pkg-a"
changelog = "pkg-a/CHANGELOG.md"

[packages."pkg-b"]
tag_prefix = "pkg-b"
path = "pkg-b"
changelog = "pkg-b/CHANGELOG.md"
`))
	r.WriteFile(t, "pkg-b/README.md", "# pkg-b\n")
	r.WriteFile(t, "pkg-b/CHANGELOG.md", "# Changelog\n\n## [Unreleased]\n\n")
	r.WriteChangeset(t, "feat-b", map[string]string{"pkg-b": "major"}, "Initial pkg-b.")
	r.CommitAll(t, "feat: register pkg-b")
	r.PushMain(t)

	r.MonorelOK(t, "auto")
	prs = r.PRs(t, "open")
	if len(prs) != 1 {
		t.Fatalf("expected 1 open PR, got %d", len(prs))
	}
	r.MergePR(t, prs[0].Number, "squash")
	r.CheckoutMain(t)
	stdout := r.MonorelOK(t, "auto")
	if !strings.Contains(stdout, "pkg-b/v1.0.0") {
		t.Errorf("expected pkg-b/v1.0.0 in output:\n%s", stdout)
	}

	tags := r.LocalTags(t)
	wantTags := map[string]bool{"pkg-a/v0.1.0": true, "pkg-b/v1.0.0": true}
	if len(tags) != len(wantTags) {
		t.Errorf("tags = %v, want %v", tags, mapKeys(wantTags))
	}
	for _, tag := range tags {
		if !wantTags[tag] {
			t.Errorf("unexpected tag: %s", tag)
		}
	}
}

// TestVersioning_SubModuleOnlyRelease covers Scenario 9: a multi-
// module monorepo where only one sub-module bumps. The other
// packages' tags don't move and their go.mod files aren't touched.
func TestVersioning_SubModuleOnlyRelease(t *testing.T) {
	r := newScenarioRepo(t, "submodule-only")
	rootKey, fooKey, barKey := r.ScaffoldMultiModule(t)
	_, _ = fooKey, barKey

	// Initial release: all three packages.
	r.WriteChangeset(t, "init", map[string]string{
		rootKey: "minor",
		fooKey:  "minor",
		barKey:  "patch",
	}, "Initial.")
	r.CommitAll(t, "init")
	r.PushMain(t)
	r.MonorelOK(t, "auto")
	prs := r.PRs(t, "open")
	r.MergePR(t, prs[0].Number, "squash")
	r.CheckoutMain(t)
	r.MonorelOK(t, "auto")

	// Now bump ONLY plugins/bar.
	r.WriteChangeset(t, "fix", map[string]string{barKey: "patch"}, "Fix bar only.")
	r.CommitAll(t, "fix: bar only")
	r.PushMain(t)
	r.MonorelOK(t, "auto")
	prs = r.PRs(t, "open")
	if len(prs) != 1 {
		t.Fatalf("expected 1 open PR, got %d", len(prs))
	}
	r.MergePR(t, prs[0].Number, "squash")
	r.CheckoutMain(t)
	stdout := r.MonorelOK(t, "auto")

	if !strings.Contains(stdout, "plugins/bar/v0.0.2") {
		t.Errorf("expected plugins/bar/v0.0.2 in output:\n%s", stdout)
	}
	if strings.Contains(stdout, "transports/foo") {
		t.Errorf("transports/foo should NOT be in output (sub-module-only release):\n%s", stdout)
	}
	if strings.Contains(stdout, "v0.2.0") || strings.Contains(stdout, "(initial)") {
		t.Errorf("output suggests a non-bar release happened:\n%s", stdout)
	}

	tags := r.LocalTags(t)
	must := []string{"v0.1.0", "transports/foo/v0.1.0", "plugins/bar/v0.0.1", "plugins/bar/v0.0.2"}
	for _, m := range must {
		found := false
		for _, t2 := range tags {
			if t2 == m {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing tag %q in %v", m, tags)
		}
	}
}

// TestVersioning_MaxBumpPrecedence covers Scenario 1: when two
// changesets bump the same package at different levels, the max
// level wins. Verifies the planner's max-across-changesets rule
// end-to-end against a real release.
func TestVersioning_MaxBumpPrecedence(t *testing.T) {
	cases := []struct {
		name    string // also used as repo-name suffix; keep alphanumeric+dash only
		bump1   string
		bump2   string
		wantTag string // tag suffix; tag prefix is "pkg-a"
	}{
		{"patch-minor", "patch", "minor", "v0.1.0"},
		{"minor-major", "minor", "major", "v1.0.0"},
		{"patch-major", "patch", "major", "v1.0.0"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newScenarioRepo(t, "max-"+tc.name)
			r.ScaffoldSinglePackage(t, "pkg-a", "pkg-a", "pkg-a")
			r.WriteChangeset(t, "first", map[string]string{"pkg-a": tc.bump1}, "First.")
			r.WriteChangeset(t, "second", map[string]string{"pkg-a": tc.bump2}, "Second.")
			r.CommitAll(t, "init+two-changesets")
			r.PushMain(t)

			r.MonorelOK(t, "auto")
			prs := r.PRs(t, "open")
			r.MergePR(t, prs[0].Number, "squash")
			r.CheckoutMain(t)
			r.MonorelOK(t, "auto")

			tags := r.LocalTags(t)
			want := "pkg-a/" + tc.wantTag
			if len(tags) != 1 || tags[0] != want {
				t.Errorf("tags = %v, want [%s]", tags, want)
			}
		})
	}
}

// TestVersioning_OverlappingChangesetsMergeBumps covers Scenario 7:
// two changesets that BOTH bump pkg-a (one :minor, one :patch) plus
// one that bumps pkg-b (:patch). The resulting plan has pkg-a at
// minor (max of minor + patch) and pkg-b at patch. Verifies the
// per-package merge across changeset files.
func TestVersioning_OverlappingChangesetsMergeBumps(t *testing.T) {
	r := newScenarioRepo(t, "overlap")
	r.WriteFile(t, "monorel.toml", `[provider]
name = "gitea"
host = "`+giteaHost+`"
owner = "`+r.Owner+`"
repo = "`+r.Name+`"

[packages."pkg-a"]
tag_prefix = "pkg-a"
path = "pkg-a"
changelog = "pkg-a/CHANGELOG.md"

[packages."pkg-b"]
tag_prefix = "pkg-b"
path = "pkg-b"
changelog = "pkg-b/CHANGELOG.md"
`)
	for _, p := range []string{"pkg-a", "pkg-b"} {
		r.WriteFile(t, p+"/README.md", "# "+p+"\n")
		r.WriteFile(t, p+"/CHANGELOG.md", "# Changelog\n\n## [Unreleased]\n\n")
	}
	r.WriteFile(t, ".changeset/README.md", "# Changesets\n")

	r.WriteChangeset(t, "feat", map[string]string{"pkg-a": "minor"}, "feat A.")
	r.WriteChangeset(t, "fix", map[string]string{"pkg-a": "patch", "pkg-b": "patch"}, "fix A+B.")
	r.CommitAll(t, "init+two-changesets")
	r.PushMain(t)

	r.MonorelOK(t, "auto")
	prs := r.PRs(t, "open")
	r.MergePR(t, prs[0].Number, "squash")
	r.CheckoutMain(t)
	r.MonorelOK(t, "auto")

	tags := r.LocalTags(t)
	want := map[string]bool{"pkg-a/v0.1.0": true, "pkg-b/v0.0.1": true}
	if len(tags) != len(want) {
		t.Errorf("tags = %v, want %v", tags, mapKeys(want))
	}
	for _, tag := range tags {
		if !want[tag] {
			t.Errorf("unexpected tag: %s", tag)
		}
	}
}

func mapKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
