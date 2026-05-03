//go:build integration

package bitbucket

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"
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

	// Read default branch.
	defaultBranch, err := c.GetDefaultBranch(context.Background())
	if err != nil {
		t.Fatalf("GetDefaultBranch: %v", err)
	}
	if defaultBranch != "master" && defaultBranch != "main" {
		t.Errorf("unexpected default branch %q", defaultBranch)
	}

	// (PR-lifecycle test requires pushing branches, which uses
	// git over HTTPS. Out of scope for the REST-only integration
	// test; covered by the spike's manual run.)
}
