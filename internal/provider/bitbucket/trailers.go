package bitbucket

import (
	"context"
	"fmt"
	"net/url"

	"monorel.disaresta.com/internal/provider"
)

// FindPRByMergeCommit queries Bitbucket's BBQL filter for a MERGED PR
// whose merge_commit.hash matches the given SHA. Returns (nil, nil)
// when no PR matches; an error only when the API call itself fails.
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
