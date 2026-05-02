//go:build livetest

// Run: go test -tags=livetest ./internal/provider/gitlab
//
// Validates the GitLab provider against a real GitLab instance. See
// the env-var setup at the top of this file.
package gitlab_test

import (
	"context"
	"os"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go"

	"monorel.disaresta.com/internal/provider"
	"monorel.disaresta.com/internal/provider/gitlab"
)

// Setup expectations:
//
//   - $MONOREL_GITLAB_HOST: the GitLab instance URL or hostname.
//     Defaults to "gitlab.com".
//   - $MONOREL_GITLAB_TOKEN: a personal access token with `api` scope.
//   - $MONOREL_GITLAB_OWNER: the user, group, or sub-group path that
//     owns the test project. May contain slashes for nested groups.
//   - $MONOREL_GITLAB_REPO: the project's path component.
//
// The test creates / closes / cleans up branches, MRs, tags, and
// releases on each run; the project must exist beforehand and have
// at least one commit on its default branch.

func liveOpts(t *testing.T) (host, owner, repo, token string) {
	t.Helper()
	host = os.Getenv("MONOREL_GITLAB_HOST")
	if host == "" {
		host = "gitlab.com"
	}
	token = os.Getenv("MONOREL_GITLAB_TOKEN")
	owner = os.Getenv("MONOREL_GITLAB_OWNER")
	repo = os.Getenv("MONOREL_GITLAB_REPO")
	if token == "" || owner == "" || repo == "" {
		t.Skip("livetest requires MONOREL_GITLAB_TOKEN, MONOREL_GITLAB_OWNER, MONOREL_GITLAB_REPO")
	}
	return host, owner, repo, token
}

// liveClientGitLab returns a raw GitLab SDK client for harness setup
// (creating branches, tags) outside the surface monorel uses in
// production.
func liveClientGitLab(t *testing.T, host, token string) *gl.Client {
	t.Helper()
	opts := []gl.ClientOptionFunc{}
	if host != "" && host != "gitlab.com" {
		baseURL := host
		if !startsWithHTTP(baseURL) {
			baseURL = "https://" + baseURL
		}
		opts = append(opts, gl.WithBaseURL(baseURL))
	}
	c, err := gl.NewClient(token, opts...)
	if err != nil {
		t.Fatalf("connect to %s: %v", host, err)
	}
	return c
}

func startsWithHTTP(s string) bool {
	return len(s) >= 7 && (s[:7] == "http://" || (len(s) >= 8 && s[:8] == "https://"))
}

func TestLive_GetDefaultBranch(t *testing.T) {
	host, owner, repo, token := liveOpts(t)
	ctx := context.Background()
	c, err := gitlab.New(ctx, gitlab.Options{
		Owner: owner,
		Repo:  repo,
		Host:  host,
		Token: token,
	})
	if err != nil {
		t.Fatalf("gitlab.New: %v", err)
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
	ctx := context.Background()
	c, err := gitlab.New(ctx, gitlab.Options{Owner: owner, Repo: repo, Host: host, Token: token})
	if err != nil {
		t.Fatalf("gitlab.New: %v", err)
	}

	raw := liveClientGitLab(t, host, token)
	pid := owner + "/" + repo

	defaultBranch, err := c.GetDefaultBranch(ctx)
	if err != nil {
		t.Fatalf("GetDefaultBranch: %v", err)
	}

	headBranch := "monorel-livetest/" + time.Now().UTC().Format("20060102-150405.000000000")
	if _, _, err := raw.Branches.CreateBranch(pid, &gl.CreateBranchOptions{
		Branch: gl.Ptr(headBranch),
		Ref:    gl.Ptr(defaultBranch),
	}); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	t.Cleanup(func() {
		_, _ = raw.Branches.DeleteBranch(pid, headBranch)
	})

	// Pre-create probe: no MR for this branch yet.
	pr, err := c.FindOpenReleasePR(ctx, headBranch)
	if err != nil {
		t.Fatalf("FindOpenReleasePR (pre-create): %v", err)
	}
	if pr != nil {
		t.Fatalf("found unexpected MR pre-create: %+v", pr)
	}

	// CreatePR.
	created, err := c.CreatePR(ctx, provider.CreatePROptions{
		Title:      "monorel livetest MR",
		Body:       "initial body",
		HeadBranch: headBranch,
		BaseBranch: defaultBranch,
	})
	if err != nil {
		t.Fatalf("CreatePR: %v", err)
	}
	t.Logf("created MR !%d: %s", created.Number, created.HTMLURL)
	t.Cleanup(func() {
		_ = c.ClosePR(ctx, created.Number)
	})

	if created.Title != "monorel livetest MR" {
		t.Errorf("created.Title = %q", created.Title)
	}
	if created.HeadRef != headBranch {
		t.Errorf("created.HeadRef = %q, want %q", created.HeadRef, headBranch)
	}

	// FindOpenReleasePR matches the new MR.
	found, err := c.FindOpenReleasePR(ctx, headBranch)
	if err != nil {
		t.Fatalf("FindOpenReleasePR (post-create): %v", err)
	}
	if found == nil || found.Number != created.Number {
		t.Errorf("FindOpenReleasePR didn't return the new MR; got %+v", found)
	}

	// UpdatePR.
	newTitle := "monorel livetest MR (updated)"
	newBody := "edited description"
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

	// Closed MR no longer returned by FindOpenReleasePR.
	postClose, err := c.FindOpenReleasePR(ctx, headBranch)
	if err != nil {
		t.Fatalf("FindOpenReleasePR (post-close): %v", err)
	}
	if postClose != nil {
		t.Errorf("FindOpenReleasePR returned a closed MR: %+v", postClose)
	}
}

func TestLive_CreateRelease(t *testing.T) {
	host, owner, repo, token := liveOpts(t)
	ctx := context.Background()
	c, err := gitlab.New(ctx, gitlab.Options{Owner: owner, Repo: repo, Host: host, Token: token})
	if err != nil {
		t.Fatalf("gitlab.New: %v", err)
	}

	raw := liveClientGitLab(t, host, token)
	pid := owner + "/" + repo

	defaultBranch, err := c.GetDefaultBranch(ctx)
	if err != nil {
		t.Fatalf("GetDefaultBranch: %v", err)
	}

	tagName := "monorel-livetest-" + time.Now().UTC().Format("20060102-150405.000000000")

	// Create a tag pointing at the default branch's HEAD via the
	// raw SDK so CreateRelease has something to attach to.
	if _, _, err := raw.Tags.CreateTag(pid, &gl.CreateTagOptions{
		TagName: gl.Ptr(tagName),
		Ref:     gl.Ptr(defaultBranch),
		Message: gl.Ptr("monorel livetest tag"),
	}); err != nil {
		t.Fatalf("CreateTag: %v", err)
	}

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

	t.Cleanup(func() {
		_, _, _ = raw.Releases.DeleteRelease(pid, tagName)
		_, _ = raw.Tags.DeleteTag(pid, tagName)
	})
}
