// Package release applies a [plan.ReleasePlan] to a working tree:
// writes per-package CHANGELOG entries, deletes the consumed changeset
// files, commits, and creates tags. Pushing the commit and tags to a
// remote is the caller's responsibility (typically the CLI release
// command in stable mode, or the GitHub Action orchestrator in
// pre-release mode).
//
// In pre-release mode the applier does NOT touch CHANGELOGs and does
// NOT delete changesets — it only increments per-package counters in
// .changeset/pre.json and creates the suffixed pre tags. The stable
// release that follows pre exit consumes the accumulated changesets
// and writes a single cumulative CHANGELOG entry per package.
package release

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/disaresta-org/monorel/internal/changelog"
	"github.com/disaresta-org/monorel/internal/changeset"
	"github.com/disaresta-org/monorel/internal/git"
	"github.com/disaresta-org/monorel/internal/plan"
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
type Result struct {
	// CommitSHA is the SHA of the release commit.
	CommitSHA string

	// Tags is every tag created, in plan order (sorted by package
	// name). Same shape as the corresponding PackageRelease.Tag.
	Tags []string
}

// ErrPlanEmpty is returned by [Apply] when the plan has no releases.
// Callers should detect "nothing to release" earlier and skip the
// applier; this guards against silently making an empty commit.
var ErrPlanEmpty = errors.New("release: empty plan")

// ErrTagExists is returned when a planned tag already exists in the
// repository. Apply aborts before any mutation, so the caller can
// safely retry after fixing the conflict (e.g. removing the stale
// tag or re-running the planner against fresh state).
var ErrTagExists = errors.New("release: tag already exists")

// Apply executes the plan. On success it returns a [Result] with the
// new commit SHA and the list of created tags.
//
// Failure modes (no mutation has occurred):
//   - opts.Plan is empty → ErrPlanEmpty.
//   - any planned tag already exists → wrapped ErrTagExists.
//   - listing tags or computing today's date fails.
//
// Failure modes (partial mutation possible):
//   - filesystem writes (CHANGELOG, pre.json) fail mid-flight.
//   - git Commit/CreateTag fails. The caller should treat the working
//     tree as untrusted and recover with `git status` and friends.
//
// Apply is intentionally not transactional: a failed `git commit`
// leaves the partial state visible so a human can inspect it. The
// invariants enforced before any mutation (tag-exists check) prevent
// the common "second-run on top of an in-flight release" footgun.
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

	if err := preflightTags(opts); err != nil {
		return nil, err
	}

	today := opts.Today
	if today == "" {
		today = changelog.Today()
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

	tags := make([]string, 0, len(opts.Plan.Releases))
	for _, r := range opts.Plan.Releases {
		if err := opts.Repo.CreateTag(r.Tag, fmt.Sprintf("Release %s %s", r.Name, r.To)); err != nil {
			return nil, fmt.Errorf("release: create tag %q: %w", r.Tag, err)
		}
		tags = append(tags, r.Tag)
	}

	return &Result{CommitSHA: commitSHA, Tags: tags}, nil
}

// preflightTags errors if any planned tag already exists. Run before
// any mutation so the abort is clean.
func preflightTags(opts Options) error {
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

// commitMessage formats the conventional-commit-style message that
// names every package in the release. Pre-mode commits get a
// "(pre-release)" suffix so reflogs make the boundary obvious.
func commitMessage(p *plan.ReleasePlan, pre bool) string {
	mode := ""
	if pre {
		mode = " (pre-release)"
	}
	if len(p.Releases) == 1 {
		r := p.Releases[0]
		return fmt.Sprintf("chore(release): %s %s%s", r.Name, r.To, mode)
	}
	// Multi-package: list the largest few and a count.
	var b []byte
	b = append(b, "chore(release): "...)
	for i, r := range p.Releases {
		if i > 0 {
			b = append(b, ", "...)
		}
		b = append(b, r.Name...)
		b = append(b, ' ')
		b = append(b, r.To...)
	}
	b = append(b, mode...)
	return string(b)
}
