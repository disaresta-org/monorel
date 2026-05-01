---
title: "loglayer-go: a 26-package migration"
description: "Worked example of migrating loglayer-go from release-please to monorel. 25 sub-modules + 1 root, mixed tag prefixes, hard-cut CHANGELOG. Real outcome with PR links."
---

# loglayer-go: a 26-package migration

`loglayer-go` is the repo whose release-please saga prompted monorel's existence. It has 26 packages: one root module at the repo root with bare `vX.Y.Z` tags, plus 25 sub-modules under `transports/`, `plugins/`, and `integrations/` with `<path>/vX.Y.Z` tags. The migration completed end-to-end on 2026-05-01.

The outcome surfaced two CI gaps in monorel itself, which were fixed in v0.1.1 and v0.1.2 mid-migration. This page walks through the actual sequence rather than the idealized version.

## Starting state

- Tag history: bare `vX.Y.Z` for the root module (latest `v1.6.2`); `<path>/vX.Y.Z` for sub-modules (most at `v1.6.1`, with outliers at `v1.0.0` for `transports/gcplogging` and `plugins/plugintest`, `v1.1.1` for `integrations/sloghandler`, and `v1.2.0` for `integrations/loghttp`). No sub-module is at `v1.6.2`; that's the root's version.
- `.release-please-config.json` with 26 packages.
- `.release-please-manifest.json` with current per-package versions.
- 25 `CHANGELOG.md` files in the tree (root + 24 sub-modules). 13 of them have hand-written preambles referencing release-please by name; the other 12 are bare release-please-generated files. The 26th package, `plugins/plugintest`, has never been released and has no `CHANGELOG.md` to migrate.
- Release workflows under `.github/workflows/release-please*.yml` (main + cleanup), plus a `release-please-state` pre-push lefthook hook backed by `scripts/check-release-please-state.sh`.
- AGENTS.md sections covering the release-please workflow, the "release-please gotchas" we accumulated, and the multi-step "adding a new transport" recipe with release-please-specific steps.

## Pre-merge audit

Before opening the migration PR, every package's latest tag was scripted-checked against the manifest. All 26 matched exactly:

```
PACKAGE                   MANIFEST  LATEST_TAG  STATUS
.                         1.6.2     1.6.2       MATCH
integrations/loghttp      1.2.0     1.2.0       MATCH
integrations/sloghandler  1.1.1     1.1.1       MATCH
plugins/datadogtrace      1.6.1     1.6.1       MATCH
plugins/fmtlog            1.6.1     1.6.1       MATCH
plugins/oteltrace         1.6.1     1.6.1       MATCH
plugins/plugintest        1.0.0     1.0.0       MATCH
plugins/redact            1.6.1     1.6.1       MATCH
plugins/sampling          1.6.1     1.6.1       MATCH
transports/blank          1.6.1     1.6.1       MATCH
transports/charmlog       1.6.1     1.6.1       MATCH
…  (all 26 matched)
```

