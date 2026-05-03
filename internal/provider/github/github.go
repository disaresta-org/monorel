// Package github is the [provider.Client] implementation for GitHub.com
// and GitHub Enterprise (via the Host option).
//
// Wraps go-github + oauth2. Constructed via [New]; the returned
// concrete type satisfies [provider.Client] structurally.
package github

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	gogh "github.com/google/go-github/v68/github"
	"golang.org/x/oauth2"

	"monorel.disaresta.com/internal/provider"
)

// Options configures a new GitHub-backed [provider.Client].
type Options struct {
	// Owner is the GitHub user or org that owns the repo (e.g.
	// "disaresta-org").
	Owner string

	// Repo is the repository name (e.g. "monorel").
	Repo string

	// Host is the API host for GitHub Enterprise (e.g.
	// "github.example.com"). Empty means use the public api.github.com.
	Host string

	// Token is the personal access token or installation token
	// used for authenticated API calls. Required for any operation
	// that creates or modifies state; reads against public repos
	// also work without one but get tighter rate limits.
	Token string
}

// ErrMissingOwnerRepo is returned when the Options doesn't carry both
// an owner and repo. Without them every API call is meaningless.
var ErrMissingOwnerRepo = errors.New("github: Owner and Repo are required")

// client is the package-private concrete impl. Callers receive it as
// [provider.Client] from [New], so the type itself is not exported.
type client struct {
	owner string
	repo  string
	gh    *gogh.Client
}

// New returns a [provider.Client] backed by go-github, authenticated by
// opts.Token (empty token yields an unauthenticated client). When
// opts.Host is non-empty, the client targets a GitHub Enterprise
// installation.
func New(ctx context.Context, opts Options) (provider.Client, error) {
	if opts.Owner == "" || opts.Repo == "" {
		return nil, ErrMissingOwnerRepo
	}
	var httpClient *http.Client
	if opts.Token != "" {
		ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: opts.Token})
		httpClient = oauth2.NewClient(ctx, ts)
	}
	gh := gogh.NewClient(httpClient)
	if opts.Host != "" {
		baseURL := fmt.Sprintf("https://%s/api/v3/", opts.Host)
		uploadURL := fmt.Sprintf("https://%s/api/uploads/", opts.Host)
		var err error
		gh, err = gh.WithEnterpriseURLs(baseURL, uploadURL)
		if err != nil {
			return nil, fmt.Errorf("github: enterprise host %q: %w", opts.Host, err)
		}
	}
	return &client{owner: opts.Owner, repo: opts.Repo, gh: gh}, nil
}

func (c *client) GetDefaultBranch(ctx context.Context) (string, error) {
	repo, _, err := c.gh.Repositories.Get(ctx, c.owner, c.repo)
	if err != nil {
		return "", fmt.Errorf("github: get repo %s/%s: %w", c.owner, c.repo, err)
	}
	if repo.DefaultBranch == nil {
		return "", fmt.Errorf("github: %s/%s has no default branch", c.owner, c.repo)
	}
	return *repo.DefaultBranch, nil
}

func (c *client) FindOpenReleasePR(ctx context.Context, headBranch string) (*provider.PullRequest, error) {
	opts := &gogh.PullRequestListOptions{
		State:       "open",
		Head:        fmt.Sprintf("%s:%s", c.owner, headBranch),
		ListOptions: gogh.ListOptions{PerPage: 100},
	}
	for {
		prs, resp, err := c.gh.PullRequests.List(ctx, c.owner, c.repo, opts)
		if err != nil {
			return nil, fmt.Errorf("github: list PRs %s/%s (head=%s): %w", c.owner, c.repo, headBranch, err)
		}
		for _, pr := range prs {
			if pr.GetHead().GetRef() == headBranch {
				return convertPR(pr), nil
			}
		}
		if resp == nil || resp.NextPage == 0 {
			return nil, nil
		}
		opts.Page = resp.NextPage
	}
}

