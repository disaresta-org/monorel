package bitbucket

import (
	"context"
	"net/url"
	"strings"

	"monorel.disaresta.com/internal/provider"
)

// CreateRelease is a no-op for Bitbucket Cloud, which has no
// first-class Release concept. Returns a synthetic *Release whose
// HTMLURL points at the tag's source view (bitbucket.org/<ws>/<repo>/src/<tag>).
//
// The orchestrator's call site treats CreateRelease as advisory:
// per-package CHANGELOG.md is the canonical release-notes source.
func (c *client) CreateRelease(_ context.Context, opts provider.CreateReleaseOptions) (*provider.Release, error) {
	htmlURL := "https://bitbucket.org/" +
		url.PathEscape(c.workspace) + "/" +
		url.PathEscape(c.repo) + "/src/" + escapeTagPath(opts.Tag)
	return &provider.Release{
		Tag:     opts.Tag,
		HTMLURL: htmlURL,
	}, nil
}

// escapeTagPath URL-escapes each /-separated segment of a tag path so
// the slashes remain literal path separators (path-prefixed tags like
// "transports/foo/v1.7.0" stay readable in the URL).
func escapeTagPath(tag string) string {
	parts := strings.Split(tag, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return strings.Join(parts, "/")
}
