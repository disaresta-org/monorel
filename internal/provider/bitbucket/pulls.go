package bitbucket

import (
	"context"
	"fmt"
	"net/url"
	"strconv"

	"monorel.disaresta.com/internal/provider"
)

// bbPullRequest is the slice of Bitbucket's PR resource shape that
// monorel cares about. JSON tags match the Bitbucket Cloud REST API
// v2 response shape (state is OPEN/MERGED/DECLINED/SUPERSEDED;
// summary.raw carries the description body; merge_commit may be null
// for unmerged PRs).
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
// State is normalized: OPEN -> "open"; everything else
// (MERGED / DECLINED / SUPERSEDED) -> "closed".
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
