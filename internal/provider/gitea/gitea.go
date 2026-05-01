// Package gitea is the [provider.Client] implementation for Gitea
// and Forgejo (a Gitea fork that maintains API compatibility).
//
// Wraps code.gitea.io/sdk/gitea. Constructed via [New]; the returned
// concrete type satisfies [provider.Client] structurally.
//
// Forgejo: pass the Forgejo instance host via Options.Host. Forgejo
// retains Gitea's REST API shape, so the same SDK works against both.
package gitea

import (
	"context"
	"errors"
	"fmt"
	"strings"

	gtsdk "code.gitea.io/sdk/gitea"

	"monorel.disaresta.com/internal/provider"
)

// Options configures a new Gitea-backed [provider.Client].
type Options struct {
	// Owner is the user or org that owns the repo.
	Owner string

	// Repo is the repository name.
	Repo string

	// Host is the Gitea instance host, e.g. "gitea.example.com" or
	// "codeberg.org". Required: Gitea has no canonical "public host"
	// equivalent to api.github.com, so callers must specify which
	// instance to talk to. Use the bare hostname (no scheme); the
	// implementation prepends https://.
	Host string

	// Token is the access token for authenticated API calls.
	// Required for any operation that creates or modifies state.
	Token string
}

// ErrMissingOwnerRepo is returned when Options doesn't carry both an
// owner and a repo. Without them every API call is meaningless.
var ErrMissingOwnerRepo = errors.New("gitea: Owner and Repo are required")

// ErrMissingHost is returned when Options doesn't carry a Host. Gitea
// has no canonical public instance, so the host is always required.
var ErrMissingHost = errors.New("gitea: Host is required (e.g. gitea.example.com)")

type client struct {
	owner string
	repo  string
	gt    *gtsdk.Client
}

// New returns a [provider.Client] backed by code.gitea.io/sdk/gitea,
// authenticated by opts.Token.
func New(ctx context.Context, opts Options) (provider.Client, error) {
	if opts.Owner == "" || opts.Repo == "" {
		return nil, ErrMissingOwnerRepo
	}
	if opts.Host == "" {
		return nil, ErrMissingHost
	}

	baseURL := "https://" + strings.TrimSuffix(opts.Host, "/")
	clientOpts := []gtsdk.ClientOption{gtsdk.SetContext(ctx)}
	if opts.Token != "" {
		clientOpts = append(clientOpts, gtsdk.SetToken(opts.Token))
	}
	gt, err := gtsdk.NewClient(baseURL, clientOpts...)
	if err != nil {
		return nil, fmt.Errorf("gitea: connect %s: %w", baseURL, err)
	}
	return &client{owner: opts.Owner, repo: opts.Repo, gt: gt}, nil
}

func (c *client) GetDefaultBranch(ctx context.Context) (string, error) {
	repo, _, err := c.gt.GetRepo(c.owner, c.repo)
	if err != nil {
		return "", fmt.Errorf("gitea: get repo %s/%s: %w", c.owner, c.repo, err)
	}
	if repo.DefaultBranch == "" {
		return "", fmt.Errorf("gitea: %s/%s has no default branch", c.owner, c.repo)
	}
	return repo.DefaultBranch, nil
}

func (c *client) FindOpenReleasePR(ctx context.Context, headBranch string) (*provider.PullRequest, error) {
	// Gitea's ListRepoPullRequests doesn't filter by head; we list
	// open PRs and match on Head.Ref ourselves. The release PR's
	// head is bot-managed (default monorel/release), so the list
	// is short in practice.
	for page := 1; ; page++ {
		prs, _, err := c.gt.ListRepoPullRequests(c.owner, c.repo, gtsdk.ListPullRequestsOptions{
			ListOptions: gtsdk.ListOptions{Page: page, PageSize: 50},
			State:       gtsdk.StateOpen,
		})
		if err != nil {
			return nil, fmt.Errorf("gitea: list PRs %s/%s: %w", c.owner, c.repo, err)
		}
		if len(prs) == 0 {
			return nil, nil
		}
		for _, pr := range prs {
			if pr.Head != nil && pr.Head.Ref == headBranch {
				return convertPR(pr), nil
			}
		}
		if len(prs) < 50 {
			return nil, nil
		}
	}
}

func (c *client) CreatePR(ctx context.Context, opts provider.CreatePROptions) (*provider.PullRequest, error) {
	pr, _, err := c.gt.CreatePullRequest(c.owner, c.repo, gtsdk.CreatePullRequestOption{
		Title: opts.Title,
		Body:  opts.Body,
		Head:  opts.HeadBranch,
		Base:  opts.BaseBranch,
	})
	if err != nil {
		return nil, fmt.Errorf("gitea: create PR %s -> %s: %w", opts.HeadBranch, opts.BaseBranch, err)
	}
	return convertPR(pr), nil
}

func (c *client) UpdatePR(ctx context.Context, number int, opts provider.UpdatePROptions) (*provider.PullRequest, error) {
	// Title is a plain string in the SDK ("" means no change); Body
	// is *string (nil means no change). Map provider.UpdatePROptions's
	// uniform *string-as-no-change shape onto each.
	pr, _, err := c.gt.EditPullRequest(c.owner, c.repo, int64(number), gtsdk.EditPullRequestOption{
		Title: derefString(opts.Title),
		Body:  opts.Body,
	})
	if err != nil {
		return nil, fmt.Errorf("gitea: edit PR #%d: %w", number, err)
	}
	return convertPR(pr), nil
}

func (c *client) ClosePR(ctx context.Context, number int) error {
	closedState := gtsdk.StateClosed
	_, _, err := c.gt.EditPullRequest(c.owner, c.repo, int64(number), gtsdk.EditPullRequestOption{
		State: &closedState,
	})
	if err != nil {
		return fmt.Errorf("gitea: close PR #%d: %w", number, err)
	}
	return nil
}

func (c *client) CreateRelease(ctx context.Context, opts provider.CreateReleaseOptions) (*provider.Release, error) {
	rel, _, err := c.gt.CreateRelease(c.owner, c.repo, gtsdk.CreateReleaseOption{
		TagName:      opts.Tag,
		Title:        opts.Name,
		Note:         opts.Body,
		IsPrerelease: opts.Prerelease,
	})
	if err != nil {
		return nil, fmt.Errorf("gitea: create release %q: %w", opts.Tag, err)
	}
	return &provider.Release{
		ID:      rel.ID,
		Tag:     rel.TagName,
		HTMLURL: rel.HTMLURL,
	}, nil
}

// convertPR maps a Gitea SDK PullRequest into the provider-neutral
// shape. Returns nil when src is nil so callers can pass through
// nil-from-FindOpenReleasePR cleanly.
func convertPR(src *gtsdk.PullRequest) *provider.PullRequest {
	if src == nil {
		return nil
	}
	pr := &provider.PullRequest{
		Number:  int(src.Index),
		State:   string(src.State),
		Title:   src.Title,
		Body:    src.Body,
		HTMLURL: src.HTMLURL,
	}
	if src.Head != nil {
		pr.HeadRef = src.Head.Ref
	}
	return pr
}

// derefString returns *s, or "" when s is nil. The Gitea SDK's
// EditPullRequestOption.Title is a plain string; Gitea treats an
// empty string as "no change" on that field, so a nil *string from
// provider.UpdatePROptions maps cleanly to "".
func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
