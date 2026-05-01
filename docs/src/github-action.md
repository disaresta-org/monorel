---
title: GitHub Action
description: "Wire up the monorel GitHub Action for the always-open release PR pattern."
---

# GitHub Action

The `disaresta-org/monorel-action` composite action wraps the monorel binary for use in GitHub Actions. It downloads the right binary for the runner OS+arch, sets up git, and invokes monorel with the requested command.

::: warning Phase 10
The action ships in Phase 10 of monorel's own development. Until then, you can run monorel directly in a workflow via `go install` or by downloading a binary from a Release.
:::

## Installation

Add the action to a workflow:

```yaml
- uses: disaresta-org/monorel-action@v1
  with:
    command: release
```

## Inputs

| Input | Default | Description |
|-------|---------|-------------|
| `command` | required | `pr` (run `monorel preview` and upsert the release PR) or `release` (run `monorel release --publish`). |
| `version` | `latest` | Pin a specific monorel version, e.g. `v1.2.3`. |
| `token` | the workflow's auto-generated `GITHUB_TOKEN` | The token used for GitHub API calls. Needs `contents: write` and `pull-requests: write` permissions on the workflow. |
| `config` | `monorel.toml` | Path to the config file. |

::: tip Token override
If you need a different token (e.g. a personal access token to bypass branch protection), pass it via the `token` input using GitHub Actions context syntax. The action uses the workflow's default token when the input is unset.
:::

## Workflows

### `release-pr.yml`: maintain the always-open release PR

```yaml
name: release-pr
on:
  push:
    branches: [main]

permissions:
  contents: write
  pull-requests: write

jobs:
  release-pr:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
      - uses: disaresta-org/monorel-action@v1
        with:
          command: pr
```

On every push to `main`, the action computes the plan and either:

- Creates / updates the release PR if there are pending changesets.
- Closes the release PR if there are no pending changesets.

### `release.yml`: publish on release-PR merge

```yaml
name: release
on:
  push:
    branches: [main]

permissions:
  contents: write

jobs:
  release:
    if: contains(github.event.head_commit.message, 'chore(release):')
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
      - uses: disaresta-org/monorel-action@v1
        with:
          command: release
```

The `if:` filter triggers only on commits whose message starts with `chore(release):` (the message monorel produces for release commits). The release PR's merge commit matches this; regular commits don't.

The action runs `monorel release --publish`, pushes the resulting tags, and creates one GitHub Release per tag.

## Branch protection

Recommended settings for the default branch:

- Require PR review.
- Require status checks (CI) to pass before merge.
- Allow squash-merge for non-release PRs; allow merge-commit (or rebase) for the release PR.

The release PR is special: monorel's `chore(release):` commit subject must reach `main` for `release.yml` to trigger. Squash-merging the release PR produces a single `chore(release): ...` commit; rebase-merging produces the same. Both work; just don't squash-merge into a different subject.

## Troubleshooting

### "tag already exists" on release

monorel aborts before any mutation if a planned tag is already present on the remote. This usually means a previous release run partially succeeded (created the tag) but failed before pushing the commit. Investigate the remote state, delete the stale tag if appropriate, and re-run.

### The release PR doesn't update

Check the `release-pr.yml` run on the latest push to `main`. Common causes:

- The workflow lacks `pull-requests: write` permission.
- The `token` input doesn't have access to PRs.
- A path filter (if you added one) excluded the change.

### `--publish` fails partway through

monorel reports `Created N/M releases before failing.` Re-running creates the remaining releases (each `CreateRelease` is idempotent on the tag name; GitHub returns an error for duplicates, which the partial-success path will surface). The tags themselves are already in place from the prior `release` run.
