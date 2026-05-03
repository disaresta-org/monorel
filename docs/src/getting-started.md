---
title: Getting Started
description: "Install monorel, scaffold the repo with `monorel init`, wire up the GitHub Action, ship your first release."
---

# Getting Started

This page walks you from a fresh repo to a published release using monorel's canonical flow: per-PR `.changeset/*.md` files, an always-open release PR, and a GitHub Action that drives the lifecycle. There's also a local-only flow for repos that don't use CI; see [Working without CI](#working-without-ci) at the bottom.

The shortest version: every release-affecting PR includes a changeset; the bot maintains an always-open release PR; you merge the release PR when you want to ship.

::: info Looking for a working example?
The [`examples/`](https://github.com/disaresta-org/monorel/tree/main/examples) directory in the monorel repo has minimal reference setups for each provider:

- [`examples/github/`](https://github.com/disaresta-org/monorel/tree/main/examples/github): composite action wrapper + two workflow files.
- [`examples/gitea/`](https://github.com/disaresta-org/monorel/tree/main/examples/gitea): same wrapper, Gitea Actions YAML format, `provider.host` set, Forgejo-compatible.
- [`examples/gitlab/`](https://github.com/disaresta-org/monorel/tree/main/examples/gitlab): single `.gitlab-ci.yml` using the published Docker image.

Each is a `monorel.toml` + workflow files + `.changeset/README.md` you can copy into your repo.

For a real production setup at scale, [loglayer-go](https://github.com/loglayer/loglayer-go) runs monorel across 25 sub-modules.
:::

## Install

```sh
go install monorel.disaresta.com/cmd/monorel@latest
```

You'll use the local binary for `monorel init` and `monorel add`. CI uses a published binary via the action wrapper (no install step in your repo).

::: tip On macOS or Windows?
Pre-built binaries are unsigned, so Gatekeeper / SmartScreen will warn the first time you run one. If that's friction, use the [container image](/docker) instead (same binary, inside Linux).
:::

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

## Wire up the GitHub Action

Two workflow files drive the release lifecycle. The walkthrough below uses GitHub; if you're on Gitea or Forgejo, see the [Gitea / Forgejo integration page](/integrations/gitea) for the equivalent workflow files.

**`.github/workflows/release-pr.yml`** maintains the always-open release PR. Fires on every push to `main`:

<!--@include: ./_partials/github-release-pr-yml.md-->

**`.github/workflows/release.yml`** cuts the release after the always-open release PR is merged:

<!--@include: ./_partials/github-release-yml.md-->

Commit both files. The `release-pr` workflow will fire on the next push to `main`; the release PR opens once there's a changeset to release. The [GitHub integration page](/integrations/github) covers the Inputs table, branch-protection setup, and the PAT / App token escalation; this Getting Started section shows the canonical YAML.

::: warning Branch protection with required status checks
If your repo enforces required status checks on the default branch, the always-open release PR will sit indefinitely on "Some checks haven't completed yet" because PRs created by the default `GITHUB_TOKEN` don't trigger workflows (GitHub anti-recursion rule). The fix is to switch the `release-pr` workflow's token to a PAT or GitHub App token. See [Tokens and required status checks](/integrations/github#tokens-and-required-status-checks) for the wiring.
:::

## How releases work

Three steps across the lifecycle:

1. **Author a feature PR.** Include a `.changeset/<name>.md` file naming the affected packages and the bump level for each. Code change and changeset land in the same PR; merge as normal.
2. **The `release-pr` workflow updates the always-open release PR.** On each push to `main`, monorel runs a speculative apply on a `monorel/release` branch (writes the staged CHANGELOG entries, deletes the consumed changeset files, makes one `chore(release): ...` commit) and force-pushes. The release PR's diff IS the file changes the next release will produce.
3. **Merge the release PR when ready to ship.** The `release` workflow reads the merge commit's body trailers, creates per-package tags, pushes them, and creates one GitHub Release per tag.

A PR without a changeset doesn't trigger a release. The release PR auto-updates as more changesets accumulate; closing it without merging cancels that release window.

For a visual end-to-end view (including the pre-release-cycle variant), see [Workflows](/workflows).

## Author your first changeset

On a feature branch, run `monorel add` and answer the prompts. It writes a `.changeset/<random-name>.md` declaring the affected packages and bump levels. Commit the file alongside your code change, open the PR, and merge it as you would any other PR.

Full reference for the file format, multi-package shape, and other authoring modes (editor-driven body, scripted): [Changesets](/changesets).

## Watch the release PR

After your PR merges, the `release-pr` workflow runs against the new `main`. It:

1. Stages a `monorel/release` branch off `main`.
2. Runs `monorel apply` on it: writes the speculative `CHANGELOG.md` entries, deletes the consumed `.changeset/*.md` files, makes one `chore(release): <pkg> <ver>` commit.
3. Force-pushes that branch to the remote.
4. Opens (or updates) the always-open release PR with the rendered plan in the body.

The release PR's diff IS the actual file changes the release will produce, so reviewers see real CHANGELOG content rather than just a body summary.

If you merge another feature PR with another changeset, the release-pr workflow reruns and the release PR's diff updates.

## Cut the release

Merge the release PR. The `release` workflow fires on the merge commit and:

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
