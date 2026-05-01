// Package release applies a [plan.ReleasePlan] to a working tree.
//
// The flow is two-phase by design:
//
//   - [Apply] writes per-package CHANGELOG entries (stable mode) or
//     increments counters in .changeset/pre.json (pre-release mode),
//     deletes the consumed changeset files (stable only), and creates
//     a single commit. The commit body carries `monorel-Release:`
//     trailers — one per planned release — plus a `monorel-PreRelease`
//     flag. No tags are created.
//   - [Tag] reads HEAD's commit trailers, looks each released package
//     up in the config, and creates the corresponding git tags.
//
// The split exists for the always-open release-PR pattern: the CI
// wrapper runs Apply on a staging branch (so the always-open PR's
// diff IS the release), then Tag runs after the PR is merged into
// main (file changes are already there; only tags need creating).
//
// For one-shot local releases, [ApplyAndTag] runs both in sequence;
// the CLI's `monorel release` command is a thin wrapper around it.
//
// Pushing commits and tags to a remote is the caller's responsibility.
package release

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"monorel.disaresta.com/changelog"
	"monorel.disaresta.com/changeset"
	"monorel.disaresta.com/config"
	"monorel.disaresta.com/internal/git"
	"monorel.disaresta.com/plan"
)

// Options bundles the inputs to [Apply]. All fields except Today are
// required.
type Options struct {
	// Plan is the release plan to apply. Must be non-empty.
	Plan *plan.ReleasePlan

	// Repo is the git interface bound to RepoDir. Apply uses
	// ListTags (conflict check), Add/Remove (staging), Commit, and
	// CreateTag. It does NOT push — the caller orchestrates that.
	Repo git.Repo

	// RepoDir is the repository root. CHANGELOG paths from the
	// plan's PackageConfig are joined to this.
	RepoDir string

	// ChangesetDir is .changeset/ under RepoDir. Apply deletes
	// consumed *.md files here (stable mode) and writes pre.json
	// (pre mode).
	ChangesetDir string

	// PreState is the pre-release state. nil means stable mode:
	// changelogs are written and changesets deleted. Non-nil means
	// pre mode: counters incremented, no CHANGELOG / no changeset
	// deletion.
	PreState *changeset.PreState

	// Today overrides the date used in CHANGELOG entries (YYYY-MM-DD).
	// Empty string falls back to [changelog.Today]. Tests pin a value
	// for deterministic output.
	Today string
}

// Result reports what [Apply] did.
//
// Releases is the source of truth for what was published. By contract
// it is fully populated when err == nil (i.e. there is no partial
// Result). On any failure during tag creation, [Apply] returns
// (nil, err); downstream consumers can rely on Result being complete.
type Result struct {
	// CommitSHA is the SHA of the release commit.
	CommitSHA string

	// Releases is one entry per created tag, in plan order (sorted by
	// package name). The downstream provider publisher iterates this.
	Releases []ReleaseInfo
}

// ReleaseInfo is the post-Apply view of a single package release.
type ReleaseInfo struct {
	// Tag is the full git tag created at HEAD, e.g.
	// "transports/foo/v1.6.0" or "v1.6.0" (bare-tag root).
	Tag string

	// Body is the rendered Keep-a-Changelog entry for this release.
	// Populated even in pre-release mode (where it isn't written to
	// CHANGELOG.md) so a downstream GitHub-Releases publisher can
	// use it as the release notes.
	Body string

	// Prerelease mirrors [plan.PackageRelease.Prerelease]: true for
	// pre-release-mode tags. The provider publisher uses this to set
	// each provider's "pre-release" flag on the created release.
	Prerelease bool
}

// Tags returns the tag names from r.Releases in plan order. Convenient
// for human-readable output.
func (r *Result) Tags() []string {
	out := make([]string, len(r.Releases))
	for i, rel := range r.Releases {
		out[i] = rel.Tag
	}
	return out
}

// ErrPlanEmpty is returned by [Apply] when the plan has no releases.
// Callers should detect "nothing to release" earlier and skip the
// applier; this guards against silently making an empty commit.
var ErrPlanEmpty = errors.New("release: empty plan")

// ErrTagExists is returned when a planned tag already exists in the
// repository. Tag (and the [ApplyAndTag] preflight) aborts before
// any mutation, so the caller can safely retry after fixing the
// conflict (e.g. removing the stale tag or re-running the planner
// against fresh state).
var ErrTagExists = errors.New("release: tag already exists")

// ErrNoReleaseCommit is returned by [Tag] when HEAD's commit message
// has no `monorel-Release:` trailers. Indicates Tag was called on a
// non-release commit; usually means the speculative-apply step
// upstream didn't run, or main was advanced past the merge by a
// non-release commit before Tag fired.
var ErrNoReleaseCommit = errors.New("release: HEAD is not a release commit (no monorel-Release: trailer)")

