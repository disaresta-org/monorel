// Package gitlab is the [provider.Client] implementation for GitLab.com
// and self-hosted GitLab (Community / Enterprise Edition).
//
// Wraps gitlab.com/gitlab-org/api/client-go (the official Go SDK,
// renamed from the long-standing community fork github.com/xanzy/go-gitlab).
// Constructed via [New]; the returned concrete type satisfies
// [provider.Client] structurally.
package gitlab

import (
	"context"
	"errors"
	"fmt"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go"

	"monorel.disaresta.com/internal/provider"
)

// Options configures a new GitLab-backed [provider.Client].
type Options struct {
	// Owner is the user, group, or sub-group path that owns the
	// project. May contain slashes for nested sub-groups
	// (e.g. "team/platform" for projects under team/platform/).
	Owner string

	// Repo is the project path component (the last segment of the
	// project's full path).
	Repo string

	// Host is the GitLab instance host. Empty defaults to
	// gitlab.com. Accepts a bare hostname ("gitlab.example.com")
	// or a fully-qualified URL ("https://gitlab.example.com").
	Host string

	// Token is the personal access token (or CI_JOB_TOKEN, or
	// project access token) used for authenticated API calls.
	// Required: GitLab rejects unauthenticated writes.
	Token string
}

// ErrMissingOwnerRepo is returned when Options doesn't carry both an
// owner and a repo.
var ErrMissingOwnerRepo = errors.New("gitlab: Owner and Repo are required")

// ErrMissingToken is returned when Options.Token is empty. GitLab's
// Merge Requests / Releases APIs reject anonymous writes.
var ErrMissingToken = errors.New("gitlab: Token is required")

type client struct {
	pid string // url-encodable "owner/repo" path
	gl  *gl.Client
}

// New returns a [provider.Client] backed by gitlab.com/gitlab-org/api/client-go.
// Empty Host defaults to gitlab.com.
func New(_ context.Context, opts Options) (provider.Client, error) {
	if opts.Owner == "" || opts.Repo == "" {
		return nil, ErrMissingOwnerRepo
	}
	if opts.Token == "" {
		return nil, ErrMissingToken
	}

	clientOpts := []gl.ClientOptionFunc{}
	if opts.Host != "" {
		clientOpts = append(clientOpts, gl.WithBaseURL(normalizeHost(opts.Host)))
	}
	c, err := gl.NewClient(opts.Token, clientOpts...)
	if err != nil {
		return nil, fmt.Errorf("gitlab: connect: %w", err)
	}
	return &client{pid: opts.Owner + "/" + opts.Repo, gl: c}, nil
}

func (c *client) GetDefaultBranch(_ context.Context) (string, error) {
	proj, _, err := c.gl.Projects.GetProject(c.pid, nil)
	if err != nil {
		return "", fmt.Errorf("gitlab: get project %s: %w", c.pid, err)
	}
	if proj.DefaultBranch == "" {
		return "", fmt.Errorf("gitlab: %s has no default branch", c.pid)
	}
	return proj.DefaultBranch, nil
}

func (c *client) FindOpenReleasePR(_ context.Context, headBranch string) (*provider.PullRequest, error) {
	var page int64 = 1
	for {
		mrs, resp, err := c.gl.MergeRequests.ListProjectMergeRequests(c.pid, &gl.ListProjectMergeRequestsOptions{
			ListOptions:  gl.ListOptions{Page: page, PerPage: 50},
			State:        gl.Ptr("opened"),
			SourceBranch: gl.Ptr(headBranch),
		})
		if err != nil {
			return nil, fmt.Errorf("gitlab: list MRs %s: %w", c.pid, err)
		}
		for _, mr := range mrs {
			if mr.SourceBranch == headBranch {
				return basicMRToPR(mr), nil
			}
		}
		if resp == nil || resp.NextPage == 0 {
			return nil, nil
		}
		page = resp.NextPage
	}
}

