// Package orchestrator drives the always-open release-PR pattern.
//
// It is the bot logic equivalent of release-please's "release PR
// updater" or changesets-bot. The CI wrapper (under ci/<provider>/)
// invokes Run on every push to the default branch; non-empty plans
// upsert the release PR, empty plans close it.
//
// Provider-neutral by virtue of using forge.Client. The branch
// management (creating / updating / pushing the release branch) is
// delegated to the CI wrapper because it's a thin shell-out to git;
// orchestrator only owns the plan-dependent decisions and the PR
// upsert call.
package orchestrator

import (
	"context"
	"errors"
	"fmt"

	"github.com/disaresta-org/monorel/internal/forge"
	"github.com/disaresta-org/monorel/internal/plan"
	"github.com/disaresta-org/monorel/internal/release"
)

// DefaultHeadBranch is the branch name the orchestrator pushes
// release-preview commits to and opens the release PR from.
const DefaultHeadBranch = "monorel/release"

// Options bundles the inputs to [Run].
type Options struct {
	// Plan is the release plan computed from the current state of
	// the default branch. Must be non-nil; an empty plan signals
	// "close any open release PR".
	Plan *plan.ReleasePlan

	// Forge is the API client for the configured forge provider.
	Forge forge.Client

	// HeadBranch is the source branch of the release PR (i.e. the
	// branch the orchestrator pushed CHANGELOG-edit commits to).
	// Empty defaults to [DefaultHeadBranch].
	HeadBranch string

	// BaseBranch is the merge target. Empty triggers a lookup via
	// forge.Client.GetDefaultBranch.
	BaseBranch string

	// Today is the YYYY-MM-DD date used in rendered CHANGELOG
	// entries embedded in the PR body. Empty defaults to today's
	// UTC date (via the changelog package).
	Today string
}

// Result reports what [Run] did.
type Result struct {
	// Action describes the orchestrator's decision in one word:
	// "created", "updated", "closed", or "noop".
	Action string

	// PR is the upserted (or closed) PR, when applicable. nil for
	// "noop".
	PR *forge.PullRequest
}

// Run drives one orchestration tick:
//
//   - Empty plan + open release PR: close the PR. Action = "closed".
//   - Empty plan + no open release PR: do nothing. Action = "noop".
//   - Non-empty plan + no open release PR: create one. Action = "created".
//   - Non-empty plan + existing open release PR: update title/body.
//     Action = "updated".
//
// Run does NOT push commits to the head branch; the CI wrapper
// orchestrates that. By the time Run is called, the head branch
// should already be at the speculative-version state the PR will
// describe.
func Run(ctx context.Context, opts Options) (*Result, error) {
	if opts.Plan == nil {
		return nil, errors.New("orchestrator: nil Plan")
	}
	if opts.Forge == nil {
		return nil, errors.New("orchestrator: nil Forge")
	}

	headBranch := opts.HeadBranch
	if headBranch == "" {
		headBranch = DefaultHeadBranch
	}

	existing, err := opts.Forge.FindOpenReleasePR(ctx, headBranch)
	if err != nil {
		return nil, fmt.Errorf("orchestrator: find existing PR: %w", err)
	}

	if opts.Plan.IsEmpty() {
		if existing == nil {
			return &Result{Action: "noop"}, nil
		}
		if err := opts.Forge.ClosePR(ctx, existing.Number); err != nil {
			return nil, fmt.Errorf("orchestrator: close PR #%d: %w", existing.Number, err)
		}
		return &Result{Action: "closed", PR: existing}, nil
	}

	baseBranch := opts.BaseBranch
	if baseBranch == "" {
		baseBranch, err = opts.Forge.GetDefaultBranch(ctx)
		if err != nil {
			return nil, fmt.Errorf("orchestrator: get default branch: %w", err)
		}
	}

	title := titleFor(opts.Plan)
	body := release.RenderPreview(opts.Plan, opts.Today)

	if existing == nil {
		pr, err := opts.Forge.CreatePR(ctx, forge.CreatePROptions{
			Title:      title,
			Body:       body,
			HeadBranch: headBranch,
			BaseBranch: baseBranch,
		})
		if err != nil {
			return nil, fmt.Errorf("orchestrator: create PR: %w", err)
		}
		return &Result{Action: "created", PR: pr}, nil
	}

	pr, err := opts.Forge.UpdatePR(ctx, existing.Number, forge.UpdatePROptions{
		Title: &title,
		Body:  &body,
	})
	if err != nil {
		return nil, fmt.Errorf("orchestrator: update PR #%d: %w", existing.Number, err)
	}
	return &Result{Action: "updated", PR: pr}, nil
}

// titleFor builds the PR title from the plan. Single-package plans
// name the package and version; multi-package plans summarize.
func titleFor(p *plan.ReleasePlan) string {
	if len(p.Releases) == 1 {
		r := p.Releases[0]
		return fmt.Sprintf("chore(release): %s %s", r.Name, r.To)
	}
	return fmt.Sprintf("chore(release): %d packages", len(p.Releases))
}
