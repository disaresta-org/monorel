//go:build e2e

package e2e_test

import (
	"strings"
	"testing"
)

// TestContent_MarkdownInChangesetBody covers Scenario 12: backticks,
// code fences, and links in a changeset body survive end-to-end —
// they reach the rendered CHANGELOG entry, the rendered PR body,
// and (after release) the Gitea Release body.
//
// monorel doesn't transform markdown, but a regression that
// double-escapes backticks or strips fenced blocks would surface
// here loudly.
func TestContent_MarkdownInChangesetBody(t *testing.T) {
	r := newScenarioRepo(t, "markdown")
	r.ScaffoldSinglePackage(t, "pkg-a", "pkg-a", "pkg-a")

	body := "Adds inline `code`, a [link](https://example.com), and a fenced block:\n\n" +
		"```go\nfmt.Println(\"hello\")\n```\n"
	r.WriteChangeset(t, "feat", map[string]string{"pkg-a": "minor"}, body)
	r.CommitAll(t, "feat: markdown body")
	r.PushMain(t)

	// Feature path: PR opens. Body should preserve the markdown verbatim.
	r.MonorelOK(t, "auto")
	prs := r.PRs(t, "open")
	if len(prs) != 1 {
		t.Fatalf("expected 1 open PR, got %d", len(prs))
	}
	for _, want := range []string{"`code`", "[link](https://example.com)", "```go", "fmt.Println"} {
		if !strings.Contains(prs[0].Body, want) {
			t.Errorf("PR body missing %q\nfull body:\n%s", want, prs[0].Body)
		}
	}

	// Release path. The Gitea Release body should also have the markdown.
	r.MergePR(t, prs[0].Number, "squash")
	r.CheckoutMain(t)
	r.MonorelOK(t, "auto")

	rels := r.Releases(t)
	if len(rels) != 1 {
		t.Fatalf("expected 1 Release, got %d", len(rels))
	}
	for _, want := range []string{"`code`", "fmt.Println"} {
		if !strings.Contains(rels[0].Body, want) {
			t.Errorf("Release body missing %q\nfull body:\n%s", want, rels[0].Body)
		}
	}
}
