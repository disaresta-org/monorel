// Package plan computes a [ReleasePlan] from the static inputs that
// drive a monorel release: the parsed monorel.toml, the set of pending
// changesets, and the list of existing git tags.
//
// Plan is a pure function. It performs no I/O; callers wire git and
// filesystem reads into the inputs. This is deliberate: the planner is
// the load-bearing logic of the tool, and "pure function over plain
// data" is the cheapest way to give it exhaustive table-driven test
// coverage.
//
// Higher layers feed Plan's output to the changelog writer, the
// release applier, and the GitHub Action orchestrator. None of those
// layers re-derive versions from changesets — the plan is the single
// source of truth for "what should release."
package plan

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/disaresta-org/monorel/internal/changeset"
	"github.com/disaresta-org/monorel/internal/config"
	"github.com/disaresta-org/monorel/internal/semver"
)

// ReleasePlan is the result of [Plan]: every package that needs to be
// released this run, with its computed version transition and the
// changesets that contributed.
//
// A plan with no Releases is a valid no-op outcome: it means there
// are no pending changesets, or every changeset was a no-op for some
// reason. Callers handle this case (close the release PR, exit 0).
type ReleasePlan struct {
	// Releases is one entry per package that has at least one
	// affecting changeset, sorted by package name. Packages with no
	// changesets don't appear.
	Releases []PackageRelease

	// Consumed is the set of changesets the plan covers. Sorted by
	// changeset name. The release applier deletes the corresponding
	// .changeset/<name>.md files after committing the release.
	//
	// A single changeset that targets multiple packages appears once
	// here, not once per affected package.
	Consumed []*changeset.Changeset
}

// IsEmpty reports whether the plan has nothing to release.
func (p *ReleasePlan) IsEmpty() bool {
	return len(p.Releases) == 0
}

// PackageRelease is one package's slice of a [ReleasePlan].
type PackageRelease struct {
	// Name is the package name as it appears in monorel.toml's
	// [packages] table and in changeset frontmatter keys.
	Name string

	// Config is the corresponding [config.PackageConfig] copy. The
	// release applier reads Path, Changelog, and TagPrefix from it.
	Config config.PackageConfig

	// From is the previous version this release bumps from, or "" if
	// the package has never been released. Excludes any pre-release
	// suffix; see [Plan]'s "pre-release tags" note.
	From string

	// To is the new version this release produces. Includes the
	// pre-release suffix when [PackageRelease.Prerelease] is true,
	// e.g. "v1.7.0-rc.2".
	To string

	// Bump is the level computed from the package's affecting
	// changesets (max across them).
	Bump semver.BumpLevel

	// Tag is the full git tag To corresponds to under the package's
	// configured tag_prefix, e.g. "transports/zerolog/v1.7.0" or
	// "v1.7.0" for a bare-tag root module.
	Tag string

	// Changesets are the changesets that contributed to this
	// release, sorted by name. A given changeset may appear in
	// multiple PackageRelease.Changesets if it targets multiple
	// packages.
	Changesets []*changeset.Changeset

	// Initial is true when this is the package's first release (no
	// prior tag exists for its tag_prefix).
	Initial bool

	// Prerelease is true when this release is part of a pre-release
	// window (a [changeset.PreState] was passed to [Plan]). The
	// release applier increments the per-package counter on the
	// PreState after a successful release.
	Prerelease bool

	// PrereleaseCounter is the counter value baked into [To] when
	// Prerelease is true. Zero for a stable release.
	PrereleaseCounter int
}

