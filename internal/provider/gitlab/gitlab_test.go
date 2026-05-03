package gitlab_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"monorel.disaresta.com/internal/provider/gitlab"
)

func TestNew_RejectsMissingOwnerRepo(t *testing.T) {
	cases := []struct {
		name string
		opts gitlab.Options
	}{
		{"no owner", gitlab.Options{Repo: "r", Token: "t"}},
		{"no repo", gitlab.Options{Owner: "o", Token: "t"}},
		{"neither", gitlab.Options{Token: "t"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := gitlab.New(context.Background(), tc.opts)
			if !errors.Is(err, gitlab.ErrMissingOwnerRepo) {
				t.Errorf("err = %v, want ErrMissingOwnerRepo", err)
			}
		})
	}
}

func TestNew_RejectsMissingToken(t *testing.T) {
	_, err := gitlab.New(context.Background(), gitlab.Options{Owner: "o", Repo: "r"})
	if !errors.Is(err, gitlab.ErrMissingToken) {
		t.Errorf("err = %v, want ErrMissingToken", err)
	}
}

func TestFindPRByMergeCommit_GitLab(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// EscapedPath reflects how the SDK URL-encoded the project ID.
		wantPath := "/api/v4/projects/team%2Fwidget/repository/commits/abc123/merge_requests"
		if r.URL.EscapedPath() != wantPath {
			t.Errorf("path = %s, want %s", r.URL.EscapedPath(), wantPath)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `[{
			"iid": 42,
			"state": "merged",
			"title": "release",
			"description": "release body",
			"source_branch": "monorel/release",
			"merge_commit_sha": "abc123",
			"web_url": "https://gitlab.com/team/widget/-/merge_requests/42"
		}]`)
	}))
	defer srv.Close()

	c, err := gitlab.New(context.Background(), gitlab.Options{
		Owner: "team",
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
		t.Errorf("got %+v, want MR #42 with MergedSHA=abc123", got)
	}
}

func TestNew_AcceptsSubgroupOwner(t *testing.T) {
	// Owner can contain slashes for nested sub-groups. The
	// constructor doesn't validate the structure; the SDK's first
	// API call is what would error against an unreachable instance.
	// Here we only verify the constructor accepts it.
	_, err := gitlab.New(context.Background(), gitlab.Options{
		Owner: "team/platform",
		Repo:  "widget",
		Token: "test-token",
		Host:  "https://gitlab.example.com",
	})
	if err != nil {
		t.Errorf("constructor rejected sub-group owner: %v", err)
	}
}
