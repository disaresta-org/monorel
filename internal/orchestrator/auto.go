package orchestrator

import (
	"context"
	"errors"
	"fmt"

	"monorel.disaresta.com/changelog"
	"monorel.disaresta.com/changeset"
	"monorel.disaresta.com/config"
	"monorel.disaresta.com/internal/detect"
	"monorel.disaresta.com/internal/git"
	"monorel.disaresta.com/internal/provider"
	"monorel.disaresta.com/internal/release"
	"monorel.disaresta.com/plan"
)

// AutoBranch describes which path Auto took. Stringly-typed at the
// wire level; constants let callers switch exhaustively.
type AutoBranch string

const (
	// AutoBranchRelease means the post-merge release flow ran:
	// monorel tag, push tags, monorel publish.
	AutoBranchRelease AutoBranch = "release"

	// AutoBranchFeature means the upstream-of-merge feature flow ran:
	// monorel apply on the speculative branch (when a plan is
	// pending), force-push the branch (likewise), then upsert the
	// release PR via the orchestrator.
	AutoBranchFeature AutoBranch = "feature"
)

// AutoOptions bundles the inputs to [Auto]. Mirrors the existing
// [Options] but adds the read-only state Auto needs to dispatch
// (changesets, tags, pre-state) plus the two paths' file-system
// dependencies (RepoDir, ChangesetDir).
type AutoOptions struct {
	// Config is the parsed monorel.toml.
	Config *config.Config

	// Repo is the git interface for the working repository.
	Repo git.Repo

	// Provider is the provider client (required; nil errors).
	Provider provider.Client

	// RepoDir is the repository root.
	RepoDir string

	// ChangesetDir is .changeset/ under RepoDir.
	ChangesetDir string

	// Changesets are the pending changesets loaded from
	// ChangesetDir. May be empty.
	Changesets []*changeset.Changeset

	// Tags is the existing tag list from Repo.ListTags("").
	Tags []string

	// PreState is the pre-release state. nil for stable mode.
	PreState *changeset.PreState

	// HeadBranch overrides the source branch for the release PR.
	// Empty defaults to [DefaultHeadBranch].
	HeadBranch string

	// BaseBranch overrides the merge target. Empty queries the
	// provider for the default branch.
	BaseBranch string

	// Today overrides the date used in CHANGELOG entries
	// (YYYY-MM-DD). Empty defaults to today's UTC date.
	Today string

	// Remote is the git remote name for fetch / push (typically
	// "origin"). Empty defaults to "origin".
	Remote string
}

// AutoResult reports what [Auto] did.
type AutoResult struct {
	// Branch is which path Auto took.
	Branch AutoBranch

	// CommitSHA is HEAD's SHA at the moment Auto started.
	CommitSHA string

	// DetectionSource is the signal that decided Branch.
	// Populated for AutoBranchRelease; detect.SourceNone otherwise.
	DetectionSource detect.Source

	// Tags lists the tags created (release branch only).
	Tags []string

	// Releases lists the provider-side releases created (release
	// branch only).
	Releases []*provider.Release

	// Action is the orchestrator's PR-side decision (feature branch
	// only). One of ActionNoop / ActionCreated / ActionUpdated /
	// ActionClosed.
	Action Action

	// PR is the upserted (or closed) PR (feature branch, when
	// applicable).
	PR *provider.PullRequest
}

// ErrConfigRequired is returned by [Auto] when AutoOptions.Config is nil.
var ErrConfigRequired = errors.New("auto: Config is required")

// ErrRepoRequired is returned by [Auto] when AutoOptions.Repo is nil.
var ErrRepoRequired = errors.New("auto: Repo is required")

// ErrProviderRequired is returned by [Auto] when AutoOptions.Provider is nil.
var ErrProviderRequired = errors.New("auto: Provider is required")

// Auto detects whether HEAD is a release-PR merge, then dispatches:
//
//   - Release: monorel tag, push tags, monorel publish.
//   - Feature, plan empty: orchestrator.Run with empty plan
//     (closes any stale release PR; otherwise no-op).
//   - Feature, plan non-empty: fetch base, checkout monorel/release
//     from origin/<base>, monorel apply, force-push monorel/release,
//     orchestrator.Run with the plan.
//
// Auto always exits with a populated AutoResult on success.
func Auto(ctx context.Context, opts AutoOptions) (*AutoResult, error) {
	if opts.Config == nil {
		return nil, ErrConfigRequired
	}
	if opts.Repo == nil {
		return nil, ErrRepoRequired
	}
	if opts.Provider == nil {
		return nil, ErrProviderRequired
	}

	remote := opts.Remote
	if remote == "" {
		remote = "origin"
	}

	headSHA, err := opts.Repo.CurrentSHA()
	if err != nil {
		return nil, fmt.Errorf("auto: read HEAD SHA: %w", err)
	}

	det, err := detect.IsReleaseMerge(ctx, opts.Repo, opts.Provider, headSHA)
	if err != nil {
		return nil, fmt.Errorf("auto: detect release merge: %w", err)
	}

	if det.IsRelease {
		return autoRelease(ctx, opts, headSHA, det.Source, remote)
	}
	return autoFeature(ctx, opts, headSHA, remote)
}

