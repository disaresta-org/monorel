package release

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/disaresta-org/monorel/internal/changelog"
	"github.com/disaresta-org/monorel/internal/config"
	"github.com/disaresta-org/monorel/internal/git"
)

// DiscoverPublishables walks the tags pointing at the repo's HEAD,
// matches each one to a configured package by tag prefix, and reads
// the matching CHANGELOG entry to build [ReleaseInfo] entries the
// forge publisher can consume.
//
// Used by `monorel publish`: after `monorel release` has created
// tags locally and `git push --follow-tags` has pushed them, the
// publisher needs to recover the per-tag body without re-running
// the planner (which sees an empty plan because the changesets are
// already deleted).
//
// In stable mode, the body comes from the package's CHANGELOG.md
// (the top entry whose version matches the tag). In pre-release
// mode, monorel does not write CHANGELOGs, so the body is empty;
// callers may want to substitute a stub like "Pre-release X.Y.Z-rc.N".
//
// Returns ([], nil) when there are no tags at HEAD — a normal
// no-op case for the publish step.
func DiscoverPublishables(cfg *config.Config, repo git.Repo, repoDir string) ([]ReleaseInfo, error) {
	tags, err := repo.TagsAtHead()
	if err != nil {
		return nil, fmt.Errorf("release: list tags at HEAD: %w", err)
	}

	// For each tag, find the package whose tag_prefix is the longest
	// match. Longest-match avoids the false positive where a package
	// with prefix "transports" claims tags that belong to a more
	// specific "transports/foo" package.
	pkgs := pkgsByPrefixLen(cfg)

	var infos []ReleaseInfo
	for _, tag := range tags {
		pkg, version := matchPackage(tag, pkgs)
		if pkg == nil || version == "" {
			continue
		}
		body, err := readChangelogBody(repoDir, *pkg, version)
		if err != nil {
			return nil, fmt.Errorf("release: read changelog for %s: %w", tag, err)
		}
		infos = append(infos, ReleaseInfo{
			Tag:        tag,
			Body:       body,
			Prerelease: strings.Contains(version, "-"),
		})
	}

	sort.Slice(infos, func(i, j int) bool { return infos[i].Tag < infos[j].Tag })
	return infos, nil
}

// pkgsByPrefixLen returns config packages sorted by tag_prefix length
// descending, so longest-match wins when two packages share a prefix.
func pkgsByPrefixLen(cfg *config.Config) []namedPkg {
	out := make([]namedPkg, 0, len(cfg.Packages))
	for name, pkg := range cfg.Packages {
		out = append(out, namedPkg{name: name, pkg: pkg})
	}
	sort.Slice(out, func(i, j int) bool {
		return len(out[i].pkg.TagPrefix) > len(out[j].pkg.TagPrefix)
	})
	return out
}

type namedPkg struct {
	name string
	pkg  config.PackageConfig
}

// matchPackage finds which package a tag belongs to, returning the
// package config and the version part. Returns (nil, "") when no
// package claims the tag.
func matchPackage(tag string, pkgs []namedPkg) (*config.PackageConfig, string) {
	for _, np := range pkgs {
		prefix := np.pkg.FullTagPrefix()
		if prefix == "" {
			// Bare-tag root: only match tags WITHOUT a "/".
			if !strings.Contains(tag, "/") {
				p := np.pkg
				return &p, tag
			}
			continue
		}
		if strings.HasPrefix(tag, prefix) {
			p := np.pkg
			return &p, strings.TrimPrefix(tag, prefix)
		}
	}
	return nil, ""
}

// readChangelogBody finds the entry for version in pkg.Changelog and
// returns its body. Missing file or missing entry returns "" with no
// error; callers can substitute a stub if they need one.
func readChangelogBody(repoDir string, pkg config.PackageConfig, version string) (string, error) {
	path := filepath.Join(repoDir, pkg.Changelog)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	want := strings.TrimPrefix(version, "v")
	topVersion, body, ok := changelog.ParseTopEntry(string(data))
	if !ok {
		return "", nil
	}
	if topVersion != want {
		// Tag's version doesn't match the top of the CHANGELOG.
		// Could happen in pre-release mode (no CHANGELOG entry yet)
		// or after a manual edit. Return empty body; let the caller
		// decide.
		return "", nil
	}
	return body, nil
}
