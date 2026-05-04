---
title: Getting Started
description: "Install monorel, scaffold the repo with `monorel init`, wire up the GitHub Action, ship your first release."
---

# Getting Started

This page walks you from a fresh repo to a published release using monorel's canonical flow: per-PR `.changeset/*.md` files, an always-open release PR, and a GitHub Action that drives the lifecycle. There's also a local-only flow for repos that don't use CI; see [Working without CI](#working-without-ci) at the bottom.

The shortest version: every release-affecting PR includes a changeset; the bot maintains an always-open release PR; you merge the release PR when you want to ship.

## Install

```sh
go install monorel.disaresta.com/cmd/monorel@latest
```

`monorel.disaresta.com` is a vanity import path that resolves to [`github.com/disaresta-org/monorel`](https://github.com/disaresta-org/monorel) via Go's `go-import` meta tag. You can also `git clone` and `go install ./cmd/monorel` directly.

You'll use the local binary for `monorel init` and `monorel add`. CI uses a published binary via the action wrapper (no install step in your repo).

## Scaffold the repo

In a git repo with at least one `go.mod` and a configured `origin` remote:

```sh
monorel init
```

This:

- Walks every `go.mod` under the working directory and writes one `[packages]` block per detected Go module to `monorel.toml`.
- Reads `git config remote.origin.url` to fill in `provider.owner` and `provider.repo`.
- Creates `.changeset/README.md` so contributors land on the format documentation when they open the directory.

Output:

```
Wrote monorel.toml with 2 package(s):
  github.com/acme/widget (path: ., tag prefix: "")
  transports/foo (path: transports/foo, tag prefix: "transports/foo")
Created .changeset/ with a README.
Next steps:
  monorel validate     # confirm the config
  monorel add          # write your first changeset
```

`monorel validate` confirms the config is loadable and the package paths exist. See [Configuration](/configuration) when you want to hand-tune `monorel.toml` (per-package `tag_prefix` overrides, self-hosted GitHub Enterprise host, etc.).

::: info Looking for a working example?
The [`examples/`](https://github.com/disaresta-org/monorel/tree/main/examples) directory in the monorel repo has minimal reference setups for each provider:

- [`examples/github/`](https://github.com/disaresta-org/monorel/tree/main/examples/github): composite action wrapper, single workflow file.
- [`examples/gitea/`](https://github.com/disaresta-org/monorel/tree/main/examples/gitea): same wrapper on Gitea Actions, `provider.host` set, Forgejo-compatible.
- [`examples/gitlab/`](https://github.com/disaresta-org/monorel/tree/main/examples/gitlab): single `.gitlab-ci.yml` using the published Docker image.

Each is a `monorel.toml` + workflow files + `.changeset/README.md` you can copy into your repo.

For a real production setup at scale, [loglayer-go](https://github.com/loglayer/loglayer-go) runs monorel across 26 sub-modules.
:::

## Wire up your provider

A single workflow file drives the entire release lifecycle. It runs `monorel auto` on every push to the default branch; auto detects whether HEAD is the merge of monorel's release PR and dispatches between the feature path (`apply` + `git push -f` + `preview --upsert`) and the release path (`tag` + `git push --follow-tags` + `publish`).

Per-provider walkthroughs cover the workflow YAML, token shape, branch-protection setup, and any provider-specific gotchas:

- [GitHub](/integrations/github)
- [Gitea / Forgejo](/integrations/gitea)
- [GitLab](/integrations/gitlab)

## How releases work

Three steps across the lifecycle:

1. **Author a feature PR.** Include a `.changeset/<name>.md` file naming the affected packages and the bump level for each. Code change and changeset land in the same PR; merge as normal.
2. **`monorel auto` (feature path) updates the always-open release PR.** On each push to `main`, the workflow detects that HEAD is not yet a release-PR merge, runs a speculative apply on a `monorel/release` branch (writes the staged CHANGELOG entries, deletes the consumed changeset files, makes one `chore(release): ...` commit), force-pushes that branch, and upserts the release PR. The PR's diff IS the file changes the next release will produce.
3. **Merge the release PR when ready to ship.** On the next push to `main` (the merge commit), `monorel auto` detects the release-PR merge, reads the body trailers, creates per-package tags, pushes them, and creates one GitHub Release per tag.

A PR without a changeset doesn't trigger a release. The release PR auto-updates as more changesets accumulate; closing it without merging cancels that release window.

For a visual end-to-end view (including the pre-release-cycle variant), see [Workflows](/workflows).

## Author your first changeset

On a feature branch, run `monorel add` and answer the prompts. It writes a `.changeset/<random-name>.md` declaring the affected packages and bump levels. Commit the file alongside your code change, open the PR, and merge it as you would any other PR.

Full reference for the file format, multi-package shape, and other authoring modes (editor-driven body, scripted): [Changesets](/changesets).

## Watch the release PR

After your PR merges, the workflow runs `monorel auto` against the new `main`. Auto detects this is a feature commit (not a release-PR merge) and:

1. Stages a `monorel/release` branch off `main`.
2. Runs `monorel apply` on it: writes the speculative `CHANGELOG.md` entries, deletes the consumed `.changeset/*.md` files, makes one `chore(release): <pkg> <ver>` commit.
3. Force-pushes that branch to the remote.
4. Opens (or updates) the always-open release PR with the rendered plan in the body.

The release PR's diff IS the actual file changes the release will produce, so reviewers see real CHANGELOG content rather than just a body summary.

If you merge another feature PR with another changeset, the workflow reruns and the release PR's diff updates.

## Cut the release

Merge the release PR. On the resulting push to `main`, the workflow runs `monorel auto` again. This time auto detects HEAD is the release-PR merge and:

1. Reads the commit's `monorel-Release:` body trailers.
2. Creates per-package annotated tags at the merge commit.
3. Pushes the tags.
4. Creates one GitHub Release per tag, body sourced from each package's CHANGELOG entry.

::: warning Squash-merge subject inheritance
The release PR's commit body carries machine-readable trailers that `monorel tag` reads post-merge. The squash-merge setting must preserve the body. See [Branch protection](/integrations/github#branch-protection) for which settings work.
:::

## Verify

```sh
git fetch --tags
git tag --list
# transports/foo/v1.7.0

cat transports/foo/CHANGELOG.md
# ## [1.7.0] - 2026-04-30
#
# ### Minor Changes
#
# - Adds Lazy() helper for deferred field evaluation.
```

The corresponding GitHub Release is at `github.com/<owner>/<repo>/releases/tag/transports/foo/v1.7.0` with the same CHANGELOG entry as its release notes.

## Working without CI

For repos that don't use CI, or for local one-shot releases (e.g. ad-hoc patches before the workflow is wired up), the local CLI does the same thing in one shot:

```sh
monorel release
```

Which:

1. Runs the same `apply` step as CI, in your working tree.
2. Creates per-package tags locally.

You then push:

```sh
git push --follow-tags
GITHUB_TOKEN=... monorel publish
```

`monorel publish` creates the GitHub Releases. Splitting `release` from `publish` is necessary because GitHub validates that the tag exists on the remote before allowing a Release to be created against it.

This flow is fine for solo projects or bootstrap. Most real consumers will use the Action-driven flow above so contributors don't need to remember the order or have a `GITHUB_TOKEN` locally.

## Next steps

- [Changesets](/changesets): file format, multi-package shape, pre-release mode interaction.
- [Configuration](/configuration): every `monorel.toml` field with examples.
- [GitHub Action](/integrations/github): every wrapper input, branch protection setup, troubleshooting.
- [CLI Reference](/cli-reference): per-command flags and output schemas.
