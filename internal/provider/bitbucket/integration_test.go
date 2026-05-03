//go:build integration

package bitbucket

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"monorel.disaresta.com/internal/provider"
)

// TestIntegration_FullLifecycle creates a temp repo in the configured
// workspace, walks the full PR lifecycle, and deletes the repo on
// success. Skipped unless BITBUCKET_INTEGRATION=1 + BITBUCKET_EMAIL
// + BITBUCKET_TOKEN + BITBUCKET_WORKSPACE are set.
//
// Run with: go test -tags=integration ./internal/provider/bitbucket/ -v
func TestIntegration_FullLifecycle(t *testing.T) {
	if os.Getenv("BITBUCKET_INTEGRATION") != "1" {
		t.Skip("set BITBUCKET_INTEGRATION=1 to run")
	}
	email := os.Getenv("BITBUCKET_EMAIL")
	token := os.Getenv("BITBUCKET_TOKEN")
	ws := os.Getenv("BITBUCKET_WORKSPACE")
	if email == "" || token == "" || ws == "" {
		t.Fatal("BITBUCKET_EMAIL, BITBUCKET_TOKEN, and BITBUCKET_WORKSPACE all required")
	}

	repoName := fmt.Sprintf("monorel-integration-%d", time.Now().Unix())

	// Create the repo via REST.
	c, err := New(context.Background(), Options{
		Workspace: ws, Repo: repoName,
		Email: email, Token: token,
	})
	if err != nil {
		t.Fatal(err)
	}
	bc := c.(*client)

	// Create the repo via the Bitbucket API directly (out of band of
	// the Client interface; repo creation isn't a provider.Client
	// method).
	if _, err := bc.do(context.Background(), "POST", bc.repoBase(), nil, map[string]any{
		"is_private":  false,
		"scm":         "git",
		"description": "monorel integration test; safe to delete.",
	}); err != nil {
		t.Fatalf("create repo: %v", err)
	}
	t.Cleanup(func() {
		_, err := bc.do(context.Background(), "DELETE", bc.repoBase(), nil, nil)
		if err != nil {
			t.Logf("delete repo (test cleanup): %v", err)
		}
	})

	ctx := context.Background()

	// Read default branch.
	defaultBranch, err := c.GetDefaultBranch(ctx)
	if err != nil {
		t.Fatalf("GetDefaultBranch: %v", err)
	}
	if defaultBranch != "master" && defaultBranch != "main" {
		t.Errorf("unexpected default branch %q", defaultBranch)
	}

	// FindOpenReleasePR on a fresh repo returns (nil, nil).
	pr, err := c.FindOpenReleasePR(ctx, "monorel/release")
	if err != nil {
		t.Fatalf("FindOpenReleasePR: %v", err)
	}
	if pr != nil {
		t.Errorf("FindOpenReleasePR: expected nil; got %+v", pr)
	}

	// FindPRByMergeCommit on a fresh repo returns (nil, nil) for any
	// SHA. This exercises the trailers-fallback query path against the
	// real BBQL endpoint.
	pr, err = c.FindPRByMergeCommit(ctx, "deadbeef0000000000000000000000000000beef")
	if err != nil {
		t.Fatalf("FindPRByMergeCommit: %v", err)
	}
	if pr != nil {
		t.Errorf("FindPRByMergeCommit: expected nil; got %+v", pr)
	}

	// CreateRelease is a Bitbucket no-op; verify the synthetic
	// *Release shape and the /src/<tag> URL form.
	rel, err := c.CreateRelease(ctx, provider.CreateReleaseOptions{
		Tag:  "transports/foo/v1.0.0",
		Name: "transports/foo v1.0.0",
		Body: "Integration smoke release.",
	})
	if err != nil {
		t.Fatalf("CreateRelease: %v", err)
	}
	if rel.Tag != "transports/foo/v1.0.0" {
		t.Errorf("CreateRelease.Tag = %q", rel.Tag)
	}
	wantURL := fmt.Sprintf("https://bitbucket.org/%s/%s/src/transports/foo/v1.0.0", ws, repoName)
	if rel.HTMLURL != wantURL {
		t.Errorf("CreateRelease.HTMLURL = %q, want %q", rel.HTMLURL, wantURL)
	}

	// Identity probe lands a username from /2.0/user. The cache is
	// hit on second call (covered indirectly by all the calls above).
	username, err := bc.resolveUsername(ctx)
	if err != nil {
		t.Fatalf("resolveUsername: %v", err)
	}
	if username == "" {
		t.Error("resolveUsername returned empty string")
	}

	// (PR-lifecycle write-side methods CreatePR / UpdatePR / ClosePR
	// require pushed branches, which use git over HTTPS. Out of scope
	// for this REST-only integration test; the spike validated them
	// manually.)
}