// Plan computes the [ReleasePlan] from the given inputs.
//
// Inputs:
//   - cfg: the parsed monorel.toml. Must already be validated.
//   - changesets: pending changesets (typically loaded from
//     .changeset/ via [changeset.LoadAll]).
//   - tags: every git tag in the repository, regardless of prefix.
//     Tags that aren't valid semver are silently ignored. Pre-release
//     tags (e.g. "v1.0.0-rc.1") are also ignored when picking the
//     "current stable version" for a package — pre-release flow uses
//     pre to override From/To after the stable computation.
//   - pre: optional pre-release state (nil means stable-mode planning).
//     When non-nil, To is suffixed with "-<channel>.<counter>" and the
//     [PackageRelease.Prerelease] / [PackageRelease.PrereleaseCounter]
//     fields are populated. The counter for each package is read from
//     pre.Counters; the planner does NOT mutate pre (the release
//     applier increments after a successful release).
//
// Errors:
//   - A changeset names a package not in cfg.Packages.
//   - InitialFromBump fails (shouldn't happen with valid bump levels).
//   - semver.Apply fails (shouldn't happen with valid existing tags).
//
// Determinism: Releases and Consumed are returned in sorted order so
// repeated calls with the same inputs produce identical output.
func Plan(cfg *config.Config, changesets []*changeset.Changeset, tags []string, pre *changeset.PreState) (*ReleasePlan, error) {
	if cfg == nil {
		return nil, errors.New("plan: nil config")
	}

	// Group changesets by affected package, validating package names
	// as we go. A single changeset can affect multiple packages; it
	// appears in multiple buckets.
	byPackage := make(map[string][]*changeset.Changeset)
	consumedSeen := make(map[string]bool)
	var consumed []*changeset.Changeset

	for _, cs := range changesets {
		for pkg := range cs.Bumps {
			if _, ok := cfg.Packages[pkg]; !ok {
				return nil, fmt.Errorf("changeset %q: unknown package %q", cs.Name, pkg)
			}
			byPackage[pkg] = append(byPackage[pkg], cs)
		}
		if !consumedSeen[cs.Name] {
			consumedSeen[cs.Name] = true
			consumed = append(consumed, cs)
		}
	}

	// Build a release entry for each package that has at least one
	// affecting changeset. Packages with no changesets don't appear
	// — release-please's "everyone bumps together" model is what
	// monorel deliberately avoids.
	var releases []PackageRelease
	for pkg, css := range byPackage {
		pkgCfg := cfg.Packages[pkg]

		// Sort the package's changesets by name so the per-package
		// changeset list is deterministic.
		sort.Slice(css, func(i, j int) bool { return css[i].Name < css[j].Name })

		// Reduce to the strongest bump level any of these changesets
		// requested for THIS package.
		bump := semver.None
		for _, cs := range css {
			if level, ok := cs.Bumps[pkg]; ok {
				bump = semver.Max(bump, level)
			}
		}

		from, fromOK := latestStableTagVersion(tags, pkgCfg)
		var stableTo string
		var initial bool
		if fromOK {
			next, err := semver.Apply(from, bump)
			if err != nil {
				return nil, fmt.Errorf("package %q: bump %s from %s: %w", pkg, bump, from, err)
			}
			stableTo = next
		} else {
			next, err := semver.InitialFromBump(bump)
			if err != nil {
				return nil, fmt.Errorf("package %q: initial release: %w", pkg, err)
			}
			stableTo = next
			initial = true
		}

		to := stableTo
		var preBool bool
		var counter int
		if pre != nil {
			counter = pre.CounterFor(pkg)
			suffixed, err := semver.ApplyPrerelease(stableTo, pre.Channel, counter)
			if err != nil {
				return nil, fmt.Errorf("package %q: pre-release suffix: %w", pkg, err)
			}
			to = suffixed
			preBool = true
		}

		releases = append(releases, PackageRelease{
			Name:              pkg,
			Config:            pkgCfg,
			From:              from,
			To:                to,
			Bump:              bump,
			Tag:               pkgCfg.TagFor(to),
			Changesets:        css,
			Initial:           initial,
			Prerelease:        preBool,
			PrereleaseCounter: counter,
		})
	}

	sort.Slice(releases, func(i, j int) bool { return releases[i].Name < releases[j].Name })
	sort.Slice(consumed, func(i, j int) bool { return consumed[i].Name < consumed[j].Name })

	return &ReleasePlan{Releases: releases, Consumed: consumed}, nil
}

// latestStableTagVersion finds the highest semver-valid, non-pre-release
// tag for pkg's configured tag_prefix and returns its version part
// (e.g. "v1.6.2"). The bool reports whether any qualifying tag existed.
//
// Tags that don't parse as semver are silently ignored; pre-release
// tags are ignored at this layer (pre-release-mode handling lives in
// a layer above the planner).
func latestStableTagVersion(tags []string, pkg config.PackageConfig) (string, bool) {
	prefix := pkg.FullTagPrefix()
	var best string
	for _, tag := range tags {
		if prefix != "" && !strings.HasPrefix(tag, prefix) {
			continue
		}
		if prefix == "" && strings.Contains(tag, "/") {
			// Bare-tag root: a tag like "transports/foo/v1.0.0"
			// must not match the empty-prefix package. Only tags
			// without a "/" qualify.
			continue
		}
		version := strings.TrimPrefix(tag, prefix)
		if !semver.IsValid(version) || semver.IsPrerelease(version) {
			continue
		}
		if best == "" || semver.Compare(version, best) > 0 {
			best = version
		}
	}
	return best, best != ""
}