// ErrUnknownReleasedPackage is returned by [Tag] when a
// `monorel-Release:` trailer names a package that no longer exists in
// monorel.toml. Usually means the config was edited between Apply
// (staging) and Tag (post-merge); rare and recoverable by either
// re-adding the package to the config or deleting the stale trailer.
var ErrUnknownReleasedPackage = errors.New("release: trailer names unknown package")

// Apply writes the file changes a release entails (CHANGELOGs in
// stable mode; pre.json counters in pre-release mode), deletes the
// consumed changeset files (stable mode only), and creates a single
// commit. The commit body carries `monorel-Release:` trailers so
// [Tag] can derive the per-package tags later without needing the
// plan in memory.
//
// Apply does NOT create tags; call [Tag] (or [ApplyAndTag] for the
// one-shot path) for that.
//
// Failure modes (no mutation has occurred):
//   - opts.Plan is empty → ErrPlanEmpty.
//   - listing tags or computing today's date fails.
//
// Failure modes (partial mutation possible):
//   - filesystem writes (CHANGELOG, pre.json) fail mid-flight.
//   - git Commit fails. The caller should treat the working tree as
//     untrusted and recover with `git status` and friends.
//
// Apply is intentionally not transactional: a failed `git commit`
// leaves the partial state visible so a human can inspect it.
func Apply(opts Options) (*Result, error) {
	if opts.Plan == nil || opts.Plan.IsEmpty() {
		return nil, ErrPlanEmpty
	}
	if opts.Repo == nil {
		return nil, errors.New("release: nil Repo")
	}
	if opts.RepoDir == "" {
		return nil, errors.New("release: empty RepoDir")
	}
	if opts.ChangesetDir == "" {
		return nil, errors.New("release: empty ChangesetDir")
	}

	today := opts.Today
	if today == "" {
		today = changelog.Today()
	}

	releases := make([]ReleaseInfo, 0, len(opts.Plan.Releases))
	for _, r := range opts.Plan.Releases {
		releases = append(releases, ReleaseInfo{
			Tag:        r.Tag,
			Body:       buildEntry(r, today).Render(),
			Prerelease: r.Prerelease,
		})
	}

	if opts.PreState == nil {
		if err := applyStable(opts, today); err != nil {
			return nil, err
		}
	} else {
		if err := applyPrerelease(opts); err != nil {
			return nil, err
		}
	}

	if err := opts.Repo.Commit(commitMessage(opts.Plan, opts.PreState != nil)); err != nil {
		return nil, fmt.Errorf("release: commit: %w", err)
	}

	commitSHA, err := opts.Repo.CurrentSHA()
	if err != nil {
		return nil, fmt.Errorf("release: read HEAD after commit: %w", err)
	}

	return &Result{CommitSHA: commitSHA, Releases: releases}, nil
}

// TagOptions bundles the inputs to [Tag].
type TagOptions struct {
	// Config is the parsed monorel.toml. Tag uses it to translate
	// each released package name into a tag prefix.
	Config *config.Config

	// Repo reads the HEAD commit message and creates the tags. Push
	// is the caller's job.
	Repo git.Repo
}

