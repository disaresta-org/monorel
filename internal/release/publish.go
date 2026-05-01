package release

import (
	"context"
	"fmt"

	"github.com/disaresta-org/monorel/internal/forge"
)

// PublishReleases creates one forge release per entry in res.Releases,
// using each entry's rendered body as the release notes and its
// Prerelease flag to mark pre-release tags. Releases are created in
// res.Releases order (which matches plan order).
//
// Returns the slice of created [forge.Release]s. On the first failure
// the partial slice is returned alongside the error so the caller can
// surface "created N/M before failing" output. Tags must already have
// been pushed to the remote before calling PublishReleases — most
// forges validate the tag exists.
//
// A forge that doesn't model first-class releases (e.g. plain
// Bitbucket) may have its [forge.Client.CreateRelease] return an
// "unsupported" error; callers can detect that and treat it as
// advisory rather than fatal.
func PublishReleases(ctx context.Context, c forge.Client, res *Result) ([]*forge.Release, error) {
	if res == nil {
		return nil, fmt.Errorf("release: nil result")
	}

	out := make([]*forge.Release, 0, len(res.Releases))
	for _, r := range res.Releases {
		rel, err := c.CreateRelease(ctx, forge.CreateReleaseOptions{
			Tag:        r.Tag,
			Name:       r.Tag,
			Body:       r.Body,
			Prerelease: r.Prerelease,
		})
		if err != nil {
			return out, fmt.Errorf("publish %s: %w", r.Tag, err)
		}
		out = append(out, rel)
	}
	return out, nil
}
