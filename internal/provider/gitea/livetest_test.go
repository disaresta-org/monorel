//go:build livetest

// Run: go test -tags=livetest ./internal/provider/gitea
//
// Validates the Gitea provider against a real Gitea instance. See
// the package's README or the bottom of this file for the container
// setup the tests expect.
package gitea_test

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	gtsdk "code.gitea.io/sdk/gitea"

	"monorel.disaresta.com/internal/provider"
	"monorel.disaresta.com/internal/provider/gitea"
)

// Setup expectations:
//
//   - A Gitea instance reachable at $MONOREL_GITEA_HOST (default "localhost:3000").
//   - $MONOREL_GITEA_TOKEN: an access token for an account with admin / write
//     access to the test repo.
//   - $MONOREL_GITEA_OWNER and $MONOREL_GITEA_REPO: the owner / repo to run
//     against. Repo must be empty or accept new branches; the test creates
//     and closes a PR each run.
//
// Quick start with Docker:
//
//   docker run -d --name monorel-gitea-test \
//     -p 3000:3000 -p 222:22 \
//     gitea/gitea:1.23
//
//   # Browse to http://localhost:3000 to complete the install wizard.
//   # Create a user, generate an access token at /user/settings/applications,
//   # create an empty repo "monorel-livetest".
//
//   export MONOREL_GITEA_HOST=localhost:3000
//   export MONOREL_GITEA_TOKEN=<the access token>
//   export MONOREL_GITEA_OWNER=<your gitea username>
//   export MONOREL_GITEA_REPO=monorel-livetest
//
//   go test -tags=livetest ./internal/provider/gitea

func liveOpts(t *testing.T) (host, owner, repo, token string) {
	t.Helper()
	host = os.Getenv("MONOREL_GITEA_HOST")
	if host == "" {
		host = "localhost:3000"
	}
	token = os.Getenv("MONOREL_GITEA_TOKEN")
	owner = os.Getenv("MONOREL_GITEA_OWNER")
	repo = os.Getenv("MONOREL_GITEA_REPO")
	if token == "" || owner == "" || repo == "" {
		t.Skip("livetest requires MONOREL_GITEA_TOKEN, MONOREL_GITEA_OWNER, MONOREL_GITEA_REPO")
	}
	return host, owner, repo, token
}

// liveClientGitea returns a raw Gitea SDK client for harness setup
// (creating branches, pushing files) that's outside the
// provider.Client surface monorel uses in production. Distinct from
// the provider.Client we're testing. Mirrors normalizeHost in the
// production package so a host with or without a scheme works.
func liveClientGitea(t *testing.T, host, token string) *gtsdk.Client {
	t.Helper()
	host = strings.TrimSuffix(host, "/")
	var candidates []string
	if strings.HasPrefix(host, "http://") || strings.HasPrefix(host, "https://") {
		candidates = []string{host}
	} else {
		candidates = []string{"https://" + host, "http://" + host}
	}
	for _, base := range candidates {
		c, err := gtsdk.NewClient(base, gtsdk.SetToken(token))
		if err == nil {
			return c
		}
	}
	t.Fatalf("connect to %s: no reachable scheme", host)
	return nil
}

// pingGitea checks the instance is reachable; skips the test if not.
// Accepts a bare host ("localhost:3000") or a fully-qualified URL
// ("http://localhost:3000"). Bare hosts try http:// then https://;
// scheme'd URLs are used as-is.
func pingGitea(t *testing.T, host string) {
	t.Helper()
	var candidates []string
	if strings.HasPrefix(host, "http://") || strings.HasPrefix(host, "https://") {
		candidates = []string{host}
	} else {
		candidates = []string{"http://" + host, "https://" + host}
	}
	for _, base := range candidates {
		req, _ := http.NewRequest("GET", base+"/api/v1/version", nil)
		req.Header.Set("Accept", "application/json")
		c := http.Client{Timeout: 2 * time.Second}
		if resp, err := c.Do(req); err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode < 500 {
				return
			}
		}
	}
	t.Skipf("gitea instance at %s not reachable; is the container running?", host)
}

func TestLive_GetDefaultBranch(t *testing.T) {
	host, owner, repo, token := liveOpts(t)
	pingGitea(t, host)

	ctx := context.Background()
	c, err := gitea.New(ctx, gitea.Options{
		Owner: owner,
		Repo:  repo,
		Host:  host,
		Token: token,
	})
	if err != nil {
		t.Fatalf("gitea.New: %v", err)
	}

	branch, err := c.GetDefaultBranch(ctx)
	if err != nil {
		t.Fatalf("GetDefaultBranch: %v", err)
	}
	if branch == "" {
		t.Errorf("got empty default branch")
	}
	t.Logf("default branch: %s", branch)
}