// Tag reads HEAD's commit trailers, looks each released package up in
// the config, and creates the corresponding annotated git tags at HEAD.
//
// HEAD must be a release commit produced by [Apply] (or [ApplyAndTag]).
// Tag returns [ErrNoReleaseCommit] if no `monorel-Release:` trailer
// is present, [ErrUnknownReleasedPackage] if a trailer names a package
// not declared in the config, and [ErrTagExists] (preflight) if any
// derived tag already exists in the repository.
//
// Tag is the post-merge step of the speculative-apply flow:
//
//   - release-pr.yml runs [Apply] on a staging branch and
//     force-pushes. The release PR's diff IS the file changes.
//   - The user merges the release PR. main now has the file changes
//     (CHANGELOGs etc) but no tags yet.
//   - release.yml runs Tag, which reads the merge commit's trailers
//     and creates the tags pointing at the merge commit.
func Tag(opts TagOptions) (*Result, error) {
	if opts.Config == nil {
		return nil, errors.New("release: nil Config")
	}
	if opts.Repo == nil {
		return nil, errors.New("release: nil Repo")
	}

	msg, err := opts.Repo.HeadCommitMessage()
	if err != nil {
		return nil, fmt.Errorf("release: read HEAD commit message: %w", err)
	}
	tagged, prerelease := parseReleaseTrailers(msg)
	if len(tagged) == 0 {
		return nil, ErrNoReleaseCommit
	}

	// Build the tag list and preflight: every named package must
	// exist in the config, no derived tag may already exist on the
	// repo. Preflight before any CreateTag so we don't half-tag a
	// release.
	releases := make([]ReleaseInfo, 0, len(tagged))
	for _, t := range tagged {
		pkg, ok := opts.Config.Packages[t.Name]
		if !ok {
			return nil, fmt.Errorf("%w: %q", ErrUnknownReleasedPackage, t.Name)
		}
		releases = append(releases, ReleaseInfo{
			Tag:        pkg.TagFor(t.Version),
			Prerelease: prerelease,
		})
	}

	existing, err := opts.Repo.ListTags("")
	if err != nil {
		return nil, fmt.Errorf("release: list tags: %w", err)
	}
	have := make(map[string]bool, len(existing))
	for _, t := range existing {
		have[t] = true
	}
	for _, r := range releases {
		if have[r.Tag] {
			return nil, fmt.Errorf("%w: %s", ErrTagExists, r.Tag)
		}
	}

	for i, r := range releases {
		// Annotated tag, message echoes the trailer for traceability.
		if err := opts.Repo.CreateTag(r.Tag, fmt.Sprintf("Release %s", tagged[i].Name)); err != nil {
			return nil, fmt.Errorf("release: create tag %q: %w", r.Tag, err)
		}
	}

	commitSHA, err := opts.Repo.CurrentSHA()
	if err != nil {
		return nil, fmt.Errorf("release: read HEAD: %w", err)
	}
	return &Result{CommitSHA: commitSHA, Releases: releases}, nil
}

// ApplyAndTag runs [Apply] then [Tag] in sequence. Convenience for
// one-shot local releases (the CLI's `monorel release` command). The
// returned Result merges Apply's bodies with Tag's tag list.
func ApplyAndTag(opts Options) (*Result, error) {
	if err := preflightPlannedTags(opts); err != nil {
		return nil, err
	}
	applyRes, err := Apply(opts)
	if err != nil {
		return nil, err
	}
	tagRes, err := Tag(TagOptions{Config: configFromPlan(opts.Plan), Repo: opts.Repo})
	if err != nil {
		return nil, err
	}
	// Merge: Apply produced the bodies; Tag produced the validated
	// tag names. Tag's order matches Apply's (both walk plan order).
	for i := range applyRes.Releases {
		applyRes.Releases[i].Tag = tagRes.Releases[i].Tag
	}
	applyRes.CommitSHA = tagRes.CommitSHA
	return applyRes, nil
}

// preflightPlannedTags errors if any planned tag already exists.
// Run before Apply in the [ApplyAndTag] path so the abort is clean.
func preflightPlannedTags(opts Options) error {
	existing, err := opts.Repo.ListTags("")
	if err != nil {
		return fmt.Errorf("release: list tags: %w", err)
	}
	have := make(map[string]bool, len(existing))
	for _, t := range existing {
		have[t] = true
	}
	for _, r := range opts.Plan.Releases {
		if have[r.Tag] {
			return fmt.Errorf("%w: %s", ErrTagExists, r.Tag)
		}
	}
	return nil
}

// configFromPlan reconstructs a minimal Config from a ReleasePlan so
// [Tag] can be called from [ApplyAndTag] without threading the
// caller's Config separately. Each plan release carries its
// PackageConfig already.
func configFromPlan(p *plan.ReleasePlan) *config.Config {
	pkgs := make(map[string]config.PackageConfig, len(p.Releases))
	for _, r := range p.Releases {
		pkgs[r.Name] = r.Config
	}
	return &config.Config{Packages: pkgs}
}

// applyStable writes CHANGELOG entries, deletes consumed changesets,
// and stages everything. Caller does the commit.
func applyStable(opts Options, today string) error {
	for _, r := range opts.Plan.Releases {
		entry := buildEntry(r, today)
		if entry.IsEmpty() {
			// Defensive: a release with no buckets means there
			// were changesets but they all had None bumps,
			// which the planner shouldn't produce. Skip.
			continue
		}
		path := filepath.Join(opts.RepoDir, r.Config.Changelog)
		if err := changelog.WriteFile(path, entry); err != nil {
			return fmt.Errorf("release: write %s: %w", path, err)
		}
		if err := opts.Repo.Add(r.Config.Changelog); err != nil {
			return fmt.Errorf("release: stage %s: %w", r.Config.Changelog, err)
		}
	}

	// Delete the consumed changesets. The planner's Consumed list
	// dedupes multi-package changesets, so we hit each file once.
	for _, cs := range opts.Plan.Consumed {
		rel := filepath.Join(".changeset", cs.Name+".md")
		if err := opts.Repo.Remove(rel); err != nil {
			return fmt.Errorf("release: remove %s: %w", rel, err)
		}
	}
	return nil
}

