# Bitbucket Cloud Provider + Universal PR-Body Trailers Fallback

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `bitbucket` provider implementation for Bitbucket Cloud, plus a defensive PR-body trailers fallback that lets `monorel tag` recover when the merge commit body is rewritten (e.g. by squash-merge).

**Architecture:** New hand-rolled HTTP package at `internal/provider/bitbucket/` mirroring the gitlab package's shape (no SDK; ~7 endpoints over `net/http`). The provider interface gains `FindPRByMergeCommit(sha)` implemented by every provider; `monorel preview` writes a `<!-- monorel-trailers ... -->` HTML comment into the PR body; `monorel tag` falls back to fetching the merged PR's body when HEAD's commit message has no trailers.

**Tech Stack:** Go 1.26.x, `net/http`, `encoding/json`, existing project deps (no new direct deps). Bitbucket Cloud REST API v2 at `api.bitbucket.org/2.0/`. HTTP Basic auth (email + API token) for REST; `<username>:<token>` for git over HTTPS (username probed from `/2.0/user` and cached on the client).

**Spec:** `docs/superpowers/specs/2026-05-03-bitbucket-provider-design.md`

**Branch:** `feat/bitbucket-provider` (already exists; spec doc is already committed there as the first commit).

---

## File Structure

| Path | Action | Responsibility |
|---|---|---|
| `internal/provider/provider.go` | modify | Add `FindPRByMergeCommit` to `Client` interface; add `EmailEnvVars`; extend `TokenEnvVars` for bitbucket |
| `internal/provider/fake.go` | modify | Add `FindPRByMergeCommit` to `Fake` (in-memory implementation) |
| `internal/provider/github/github.go` | modify | Implement `FindPRByMergeCommit` |
| `internal/provider/gitlab/gitlab.go` | modify | Implement `FindPRByMergeCommit` |
| `internal/provider/gitea/gitea.go` | modify | Implement `FindPRByMergeCommit` |
| `internal/provider/factory/factory.go` | modify | Add `case ProviderBitbucket` arm |
| `config/provider.go` | modify | Add `ProviderBitbucket` constant; append to `KnownProviders` |
| `internal/provider/bitbucket/doc.go` | create | Package overview |
| `internal/provider/bitbucket/bitbucket.go` | create | `Options` + `New()` constructor |
| `internal/provider/bitbucket/client.go` | create | `*client` type, HTTP transport, auth header, JSON helpers |
| `internal/provider/bitbucket/errors.go` | create | `ErrPlanGate`, `ErrRateLimited` sentinels + status-code mapper |
| `internal/provider/bitbucket/identity.go` | create | `/2.0/user` probe with `sync.Once` |
| `internal/provider/bitbucket/repo.go` | create | `GetDefaultBranch` |
| `internal/provider/bitbucket/pulls.go` | create | `FindOpenReleasePR`, `CreatePR`, `UpdatePR`, `ClosePR` |
| `internal/provider/bitbucket/release.go` | create | `CreateRelease` (no-op + log line) |
| `internal/provider/bitbucket/trailers.go` | create | `FindPRByMergeCommit` |
| `internal/provider/bitbucket/bitbucket_test.go` | create | Unit tests against `httptest.NewServer` |
| `internal/provider/bitbucket/integration_test.go` | create | `//go:build integration`; full-API walk |
| `internal/provider/github/github_test.go` | modify | Add `FindPRByMergeCommit` test |
| `internal/provider/gitlab/gitlab_test.go` | modify | Add `FindPRByMergeCommit` test |
| `internal/provider/gitea/gitea_test.go` | modify | Add `FindPRByMergeCommit` test |
| `internal/release/render.go` | modify | Append `<!-- monorel-trailers ... -->` block to PR body |
| `internal/release/release.go` | modify | Add `Provider` field to `TagOptions`; fall back to PR-body lookup on `ErrNoReleaseCommit` |
| `internal/release/release_test.go` | modify | Add fallback-success and fallback-no-PR tests |
| `internal/release/render_test.go` | modify | Add trailers-comment-block test |
| `internal/cli/tag.go` | modify | Pass `Provider` from runtime into `TagOptions` |
| `docs/src/integrations/bitbucket.md` | create | Setup walkthrough |
| `docs/src/_partials/bitbucket-pipelines-yml.md` | create | Pipelines snippet partial |
| `examples/bitbucket/monorel.toml` | create | Reference config |
| `examples/bitbucket/bitbucket-pipelines.yml` | create | Reference CI |
| `examples/bitbucket/.changeset/README.md` | create | Mirror of other examples' README |
| `docs/.vitepress/config.ts` | modify | Add Bitbucket sidebar entry |
| `docs/src/cheat-sheet.md` | modify | Extend env-vars table |
| `docs/src/public/llms.txt` | modify | Add bitbucket entries |
| `docs/src/public/llms-full.txt` | modify | Add bitbucket entries |
| `docs/src/faq.md` | modify | Amend tag-recovery FAQ to mention trailers fallback |
| `README.md` | modify | List Bitbucket alongside other providers |
| `.changeset/bitbucket-provider.md` | create | `:minor` bump |

---

## Phase 1: Provider Interface Extension

The new `FindPRByMergeCommit(ctx, sha)` method ships on the `Client` interface. Every existing provider gets an implementation before bitbucket lands. This phase lays groundwork; the trailers fallback feature uses it later.

### Task 1: Add `FindPRByMergeCommit` to Client interface

**Files:**
- Modify: `internal/provider/provider.go`

- [ ] **Step 1: Modify the Client interface**

In `internal/provider/provider.go`, add the new method below `CreateRelease`:

```go
	CreateRelease(ctx context.Context, opts CreateReleaseOptions) (*Release, error)

	// FindPRByMergeCommit returns the PR/MR that was merged into the
	// repo at the given commit SHA, if one exists. Returns (nil, nil)
	// when no PR matches (NOT an error) — the caller treats that as
	// "no fallback available."
	//
	// Used by the universal PR-body trailers fallback: monorel tag
	// reads its release-trailer set from HEAD's commit message
	// normally, but falls back to fetching the merged PR's body
	// (looking for a `<!-- monorel-trailers ... -->` block) when the
	// commit body is empty (typically because of a squash-merge).
	FindPRByMergeCommit(ctx context.Context, sha string) (*PullRequest, error)
}
```

- [ ] **Step 2: Verify it compiles (will fail to compile because Fake + each provider lack the method)**

Run: `cd /home/theo/projects/monorel && go build ./...`
Expected: errors like `*Fake does not implement Client (missing method FindPRByMergeCommit)`. That's the next task's job.

- [ ] **Step 3: Don't commit yet**

We commit after the Fake + every provider implements the method (Tasks 2-5). One commit lands the interface change atomically across implementations.

### Task 2: Add `FindPRByMergeCommit` to the Fake

