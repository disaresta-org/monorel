---
title: "loglayer-go: a 25-module migration"
description: "Worked example of migrating loglayer-go from release-please to monorel. 25 sub-modules, mixed tag prefixes, hard-cut CHANGELOG."
---

# loglayer-go: a 25-module migration

`loglayer-go` is the repo whose release-please saga prompted monorel's existence. It has 25 modules: one main module at the repo root with bare `vX.Y.Z` tags, plus 24 sub-modules under `transports/`, `plugins/`, and `integrations/`.

This page walks through the migration as a worked example.

::: warning Pending
This recipe is a placeholder until the loglayer-go migration actually happens. The shape below is the intended outcome; commit-by-commit details land here once monorel itself reaches v1 and the migration is performed.
:::

## Starting state

- Tag history: bare `vX.Y.Z` for the main module, `<path>/vX.Y.Z` for sub-modules.
- `.release-please-config.json` with 25 packages.
- `.release-please-manifest.json` with current per-package versions.
- A root `CHANGELOG.md` and per-sub-module `CHANGELOG.md` files, all release-please-formatted.
- Release workflows under `.github/workflows/release-please*.yml`.

## Generated `monorel.toml`

A snippet (the full file has 25 package blocks):

```toml
[forge]
provider = "github"
owner    = "loglayer"
repo     = "loglayer-go"

[packages."go.loglayer.dev"]
tag_prefix = ""
path       = "."
changelog  = "CHANGELOG.md"

[packages."transports/zerolog"]
tag_prefix = "transports/zerolog"
path       = "transports/zerolog"
changelog  = "transports/zerolog/CHANGELOG.md"

[packages."transports/zap"]
tag_prefix = "transports/zap"
path       = "transports/zap"
changelog  = "transports/zap/CHANGELOG.md"

# ... 22 more ...
```

## Verification before merge

```sh
monorel plan
# No pending changesets. Nothing to release.

# Spot-check: planner picks v1.6.1 for transports/zerolog
git tag --list 'transports/zerolog/v*' | sort -V | tail -3
# transports/zerolog/v1.6.1
# transports/zerolog/v1.6.0
# transports/zerolog/v1.5.0
```

The planner's "From" should match `release-please-manifest.json[transports/zerolog]` for every package.

## After merge

The next time someone bumps a transport's API:

```sh
monorel add \
  --package "transports/zerolog:minor" \
  --message "Adds Lazy() helper for deferred field evaluation."
```

The release PR updates automatically on push to `main`. Merging it produces:

- Tag `transports/zerolog/v1.7.0`.
- Entry in `transports/zerolog/CHANGELOG.md` under `## [1.7.0] - YYYY-MM-DD` / `### Minor Changes`.
- A GitHub Release for `transports/zerolog/v1.7.0` with the body sourced from the changeset.

The other 24 packages don't release; they have no pending changesets.

## What stays unchanged

- Every existing tag.
- Every existing GitHub Release.
- The hand-written `[Unreleased]` section in the root CHANGELOG (if present).
- The release-please-style headers for prior versions.

monorel inserts new entries above the existing content; it never rewrites old entries.

## Trade-offs the migration confirmed

- **No tool can save you from forgetting the changeset.** Reviewers have to enforce "every release-affecting PR has a changeset" the way they previously enforced "every release-affecting PR has the right Conventional Commits subject."
- **The CHANGELOG format split is real.** Pre-migration entries look different from post-migration entries. After a year, this is barely visible (only old entries diverge). Day-of, it's noticeable.
- **The release PR's commit subject format changes** (release-please's `chore(main): release X` → monorel's `chore(release): X`). If you have automation keying on the old subject, update it.
