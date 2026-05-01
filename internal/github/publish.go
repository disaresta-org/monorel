package github

import (
	"context"
	"fmt"
	"strings"

	"github.com/disaresta-org/monorel/internal/plan"
	"github.com/disaresta-org/monorel/internal/release"
)

// PublishReleases creates one GitHub Release per tag in res, using the
// rendered body from res.Bodies as the release notes. Pre-release tags
// (those carrying a SemVer pre-release suffix) are flagged as
// pre-releases on GitHub.
//
// Returns the slice of created Releases in plan order. On the first
// failure the error is returned along with whatever Releases have
// been successfully created so the caller can decide whether to
// retry, roll back, or surface a partial-success message.
//
// Tags must already have been pushed to the remote before calling
// PublishReleases — GitHub validates the tag exists.
func PublishReleases(ctx context.Context, c Client, res *release.Result, p *plan.ReleasePlan) ([]*Release, error) {
	if res == nil {
		return nil, fmt.Errorf("github: nil release result")
	}
	if p == nil {
		return nil, fmt.Errorf("github: nil plan")
	}

	out := make([]*Release, 0, len(res.Tags))
	for _, r := range p.Releases {
		body := res.Bodies[r.Tag]
		rel, err := c.CreateRelease(ctx, CreateReleaseOptions{
			Tag:        r.Tag,
			Name:       r.Tag,
			Body:       body,
			Prerelease: r.Prerelease || strings.Contains(r.To, "-"),
		})
		if err != nil {
			return out, fmt.Errorf("publish %s: %w", r.Tag, err)
		}
		out = append(out, rel)
	}
	return out, nil
}
