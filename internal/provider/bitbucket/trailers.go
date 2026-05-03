package bitbucket

import (
	"context"
	"fmt"

	"monorel.disaresta.com/internal/provider"
)

// FindPRByMergeCommit queries Bitbucket's BBQL filter for a MERGED PR
// whose merge_commit.hash matches the given SHA. Returns (nil, nil)
// when no PR matches; an error only when the API call itself fails.
func (c *client) FindPRByMergeCommit(ctx context.Context, sha string) (*provider.PullRequest, error) {
	return c.firstPRMatching(ctx, fmt.Sprintf(`state="MERGED" AND merge_commit.hash=%q`, sha))
}
