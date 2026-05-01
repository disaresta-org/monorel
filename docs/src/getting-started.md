---
title: Getting Started
description: "Install monorel, write monorel.toml, author your first changeset, cut your first release."
---

# Getting Started

monorel is a single static binary. There's no daemon and no per-repo install beyond a config file and (optionally) a workflow.

## Install

```sh
go install github.com/disaresta-org/monorel/cmd/monorel@latest
```

Or in CI via the GitHub Action wrapper (see [GitHub Action](/github-action)):

```yaml
- uses: disaresta-org/monorel-action@v1
  with:
    command: release
```

## Initialize the repo

Pick a `monorel.toml` shape that matches your repo. The minimal single-package config:

```toml
[forge]
owner = "acme"
repo  = "widget"

[packages."github.com/acme/widget"]
tag_prefix = ""
path       = "."
changelog  = "CHANGELOG.md"
```

For a monorepo with sub-modules, add one `[packages."<name>"]` block per package:

```toml
[forge]
owner = "acme"
repo  = "widget"

[packages."github.com/acme/widget"]
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
```

The package name is the key inside `[packages.<name>]`; conventionally the import path for the root module and the relative directory for sub-modules. See [Configuration](/configuration) for the full reference.

Create the `.changeset/` directory:

```sh
mkdir -p .changeset
```

## Author a changeset

Each PR that needs a release includes a `.changeset/<name>.md` file. Use `monorel add` to write one:

```sh
monorel add \
  --package "transports/zerolog:minor" \
  --message "Adds Lazy() helper for deferred field evaluation."
```

This writes a file like `.changeset/quick-otter.md`:

```markdown
---
"transports/zerolog": minor
---

Adds Lazy() helper for deferred field evaluation.
```

A single changeset can target multiple packages with different bump levels:

```sh
monorel add \
  --package "transports/zerolog:major" \
  --package "github.com/acme/widget:patch" \
  --message "Reshape the zerolog Config; pass-through fix in the root."
```

Or run `monorel add` with no arguments for an interactive flow.

## Preview the release

```sh
monorel plan
```

```text
PACKAGE             FROM     BUMP   TO       TAG                          CHANGESETS
transports/zerolog  v1.6.1   minor  v1.7.0   transports/zerolog/v1.7.0    quick-otter

1 package(s) to release; 1 changeset(s) consumed.
```

`monorel status` lists pending changesets one row per (changeset, package) pair without computing bumps.

For machine-readable output, `monorel plan --json` emits a stable schema (see [CLI Reference](/cli-reference#plan)).

## Cut the release

```sh
monorel release
```

This:

1. Writes the CHANGELOG entry for each affected package.
2. Deletes the consumed `.changeset/*.md` files.
3. Creates a single commit (`chore(release): ...`).
4. Creates one annotated git tag per package release.

::: warning monorel does not push
The applier creates the commit and tags locally. Push is the caller's responsibility:

```sh
git push --follow-tags
```

The [GitHub Action](/github-action) orchestrates the push step in CI.
:::

To also create one forge release per tag (with the rendered CHANGELOG entry as release notes), pass `--publish`:

```sh
git push --follow-tags
GITHUB_TOKEN=... monorel release --publish
```

For pre-release rcs, see [pre-release mode](/cli-reference#pre-release-mode).

## Verify

```sh
git tag --list
# transports/zerolog/v1.7.0

cat transports/zerolog/CHANGELOG.md
# ## [1.7.0] - 2026-04-30
#
# ### Minor Changes
#
# - Adds Lazy() helper for deferred field evaluation.
```

## Next steps

- Wire up the [GitHub Action](/github-action) for the always-open release PR pattern.
- Skim [Changesets](/changesets) for the file format and authoring conventions.
- Read [Configuration](/configuration) when you're ready to tune `monorel.toml`.
