// Package orchestrator drives the always-open release-PR pattern.
//
// It is the bot logic equivalent of release-please's "release PR
// updater" or changesets-bot. The CI wrapper (under ci/<provider>/)
// invokes Run on every push to the default branch; non-empty plans
// upsert the release PR, empty plans close it.
//
// Provider-neutral by virtue of using provider.Client. The branch
// management (creating / updating / pushing the release branch) is
// delegated to the CI wrapper because it's a thin shell-out to git;
// orchestrator only owns the plan-dependent decisions and the PR
// upsert call.
package orchestrator

import (
	"context"
	"errors"
	"fmt"

	"monorel.disaresta.com/internal/provider"
	"monorel.disaresta.com/internal/release"
	"monorel.disaresta.com/plan"
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

	// Provider is the API client for the configured provider.
	Provider provider.Client

	// HeadBranch is the source branch of the release PR (i.e. the
	// branch the orchestrator pushed CHANGELOG-edit commits to).
	// Empty defaults to [DefaultHeadBranch].
	HeadBranch string

	// BaseBranch is the merge target. Empty triggers a lookup via
	// provider.Client.GetDefaultBranch.
	BaseBranch string

	// Today is the YYYY-MM-DD date used in rendered CHANGELOG
	// entries embedded in the PR body. Empty defaults to today's
	// UTC date (via the changelog package).
	Today string
}

// Action is the orchestrator's decision for a single Run invocation.
// Stringly-typed at the wire level (JSON, logs) but constants allow
// callers to switch exhaustively.
type Action string

const (
	ActionNoop    Action = "noop"
	ActionClosed  Action = "closed"
	ActionCreated Action = "created"
	ActionUpdated Action = "updated"
)

// Result reports what [Run] did.
type Result struct {
	// Action describes the orchestrator's decision.
	Action Action

	// PR is the upserted (or closed) PR, when applicable. nil for
	// [ActionNoop].
	PR *provider.PullRequest
}

// Run drives one orchestration tick:
//
//   - Empty plan + open release PR: close the PR. Action = ActionClosed.
//   - Empty plan + no open release PR: do nothing. Action = ActionNoop.
//   - Non-empty plan + no open release PR: create one. Action = ActionCreated.
//   - Non-empty plan + existing open release PR: update title/body.
//     Action = ActionUpdated.
//
// Run does NOT push commits to the head branch. In a "fully
// orchestrated" world the CI wrapper would force-push speculative-
// version commits (CHANGELOG edits, deleted changesets) to the head
// branch before invoking Run, so the upserted PR shows the merged-
// release diff. The v1 wrapper does NOT yet do this staging — the
// PR body shows the rendered plan but the branch's diff is empty.
// See ci/github/README.md and the "Notes" section of
// docs/src/github-action.md.
func Run(ctx context.Context, opts Options) (*Result, error) {
	if opts.Plan == nil {
		return nil, errors.New("orchestrator: nil Plan")
	}
	if opts.Provider == nil {
		return nil, errors.New("orchestrator: nil Provider")
	}

	headBranch := opts.HeadBranch
	if headBranch == "" {
		headBranch = DefaultHeadBranch
	}

	existing, err := opts.Provider.FindOpenReleasePR(ctx, headBranch)
	if err != nil {
		return nil, fmt.Errorf("orchestrator: find existing PR: %w", err)
	}

	if opts.Plan.IsEmpty() {
		if existing == nil {
			return &Result{Action: ActionNoop}, nil
		}
		if err := opts.Provider.ClosePR(ctx, existing.Number); err != nil {
			return nil, fmt.Errorf("orchestrator: close PR #%d: %w", existing.Number, err)
		}
		return &Result{Action: ActionClosed, PR: existing}, nil
	}

	baseBranch := opts.BaseBranch
	if baseBranch == "" {
		baseBranch, err = opts.Provider.GetDefaultBranch(ctx)
		if err != nil {
			return nil, fmt.Errorf("orchestrator: get default branch: %w", err)
		}
	}

	title := titleFor(opts.Plan)
	body := renderBodyWithinLimit(opts.Plan, opts.Today)

	if existing == nil {
		pr, err := opts.Provider.CreatePR(ctx, provider.CreatePROptions{
			Title:      title,
			Body:       body,
			HeadBranch: headBranch,
			BaseBranch: baseBranch,
		})
		if err != nil {
			return nil, fmt.Errorf("orchestrator: create PR: %w", err)
		}
		return &Result{Action: ActionCreated, PR: pr}, nil
	}

	pr, err := opts.Provider.UpdatePR(ctx, existing.Number, provider.UpdatePROptions{
		Title: &title,
		Body:  &body,
	})
	if err != nil {
		return nil, fmt.Errorf("orchestrator: update PR #%d: %w", existing.Number, err)
	}
	return &Result{Action: ActionUpdated, PR: pr}, nil
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

// MaxPRBodyBytes is the conservative upper bound used for PR / MR
// body content across providers. GitHub caps PR descriptions at
// 65,536 bytes and Gitea matches; GitLab's MR description allows
// ~1 MiB but we use the same conservative bound so a release plan
// that fits one provider fits all.
const MaxPRBodyBytes = 65536

// renderBodyWithinLimit returns the rendered release-PR body, falling
// back through three steps when the rendering would exceed
// [MaxPRBodyBytes]:
//
//  1. Full [release.RenderPreview] (header + per-package sections).
//  2. [release.RenderPreviewCompact] (header + version table only;
//     per-package release notes elided with a pointer to each
//     package's CHANGELOG).
//  3. Hard byte-truncation with a trailing marker, for the rare
//     case where even the compact form is too large (e.g. hundreds
//     of packages where the table itself exceeds the limit).
//
// Step 1 succeeds for the vast majority of release plans; step 2
// kicks in only when a changeset body × package-count blows past
// the limit; step 3 is a last-resort safety net.
func renderBodyWithinLimit(p *plan.ReleasePlan, today string) string {
	body := release.RenderPreview(p, today)
	if len(body) <= MaxPRBodyBytes {
		return body
	}
	body = release.RenderPreviewCompact(p, today)
	if len(body) <= MaxPRBodyBytes {
		return body
	}
	// Compact still doesn't fit (huge release with hundreds of
	// packages). Hard-truncate at a byte boundary, leaving room
	// for a trailing marker so the body remains valid markdown
	// and the reader knows content was dropped.
	const marker = "\n\n_…(release-PR body truncated by monorel; see each package's CHANGELOG.md for the full content)._\n"
	cut := MaxPRBodyBytes - len(marker)
	if cut < 0 {
		cut = 0
	}
	if cut > len(body) {
		cut = len(body)
	}
	return body[:cut] + marker
}
