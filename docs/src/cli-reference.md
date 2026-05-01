---
title: CLI Reference
description: "Per-command reference for monorel: add, plan, status, release, preview, pre, init."
---

# CLI Reference

monorel ships as a single binary with the following subcommands. All commands take a `--config <path>` flag (default `monorel.toml`).

## `monorel add`

Create a new changeset describing pending package changes.

```sh
# Non-interactive
monorel add \
  --package "transports/zerolog:minor" \
  --package "github.com/acme/widget:patch" \
  --message "Adds Lazy() helper; pass-through fix in the root."

# Interactive (prompts for packages, levels, body)
monorel add
```

| Flag | Type | Description |
|------|------|-------------|
| `-p, --package <name>:<level>` | string (repeatable) | Package and bump level. Triggers non-interactive mode when given. |
| `-m, --message <text>` | string | Changeset body (the changelog entry). May be empty. |
| `--name <name>` | string | Override the auto-generated changeset filename (default: random `<adj>-<noun>`). Lowercase letters, digits, hyphens only. |

Validates: package exists in `monorel.toml`, bump level is `major|minor|patch`, no duplicate package across `--package` flags.

Writes to `.changeset/<name>.md`.

## `monorel plan`

Compute the proposed releases without applying them. Reads `.changeset/*.md`, the configured packages, and the latest matching git tag per package.

```sh
monorel plan
# PACKAGE             FROM     BUMP   TO       TAG                          CHANGESETS
# transports/zerolog  v1.6.1   minor  v1.7.0   transports/zerolog/v1.7.0    quick-otter
#
# 1 package(s) to release; 1 changeset(s) consumed.

monorel plan --json
# {"releases": [...], "consumed": [...]}
```

| Flag | Type | Description |
|------|------|-------------|
| `--json` | bool | Emit the plan as JSON. Stable schema across versions. |

## `monorel status`

Print pending changesets and which packages they affect. Doesn't compute bumps.

```sh
monorel status
# CHANGESET    PACKAGE             BUMP    SUMMARY
# quick-otter  transports/zerolog  minor   Adds Lazy() helper.
#
# 1 changeset(s) pending.
```

## `monorel release`

Apply pending changesets: bump versions, write changelogs, tag, commit. Idempotency: aborts if a planned tag already exists.

```sh
monorel release
# Released 1 package(s) at a4f77ab:
#   transports/zerolog/v1.7.0
# Run `git push --follow-tags` to publish.
```

| Flag | Type | Description |
|------|------|-------------|
| `--publish` | bool | After tagging, create one forge release per tag using the configured forge provider. Requires the provider's auth token in the environment and that tags have been pushed. |

In stable mode `release` writes per-package CHANGELOG entries, deletes the consumed `.changeset/*.md` files, makes a single commit, and creates per-package annotated tags at HEAD.

::: warning Push is the caller's job
The applier creates the commit and tags locally. Run `git push --follow-tags` (or use the GitHub Action) to publish.
:::

::: warning --publish requires tags already pushed
GitHub validates that the tag exists when creating a Release. Push tags first, then run `release --publish`, or run `release` locally and let the GitHub Action's release workflow handle the publish step.
:::

### Pre-release mode

When `.changeset/pre.json` exists (i.e. after `monorel pre enter`), `release` behaves differently:

- CHANGELOG entries are NOT written.
- `.changeset/*.md` files are NOT deleted.
- Per-package counters in `pre.json` are incremented.
- Tags carry the suffix (e.g. `transports/zerolog/v1.7.0-rc.0`).

Accumulated changes are emitted to CHANGELOGs at the next stable release after `pre exit`.

## `monorel preview`

(Planned for Phase 9.) Renders the release plan as markdown for an always-open release PR body. Used by the GitHub Action wrapper.

## `monorel pre`

Manage pre-release mode (rc / beta / alpha). Pre-release mode causes `release` to append a `-<channel>.N` suffix to next versions and increment N per release.

### `monorel pre enter <channel>`

Start pre-release mode with the given channel name (`rc`, `beta`, `alpha`, or any non-empty SemVer-2.0-compatible identifier).

```sh
monorel pre enter rc
# entered pre-release mode (channel "rc"). Subsequent releases will be tagged vX.Y.Z-rc.N.
```

Errors if pre-release mode is already active. To switch channels, run `pre exit` first.

### `monorel pre exit`

Return to stable releases. Removes `.changeset/pre.json`. Idempotent.

```sh
monorel pre exit
# exited pre-release mode (was channel "rc"). Next release is a stable version.
```

The next stable release does NOT re-bump from the pre version; it bumps from the last stable tag, applying every changeset accumulated since.

### `monorel pre status`

Print the current pre-release state, if any.

```sh
monorel pre status
# pre-release mode: channel="rc"
#   transports/zerolog: counter=2
```

## `monorel init`

(Planned.) Scaffold a `monorel.toml` and `.changeset/` for a new repo.

## Persistent flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--config <path>` | string | `monorel.toml` | Path to the config file. The repo root is its parent directory. |

## Exit codes

- `0`: success.
- non-zero: an error occurred. The CLI prints the error to stderr; commands that do partial work (e.g. `release --publish` failing on the second forge release) print a "created N/M" line before the error.