func autoRelease(ctx context.Context, opts AutoOptions, headSHA string, src detect.Source, remote string) (*AutoResult, error) {
	tagRes, err := release.Tag(release.TagOptions{
		Config:   opts.Config,
		Repo:     opts.Repo,
		Provider: opts.Provider,
	})
	if err != nil {
		return nil, fmt.Errorf("auto: tag: %w", err)
	}

	if err := opts.Repo.PushTags(remote); err != nil {
		return nil, fmt.Errorf("auto: push tags: %w", err)
	}

	// release.Tag's Result has empty Body fields (Tag works from
	// commit trailers, not CHANGELOG content). Use DiscoverPublishables
	// to read each tagged package's CHANGELOG entry; this matches the
	// `monorel publish` CLI's body-population path. Without it, the
	// provider Releases would be created with empty bodies.
	infos, err := release.DiscoverPublishables(opts.Config, opts.Repo, opts.RepoDir)
	if err != nil {
		return nil, fmt.Errorf("auto: discover publishables: %w", err)
	}
	publishRes := &release.Result{CommitSHA: tagRes.CommitSHA, Releases: infos}

	releases, err := release.PublishReleases(ctx, opts.Provider, publishRes)
	if err != nil {
		return nil, fmt.Errorf("auto: publish: %w", err)
	}

	return &AutoResult{
		Branch:          AutoBranchRelease,
		CommitSHA:       headSHA,
		DetectionSource: src,
		Tags:            tagRes.Tags(),
		Releases:        releases,
	}, nil
}

func autoFeature(ctx context.Context, opts AutoOptions, headSHA, remote string) (*AutoResult, error) {
	p, err := plan.Plan(opts.Config, opts.Changesets, opts.Tags, opts.PreState)
	if err != nil {
		return nil, fmt.Errorf("auto: plan: %w", err)
	}

	headBranch := opts.HeadBranch
	if headBranch == "" {
		headBranch = DefaultHeadBranch
	}

	// Default Today once, before either consumer (release.Apply or
	// orchestrator.Run) sees it. Both fall back to changelog.Today()
	// independently when given "", and a UTC-midnight crossover
	// between the two calls would otherwise produce different dates
	// in the written CHANGELOG vs the rendered PR body.
	today := opts.Today
	if today == "" {
		today = changelog.Today()
	}

	// Non-empty plan: stage on monorel/release, push, then upsert.
	if !p.IsEmpty() {
		baseBranch := opts.BaseBranch
		if baseBranch == "" {
			baseBranch, err = opts.Provider.GetDefaultBranch(ctx)
			if err != nil {
				return nil, fmt.Errorf("auto: get default branch: %w", err)
			}
		}

		if err := opts.Repo.Fetch(remote, baseBranch); err != nil {
			return nil, fmt.Errorf("auto: fetch %s/%s: %w", remote, baseBranch, err)
		}
		if err := opts.Repo.CheckoutNewBranch(headBranch, remote+"/"+baseBranch); err != nil {
			return nil, fmt.Errorf("auto: checkout %s from %s/%s: %w", headBranch, remote, baseBranch, err)
		}

		if _, err := release.Apply(release.Options{
			Plan:         p,
			Config:       opts.Config,
			Repo:         opts.Repo,
			RepoDir:      opts.RepoDir,
			ChangesetDir: opts.ChangesetDir,
			PreState:     opts.PreState,
			Today:        today,
		}); err != nil {
			return nil, fmt.Errorf("auto: apply: %w", err)
		}

		if err := opts.Repo.Push(remote, headBranch, true); err != nil {
			return nil, fmt.Errorf("auto: push %s: %w", headBranch, err)
		}
	}

	res, err := Run(ctx, Options{
		Plan:       p,
		Provider:   opts.Provider,
		HeadBranch: headBranch,
		BaseBranch: opts.BaseBranch,
		Today:      today,
	})
	if err != nil {
		return nil, fmt.Errorf("auto: orchestrator: %w", err)
	}

	return &AutoResult{
		Branch:    AutoBranchFeature,
		CommitSHA: headSHA,
		Action:    res.Action,
		PR:        res.PR,
	}, nil
}
