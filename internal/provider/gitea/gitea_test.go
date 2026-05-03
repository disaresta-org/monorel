package gitea_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"monorel.disaresta.com/internal/provider/gitea"
)

func TestNew_RejectsMissingOwnerRepo(t *testing.T) {
	cases := []struct {
		name string
		opts gitea.Options
	}{
		{"no owner", gitea.Options{Repo: "r", Host: "gitea.example.com"}},
		{"no repo", gitea.Options{Owner: "o", Host: "gitea.example.com"}},
		{"neither", gitea.Options{Host: "gitea.example.com"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := gitea.New(context.Background(), tc.opts)
			if !errors.Is(err, gitea.ErrMissingOwnerRepo) {
				t.Errorf("err = %v, want ErrMissingOwnerRepo", err)
			}
		})
	}
}

func TestNew_RejectsMissingHost(t *testing.T) {
	_, err := gitea.New(context.Background(), gitea.Options{Owner: "o", Repo: "r"})
	if !errors.Is(err, gitea.ErrMissingHost) {
		t.Errorf("err = %v, want ErrMissingHost", err)
	}
}

func TestFindPRByMergeCommit_Gitea(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/api/v1/version"):
			fmt.Fprintln(w, `{"version":"1.23.0"}`)
		case strings.Contains(r.URL.Path, "/pulls"):
			fmt.Fprintln(w, `[{
				"number": 42,
				"state": "closed",
				"merged": true,
				"title": "release",
				"body": "release body",
				"head": {"ref": "monorel/release"},
				"merge_commit_sha": "abc123",
				"html_url": "https://gitea.example.com/acme/widget/pulls/42"
			}]`)
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c, err := gitea.New(context.Background(), gitea.Options{
		Owner: "acme",
		Repo:  "widget",
		Host:  srv.URL,
		Token: "tok",
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.FindPRByMergeCommit(context.Background(), "abc123")
	if err != nil {
		t.Fatalf("FindPRByMergeCommit: %v", err)
	}
	if got == nil || got.Number != 42 || got.MergedSHA != "abc123" {
		t.Errorf("got %+v, want #42 with MergedSHA=abc123", got)
	}
}
