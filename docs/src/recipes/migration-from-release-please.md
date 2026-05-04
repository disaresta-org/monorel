---
title: Migrating from release-please
description: "Step-by-step migration from release-please to monorel: hard cut to Keep-a-Changelog, preserved history, no manual tag rewrites."
---

# Migrating from release-please

monorel can take over from release-please in a single PR. The migration is hard-cut: existing CHANGELOG entries stay verbatim, new entries are Keep-a-Changelog format. No tag rewrites; the tag history release-please produced becomes the input to monorel's planner unchanged.

## Before you start

Audit your current release-please setup:

- `.release-please-config.json`: every entry under `packages` becomes a `[packages."<name>"]` block in `monorel.toml`.
- `.release-please-manifest.json`: the current per-package versions. monorel reads these from git tags instead of a manifest, but the file is the canonical "current version" source for cross-checking the migration.
- `CHANGELOG.md`: stays. monorel inserts new entries above the first `## ` heading, preserving everything below.

## Migration steps

### 1. Write `monorel.toml`

For each entry under `release-please`'s `packages`, create a `[packages."<name>"]` block with the same `tag_prefix` and `path`. Pick a `changelog` path:

```toml
[provider]
name = "github"
owner = "acme"
repo = "widget"

[packages."github.com/acme/widget"]
tag_prefix = ""
path       = "."
changelog  = "CHANGELOG.md"

[packages."transports/zerolog"]
tag_prefix = "transports/zerolog"
path       = "transports/zerolog"
changelog  = "transports/zerolog/CHANGELOG.md"

# ... one block per package ...
```

::: tip release-please's "include-component-in-tag: false"
Maps to monorel's `tag_prefix = ""`. Both produce bare `vX.Y.Z`.
:::

### 2. Verify the planner sees the right "current versions"

```sh
monorel plan
# Should report "No pending changesets. Nothing to release." (you haven't added any yet).

git tag --list "transports/zerolog/v*" | sort -V | tail -1
# Should show the latest stable tag for each package.
```

If the latest tag the planner picks doesn't match `release-please-manifest.json`, investigate before proceeding. The planner ignores non-semver tags and pre-release tags; that's usually the cause.

### 3. Remove release-please workflows

Delete:

- `.github/workflows/release-please.yml` (or whatever you named it).
- `.release-please-config.json`.
- `.release-please-manifest.json`.

The workflow will be replaced by a single `release.yml` that runs `monorel auto` (see [GitHub Action](/integrations/github)).

### 4. Add the monorel workflow

Create `.github/workflows/release.yml` per the [GitHub Action](/integrations/github) page. The single workflow runs `monorel auto` on every push to the default branch and dispatches between feature-path (refresh the release PR) and release-path (cut tags + publish) automatically.

### 5. Initialize `.changeset/`

```sh
mkdir -p .changeset
```

### 6. Open the migration PR

Stage all of the above:

```sh
git add monorel.toml .github/workflows/release.yml .changeset/
git rm .release-please-config.json .release-please-manifest.json
git rm .github/workflows/release-please.yml
git commit -m "chore: migrate from release-please to monorel"
```

Open the PR. Note that this PR doesn't include a changeset file; monorel won't try to release anything from it. After merge, your next PR with a `.changeset/*.md` file will trigger the always-open release PR.

## What survives, what changes

| Concern | Before (release-please) | After (monorel) |
|---------|------------------------|-----------------|
| Existing tag history | Preserved | Preserved |
| Existing CHANGELOG | Preserved | Preserved (new entries inserted above) |
| Existing release artifacts (GitHub Releases) | Preserved | Preserved |
| Bump-level signal | Conventional commits + `Release-As:` | `.changeset/*.md` files |
| Release PR | release-please opens one | monorel orchestrator opens one |
| Release commit subject | `chore(main): release ...` | `chore(release): ...` |
| New CHANGELOG format | release-please style | Keep-a-Changelog |

The CHANGELOG format change is forward-only. Old entries stay in release-please format; new entries use Keep-a-Changelog. Both render fine on GitHub.

## After migration

- Authors write changesets via `monorel add` (or hand-write the file).
- The release PR opens automatically; merging it cuts releases.
- Pre-release windows use `monorel pre enter <channel>` / `pre exit`.