func TestLive_PRLifecycle(t *testing.T) {
	host, owner, repo, token := liveOpts(t)
	pingGitea(t, host)

	ctx := context.Background()
	c, err := gitea.New(ctx, gitea.Options{Owner: owner, Repo: repo, Host: host, Token: token})
	if err != nil {
		t.Fatalf("gitea.New: %v", err)
	}

	// Provision a branch via the raw SDK so the PR has somewhere to
	// open against.
	raw := liveClientGitea(t, host, token)
	headBranch := "monorel-livetest/" + time.Now().UTC().Format("20060102-150405.000000000")
	defaultBranch, err := c.GetDefaultBranch(ctx)
	if err != nil {
		t.Fatalf("GetDefaultBranch: %v", err)
	}
	if _, _, err := raw.CreateBranch(owner, repo, gtsdk.CreateBranchOption{
		BranchName:    headBranch,
		OldBranchName: defaultBranch,
	}); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	t.Cleanup(func() {
		_, _, _ = raw.DeleteRepoBranch(owner, repo, headBranch)
	})

	// FindOpenReleasePR should report nil for a brand-new branch.
	pr, err := c.FindOpenReleasePR(ctx, headBranch)
	if err != nil {
		t.Fatalf("FindOpenReleasePR (pre-create): %v", err)
	}
	if pr != nil {
		t.Fatalf("found unexpected PR before CreatePR: %+v", pr)
	}

	// CreatePR.
	created, err := c.CreatePR(ctx, provider.CreatePROptions{
		Title:      "monorel livetest PR",
		Body:       "initial body",
		HeadBranch: headBranch,
		BaseBranch: defaultBranch,
	})
	if err != nil {
		t.Fatalf("CreatePR: %v", err)
	}
	t.Logf("created PR #%d: %s", created.Number, created.HTMLURL)
	t.Cleanup(func() {
		_ = c.ClosePR(ctx, created.Number)
	})

	if created.Title != "monorel livetest PR" {
		t.Errorf("created.Title = %q", created.Title)
	}
	if created.HeadRef != headBranch {
		t.Errorf("created.HeadRef = %q, want %q", created.HeadRef, headBranch)
	}

	// FindOpenReleasePR now matches.
	found, err := c.FindOpenReleasePR(ctx, headBranch)
	if err != nil {
		t.Fatalf("FindOpenReleasePR (post-create): %v", err)
	}
	if found == nil || found.Number != created.Number {
		t.Errorf("FindOpenReleasePR didn't return the new PR; got %+v", found)
	}

	// UpdatePR title + body.
	newTitle := "monorel livetest PR (updated)"
	newBody := "edited body"
	updated, err := c.UpdatePR(ctx, created.Number, provider.UpdatePROptions{
		Title: &newTitle,
		Body:  &newBody,
	})
	if err != nil {
		t.Fatalf("UpdatePR: %v", err)
	}
	if updated.Title != newTitle {
		t.Errorf("updated.Title = %q, want %q", updated.Title, newTitle)
	}
	if updated.Body != newBody {
		t.Errorf("updated.Body = %q, want %q", updated.Body, newBody)
	}

	// ClosePR.
	if err := c.ClosePR(ctx, created.Number); err != nil {
		t.Fatalf("ClosePR: %v", err)
	}

	// Closed PR should no longer be returned by FindOpenReleasePR.
	postClose, err := c.FindOpenReleasePR(ctx, headBranch)
	if err != nil {
		t.Fatalf("FindOpenReleasePR (post-close): %v", err)
	}
	if postClose != nil {
		t.Errorf("FindOpenReleasePR returned a closed PR: %+v", postClose)
	}
}

func TestLive_CreateRelease(t *testing.T) {
	host, owner, repo, token := liveOpts(t)
	pingGitea(t, host)

	ctx := context.Background()
	c, err := gitea.New(ctx, gitea.Options{Owner: owner, Repo: repo, Host: host, Token: token})
	if err != nil {
		t.Fatalf("gitea.New: %v", err)
	}

	raw := liveClientGitea(t, host, token)
	defaultBranch, err := c.GetDefaultBranch(ctx)
	if err != nil {
		t.Fatalf("GetDefaultBranch: %v", err)
	}

	// Create a unique tag against the default branch's HEAD via the
	// raw SDK so CreateRelease has something to point at.
	tagName := "monorel-livetest-" + time.Now().UTC().Format("20060102-150405.000000000")
	commit, _, err := raw.GetSingleCommit(owner, repo, defaultBranch)
	if err != nil {
		t.Fatalf("GetSingleCommit: %v", err)
	}
	if _, _, err := raw.CreateTag(owner, repo, gtsdk.CreateTagOption{
		TagName: tagName,
		Target:  commit.SHA,
		Message: "monorel livetest tag",
	}); err != nil {
		t.Fatalf("CreateTag: %v", err)
	}

	// CreateRelease against that tag.
	rel, err := c.CreateRelease(ctx, provider.CreateReleaseOptions{
		Tag:        tagName,
		Name:       tagName,
		Body:       "monorel livetest release",
		Prerelease: false,
	})
	if err != nil {
		t.Fatalf("CreateRelease: %v", err)
	}
	if rel.Tag != tagName {
		t.Errorf("rel.Tag = %q, want %q", rel.Tag, tagName)
	}
	t.Logf("created release: %s", rel.HTMLURL)

	// Cleanup: delete the release + tag we just made.
	t.Cleanup(func() {
		_, _ = raw.DeleteRelease(owner, repo, rel.ID)
		_, _ = raw.DeleteTag(owner, repo, tagName)
	})
}