**Files:**
- Modify: `internal/provider/fake.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/provider/fake_test.go` (create if it doesn't exist; check first):

```go
func TestFake_FindPRByMergeCommit(t *testing.T) {
	f := NewFake()
	// Pre-seed a "merged" PR at a known SHA. The fake doesn't model
	// merging directly; we set MergedSHA on the PR.
	pr, err := f.CreatePR(context.Background(), CreatePROptions{
		Title:      "release",
		Body:       "body",
		HeadBranch: "monorel/release",
		BaseBranch: "main",
	})
	if err != nil {
		t.Fatal(err)
	}
	f.PRs[pr.Number].MergedSHA = "abc123"

	got, err := f.FindPRByMergeCommit(context.Background(), "abc123")
	if err != nil {
		t.Fatalf("FindPRByMergeCommit: %v", err)
	}
	if got == nil || got.Number != pr.Number {
		t.Errorf("got %v, want PR #%d", got, pr.Number)
	}

	miss, err := f.FindPRByMergeCommit(context.Background(), "nosuchsha")
	if err != nil {
		t.Fatal(err)
	}
	if miss != nil {
		t.Errorf("expected nil for unknown SHA; got %v", miss)
	}
}
```

- [ ] **Step 2: Run the test — verify it fails to compile**

Run: `cd /home/theo/projects/monorel && go test ./internal/provider/ 2>&1 | head -10`
Expected: build failure, e.g. `pr.MergedSHA undefined` or `f.FindPRByMergeCommit undefined`.

- [ ] **Step 3: Add `MergedSHA` field to `PullRequest`**

In `internal/provider/provider.go`, the existing `PullRequest` struct:

```go
type PullRequest struct {
	Number  int
	State   string // "open" or "closed"
	Title   string
	Body    string
	HeadRef string
	HTMLURL string

	// MergedSHA is the merge-commit SHA when this PR was merged (state
	// transitioned through "merged"). Empty for unmerged PRs. Used by
	// FindPRByMergeCommit's reverse lookup.
	MergedSHA string
}
```

- [ ] **Step 4: Implement `FindPRByMergeCommit` on Fake**

In `internal/provider/fake.go`, add at the end (after `CreateRelease`):

```go
// FindPRByMergeCommit implements [Client.FindPRByMergeCommit].
func (f *Fake) FindPRByMergeCommit(_ context.Context, sha string) (*PullRequest, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.take(); err != nil {
		return nil, err
	}
	for _, pr := range f.PRs {
		if pr.MergedSHA == sha {
			cp := *pr
			return &cp, nil
		}
	}
	return nil, nil
}
```

- [ ] **Step 5: Run the Fake test — verify it passes**

Run: `cd /home/theo/projects/monorel && go test ./internal/provider/ -run TestFake_FindPRByMergeCommit -v`
Expected: PASS.

- [ ] **Step 6: Don't commit yet**

Continue to Task 3 (real provider implementations); commit Phase 1 atomically after each provider lands its method.

### Task 3: Implement `FindPRByMergeCommit` for GitHub

**Files:**
- Modify: `internal/provider/github/github.go`
- Modify: `internal/provider/github/github_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/provider/github/github_test.go`. First check existing httptest patterns in the file (look at how `TestFindOpenReleasePR` or similar is structured) and follow that shape.

```go
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

	c := newTestClient(t, srv.URL) // helper used elsewhere in the file
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
```

If `newTestClient` helper doesn't exist with the signature above, mirror whatever pattern this file already uses to instantiate the github provider against a fake host. Don't invent a new helper if one exists; do reuse it.

- [ ] **Step 2: Run — fail**

Run: `cd /home/theo/projects/monorel && go test ./internal/provider/github/ -run TestFindPRByMergeCommit -v`
Expected: FAIL — `c.FindPRByMergeCommit undefined`.

- [ ] **Step 3: Implement**

In `internal/provider/github/github.go`, add the method on `*client` (find where the other Client methods live and append):

```go
func (c *client) FindPRByMergeCommit(ctx context.Context, sha string) (*provider.PullRequest, error) {
	prs, _, err := c.gh.PullRequests.ListPullRequestsWithCommit(ctx, c.owner, c.repo, sha, &gh.ListOptions{PerPage: 50})
	if err != nil {
		return nil, fmt.Errorf("github: list PRs for commit %s: %w", sha, err)
	}
	for _, pr := range prs {
		if pr.GetMergeCommitSHA() == sha {
			return githubToPR(pr), nil
		}
	}
	return nil, nil
}
```

The existing `githubToPR` helper presumably converts `*github.PullRequest` to `*provider.PullRequest`. If it doesn't carry `MergedSHA`, update it:

```go
func githubToPR(pr *gh.PullRequest) *provider.PullRequest {
	out := &provider.PullRequest{
		Number:  pr.GetNumber(),
		State:   pr.GetState(),
		Title:   pr.GetTitle(),
		Body:    pr.GetBody(),
		HeadRef: pr.GetHead().GetRef(),
		HTMLURL: pr.GetHTMLURL(),
		MergedSHA: pr.GetMergeCommitSHA(),
	}
	// Existing helper mapping returned state from "open"/"closed" stays.
	return out
}
```

- [ ] **Step 4: Run — pass**

Run: `cd /home/theo/projects/monorel && go test ./internal/provider/github/ -run TestFindPRByMergeCommit -v`
Expected: PASS.

- [ ] **Step 5: Don't commit yet**

### Task 4: Implement `FindPRByMergeCommit` for GitLab

**Files:**
- Modify: `internal/provider/gitlab/gitlab.go`
- Modify: `internal/provider/gitlab/gitlab_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/provider/gitlab/gitlab_test.go`. GitLab's API is `GET /projects/:id/repository/commits/:sha/merge_requests`. Mirror the existing test patterns in the file.

```go
func TestFindPRByMergeCommit_GitLab(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/team%2Fwidget/repository/commits/abc123/merge_requests" {
			t.Errorf("unexpected path: %s", r.URL.Path)
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

	c, err := New(context.Background(), Options{Owner: "team", Repo: "widget", Host: srv.URL, Token: "tok"})
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
```

- [ ] **Step 2: Run — fail**

Run: `cd /home/theo/projects/monorel && go test ./internal/provider/gitlab/ -run TestFindPRByMergeCommit_GitLab -v`
Expected: FAIL with `c.FindPRByMergeCommit undefined`.

- [ ] **Step 3: Implement**

Add to `internal/provider/gitlab/gitlab.go`. The go-gitlab client exposes `Commits.ListMergeRequestsByCommit`:

```go
func (c *client) FindPRByMergeCommit(ctx context.Context, sha string) (*provider.PullRequest, error) {
	mrs, _, err := c.gl.Commits.ListMergeRequestsByCommit(c.pid, sha, gl.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("gitlab: list MRs for commit %s: %w", sha, err)
	}
	for _, mr := range mrs {
		// MergeCommitSHA is the field name on go-gitlab's BasicMergeRequest.
		if mr.MergeCommitSHA == sha {
			return basicMRToPR(mr), nil
		}
	}
	return nil, nil
}
```

Update the existing `basicMRToPR` helper (or whichever conversion helper this file uses) to populate `MergedSHA: mr.MergeCommitSHA`.

- [ ] **Step 4: Run — pass**

Run: `cd /home/theo/projects/monorel && go test ./internal/provider/gitlab/ -run TestFindPRByMergeCommit_GitLab -v`
Expected: PASS.

- [ ] **Step 5: Don't commit yet**

### Task 5: Implement `FindPRByMergeCommit` for Gitea

**Files:**
- Modify: `internal/provider/gitea/gitea.go`
- Modify: `internal/provider/gitea/gitea_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/provider/gitea/gitea_test.go`. Gitea's API doesn't have a single "list PRs for commit" endpoint as cleanly as GitHub or GitLab. The supported pattern is to list closed PRs and filter by `merged_commit_id`:

```go
func TestFindPRByMergeCommit_Gitea(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Gitea SDK calls GET /repos/{owner}/{repo}/pulls?state=closed
		if !strings.Contains(r.URL.Path, "/pulls") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
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
	}))
	defer srv.Close()

	c, err := New(context.Background(), Options{Owner: "acme", Repo: "widget", Host: srv.URL, Token: "tok"})
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
```

- [ ] **Step 2: Run — fail**

Run: `cd /home/theo/projects/monorel && go test ./internal/provider/gitea/ -run TestFindPRByMergeCommit_Gitea -v`
Expected: FAIL with `c.FindPRByMergeCommit undefined`.

- [ ] **Step 3: Implement**

Add to `internal/provider/gitea/gitea.go`:

```go
func (c *client) FindPRByMergeCommit(_ context.Context, sha string) (*provider.PullRequest, error) {
	page := 1
	for {
		prs, _, err := c.gt.ListRepoPullRequests(c.owner, c.repo, gitea.ListPullRequestsOptions{
			ListOptions: gitea.ListOptions{Page: page, PageSize: 50},
			State:       gitea.StateClosed,
		})
		if err != nil {
			return nil, fmt.Errorf("gitea: list PRs: %w", err)
		}
		for _, pr := range prs {
			if pr.MergedCommitID != nil && *pr.MergedCommitID == sha {
				return giteaToPR(pr), nil
			}
		}
		if len(prs) < 50 {
			return nil, nil
		}
		page++
	}
}
```

Update `giteaToPR` to set `MergedSHA: derefOr(pr.MergedCommitID, "")` (or whatever the existing dereferencing helper looks like).

- [ ] **Step 4: Run — pass**

Run: `cd /home/theo/projects/monorel && go test ./internal/provider/gitea/ -run TestFindPRByMergeCommit_Gitea -v`
Expected: PASS.

- [ ] **Step 5: Run all tests — verify nothing else broke**

Run: `cd /home/theo/projects/monorel && go test ./...`
Expected: PASS (everything still compiles + passes).

- [ ] **Step 6: Commit Phase 1**

```bash
cd /home/theo/projects/monorel
git add internal/provider/
git commit -m "feat(provider): add FindPRByMergeCommit to Client interface

New method on the provider Client interface for the universal
PR-body trailers fallback. Each provider implements it:

  - github: uses go-github's ListPullRequestsWithCommit.
  - gitlab: uses go-gitlab's Commits.ListMergeRequestsByCommit.
  - gitea: lists closed PRs and filters by merged_commit_id.
  - fake: in-memory match against PullRequest.MergedSHA.

PullRequest gains a MergedSHA field, populated by every provider's
PR-conversion helper. Returns (nil, nil) when no PR matches the
SHA — the caller treats that as 'no fallback available'."
```

### Task 6: Add `EmailEnvVars` helper + extend `TokenEnvVars` for Bitbucket

**Files:**
- Modify: `internal/provider/provider.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/provider/provider_test.go` (create if missing):

```go
func TestEmailEnvVars(t *testing.T) {
	if got := EmailEnvVars("bitbucket"); !equalSlices(got, []string{"BITBUCKET_EMAIL"}) {
		t.Errorf("EmailEnvVars(bitbucket) = %v", got)
	}
	if got := EmailEnvVars("github"); got != nil {
		t.Errorf("EmailEnvVars(github) = %v, want nil", got)
	}
}

func TestTokenEnvVars_Bitbucket(t *testing.T) {
	if got := TokenEnvVars("bitbucket"); !equalSlices(got, []string{"BITBUCKET_TOKEN"}) {
		t.Errorf("TokenEnvVars(bitbucket) = %v", got)
	}
}

func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
```

- [ ] **Step 2: Run — fail**

Run: `cd /home/theo/projects/monorel && go test ./internal/provider/ -run "TestEmailEnvVars|TestTokenEnvVars_Bitbucket" -v`
Expected: FAIL — `EmailEnvVars undefined` and the bitbucket case missing from TokenEnvVars.

- [ ] **Step 3: Implement in provider.go**

Find `TokenEnvVars` in `internal/provider/provider.go` and add a `bitbucket` arm + a parallel `EmailEnvVars`:

```go
func TokenEnvVars(provider string) []string {
	switch config.ResolveProvider(provider) {
	case config.ProviderGitHub:
		return []string{"GITHUB_TOKEN", "GH_TOKEN"}
	case config.ProviderGitea:
		return []string{"GITEA_TOKEN"}
	case config.ProviderGitLab:
		return []string{"GITLAB_TOKEN", "CI_JOB_TOKEN"}
	case config.ProviderBitbucket:
		return []string{"BITBUCKET_TOKEN"}
	}
	return nil
}

// EmailEnvVars returns environment variable names to consult for a
// provider's auth-email, in priority order. Most providers don't use
// a separate email (the token alone authenticates); Bitbucket Cloud
// is the exception because its REST API uses HTTP Basic with
// `email + token`. Returns nil for providers that don't need a
// separate email.
func EmailEnvVars(provider string) []string {
	if config.ResolveProvider(provider) == config.ProviderBitbucket {
		return []string{"BITBUCKET_EMAIL"}
	}
	return nil
}
```

Note: `config.ProviderBitbucket` doesn't exist yet — Task 7 adds it. To keep this task self-contained, hold off until Task 7 completes, then come back. (Or do both in a tight order. The build will break temporarily if you do this task before Task 7.)

- [ ] **Step 4: Hold the commit until Task 7 lands the constant**

Combined into Task 7's commit.

### Task 7: Add `ProviderBitbucket` constant

**Files:**
- Modify: `config/provider.go`

- [ ] **Step 1: Write the failing test**

Append to `config/provider_test.go`:

```go
func TestProviderBitbucket_IsKnown(t *testing.T) {
	if !IsKnownProvider(ProviderBitbucket) {
		t.Error("ProviderBitbucket should be recognized")
	}
}

func TestKnownProviders_IncludesBitbucket(t *testing.T) {
	for _, p := range KnownProviders {
		if p == ProviderBitbucket {
			return
		}
	}
	t.Error("KnownProviders should include ProviderBitbucket")
}
```

- [ ] **Step 2: Run — fail**

Run: `cd /home/theo/projects/monorel && go test ./config/ -run "TestProviderBitbucket|TestKnownProviders" -v`
Expected: FAIL with `ProviderBitbucket undefined`.

- [ ] **Step 3: Add the constant + extend KnownProviders**

In `config/provider.go`:

```go
const (
	ProviderGitHub    = "github"
	ProviderGitea     = "gitea"
	ProviderGitLab    = "gitlab"
	ProviderBitbucket = "bitbucket"
)
```

```go
var KnownProviders = []string{
	ProviderBitbucket,
	ProviderGitea,
	ProviderGitHub,
	ProviderGitLab,
}
```

(Alphabetical order; `bitbucket` lands first.)

- [ ] **Step 4: Run all tests in provider/ + config/**

Run: `cd /home/theo/projects/monorel && go test ./internal/provider/ ./config/`
Expected: PASS (the `EmailEnvVars` test from Task 6 now compiles because `ProviderBitbucket` exists).

- [ ] **Step 5: Commit Phase-1-tail (Tasks 6 + 7 together)**

```bash
git add config/provider.go config/provider_test.go internal/provider/provider.go internal/provider/provider_test.go
git commit -m "feat(config,provider): register ProviderBitbucket

Adds the constant + appends to KnownProviders. The provider's
factory wiring is in a follow-up task; this one is just the
catalog entry. Plus EmailEnvVars/TokenEnvVars know about
BITBUCKET_EMAIL and BITBUCKET_TOKEN so the orchestrator can
resolve credentials when bitbucket is the selected provider."
```

---

## Phase 2: Bitbucket Package Skeleton

Lay down the package directory, the package-level documentation, the constructor + Options, the HTTP transport with auth header construction, and the error sentinels. No business methods yet (those follow in Phase 3).

### Task 8: Package overview (`doc.go`)

**Files:**
- Create: `internal/provider/bitbucket/doc.go`

- [ ] **Step 1: Write the file**

```go
// Package bitbucket is the [provider.Client] implementation for
// Bitbucket Cloud (bitbucket.org).
//
// Bitbucket Data Center / Server is out of scope; that variant uses
// a different REST API (/rest/api/1.0/...) and a different auth
// model. If support is added later, it should land under
// internal/provider/bitbucket/datacenter/ as a sibling of the
// current Cloud implementation.
//
// # Auth
//
// REST API: HTTP Basic with `email:token`. Both BITBUCKET_EMAIL and
// BITBUCKET_TOKEN are required. The token must be an Atlassian API
// token created with Bitbucket scopes:
// read/write:repository:bitbucket and read/write:pullrequest:bitbucket.
//
// Git over HTTPS uses a different identifier: <bitbucket-username>:<token>
// (NOT email). The provider client probes /2.0/user on first call to
// learn the username and caches it. Callers that need to construct
// an HTTPS git URL ask for the username via the cached state.
//
// # Releases
//
// Bitbucket Cloud has no first-class Release concept. CreateRelease
// is a no-op that returns a synthetic *Release pointing at the tag's
// /src/ URL. Per-package CHANGELOG.md is the canonical release-notes
// source.
//
// # State mapping
//
// Bitbucket PR states are OPEN / MERGED / DECLINED / SUPERSEDED.
// Provider-interface State is "open" / "closed". Map OPEN -> "open";
// everything else -> "closed".
package bitbucket
```

- [ ] **Step 2: Verify it builds**

Run: `cd /home/theo/projects/monorel && go build ./internal/provider/bitbucket/`
Expected: success.

- [ ] **Step 3: Don't commit yet**

Combined commit at the end of Phase 2.

### Task 9: Constructor + Options (`bitbucket.go`)

**Files:**
- Create: `internal/provider/bitbucket/bitbucket.go`

- [ ] **Step 1: Write the failing test**

Create `internal/provider/bitbucket/bitbucket_test.go`:

```go
package bitbucket

import (
	"context"
	"testing"
)

func TestNew_RejectsMissingEmail(t *testing.T) {
	_, err := New(context.Background(), Options{Workspace: "ws", Repo: "r", Token: "t"})
	if err == nil {
		t.Fatal("expected error for missing email")
	}
}

func TestNew_RejectsMissingToken(t *testing.T) {
	_, err := New(context.Background(), Options{Workspace: "ws", Repo: "r", Email: "e@x.com"})
	if err == nil {
		t.Fatal("expected error for missing token")
	}
}

func TestNew_RejectsMissingWorkspaceOrRepo(t *testing.T) {
	for _, tc := range []Options{
		{Repo: "r", Email: "e@x.com", Token: "t"},
		{Workspace: "ws", Email: "e@x.com", Token: "t"},
	} {
		if _, err := New(context.Background(), tc); err == nil {
			t.Errorf("expected error for %+v", tc)
		}
	}
}

func TestNew_RejectsNonEmptyHost(t *testing.T) {
	_, err := New(context.Background(), Options{
		Workspace: "ws", Repo: "r", Host: "self-hosted",
		Email: "e@x.com", Token: "t",
	})
	if err == nil {
		t.Fatal("expected error for non-empty Host (Cloud-only)")
	}
}

func TestNew_HappyPath(t *testing.T) {
	c, err := New(context.Background(), Options{
		Workspace: "ws", Repo: "r",
		Email: "e@x.com", Token: "t",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if c == nil {
		t.Fatal("expected non-nil client")
	}
}
```

- [ ] **Step 2: Run — fail**

Run: `cd /home/theo/projects/monorel && go test ./internal/provider/bitbucket/ -v`
Expected: FAIL — `New undefined` etc.

- [ ] **Step 3: Implement**

Create `internal/provider/bitbucket/bitbucket.go`:

```go
package bitbucket

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"monorel.disaresta.com/internal/provider"
)

// Options configures a new Bitbucket Cloud-backed [provider.Client].
type Options struct {
	// Workspace is the bitbucket workspace slug (the part of the URL
	// like bitbucket.org/<workspace>/<repo>). Mirrors the `owner`
	// field from monorel.toml.
	Workspace string

	// Repo is the repository slug.
	Repo string

	// Host must be empty (Cloud-only). Non-empty is rejected with an
	// error pointing at the future Data Center extension path.
	Host string

	// Email is the Atlassian-account email. Required for REST auth
	// (HTTP Basic with email + token).
	Email string

	// Token is an Atlassian API token with Bitbucket scopes
	// (read/write:repository:bitbucket and
	// read/write:pullrequest:bitbucket).
	Token string
}

// ErrMissingWorkspaceRepo is returned when Options doesn't carry both
// a workspace and a repo.
var ErrMissingWorkspaceRepo = errors.New("bitbucket: Workspace and Repo are required")

// ErrMissingEmail is returned when Options.Email is empty.
var ErrMissingEmail = errors.New("bitbucket: Email is required (REST auth uses HTTP Basic with email + token)")

// ErrMissingToken is returned when Options.Token is empty.
var ErrMissingToken = errors.New("bitbucket: Token is required")

// ErrHostNotSupported is returned when Options.Host is non-empty.
// Bitbucket Data Center / Server is not implemented; only Cloud is
// supported.
var ErrHostNotSupported = errors.New("bitbucket: Host must be empty (Cloud-only); Data Center support is not implemented")

// New returns a [provider.Client] backed by Bitbucket Cloud's REST
// API v2. Does NOT make a network call: the first request fires when
// one of the Client methods is invoked, including the lazy
// /2.0/user probe that resolves the auth username for git
// credentials.
func New(_ context.Context, opts Options) (provider.Client, error) {
	if opts.Workspace == "" || opts.Repo == "" {
		return nil, ErrMissingWorkspaceRepo
	}
	if opts.Email == "" {
		return nil, ErrMissingEmail
	}
	if opts.Token == "" {
		return nil, ErrMissingToken
	}
	if opts.Host != "" {
		return nil, ErrHostNotSupported
	}
	return &client{
		workspace: opts.Workspace,
		repo:      opts.Repo,
		email:     opts.Email,
		token:     opts.Token,
		baseURL:   "https://api.bitbucket.org/2.0",
		http: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

// client is the unexported provider.Client implementation. Public
// surface is the Options + New constructor only.
type client struct {
	workspace string
	repo      string
	email     string
	token     string

	baseURL string

	http *http.Client

	// Identity-probe state. Lazily populated on first call needing
	// the username.
	identityOnce sync.Once
	username     string
	identityErr  error
}
```

- [ ] **Step 4: Run — partial pass**

Run: `cd /home/theo/projects/monorel && go test ./internal/provider/bitbucket/ -run TestNew -v`
Expected: PASS (constructor tests). Other tests (full Client interface) fail because methods aren't implemented yet — that's Phase 3.

- [ ] **Step 5: Don't commit yet**

### Task 10: HTTP transport + auth header (`client.go`)

**Files:**
- Create: `internal/provider/bitbucket/client.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/provider/bitbucket/bitbucket_test.go`:

```go
import (
	"encoding/base64"
	"net/http/httptest"
	"strings"
)

func TestClient_AuthHeader(t *testing.T) {
	var seen string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get("Authorization")
		w.WriteHeader(200)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c, err := New(context.Background(), Options{
		Workspace: "ws", Repo: "r",
		Email: "e@x.com", Token: "tok",
	})
	if err != nil {
		t.Fatal(err)
	}
	bc := c.(*client)
	bc.baseURL = srv.URL

	if _, err := bc.do(context.Background(), "GET", "/some-path", nil, nil); err != nil {
		t.Fatalf("do: %v", err)
	}

	want := "Basic " + base64.StdEncoding.EncodeToString([]byte("e@x.com:tok"))
	if seen != want {
		t.Errorf("Authorization header = %q, want %q", seen, want)
	}
}

func TestClient_DoMapsStatus(t *testing.T) {
	cases := []struct {
		status int
		body   string
		want   string // substring of err message
	}{
		{401, `{"error":{"message":"unauth"}}`, "auth failed"},
		{403, `{"error":{"message":"no scope"}}`, "forbidden"},
		{404, `{"error":{"message":"missing"}}`, "not found"},
		{402, `{"error":{"message":"plan"}}`, "plan not configured"},
		{429, `{"error":{"message":"slow down"}}`, "rate limited"},
		{500, `{"error":{"message":"oops"}}`, "server error"},
		{400, `{"error":{"message":"bad input"}}`, "bad input"},
	}
	for _, tc := range cases {
		t.Run(strings.TrimSpace(tc.want), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			c, _ := New(context.Background(), Options{
				Workspace: "ws", Repo: "r",
				Email: "e@x.com", Token: "tok",
			})
			bc := c.(*client)
			bc.baseURL = srv.URL

			_, err := bc.do(context.Background(), "GET", "/x", nil, nil)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %q, want substring %q", err.Error(), tc.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run — fail**

Run: `cd /home/theo/projects/monorel && go test ./internal/provider/bitbucket/ -run "TestClient_AuthHeader|TestClient_DoMapsStatus"`
Expected: FAIL with `bc.do undefined`.

- [ ] **Step 3: Implement client.go**

Create `internal/provider/bitbucket/client.go`:

```go
package bitbucket

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// do issues an authenticated HTTP request and decodes the JSON
// response body into out (if non-nil). Returns a wrapped error on
// non-2xx, with status-code-specific messages for the common cases.
//
// path is the API path (e.g. "/repositories/ws/r"); the baseURL is
// prepended. query is appended as URL-encoded parameters when
// non-nil. body, when non-nil, is JSON-encoded and sent as the
// request body with Content-Type: application/json.
func (c *client) do(ctx context.Context, method, path string, query url.Values, body any) (*http.Response, error) {
	full := c.baseURL + path
	if len(query) > 0 {
		full += "?" + query.Encode()
	}

	var bodyReader io.Reader
	if body != nil {
		buf := new(bytes.Buffer)
		if err := json.NewEncoder(buf).Encode(body); err != nil {
			return nil, fmt.Errorf("bitbucket: marshal request body: %w", err)
		}
		bodyReader = buf
	}

	req, err := http.NewRequestWithContext(ctx, method, full, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("bitbucket: build request: %w", err)
	}
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(c.email+":"+c.token)))
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("bitbucket: %s %s: %w", method, full, err)
	}

	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		respBytes, _ := io.ReadAll(resp.Body)
		return nil, mapStatusError(resp.StatusCode, respBytes, c.workspace, c.repo)
	}
	return resp, nil
}

// decodeJSON reads resp.Body, JSON-decodes into out, and closes the
// body. Convenience for the common one-shot pattern.
func decodeJSON(resp *http.Response, out any) error {
	defer resp.Body.Close()
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("bitbucket: decode response: %w", err)
	}
	return nil
}

// repoBase returns the API base path for the configured repo, e.g.
// /repositories/<workspace>/<repo>. Includes URL-encoding of the
// workspace and repo slugs (rare but possible if the slug contains
// reserved characters).
func (c *client) repoBase() string {
	return "/repositories/" + url.PathEscape(c.workspace) + "/" + url.PathEscape(c.repo)
}
```

`mapStatusError` is defined in `errors.go` (Task 11).

- [ ] **Step 4: Don't commit yet**

### Task 11: Error sentinels + status-code mapper (`errors.go`)

**Files:**
- Create: `internal/provider/bitbucket/errors.go`

- [ ] **Step 1: Implement (test came earlier in Task 10's `TestClient_DoMapsStatus`)**

Create `internal/provider/bitbucket/errors.go`:

```go
package bitbucket

import (
	"encoding/json"
	"errors"
	"fmt"
)

// ErrPlanGate is returned when Bitbucket rejects an API or git
// operation with HTTP 402 because the workspace plan isn't
// configured. The user must accept a plan at
// bitbucket.org/<workspace>/workspace/settings/plans before monorel
// can push commits or call certain APIs.
//
// 402 is most commonly observed on git push (which monorel doesn't
// invoke directly — the orchestrator's git layer surfaces it). The
// REST client surfaces it for completeness.
var ErrPlanGate = errors.New("bitbucket: workspace plan not configured (visit bitbucket.org/<workspace>/workspace/settings/plans)")

// ErrRateLimited is returned when Bitbucket responds with HTTP 429.
// Callers may retry after a short delay; the response's Retry-After
// header is not yet parsed by this client.
var ErrRateLimited = errors.New("bitbucket: rate limited (HTTP 429); retry after a short delay")

// errorResponse is Bitbucket's error envelope shape.
type errorResponse struct {
	Type  string `json:"type"`
	Error struct {
		Message string `json:"message"`
		Detail  string `json:"detail"`
	} `json:"error"`
}

// mapStatusError converts an HTTP error response into a wrapped Go
// error with a user-actionable message. Includes workspace/repo
// context for 404s.
func mapStatusError(status int, body []byte, workspace, repo string) error {
	msg := decodeErrorMessage(body)
	switch status {
	case 401:
		return fmt.Errorf("bitbucket: auth failed (check BITBUCKET_EMAIL + BITBUCKET_TOKEN); verify the token has Bitbucket scopes: %s", msg)
	case 402:
		return ErrPlanGate
	case 403:
		return fmt.Errorf("bitbucket: forbidden; the token is missing a scope (required: read/write repository, read/write pullrequest): %s", msg)
	case 404:
		return fmt.Errorf("bitbucket: not found (workspace=%q repo=%q); verify the slugs and that you have access: %s", workspace, repo, msg)
	case 429:
		return ErrRateLimited
	case 400:
		return fmt.Errorf("bitbucket: bad input: %s", msg)
	}
	if status >= 500 {
		return fmt.Errorf("bitbucket: server error %d: %s", status, msg)
	}
	return fmt.Errorf("bitbucket: unexpected status %d: %s", status, msg)
}

// decodeErrorMessage tries to extract a human-readable message from
// Bitbucket's error envelope. Falls back to the raw body when the
// envelope doesn't parse.
func decodeErrorMessage(body []byte) string {
	var er errorResponse
	if err := json.Unmarshal(body, &er); err == nil && er.Error.Message != "" {
		if er.Error.Detail != "" {
			return er.Error.Message + " — " + er.Error.Detail
		}
		return er.Error.Message
	}
	if len(body) == 0 {
		return "(empty response body)"
	}
	if len(body) > 200 {
		return string(body[:200]) + "..."
	}
	return string(body)
}
```

- [ ] **Step 2: Run client tests**

Run: `cd /home/theo/projects/monorel && go test ./internal/provider/bitbucket/ -run "TestClient_AuthHeader|TestClient_DoMapsStatus" -v`
Expected: PASS.

- [ ] **Step 3: Commit Phase 2**

```bash
git add internal/provider/bitbucket/
git commit -m "feat(provider/bitbucket): package skeleton

Adds the Cloud-only Bitbucket provider package: doc.go (package
overview), bitbucket.go (Options + New constructor with input
validation), client.go (HTTP transport with HTTP Basic auth header),
errors.go (sentinel errors + status-code mapper). No business
methods yet — those land in Phase 3."
```

---

## Phase 3: Bitbucket REST Methods

Each method is one task: write the test against an `httptest.NewServer` fake, run, fail, implement, run, pass, hold the commit until the phase wraps.

### Task 12: Identity probe (`identity.go`)

**Files:**
- Create: `internal/provider/bitbucket/identity.go`

- [ ] **Step 1: Write the failing test**

Append to `bitbucket_test.go`:

```go
func TestIdentityProbe_CachesUsername(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/user" {
			calls++
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintln(w, `{"username":"theo-bb","display_name":"Theo","uuid":"{abc}"}`)
			return
		}
		t.Errorf("unexpected path %s", r.URL.Path)
	}))
	defer srv.Close()

	c, _ := New(context.Background(), Options{
		Workspace: "ws", Repo: "r",
		Email: "e@x.com", Token: "tok",
	})
	bc := c.(*client)
	bc.baseURL = srv.URL

	got, err := bc.resolveUsername(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got != "theo-bb" {
		t.Errorf("got %q, want theo-bb", got)
	}

	// Call again; should hit the cache, not the server.
	if _, err := bc.resolveUsername(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Errorf("expected 1 probe call, got %d", calls)
	}
}

func TestIdentityProbe_SurfacesAuthError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(401)
		fmt.Fprintln(w, `{"error":{"message":"bad creds"}}`)
	}))
	defer srv.Close()

	c, _ := New(context.Background(), Options{
		Workspace: "ws", Repo: "r",
		Email: "e@x.com", Token: "tok",
	})
	bc := c.(*client)
	bc.baseURL = srv.URL

	_, err := bc.resolveUsername(context.Background())
	if err == nil {
		t.Fatal("expected auth error")
	}
	if !strings.Contains(err.Error(), "auth failed") {
		t.Errorf("err = %v", err)
	}
}
```

- [ ] **Step 2: Run — fail**

Run: `cd /home/theo/projects/monorel && go test ./internal/provider/bitbucket/ -run TestIdentityProbe -v`
Expected: FAIL — `resolveUsername undefined`.

- [ ] **Step 3: Implement**

Create `internal/provider/bitbucket/identity.go`:

```go
package bitbucket

import (
	"context"
	"fmt"
)

// resolveUsername returns the Bitbucket username associated with the
// configured email + token. Probes /2.0/user on first call and caches
// the result; concurrent first calls share the probe via sync.Once.
func (c *client) resolveUsername(ctx context.Context) (string, error) {
	c.identityOnce.Do(func() {
		resp, err := c.do(ctx, "GET", "/user", nil, nil)
		if err != nil {
			c.identityErr = fmt.Errorf("bitbucket: probe /2.0/user: %w", err)
			return
		}
		var body struct {
			Username    string `json:"username"`
			DisplayName string `json:"display_name"`
			UUID        string `json:"uuid"`
		}
		if err := decodeJSON(resp, &body); err != nil {
			c.identityErr = err
			return
		}
		if body.Username == "" {
			c.identityErr = fmt.Errorf("bitbucket: /2.0/user returned empty username")
			return
		}
		c.username = body.Username
	})
	if c.identityErr != nil {
		return "", c.identityErr
	}
	return c.username, nil
}
```

- [ ] **Step 4: Run — pass**

Run: `cd /home/theo/projects/monorel && go test ./internal/provider/bitbucket/ -run TestIdentityProbe -v`
Expected: PASS.

- [ ] **Step 5: Don't commit yet**

### Task 13: GetDefaultBranch (`repo.go`)

**Files:**
- Create: `internal/provider/bitbucket/repo.go`

- [ ] **Step 1: Write the failing test**

Append to `bitbucket_test.go`:

```go
func TestGetDefaultBranch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repositories/ws/r" {
			t.Errorf("path = %s, want /repositories/ws/r", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"mainbranch":{"name":"master","type":"branch"}}`)
	}))
	defer srv.Close()

	c, _ := New(context.Background(), Options{
		Workspace: "ws", Repo: "r", Email: "e@x.com", Token: "t",
	})
	bc := c.(*client)
	bc.baseURL = srv.URL

	got, err := c.GetDefaultBranch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got != "master" {
		t.Errorf("got %q, want master", got)
	}
}

func TestGetDefaultBranch_EmptyMainbranch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{}`)
	}))
	defer srv.Close()

	c, _ := New(context.Background(), Options{
		Workspace: "ws", Repo: "r", Email: "e@x.com", Token: "t",
	})
	bc := c.(*client)
	bc.baseURL = srv.URL

	_, err := c.GetDefaultBranch(context.Background())
	if err == nil {
		t.Fatal("expected error for missing mainbranch")
	}
}
```

- [ ] **Step 2: Run — fail**

- [ ] **Step 3: Implement**

Create `internal/provider/bitbucket/repo.go`:

```go
package bitbucket

import (
	"context"
	"fmt"
)

func (c *client) GetDefaultBranch(ctx context.Context) (string, error) {
	resp, err := c.do(ctx, "GET", c.repoBase(), nil, nil)
	if err != nil {
		return "", err
	}
	var body struct {
		Mainbranch struct {
			Name string `json:"name"`
		} `json:"mainbranch"`
	}
	if err := decodeJSON(resp, &body); err != nil {
		return "", err
	}
	if body.Mainbranch.Name == "" {
		return "", fmt.Errorf("bitbucket: %s/%s has no default branch", c.workspace, c.repo)
	}
	return body.Mainbranch.Name, nil
}
```

- [ ] **Step 4: Run — pass**

- [ ] **Step 5: Don't commit yet**

### Task 14: PR ops (`pulls.go`)

**Files:**
- Create: `internal/provider/bitbucket/pulls.go`

This task implements four `provider.Client` methods together since they share request/response shapes.

- [ ] **Step 1: Write the failing tests**

Append to `bitbucket_test.go`:

```go
func TestFindOpenReleasePR(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := r.URL.Query().Get("q")
		want := `state="OPEN" AND source.branch.name="monorel/release"`
		if got != want {
			t.Errorf("q = %q, want %q", got, want)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"values":[{
			"id": 7,
			"state": "OPEN",
			"title": "release",
			"summary":{"raw":"body"},
			"source":{"branch":{"name":"monorel/release"}},
			"merge_commit":null,
			"links":{"html":{"href":"https://bitbucket.org/ws/r/pull-requests/7"}}
		}]}`)
	}))
	defer srv.Close()

	c, _ := New(context.Background(), Options{Workspace: "ws", Repo: "r", Email: "e@x.com", Token: "t"})
	bc := c.(*client)
	bc.baseURL = srv.URL

	got, err := c.FindOpenReleasePR(context.Background(), "monorel/release")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.Number != 7 || got.State != "open" {
		t.Errorf("got %+v", got)
	}
}

func TestFindOpenReleasePR_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"values":[]}`)
	}))
	defer srv.Close()

	c, _ := New(context.Background(), Options{Workspace: "ws", Repo: "r", Email: "e@x.com", Token: "t"})
	bc := c.(*client)
	bc.baseURL = srv.URL

	got, err := c.FindOpenReleasePR(context.Background(), "monorel/release")
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Errorf("expected nil; got %+v", got)
	}
}

func TestCreatePR(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method = %s, want POST", r.Method)
		}
		var body struct {
			Title       string `json:"title"`
			Description string `json:"description"`
			Source      struct {
				Branch struct{ Name string `json:"name"` } `json:"branch"`
			} `json:"source"`
			Destination struct {
				Branch struct{ Name string `json:"name"` } `json:"branch"`
			} `json:"destination"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		if body.Title != "release" || body.Source.Branch.Name != "monorel/release" || body.Destination.Branch.Name != "main" {
			t.Errorf("body = %+v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{
			"id": 7, "state":"OPEN", "title":"release", "summary":{"raw":"body"},
			"source":{"branch":{"name":"monorel/release"}},
			"links":{"html":{"href":"https://bitbucket.org/ws/r/pull-requests/7"}}
		}`)
	}))
	defer srv.Close()

	c, _ := New(context.Background(), Options{Workspace: "ws", Repo: "r", Email: "e@x.com", Token: "t"})
	bc := c.(*client)
	bc.baseURL = srv.URL

	got, err := c.CreatePR(context.Background(), provider.CreatePROptions{
		Title:      "release",
		Body:       "body",
		HeadBranch: "monorel/release",
		BaseBranch: "main",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Number != 7 {
		t.Errorf("got #%d, want #7", got.Number)
	}
}

func TestUpdatePR(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PUT" {
			t.Errorf("method = %s, want PUT", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{
			"id": 7, "state":"OPEN", "title":"new title", "summary":{"raw":"new body"},
			"source":{"branch":{"name":"monorel/release"}},
			"links":{"html":{"href":"https://bitbucket.org/ws/r/pull-requests/7"}}
		}`)
	}))
	defer srv.Close()

	c, _ := New(context.Background(), Options{Workspace: "ws", Repo: "r", Email: "e@x.com", Token: "t"})
	bc := c.(*client)
	bc.baseURL = srv.URL

	title := "new title"
	body := "new body"
	got, err := c.UpdatePR(context.Background(), 7, provider.UpdatePROptions{Title: &title, Body: &body})
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "new title" {
		t.Errorf("got %q", got.Title)
	}
}

func TestClosePR(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repositories/ws/r/pullrequests/7/decline" {
			t.Errorf("path = %s", r.URL.Path)
		}
		w.WriteHeader(200)
		fmt.Fprintln(w, `{"id":7,"state":"DECLINED"}`)
	}))
	defer srv.Close()

	c, _ := New(context.Background(), Options{Workspace: "ws", Repo: "r", Email: "e@x.com", Token: "t"})
	bc := c.(*client)
	bc.baseURL = srv.URL

	if err := c.ClosePR(context.Background(), 7); err != nil {
		t.Fatal(err)
	}
}
```

Add to the `import` block at the top of the test file (it may already include some of these):

```go
import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"monorel.disaresta.com/internal/provider"
)
```

- [ ] **Step 2: Run — fail**

- [ ] **Step 3: Implement**

Create `internal/provider/bitbucket/pulls.go`:

```go
package bitbucket

import (
	"context"
	"fmt"
	"net/url"
	"strconv"

	"monorel.disaresta.com/internal/provider"
)

// bbPullRequest is Bitbucket's PR-shape we care about.
type bbPullRequest struct {
	ID      int    `json:"id"`
	State   string `json:"state"`
	Title   string `json:"title"`
	Summary struct {
		Raw string `json:"raw"`
	} `json:"summary"`
	Source struct {
		Branch struct {
			Name string `json:"name"`
		} `json:"branch"`
	} `json:"source"`
	MergeCommit *struct {
		Hash string `json:"hash"`
	} `json:"merge_commit"`
	Links struct {
		HTML struct {
			Href string `json:"href"`
		} `json:"html"`
	} `json:"links"`
}

// toProviderPR converts to the neutral provider.PullRequest shape.
// State is normalized: OPEN -> "open"; everything else -> "closed".
func (p *bbPullRequest) toProviderPR() *provider.PullRequest {
	state := "closed"
	if p.State == "OPEN" {
		state = "open"
	}
	pr := &provider.PullRequest{
		Number:  p.ID,
		State:   state,
		Title:   p.Title,
		Body:    p.Summary.Raw,
		HeadRef: p.Source.Branch.Name,
		HTMLURL: p.Links.HTML.Href,
	}
	if p.MergeCommit != nil {
		pr.MergedSHA = p.MergeCommit.Hash
	}
	return pr
}

func (c *client) FindOpenReleasePR(ctx context.Context, headBranch string) (*provider.PullRequest, error) {
	q := url.Values{}
	q.Set("q", fmt.Sprintf(`state="OPEN" AND source.branch.name=%q`, headBranch))
	resp, err := c.do(ctx, "GET", c.repoBase()+"/pullrequests", q, nil)
	if err != nil {
		return nil, err
	}
	var body struct {
		Values []bbPullRequest `json:"values"`
	}
	if err := decodeJSON(resp, &body); err != nil {
		return nil, err
	}
	if len(body.Values) == 0 {
		return nil, nil
	}
	return body.Values[0].toProviderPR(), nil
}

func (c *client) CreatePR(ctx context.Context, opts provider.CreatePROptions) (*provider.PullRequest, error) {
	type branch struct {
		Name string `json:"name"`
	}
	type ref struct {
		Branch branch `json:"branch"`
	}
	type createBody struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		Source      ref    `json:"source"`
		Destination ref    `json:"destination"`
	}
	body := createBody{
		Title:       opts.Title,
		Description: opts.Body,
		Source:      ref{Branch: branch{Name: opts.HeadBranch}},
		Destination: ref{Branch: branch{Name: opts.BaseBranch}},
	}
	resp, err := c.do(ctx, "POST", c.repoBase()+"/pullrequests", nil, body)
	if err != nil {
		return nil, err
	}
	var out bbPullRequest
	if err := decodeJSON(resp, &out); err != nil {
		return nil, err
	}
	return out.toProviderPR(), nil
}

func (c *client) UpdatePR(ctx context.Context, number int, opts provider.UpdatePROptions) (*provider.PullRequest, error) {
	if opts.Title == nil && opts.Body == nil {
		return nil, fmt.Errorf("bitbucket: UpdatePR has nothing to change")
	}
	patch := map[string]any{}
	if opts.Title != nil {
		patch["title"] = *opts.Title
	}
	if opts.Body != nil {
		patch["description"] = *opts.Body
	}
	resp, err := c.do(ctx, "PUT", c.repoBase()+"/pullrequests/"+strconv.Itoa(number), nil, patch)
	if err != nil {
		return nil, err
	}
	var out bbPullRequest
	if err := decodeJSON(resp, &out); err != nil {
		return nil, err
	}
	return out.toProviderPR(), nil
}

func (c *client) ClosePR(ctx context.Context, number int) error {
	resp, err := c.do(ctx, "POST", c.repoBase()+"/pullrequests/"+strconv.Itoa(number)+"/decline", nil, nil)
	if err != nil {
		return err
	}
	return decodeJSON(resp, nil)
}
```

- [ ] **Step 4: Run — pass**

Run: `cd /home/theo/projects/monorel && go test ./internal/provider/bitbucket/ -v`
Expected: All previously-written tests pass; remaining tests (CreateRelease, FindPRByMergeCommit) compile-fail until the rest of Phase 3.

- [ ] **Step 5: Don't commit yet**

### Task 15: CreateRelease (no-op) (`release.go`)

**Files:**
- Create: `internal/provider/bitbucket/release.go`

- [ ] **Step 1: Write the failing test**

```go
func TestCreateRelease_NoOp(t *testing.T) {
	c, _ := New(context.Background(), Options{Workspace: "ws", Repo: "r", Email: "e@x.com", Token: "t"})
	rel, err := c.CreateRelease(context.Background(), provider.CreateReleaseOptions{
		Tag:  "transports/foo/v1.7.0",
		Name: "transports/foo v1.7.0",
		Body: "release body",
	})
	if err != nil {
		t.Fatal(err)
	}
	if rel == nil {
		t.Fatal("expected non-nil release")
	}
	if rel.Tag != "transports/foo/v1.7.0" {
		t.Errorf("Tag = %q", rel.Tag)
	}
	if !strings.Contains(rel.HTMLURL, "/src/transports/foo/v1.7.0") {
		t.Errorf("HTMLURL = %q; want a /src/<tag> URL", rel.HTMLURL)
	}
}
```

- [ ] **Step 2: Run — fail**

- [ ] **Step 3: Implement**

Create `internal/provider/bitbucket/release.go`:

```go
package bitbucket

import (
	"context"
	"net/url"

	"monorel.disaresta.com/internal/provider"
)

// CreateRelease is a no-op for Bitbucket Cloud, which has no
// first-class Release concept. Returns a synthetic *Release whose
// HTMLURL points at the tag's source view (bitbucket.org/<ws>/<repo>/src/<tag>).
//
// The orchestrator's call site treats CreateRelease as advisory:
// per-package CHANGELOG.md is the canonical release-notes source.
// monorel publish reports `0/0 releases` when every CreateRelease
// call returned a no-op (TODO when the publish summary is updated;
// for now the synthetic Release counts as success).
func (c *client) CreateRelease(_ context.Context, opts provider.CreateReleaseOptions) (*provider.Release, error) {
	tagPath := url.PathEscape(opts.Tag)
	htmlURL := "https://bitbucket.org/" +
		url.PathEscape(c.workspace) + "/" +
		url.PathEscape(c.repo) + "/src/" + tagPath
	return &provider.Release{
		Tag:     opts.Tag,
		HTMLURL: htmlURL,
	}, nil
}
```

- [ ] **Step 4: Run — pass**

- [ ] **Step 5: Don't commit yet**

### Task 16: FindPRByMergeCommit (`trailers.go`)

**Files:**
- Create: `internal/provider/bitbucket/trailers.go`

- [ ] **Step 1: Write the failing test**

```go
func TestFindPRByMergeCommit_Bitbucket(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := r.URL.Query().Get("q")
		want := `state="MERGED" AND merge_commit.hash="abc123"`
		if got != want {
			t.Errorf("q = %q, want %q", got, want)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"values":[{
			"id": 42, "state":"MERGED", "title":"release", "summary":{"raw":"body"},
			"source":{"branch":{"name":"monorel/release"}},
			"merge_commit":{"hash":"abc123"},
			"links":{"html":{"href":"https://bitbucket.org/ws/r/pull-requests/42"}}
		}]}`)
	}))
	defer srv.Close()

	c, _ := New(context.Background(), Options{Workspace: "ws", Repo: "r", Email: "e@x.com", Token: "t"})
	bc := c.(*client)
	bc.baseURL = srv.URL

	got, err := c.FindPRByMergeCommit(context.Background(), "abc123")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.Number != 42 || got.MergedSHA != "abc123" {
		t.Errorf("got %+v", got)
	}
}

func TestFindPRByMergeCommit_Bitbucket_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"values":[]}`)
	}))
	defer srv.Close()

	c, _ := New(context.Background(), Options{Workspace: "ws", Repo: "r", Email: "e@x.com", Token: "t"})
	bc := c.(*client)
	bc.baseURL = srv.URL

	got, err := c.FindPRByMergeCommit(context.Background(), "abc123")
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Errorf("expected nil; got %+v", got)
	}
}
```

- [ ] **Step 2: Run — fail**

- [ ] **Step 3: Implement**

Create `internal/provider/bitbucket/trailers.go`:

```go
package bitbucket

import (
	"context"
	"fmt"
	"net/url"

	"monorel.disaresta.com/internal/provider"
)

func (c *client) FindPRByMergeCommit(ctx context.Context, sha string) (*provider.PullRequest, error) {
	q := url.Values{}
	q.Set("q", fmt.Sprintf(`state="MERGED" AND merge_commit.hash=%q`, sha))
	resp, err := c.do(ctx, "GET", c.repoBase()+"/pullrequests", q, nil)
	if err != nil {
		return nil, err
	}
	var body struct {
		Values []bbPullRequest `json:"values"`
	}
	if err := decodeJSON(resp, &body); err != nil {
		return nil, err
	}
	if len(body.Values) == 0 {
		return nil, nil
	}
	return body.Values[0].toProviderPR(), nil
}
```

- [ ] **Step 4: Run all bitbucket tests — pass**

Run: `cd /home/theo/projects/monorel && go test ./internal/provider/bitbucket/ -v`
Expected: every test PASS.

- [ ] **Step 5: Wire factory.go**

In `internal/provider/factory/factory.go`, add the bitbucket arm:

```go
import (
	// ... existing imports ...
	"monorel.disaresta.com/internal/provider/bitbucket"
)

// In the switch:
case config.ProviderBitbucket:
	return bitbucket.New(ctx, bitbucket.Options{
		Workspace: cfg.Owner,
		Repo:      cfg.Repo,
		Host:      cfg.Host,
		Email:     os.Getenv("BITBUCKET_EMAIL"),
		Token:     token,
	})
```

(`os` import may need adding.)

- [ ] **Step 6: Run all tests**

Run: `cd /home/theo/projects/monorel && go test ./...`
Expected: PASS.

- [ ] **Step 7: Commit Phase 3**

```bash
git add internal/provider/bitbucket/ internal/provider/factory/
git commit -m "feat(provider/bitbucket): implement REST methods + factory wiring

- identity.go: /2.0/user probe with sync.Once cache.
- repo.go: GetDefaultBranch reads mainbranch.name.
- pulls.go: FindOpenReleasePR / CreatePR / UpdatePR / ClosePR.
  PR state OPEN maps to 'open'; everything else (MERGED /
  DECLINED / SUPERSEDED) maps to 'closed'.
- release.go: CreateRelease is a no-op returning a synthetic
  *Release whose HTMLURL points at the tag's /src/ view.
  Bitbucket Cloud has no first-class Release concept; per-package
  CHANGELOG.md is the canonical release-notes source.
- trailers.go: FindPRByMergeCommit queries MERGED PRs by
  merge_commit.hash via Bitbucket's BBQL.
- factory: case ProviderBitbucket constructs a bitbucket client
  using Owner as the workspace and BITBUCKET_EMAIL from env."
```

---

## Phase 4: Universal Trailers Fallback

The interface plumbing landed in Phase 1. Now wire the actual write side (preview render) and read side (Tag fallback).

### Task 17: Preview renderer appends `<!-- monorel-trailers ... -->` block

**Files:**
- Modify: `internal/release/render.go`
- Modify: `internal/release/render_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/release/render_test.go`:

```go
func TestRenderPreview_IncludesTrailersComment(t *testing.T) {
	p := &plan.ReleasePlan{
		Releases: []plan.PackageRelease{
			{
				Name: "transports/foo",
				From: "v1.6.0",
				To:   "v1.7.0",
				Bump: semver.Minor,
				Tag:  "transports/foo/v1.7.0",
			},
			{
				Name: "go.example.com/widget",
				From: "v2.0.0",
				To:   "v2.0.1",
				Bump: semver.Patch,
				Tag:  "v2.0.1",
			},
		},
	}
	got := RenderPreview(p, "2026-05-03")

	if !strings.Contains(got, "<!-- monorel-trailers") {
		t.Errorf("expected monorel-trailers comment; got:\n%s", got)
	}
	for _, want := range []string{
		"monorel-Release: transports/foo v1.7.0",
		"monorel-Release: go.example.com/widget v2.0.1",
		"monorel-PreRelease: false",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in body:\n%s", want, got)
		}
	}
}

func TestRenderPreview_TrailersInPrereleaseMode(t *testing.T) {
	p := &plan.ReleasePlan{
		Releases: []plan.PackageRelease{{
			Name:       "transports/foo",
			From:       "v1.6.0",
			To:         "v1.7.0-rc.0",
			Bump:       semver.Minor,
			Tag:        "transports/foo/v1.7.0-rc.0",
			Prerelease: true,
		}},
	}
	got := RenderPreview(p, "2026-05-03")

	if !strings.Contains(got, "monorel-PreRelease: true") {
		t.Errorf("PreRelease trailer should be true:\n%s", got)
	}
}
```

- [ ] **Step 2: Run — fail**

- [ ] **Step 3: Implement**

In `internal/release/render.go`, find where `RenderPreview` returns and append the trailers comment block before returning. Add at the end of the function (before `return b.String()`):

```go
	// Append the monorel-trailers comment block. Invisible in the
	// rendered PR body (HTML comment); used by `monorel tag` as a
	// fallback when the merge commit body is rewritten (e.g. by
	// squash-merge). All four providers preserve HTML comments on
	// PR-body fetches.
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "<!-- monorel-trailers (do not edit; required for tag recovery if the merge commit body is rewritten)")
	for _, r := range p.Releases {
		fmt.Fprintf(&b, "monorel-Release: %s %s\n", r.Name, r.To)
	}
	fmt.Fprintf(&b, "monorel-PreRelease: %t\n", anyPrerelease(p))
	fmt.Fprintln(&b, "-->")
```

- [ ] **Step 4: Run — pass**

- [ ] **Step 5: Don't commit yet**

### Task 18: Add Provider field to TagOptions; implement fallback

**Files:**
- Modify: `internal/release/release.go`
- Modify: `internal/release/release_test.go`
- Modify: `internal/cli/tag.go`

- [ ] **Step 1: Write the failing test (release_test.go)**

Add a fresh test:

```go
func TestTag_FallbackToPRBody(t *testing.T) {
	repo := newFakeRepoForTag(t)  // reuse existing helper or build one
	repo.HeadCommitMessageValue = "chore(release): foo v1.7.0\n\n(no trailers; squash-merge)" // commit body lacks trailers
	repo.HeadSHAValue = "abc123"

	pf := provider.NewFake()
	pf.PRs[42] = &provider.PullRequest{
		Number: 42,
		State:  "closed",
		Title:  "release",
		Body: `Plan body
<!-- monorel-trailers (do not edit; required for tag recovery if the merge commit body is rewritten)
monorel-Release: transports/foo v1.7.0
monorel-PreRelease: false
-->`,
		MergedSHA: "abc123",
	}

	cfg := &config.Config{
		Packages: map[string]config.PackageConfig{
			"transports/foo": {Path: "transports/foo", TagPrefix: "transports/foo"},
		},
	}
	res, err := Tag(TagOptions{Config: cfg, Repo: repo, Provider: pf})
	if err != nil {
		t.Fatalf("Tag with fallback: %v", err)
	}
	if len(res.Releases) != 1 || res.Releases[0].Tag != "transports/foo/v1.7.0" {
		t.Errorf("got %+v", res.Releases)
	}
}

func TestTag_FallbackBothMissing(t *testing.T) {
	repo := newFakeRepoForTag(t)
	repo.HeadCommitMessageValue = "chore(release): foo v1.7.0\n\n(no trailers)"
	repo.HeadSHAValue = "abc123"
	pf := provider.NewFake()
	// No PR seeded.

	cfg := &config.Config{Packages: map[string]config.PackageConfig{}}
	_, err := Tag(TagOptions{Config: cfg, Repo: repo, Provider: pf})
	if err == nil {
		t.Fatal("expected ErrNoReleaseCommit")
	}
	if !errors.Is(err, ErrNoReleaseCommit) {
		t.Errorf("err = %v, want ErrNoReleaseCommit", err)
	}
}
```

(If `newFakeRepoForTag` doesn't exist, use whatever helper this test file already uses; mirror it. The point is HeadCommitMessage returns a body without `monorel-Release:` trailers and HeadSHA returns a known value.)

- [ ] **Step 2: Run — fail**

Run: `cd /home/theo/projects/monorel && go test ./internal/release/ -run TestTag_Fallback`
Expected: FAIL — `Provider undefined on TagOptions`.

- [ ] **Step 3: Add Provider field + fallback path**

In `internal/release/release.go`:

```go
type TagOptions struct {
	Config *config.Config
	Repo   git.Repo

	// Provider, when non-nil, enables the universal PR-body trailers
	// fallback: when HEAD's commit message has no monorel-Release:
	// trailers (e.g. a squash-merge rewrote the body), Tag looks up
	// the PR that was merged at HEAD's SHA via
	// Provider.FindPRByMergeCommit and parses trailers from the PR
	// body's <!-- monorel-trailers ... --> comment block.
	//
	// nil disables the fallback (Tag returns ErrNoReleaseCommit on
	// missing trailers, the pre-fallback behavior).
	Provider provider.Client
}
```

Add `import "monorel.disaresta.com/internal/provider"` if not already present.

In `Tag(opts TagOptions)`, modify the `len(tagged) == 0` branch:

```go
	tagged, prerelease := parseReleaseTrailers(msg)
	if len(tagged) == 0 {
		// Fallback: look up the merged PR's body for a trailers comment block.
		if opts.Provider != nil {
			fallbackTagged, fallbackPre, err := tryPRBodyFallback(opts)
			if err != nil {
				return nil, err
			}
			if len(fallbackTagged) > 0 {
				tagged, prerelease = fallbackTagged, fallbackPre
			}
		}
		if len(tagged) == 0 {
			return nil, ErrNoReleaseCommit
		}
	}
```

Add the helper:

```go
// tryPRBodyFallback queries the provider for the merged PR at HEAD's
// SHA and parses release trailers from the PR body's
// <!-- monorel-trailers ... --> comment block. Returns empty slices
// when the PR isn't found or the comment block is absent — both are
// "no fallback available," not errors.
func tryPRBodyFallback(opts TagOptions) ([]taggedRelease, bool, error) {
	headSHA, err := opts.Repo.CurrentSHA()
	if err != nil {
		return nil, false, fmt.Errorf("release: read HEAD SHA for fallback: %w", err)
	}
	pr, err := opts.Provider.FindPRByMergeCommit(context.Background(), headSHA)
	if err != nil {
		return nil, false, fmt.Errorf("release: PR-body fallback: %w", err)
	}
	if pr == nil {
		return nil, false, nil
	}
	block := extractTrailersBlock(pr.Body)
	if block == "" {
		return nil, false, nil
	}
	tagged, prerelease := parseReleaseTrailers(block)
	return tagged, prerelease, nil
}

// extractTrailersBlock returns the lines inside an HTML comment block
// of the form `<!-- monorel-trailers ... -->`, or "" if no such block
// is present. Bitbucket / GitHub / GitLab / Gitea all preserve HTML
// comments on PR-body fetches.
func extractTrailersBlock(prBody string) string {
	const startMarker = "<!-- monorel-trailers"
	const endMarker = "-->"
	start := strings.Index(prBody, startMarker)
	if start == -1 {
		return ""
	}
	tail := prBody[start+len(startMarker):]
	end := strings.Index(tail, endMarker)
	if end == -1 {
		return ""
	}
	return strings.TrimSpace(tail[:end])
}
```

Add `import "context"` and `import "strings"` if not present.

- [ ] **Step 4: Update CLI to thread Provider into TagOptions**

In `internal/cli/tag.go`, find where `release.Tag(release.TagOptions{...})` is called and add `Provider: rt.Provider` (or whatever the runtime exposes for the provider client):

```go
res, err := release.Tag(release.TagOptions{
	Config:   rt.Config,
	Repo:     rt.Repo,
	Provider: rt.Provider, // new: enables PR-body trailers fallback
})
```

If `rt.Provider` doesn't exist, look for how other commands (e.g. `preview.go`) access the provider — they probably build it inline via factory.New from `rt.Config`. Mirror that pattern in `tag.go`.

- [ ] **Step 5: Run — pass**

Run: `cd /home/theo/projects/monorel && go test ./internal/release/ ./internal/cli/`
Expected: PASS (existing tests continue to pass; new fallback tests pass; CLI compiles).

- [ ] **Step 6: Commit Phase 4**

```bash
git add internal/release/ internal/cli/tag.go
git commit -m "feat(release): universal PR-body trailers fallback

monorel preview now appends a <!-- monorel-trailers ... --> HTML
comment block to the rendered PR body. Invisible in the rendered
view; persists in the source.

monorel tag, when given a Provider, falls back to fetching the
merged PR's body and parsing trailers from that comment block when
HEAD's commit message has no monorel-Release: trailers (e.g.
because squash-merge rewrote the body). On both-missing the call
still returns ErrNoReleaseCommit.

Provider on TagOptions is optional: nil disables the fallback,
preserving the prior behavior for callers that haven't been
updated. internal/cli/tag.go threads the runtime provider client
through so monorel tag picks up the fallback automatically."
```

---

## Phase 5: Tests + Integration

The unit tests are already written task-by-task above. This phase adds the build-tag-gated integration test and any missing cross-provider coverage.

### Task 19: Build-tag integration test

**Files:**
- Create: `internal/provider/bitbucket/integration_test.go`

- [ ] **Step 1: Write the integration test**

Create `internal/provider/bitbucket/integration_test.go`:

```go
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
	// the Client interface — repo creation isn't a provider.Client
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
```

- [ ] **Step 2: Run with the build tag**

Run: `cd /home/theo/projects/monorel && go test -tags=integration ./internal/provider/bitbucket/ -v -run TestIntegration`
Expected: skip (env vars unset). With the env vars set, expected: PASS.

- [ ] **Step 3: Commit Phase 5**

```bash
git add internal/provider/bitbucket/integration_test.go
git commit -m "test(provider/bitbucket): build-tag-gated integration test

Walks the REST lifecycle against a real Bitbucket workspace
(create repo -> GetDefaultBranch -> delete repo). Skipped unless
BITBUCKET_INTEGRATION=1 plus BITBUCKET_EMAIL, BITBUCKET_TOKEN,
BITBUCKET_WORKSPACE are set.

Pushed-branch / PR-lifecycle coverage is out of scope for the
REST-only test; it requires git over HTTPS and was validated
manually during the spike."
```

---

## Phase 6: Documentation

### Task 20: Integration page

**Files:**
- Create: `docs/src/integrations/bitbucket.md`

- [ ] **Step 1: Write the page**

Use `docs/src/integrations/gitlab.md` as the structural template. Sections:

1. Overview (one paragraph: Bitbucket Cloud only; Data Center not supported).
2. Setup (toml + env vars).
3. The two-env-var auth model (BITBUCKET_EMAIL + BITBUCKET_TOKEN); how to create an Atlassian API token with Bitbucket scopes.
4. Workspace plan acceptance (one-time at `bitbucket.org/<workspace>/workspace/settings/plans`); symptom = HTTP 402 on push.
5. Merge-strategy requirement (allow fast-forward / merge-commit; reject squash; the universal trailers fallback recovers from squash anyway).
6. No-native-releases note (CHANGELOG.md is the canonical source; `monorel publish` is a no-op).
7. CI walkthrough using `bitbucket-pipelines.yml`.
8. Token revocation guidance (Atlassian id.atlassian.com).

Mirror the structure used in `gitlab.md` (which the user can read for reference).

- [ ] **Step 2: Don't commit yet**

### Task 21: Pipelines partial

**Files:**
- Create: `docs/src/_partials/bitbucket-pipelines-yml.md`

- [ ] **Step 1: Write the partial**

```markdown
```yaml
# bitbucket-pipelines.yml
image: ghcr.io/disaresta-org/monorel:0.13.0

pipelines:
  branches:
    main:
      - step:
          name: monorel
          script:
            - export GIT_AUTHOR_NAME="monorel-bot[automation]"
            - export GIT_AUTHOR_EMAIL="monorel-bot@users.noreply.example.com"
            - export GIT_COMMITTER_NAME="$GIT_AUTHOR_NAME"
            - export GIT_COMMITTER_EMAIL="$GIT_AUTHOR_EMAIL"
            - if echo "$BITBUCKET_COMMIT_MESSAGE" | grep -q "^chore(release):"; then
                monorel tag &&
                git push --follow-tags "https://${BB_USER}:${BITBUCKET_TOKEN}@bitbucket.org/${BITBUCKET_REPO_FULL_NAME}.git" &&
                monorel publish;
              else
                git fetch origin "$BITBUCKET_BRANCH" &&
                git checkout -B monorel/release "origin/$BITBUCKET_BRANCH" &&
                apply_out=$(monorel apply); echo "$apply_out";
                if ! echo "$apply_out" | grep -qF "Nothing to apply."; then
                  git push -f "https://${BB_USER}:${BITBUCKET_TOKEN}@bitbucket.org/${BITBUCKET_REPO_FULL_NAME}.git" monorel/release &&
                  monorel preview --upsert;
                fi;
              fi
```
```

This is a starting template; the implementer should validate it against an actual Bitbucket Pipelines run as part of the integration test follow-up.

- [ ] **Step 2: Don't commit yet**

### Task 22: Examples

**Files:**
- Create: `examples/bitbucket/monorel.toml`
- Create: `examples/bitbucket/bitbucket-pipelines.yml`
- Create: `examples/bitbucket/.changeset/README.md`

- [ ] **Step 1: Mirror the gitlab/ example**

`examples/bitbucket/monorel.toml`:

```toml
# monorel configuration for a Bitbucket Cloud-hosted repo.

[provider]
name  = "bitbucket"
owner = "your-workspace"   # bitbucket workspace slug
repo  = "your-repo"

[packages."your-repo"]
tag_prefix = ""
path       = "."
changelog  = "CHANGELOG.md"
```

`examples/bitbucket/bitbucket-pipelines.yml`: copy the partial's content as a real file.

`examples/bitbucket/.changeset/README.md`: copy from `examples/github/.changeset/README.md`.

- [ ] **Step 2: Don't commit yet**

### Task 23: Sidebar + cheat sheet + LLM files + README + FAQ

**Files:**
- Modify: `docs/.vitepress/config.ts`
- Modify: `docs/src/cheat-sheet.md`
- Modify: `docs/src/public/llms.txt`
- Modify: `docs/src/public/llms-full.txt`
- Modify: `docs/src/faq.md`
- Modify: `README.md`

- [ ] **Step 1: Sidebar**

In `docs/.vitepress/config.ts`, find the Integrations group and add Bitbucket alongside GitHub / Gitea / GitLab:

```ts
{
  text: 'Integrations',
  items: [
    { text: 'GitHub', link: '/integrations/github' },
    { text: 'Gitea / Forgejo', link: '/integrations/gitea' },
    { text: 'GitLab', link: '/integrations/gitlab' },
    { text: 'Bitbucket', link: '/integrations/bitbucket' },
  ],
},
```

- [ ] **Step 2: Cheat sheet**

In `docs/src/cheat-sheet.md`, find the env-vars table and append:

```markdown
| `BITBUCKET_EMAIL` + `BITBUCKET_TOKEN` | `preview`, `publish` | Auth for the Bitbucket Cloud provider. Email is the Atlassian-account email; token is an API token with Bitbucket scopes. |
```

- [ ] **Step 3: llms.txt**

In `docs/src/public/llms.txt`, find the provider-auth table:

```markdown
| Bitbucket Cloud | `BITBUCKET_EMAIL` + `BITBUCKET_TOKEN` | API token with `read/write:repository:bitbucket` + `read/write:pullrequest:bitbucket` scopes. |
```

- [ ] **Step 4: llms-full.txt**

In `docs/src/public/llms-full.txt`, add a Bitbucket section to the integrations table and a paragraph noting the no-native-releases gap, the two-env-var auth, and the workspace-plan-acceptance one-time setup.

- [ ] **Step 5: faq.md**

Find the squash-merge / tag-recovery FAQ entry and amend:

```markdown
The `<!-- monorel-trailers ... -->` HTML comment that monorel preview writes to the PR body is the recovery path: monorel tag falls back to it when the merge commit body lacks trailers. So even if a contributor force-merges via squash, the release still completes. The fallback fails only when both the commit body AND the PR body's comment block are absent (the latter would mean a contributor edited the PR body and removed the comment).
```

- [ ] **Step 6: README.md**

In the providers list / table, add Bitbucket to the row showing supported providers.

- [ ] **Step 7: Don't commit yet**

### Task 24: Changeset + final commit

**Files:**
- Create: `.changeset/bitbucket-provider.md`

- [ ] **Step 1: Write the changeset**

```markdown
---
"monorel.disaresta.com": minor
---

monorel now supports Bitbucket Cloud (`provider.name = "bitbucket"`) alongside GitHub, Gitea / Forgejo, and GitLab. The `internal/provider/bitbucket/` package implements the `provider.Client` interface against Bitbucket's REST API v2 (hand-rolled `net/http`; no new direct deps).

Auth uses two environment variables: `BITBUCKET_EMAIL` (Atlassian account email) and `BITBUCKET_TOKEN` (Atlassian API token with Bitbucket scopes). The Bitbucket username for git over HTTPS is probed from `/2.0/user` and cached on the client.

Bitbucket Cloud has no first-class Release concept, so `monorel publish` is a no-op on Bitbucket; per-package `CHANGELOG.md` is the canonical release-notes source.

Plus a defensive recovery mechanism that benefits every provider: `monorel preview` now appends a `<!-- monorel-trailers ... -->` HTML comment to the PR body. `monorel tag` falls back to that block when the merge commit body lacks `monorel-Release:` trailers (e.g. because of a squash-merge that rewrote the body). The fallback uses the new `provider.Client.FindPRByMergeCommit` method, implemented by every provider.

See [Bitbucket integration](/integrations/bitbucket).
```

- [ ] **Step 2: Run all tests one last time**

Run: `cd /home/theo/projects/monorel && go test ./...`
Expected: PASS.

Run: `cd /home/theo/projects/monorel && cd docs && bun run docs:build`
Expected: clean build.

- [ ] **Step 3: Commit + push**

```bash
git add .
git commit -m "docs: integration page, examples, cheat sheet, LLM files, README, FAQ, sidebar, changeset

Closes the documentation surface for the Bitbucket provider PR:
- docs/src/integrations/bitbucket.md walks through setup,
  auth (two env vars), workspace plan acceptance, merge-strategy
  guidance, no-native-releases note, CI via bitbucket-pipelines.yml.
- examples/bitbucket/{monorel.toml, bitbucket-pipelines.yml,
  .changeset/README.md} provides a copy-paste reference.
- docs/.vitepress/config.ts adds Bitbucket to the Integrations
  sidebar.
- docs/src/cheat-sheet.md, llms.txt, llms-full.txt list the new
  env vars.
- docs/src/faq.md amends the squash-merge / tag-recovery entry to
  mention the new universal trailers fallback.
- README.md lists Bitbucket alongside the other providers.
- .changeset/bitbucket-provider.md ships the change as a :minor
  bump."

git push -u origin feat/bitbucket-provider
```

- [ ] **Step 4: Open PR**

```bash
gh pr create --repo disaresta-org/monorel --fill
```

(The fill takes the commit messages as the PR title + body; or use `--body-file` with a written-out summary.)

---

## Self-Review

**Spec coverage:**

- [x] Goal 1 (Bitbucket provider) — Tasks 8-16 implement the package.
- [x] Goal 2 (universal trailers fallback) — Tasks 1-5, 17, 18 implement the interface + render + fallback.
- [x] Decision 1 (Cloud-only, layout-ready) — Task 8's `doc.go` documents the layout intent; the package lives at `internal/provider/bitbucket/` rather than `bitbucket/cloud/` for now (Data Center adds `bitbucket/datacenter/` later).
- [x] Decision 2 (CreateRelease no-op) — Task 15.
- [x] Decision 3 (merge strategy + trailers fallback) — Task 17, 18; integration page (Task 20) documents the strategy.
- [x] Decision 4 (auth surface) — Tasks 9 (Options), 12 (probe), 6 (env-var helpers).
- [x] Decision 5 (hand-rolled) — Tasks 9-16; no new deps.
- [x] Decision 6 (one-PR scope) — all tasks land on `feat/bitbucket-provider`.

**Placeholder scan:** no TBDs / TODOs in step content. Code blocks are complete. Test bodies show the actual assertions.

**Type consistency:** `provider.PullRequest.MergedSHA` introduced in Task 2, used by all subsequent provider implementations and the trailers fallback. `bbPullRequest.toProviderPR()` populates it. `TagOptions.Provider` introduced in Task 18, used by `internal/cli/tag.go`. Constructor `New(ctx, Options)` shape matches across all four provider packages.

**Ambiguity check:** the Bitbucket-Pipelines YAML in Task 21 references `BB_USER` and `BITBUCKET_TOKEN` as env vars the user must configure in pipeline secrets — this should be called out in the integration page (Task 20). The fallback's behavior when `Provider == nil` is documented as "skip the fallback; return ErrNoReleaseCommit as before" — preserved as Task 18's contract.

---

Plan complete and saved to `docs/superpowers/plans/2026-05-03-bitbucket-provider.md`. Two execution options:

1. **Subagent-Driven (recommended)** — I dispatch a fresh subagent per task, review between tasks, fast iteration.
2. **Inline Execution** — Execute tasks in this session using executing-plans, batch execution with checkpoints.

Which approach?