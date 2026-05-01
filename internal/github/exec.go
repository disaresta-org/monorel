package github

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	gogh "github.com/google/go-github/v68/github"
	"golang.org/x/oauth2"
)

// Exec is a [Client] backed by go-github + oauth2. Constructed via
// [New]; no zero-value usage.
type Exec struct {
	owner string
	repo  string
	gh    *gogh.Client
}

// Options configures a new go-github-backed [Exec].
type Options struct {
	// Owner is the GitHub user or org that owns the repo (e.g.
	// "disaresta-org").
	Owner string

	// Repo is the repository name (e.g. "monorel").
	Repo string

	// Token is the personal access token or installation token
	// used for authenticated API calls. Required for any operation
	// that creates or modifies state; reads against public repos
	// also work without one but get tighter rate limits.
	Token string
}

// ErrMissingOwnerRepo is returned when the Options doesn't carry both
// an owner and repo. Without them every API call is meaningless.
var ErrMissingOwnerRepo = errors.New("github: Owner and Repo are required")

// New returns an [Exec] authenticated by Options.Token. An empty
// token yields an unauthenticated client (useful for read-only
// access to public repos in tests).
func New(ctx context.Context, opts Options) (*Exec, error) {
	if opts.Owner == "" || opts.Repo == "" {
		return nil, ErrMissingOwnerRepo
	}
	var httpClient *http.Client
	if opts.Token != "" {
		ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: opts.Token})
		httpClient = oauth2.NewClient(ctx, ts)
	}
	return &Exec{
		owner: opts.Owner,
		repo:  opts.Repo,
		gh:    gogh.NewClient(httpClient),
	}, nil
}

// GetDefaultBranch implements [Client.GetDefaultBranch].
func (e *Exec) GetDefaultBranch(ctx context.Context) (string, error) {
	repo, _, err := e.gh.Repositories.Get(ctx, e.owner, e.repo)
	if err != nil {
		return "", fmt.Errorf("get repo %s/%s: %w", e.owner, e.repo, err)
	}
	if repo.DefaultBranch == nil {
		return "", errors.New("repo has no default branch")
	}
	return *repo.DefaultBranch, nil
}

// FindOpenReleasePR implements [Client.FindOpenReleasePR].
func (e *Exec) FindOpenReleasePR(ctx context.Context, headBranch string) (*PullRequest, error) {
	// The GitHub API expects "owner:branch" for cross-repo PRs but
	// "branch" for same-repo PRs. monorel always operates within a
	// single repo, so the bare branch name suffices.
	prs, _, err := e.gh.PullRequests.List(ctx, e.owner, e.repo, &gogh.PullRequestListOptions{
		State: "open",
		Head:  fmt.Sprintf("%s:%s", e.owner, headBranch),
	})
	if err != nil {
		return nil, fmt.Errorf("list PRs (head=%s): %w", headBranch, err)
	}
	for _, pr := range prs {
		if pr.GetHead().GetRef() == headBranch {
			return convertPR(pr), nil
		}
	}
	return nil, nil
}

// CreatePR implements [Client.CreatePR].
func (e *Exec) CreatePR(ctx context.Context, opts CreatePROptions) (*PullRequest, error) {
	if opts.Title == "" {
		return nil, errors.New("github: CreatePR title is empty")
	}
	if opts.HeadBranch == "" || opts.BaseBranch == "" {
		return nil, errors.New("github: CreatePR HeadBranch and BaseBranch are required")
	}
	pr, _, err := e.gh.PullRequests.Create(ctx, e.owner, e.repo, &gogh.NewPullRequest{
		Title: gogh.Ptr(opts.Title),
		Body:  gogh.Ptr(opts.Body),
		Head:  gogh.Ptr(opts.HeadBranch),
		Base:  gogh.Ptr(opts.BaseBranch),
	})
	if err != nil {
		return nil, fmt.Errorf("create PR: %w", err)
	}
	return convertPR(pr), nil
}

// UpdatePR implements [Client.UpdatePR].
func (e *Exec) UpdatePR(ctx context.Context, number int, opts UpdatePROptions) (*PullRequest, error) {
	patch := &gogh.PullRequest{}
	dirty := false
	if opts.Title != nil {
		patch.Title = opts.Title
		dirty = true
	}
	if opts.Body != nil {
		patch.Body = opts.Body
		dirty = true
	}
	if !dirty {
		return nil, errors.New("github: UpdatePR has nothing to change")
	}
	pr, _, err := e.gh.PullRequests.Edit(ctx, e.owner, e.repo, number, patch)
	if err != nil {
		return nil, fmt.Errorf("edit PR #%d: %w", number, err)
	}
	return convertPR(pr), nil
}

// ClosePR implements [Client.ClosePR].
func (e *Exec) ClosePR(ctx context.Context, number int) error {
	_, _, err := e.gh.PullRequests.Edit(ctx, e.owner, e.repo, number, &gogh.PullRequest{
		State: gogh.Ptr("closed"),
	})
	if err != nil {
		return fmt.Errorf("close PR #%d: %w", number, err)
	}
	return nil
}

// CreateRelease implements [Client.CreateRelease].
func (e *Exec) CreateRelease(ctx context.Context, opts CreateReleaseOptions) (*Release, error) {
	if opts.Tag == "" {
		return nil, errors.New("github: CreateRelease Tag is empty")
	}
	name := opts.Name
	if name == "" {
		name = opts.Tag
	}
	rel, _, err := e.gh.Repositories.CreateRelease(ctx, e.owner, e.repo, &gogh.RepositoryRelease{
		TagName:    gogh.Ptr(opts.Tag),
		Name:       gogh.Ptr(name),
		Body:       gogh.Ptr(opts.Body),
		Prerelease: gogh.Ptr(opts.Prerelease),
	})
	if err != nil {
		return nil, fmt.Errorf("create release %q: %w", opts.Tag, err)
	}
	return &Release{
		ID:      rel.GetID(),
		Tag:     rel.GetTagName(),
		HTMLURL: rel.GetHTMLURL(),
	}, nil
}

func convertPR(pr *gogh.PullRequest) *PullRequest {
	if pr == nil {
		return nil
	}
	out := &PullRequest{
		Number:  pr.GetNumber(),
		State:   strings.ToLower(pr.GetState()),
		Title:   pr.GetTitle(),
		Body:    pr.GetBody(),
		HTMLURL: pr.GetHTMLURL(),
	}
	if h := pr.GetHead(); h != nil {
		out.HeadRef = h.GetRef()
	}
	return out
}