A mismatch would have meant either the manifest was stale (safe to proceed; monorel's planner would pick the correct tag) or the latest tag was wrong (would have required deletion before merge). Neither case applied here.

## The migration PR

[loglayer/loglayer-go#44](https://github.com/loglayer/loglayer-go/pull/44) — single commit: 25 modified, 5 added, 5 deleted.

Generated `monorel.toml` mirrors the previous manifest 1:1 in the same order so the diff reviews as "delete config, add toml":

```toml
[provider]
name = "github"
owner = "loglayer"
repo = "loglayer-go"

[packages."go.loglayer.dev"]
tag_prefix = ""
path       = "."
changelog  = "CHANGELOG.md"

[packages."transports/zerolog"]
tag_prefix = "transports/zerolog"
path       = "transports/zerolog"
changelog  = "transports/zerolog/CHANGELOG.md"

# … 24 more ...
```

Workflow swap: `release-please.yml` + `release-please-cleanup.yml` deleted; `release-pr.yml` + `release.yml` added, both wrapping `disaresta-org/monorel/ci/github`.

Prose rewrite: AGENTS.md release sections, README.md, CONTRIBUTING.md, package.json, `.claude/rules/documentation.md`, `scripts/lint-commit.mjs` all dropped "the same parser release-please uses" framing in favour of describing the conventional-commit linter on its own terms (releases are driven by changesets, not commit messages, so the lint stands on its own as a hygiene tool).

13 per-package `CHANGELOG.md` preambles (the ones that referenced release-please by name) were rewritten to point at monorel; the 12 release-please-generated bare `CHANGELOG.md` files were left alone. `plugins/plugintest` had no `CHANGELOG.md` to touch (no prior releases).

The PR shipped no `.changeset/*.md` files — the migration is tooling-only, not a release. The first real release happens in a follow-up PR.

## Mid-migration: two monorel fixes

The follow-up smoke-test PR ([loglayer-go#45](https://github.com/loglayer/loglayer-go/pull/45)) added a single `:patch` changeset on `transports/blank` (the lowest-blast-radius package — a no-op template transport). Merging it should have opened the always-open release PR. It didn't:

```
orchestrator: create PR: 422 Validation Failed [Field:head Code:invalid]
```

GitHub's PR-create API rejects PRs whose head branch doesn't exist on the remote. Monorel v0.1.0's CI wrapper documented "branch staging is a follow-up" but the always-open pattern fundamentally requires the branch to exist before the orchestrator can call CreatePR. loglayer-go was the first repo to surface this.

### v0.1.1: branch staging

[disaresta-org/monorel#1](https://github.com/disaresta-org/monorel/pull/1) — `ci/github/action.yml` and `.github/workflows/release-pr.yml` (monorel's own self-hosted version) now create `monorel/release` from `origin/<default-branch>` plus one empty marker commit before invoking `monorel preview --upsert`. The branch's diff stays empty by design — the rendered plan goes in the PR body.

The fix self-validated: merging PR #1 ran `release-pr` on monorel's main, which staged the branch successfully and opened monorel's first-ever monorel-driven release PR for v0.1.1.

### v0.1.2: workflow_call asset chain

v0.1.1's tag and GitHub Release were created, but the release had no binary assets attached and no GHCR image was pushed. Same anti-recursion gap that `docs.yml` already worked around: GitHub's `GITHUB_TOKEN` doesn't propagate `push: tags` events to other workflows, so `build-release-binaries.yml` and `build-image.yml` silently didn't fire. v0.1.0 was unaffected only because it was cut via `workflow_dispatch` (a user-initiated run that doesn't trip anti-recursion).

[disaresta-org/monorel#3](https://github.com/disaresta-org/monorel/pull/3) chained both build workflows from `release.yml` via `workflow_call`, mirroring how `docs.yml` is chained. Both build workflows now accept a `tag` input via `workflow_call` and `workflow_dispatch`; the natural `push: tags` trigger is preserved for direct-`git push` flows.

[disaresta-org/monorel#5](https://github.com/disaresta-org/monorel/pull/5) bundled in: `docs.yml` switched from "deploy on every push to main" to "deploy on release / dispatch / workflow_call," and `release.yml` now also chains `docs.yml` so monorel-driven releases trigger the docs deploy directly.

Both fixes shipped in v0.1.2. v0.1.1 is left as-is — released but assetless. Consumers should pin to v0.1.2 directly.

## Wrapping up the migration

[loglayer-go#46](https://github.com/loglayer/loglayer-go/pull/46) bumped both action wrapper pins from `@v0.1.0` to `@v0.1.2`. After merge, the `release-pr` workflow ran cleanly on the smoke-test merge commit, staged `monorel/release`, picked up the changeset, and opened the always-open release PR.

[loglayer-go#47](https://github.com/loglayer/loglayer-go/pull/47) — `chore(release): transports/blank v1.6.2` — was loglayer-go's first monorel-driven release PR. Merging it ran `release.yml`'s pipeline:

- `monorel release`: wrote `transports/blank/CHANGELOG.md` entry, deleted the changeset, created the release commit and tag locally.
- `git push --follow-tags`: pushed both to origin.
- `monorel publish`: created the GitHub Release with the rendered changelog body.
- `deploy-docs / Build` + `deploy-docs / Deploy to GitHub Pages`: docs site rebuilt and deployed via `workflow_call`.

All four jobs green; tag `transports/blank/v1.6.2` lands on origin. The `workflow_call` docs-deploy chain — flagged as unverified in the migration spec — works as designed without requiring a Personal Access Token.

## What stayed unchanged

- Every existing tag.
- Every existing GitHub Release.
- All 25 existing `CHANGELOG.md` historical entries (verbatim below the rewritten preambles).
- The hand-written `[Unreleased]` section in the root CHANGELOG.

monorel inserts new entries above the existing content; it never rewrites old entries.

## Take-aways

- **The CI-wrapper layer is where the always-open PR pattern needs branch staging.** The orchestrator's `Run` doc explicitly delegates branch management to the wrapper because git is shell-out-friendly there. Putting it in Go was tempting but unnecessary; v0.1.1 fixed it where it belongs.
- **Anti-recursion-safe chaining matters for every downstream workflow.** Docs deploy was the known case; binary builds and image pushes weren't. Once one downstream chain is `workflow_call`-shaped, the others probably should be too.
- **Self-hosting catches gaps fast.** monorel's first PR (PR #1) and first changeset-driven release (v0.1.1) both surfaced bugs no one would have hit unless they actually used the always-open PR pattern. Bootstrap-via-`workflow_dispatch` doesn't exercise either path.
- **No tool removes the changeset discipline.** Reviewers still have to enforce "every release-affecting PR has a changeset," the way they previously enforced "every release-affecting PR has a Conventional Commits subject the parser could read."
- **The CHANGELOG format split is real but localized.** Pre-migration entries are release-please-formatted (`## [1.6.1] (compare) (date)`); post-migration entries are Keep-a-Changelog (`## [1.7.0] - 2026-05-01` / `### Minor Changes`). Both render fine on GitHub. After a year, only the oldest entries look different.