func (c *client) CreatePR(_ context.Context, opts provider.CreatePROptions) (*provider.PullRequest, error) {
	mr, _, err := c.gl.MergeRequests.CreateMergeRequest(c.pid, &gl.CreateMergeRequestOptions{
		Title:        gl.Ptr(opts.Title),
		Description:  gl.Ptr(opts.Body),
		SourceBranch: gl.Ptr(opts.HeadBranch),
		TargetBranch: gl.Ptr(opts.BaseBranch),
	})
	if err != nil {
		return nil, fmt.Errorf("gitlab: create MR %s -> %s: %w", opts.HeadBranch, opts.BaseBranch, err)
	}
	return mrToPR(mr), nil
}

func (c *client) UpdatePR(_ context.Context, number int, opts provider.UpdatePROptions) (*provider.PullRequest, error) {
	patch := &gl.UpdateMergeRequestOptions{
		Title:       opts.Title,
		Description: opts.Body,
	}
	mr, _, err := c.gl.MergeRequests.UpdateMergeRequest(c.pid, int64(number), patch)
	if err != nil {
		return nil, fmt.Errorf("gitlab: update MR !%d: %w", number, err)
	}
	return mrToPR(mr), nil
}

func (c *client) ClosePR(_ context.Context, number int) error {
	_, _, err := c.gl.MergeRequests.UpdateMergeRequest(c.pid, int64(number), &gl.UpdateMergeRequestOptions{
		StateEvent: gl.Ptr("close"),
	})
	if err != nil {
		return fmt.Errorf("gitlab: close MR !%d: %w", number, err)
	}
	return nil
}

func (c *client) CreateRelease(_ context.Context, opts provider.CreateReleaseOptions) (*provider.Release, error) {
	// GitLab has no first-class "prerelease" flag. The SemVer pre-
	// release suffix on the tag (e.g. "-rc.0") is the only signal;
	// monorel's tag naming already encodes it.
	rel, _, err := c.gl.Releases.CreateRelease(c.pid, &gl.CreateReleaseOptions{
		TagName:     gl.Ptr(opts.Tag),
		Name:        gl.Ptr(opts.Name),
		Description: gl.Ptr(opts.Body),
	})
	if err != nil {
		return nil, fmt.Errorf("gitlab: create release %q: %w", opts.Tag, err)
	}
	return &provider.Release{
		// GitLab releases have no integer ID; they're addressed by
		// tag name. ID stays zero.
		Tag:     rel.TagName,
		HTMLURL: rel.Links.Self,
	}, nil
}

// mrToPR maps a GitLab full MergeRequest into the provider-neutral
// shape. Number is the per-project IID (`!N` in the GitLab UI), not
// the global ID; the provider interface uses IID throughout.
func mrToPR(src *gl.MergeRequest) *provider.PullRequest {
	if src == nil {
		return nil
	}
	return &provider.PullRequest{
		Number:  int(src.IID),
		State:   src.State,
		Title:   src.Title,
		Body:    src.Description,
		HeadRef: src.SourceBranch,
		HTMLURL: src.WebURL,
	}
}

// basicMRToPR mirrors mrToPR for the BasicMergeRequest shape returned
// by list endpoints. Same field set, different wrapper struct in the
// SDK.
func basicMRToPR(src *gl.BasicMergeRequest) *provider.PullRequest {
	if src == nil {
		return nil
	}
	return &provider.PullRequest{
		Number:  int(src.IID),
		State:   src.State,
		Title:   src.Title,
		Body:    src.Description,
		HeadRef: src.SourceBranch,
		HTMLURL: src.WebURL,
	}
}

// normalizeHost turns Options.Host into the base URL the SDK expects.
// Mirrors the gitea provider's helper: bare hostnames default to
// https://; fully-qualified URLs are passed through. Scheme detection
// is case-insensitive.
func normalizeHost(host string) string {
	host = strings.TrimSuffix(host, "/")
	lower := strings.ToLower(host)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return host
	}
	return "https://" + host
}
