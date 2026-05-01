---
title: GitHub Action
description: "Wire up the monorel GitHub Action for the always-open release PR pattern."
---

# GitHub Action

The `disaresta-org/monorel/ci/github` composite action wraps the monorel binary for use in GitHub Actions. It downloads the right binary for the runner OS+arch, sets up git, and invokes monorel with the requested command.

## Installation

Add the action to a workflow:

```yaml
- uses: disaresta-org/monorel/ci/github@v1
  with:
    command: release
```

## Inputs

| Input | Default | Description |
|-------|---------|-------------|
| `command` | required | `pr` (run `monorel preview --upsert` to maintain the release PR) or `release` (run the local→push→publish pipeline). |
| `version` | `latest` | Pin a specific monorel version, e.g. `v1.2.3`. |
| `token` | the workflow's auto-generated `GITHUB_TOKEN` | Token used for GitHub API calls. Needs `contents: write` and `pull-requests: write` permissions on the workflow. |
| `config` | `monorel.toml` | Path to the config file. |

::: tip Token override
If you need a different token (e.g. a personal access token to bypass branch protection), pass it via the `token` input using GitHub Actions context syntax. The action uses the workflow's default token when the input is unset.
:::

The `release` command runs three monorel invocations in order:

1. `monorel release` — local file mutations + commit + tag.
2. `git push --follow-tags` — publish commits and tags to the remote.
3. `monorel publish` — create one GitHub Release per tag at HEAD; body sourced from each package's CHANGELOG entry.

The split exists because GitHub validates that the tag is already on the remote before allowing a Release to be created against it.

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
      - uses: disaresta-org/monorel/ci/github@v1
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
      - uses: disaresta-org/monorel/ci/github@v1
        with:
          command: release
```

The `if:` filter triggers only on commits whose message starts with `chore(release):` (the message monorel produces for release commits). The release PR's merge commit matches this; regular commits don't.

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

### `monorel publish` fails partway through

monorel reports `Created N/M releases before failing.` Re-running publishes the remaining tags (each `CreateRelease` is idempotent on the tag name; the forge returns an error for duplicates, which the partial-success path surfaces). Tags from the prior `release` step are already in place.

## Notes

- The `pr` command currently runs `monorel preview --upsert` only. It does not yet stage CHANGELOG edits on a release branch; the upserted PR body shows the rendered plan but does not include file diffs. The action will gain branch staging in a follow-up release.