// applyPrerelease increments per-package counters, writes pre.json,
// and stages it.
func applyPrerelease(opts Options) error {
	for _, r := range opts.Plan.Releases {
		opts.PreState.IncrementCounter(r.Name)
	}
	if err := opts.PreState.Write(opts.ChangesetDir); err != nil {
		return fmt.Errorf("release: write pre.json: %w", err)
	}
	rel := filepath.Join(".changeset", changeset.PreStateFilename)
	if err := opts.Repo.Add(rel); err != nil {
		return fmt.Errorf("release: stage %s: %w", rel, err)
	}
	return nil
}

// buildEntry collects the changeset bodies for r and buckets them by
// the bump level each changeset requested for THIS package. The
// per-package level may differ from the cumulative max recorded in
// r.Bump (e.g. a multi-package changeset with major for foo and patch
// for bar).
func buildEntry(r plan.PackageRelease, today string) *changelog.Entry {
	e := &changelog.Entry{
		Version: r.To,
		Date:    today,
	}
	for _, cs := range r.Changesets {
		level, ok := cs.Bumps[r.Name]
		if !ok {
			continue
		}
		body := cs.Body
		switch level.String() {
		case "major":
			e.Major = append(e.Major, body)
		case "minor":
			e.Minor = append(e.Minor, body)
		case "patch":
			e.Patch = append(e.Patch, body)
		}
	}
	return e
}

// commitMessage formats the release commit's full message: a
// conventional-commit-style subject naming every package, followed by
// machine-readable trailers ([Tag] reads these to derive tag names).
//
// Trailer format (one `monorel-Release:` per package, in plan order;
// `monorel-PreRelease:` once at the end):
//
//	monorel-Release: <package-name> <version-with-leading-v>
//	monorel-PreRelease: true|false
//
// Trailers are stable across the v0.2 → v0.x line; they're the
// contract Apply / Tag share, replacing the old in-process plan
// hand-off.
func commitMessage(p *plan.ReleasePlan, pre bool) string {
	mode := ""
	if pre {
		mode = " (pre-release)"
	}

	var subject string
	if len(p.Releases) == 1 {
		r := p.Releases[0]
		subject = fmt.Sprintf("chore(release): %s %s%s", r.Name, r.To, mode)
	} else {
		var b strings.Builder
		b.WriteString("chore(release): ")
		for i, r := range p.Releases {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(r.Name)
			b.WriteString(" ")
			b.WriteString(r.To)
		}
		b.WriteString(mode)
		subject = b.String()
	}

	var trailers strings.Builder
	for _, r := range p.Releases {
		fmt.Fprintf(&trailers, "monorel-Release: %s %s\n", r.Name, r.To)
	}
	fmt.Fprintf(&trailers, "monorel-PreRelease: %t\n", pre)

	return subject + "\n\n" + trailers.String()
}

// taggedRelease is a single (name, version) pair extracted from a
// release commit's `monorel-Release:` trailer.
type taggedRelease struct {
	Name    string
	Version string
}

// parseReleaseTrailers walks a commit message looking for
// `monorel-Release:` and `monorel-PreRelease:` trailers. Returns the
// list of (name, version) pairs in the order they appeared, plus the
// pre-release flag. Tolerates extra surrounding whitespace and an
// arbitrary number of body paragraphs between the subject and the
// trailers.
//
// Lines that don't match either trailer form are ignored. An empty
// return slice means "no monorel trailers found"; the caller should
// treat that as "this is not a release commit."
func parseReleaseTrailers(msg string) ([]taggedRelease, bool) {
	var (
		releases   []taggedRelease
		prerelease bool
	)
	for _, line := range strings.Split(msg, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "monorel-Release:"):
			rest := strings.TrimSpace(strings.TrimPrefix(line, "monorel-Release:"))
			// Last whitespace-separated token is the version; the
			// rest is the package name (which may itself contain
			// whitespace? convention says no, but be tolerant).
			fields := strings.Fields(rest)
			if len(fields) < 2 {
				continue
			}
			name := strings.Join(fields[:len(fields)-1], " ")
			version := fields[len(fields)-1]
			releases = append(releases, taggedRelease{Name: name, Version: version})
		case strings.HasPrefix(line, "monorel-PreRelease:"):
			rest := strings.TrimSpace(strings.TrimPrefix(line, "monorel-PreRelease:"))
			prerelease = strings.EqualFold(rest, "true")
		}
	}
	return releases, prerelease
}
