package github

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	gogh "github.com/google/go-github/v68/github"
)

// newTestClient builds a *client whose underlying go-github Client points
// at the given httptest server. Bypasses [New] (which insists on building a
// real https URL) so the tests can drive every code path against a local
// fake.
func newTestClient(t *testing.T, baseURL, owner, repo string) *client {
	t.Helper()
	u, err := url.Parse(strings.TrimRight(baseURL, "/") + "/")
	if err != nil {
		t.Fatalf("parse baseURL: %v", err)
	}
	gh := gogh.NewClient(nil)
	gh.BaseURL = u
	gh.UploadURL = u
	return &client{owner: owner, repo: repo, gh: gh}
}

func TestFindPRByMergeCommit(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/acme/widget/commits/abc123/pulls", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("method = %s, want GET", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `[{
			"number": 42,
			"state": "closed",
			"title": "release",
			"body": "release body",
			"head": {"ref": "monorel/release"},
			"merge_commit_sha": "abc123",
			"merged_at": "2026-05-03T00:00:00Z",
			"html_url": "https://github.com/acme/widget/pull/42"
		}]`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := newTestClient(t, srv.URL, "acme", "widget")
	got, err := c.FindPRByMergeCommit(context.Background(), "abc123")
	if err != nil {
		t.Fatalf("FindPRByMergeCommit: %v", err)
	}
	if got == nil {
		t.Fatal("expected PR; got nil")
	}
	if got.Number != 42 || got.MergedSHA != "abc123" {
		t.Errorf("got %+v, want #42 with MergedSHA=abc123", got)
	}
}

func TestFindPRByMergeCommit_NoMatch(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/acme/widget/commits/zzz999/pulls", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `[]`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := newTestClient(t, srv.URL, "acme", "widget")
	got, err := c.FindPRByMergeCommit(context.Background(), "zzz999")
	if err != nil {
		t.Fatalf("FindPRByMergeCommit: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil; got %+v", got)
	}
}