func (c *client) CreatePR(ctx context.Context, opts provider.CreatePROptions) (*provider.PullRequest, error) {
	if opts.Title == "" {
		return nil, errors.New("github: CreatePR title is empty")
	}
	if opts.HeadBranch == "" || opts.BaseBranch == "" {
		return nil, errors.New("github: CreatePR HeadBranch and BaseBranch are required")
	}
	pr, _, err := c.gh.PullRequests.Create(ctx, c.owner, c.repo, &gogh.NewPullRequest{
		Title: gogh.Ptr(opts.Title),
		Body:  gogh.Ptr(opts.Body),
		Head:  gogh.Ptr(opts.HeadBranch),
		Base:  gogh.Ptr(opts.BaseBranch),
	})
	if err != nil {
		return nil, fmt.Errorf("github: create PR %s -> %s: %w", opts.HeadBranch, opts.BaseBranch, err)
	}
	return convertPR(pr), nil
}

func (c *client) UpdatePR(ctx context.Context, number int, opts provider.UpdatePROptions) (*provider.PullRequest, error) {
	patch := &gogh.PullRequest{}
	if opts.Title != nil {
		patch.Title = opts.Title
	}
	if opts.Body != nil {
		patch.Body = opts.Body
	}
	pr, _, err := c.gh.PullRequests.Edit(ctx, c.owner, c.repo, number, patch)
	if err != nil {
		return nil, fmt.Errorf("github: edit PR #%d: %w", number, err)
	}
	return convertPR(pr), nil
}

func (c *client) ClosePR(ctx context.Context, number int) error {
	_, _, err := c.gh.PullRequests.Edit(ctx, c.owner, c.repo, number, &gogh.PullRequest{
		State: gogh.Ptr("closed"),
	})
	if err != nil {
		return fmt.Errorf("github: close PR #%d: %w", number, err)
	}
	return nil
}

func (c *client) CreateRelease(ctx context.Context, opts provider.CreateReleaseOptions) (*provider.Release, error) {
	if opts.Tag == "" {
		return nil, errors.New("github: CreateRelease Tag is empty")
	}
	name := opts.Name
	if name == "" {
		name = opts.Tag
	}
	rel, _, err := c.gh.Repositories.CreateRelease(ctx, c.owner, c.repo, &gogh.RepositoryRelease{
		TagName:    gogh.Ptr(opts.Tag),
		Name:       gogh.Ptr(name),
		Body:       gogh.Ptr(opts.Body),
		Prerelease: gogh.Ptr(opts.Prerelease),
	})
	if err != nil {
		return nil, fmt.Errorf("github: create release %q: %w", opts.Tag, err)
	}
	return &provider.Release{
		ID:      rel.GetID(),
		Tag:     rel.GetTagName(),
		HTMLURL: rel.GetHTMLURL(),
	}, nil
}

func (c *client) FindPRByMergeCommit(ctx context.Context, sha string) (*provider.PullRequest, error) {
	prs, _, err := c.gh.PullRequests.ListPullRequestsWithCommit(ctx, c.owner, c.repo, sha, &gogh.ListOptions{PerPage: 50})
	if err != nil {
		return nil, fmt.Errorf("github: list PRs for commit %s: %w", sha, err)
	}
	for _, pr := range prs {
		if pr.GetMergeCommitSHA() == sha {
			return convertPR(pr), nil
		}
	}
	return nil, nil
}

func convertPR(pr *gogh.PullRequest) *provider.PullRequest {
	if pr == nil {
		return nil
	}
	out := &provider.PullRequest{
		Number:    pr.GetNumber(),
		State:     strings.ToLower(pr.GetState()),
		Title:     pr.GetTitle(),
		Body:      pr.GetBody(),
		HTMLURL:   pr.GetHTMLURL(),
		MergedSHA: pr.GetMergeCommitSHA(),
	}
	if h := pr.GetHead(); h != nil {
		out.HeadRef = h.GetRef()
	}
	return out
}
