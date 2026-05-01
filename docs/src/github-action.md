---
title: GitHub Action
description: "Wire up the monorel GitHub Action for the always-open release PR pattern."
---

# GitHub Action

The `disaresta-org/monorel/ci/github` composite action wraps the monorel binary for use in GitHub Actions. It downloads the right binary for the runner OS+arch, sets up git, stages the release branch (for the `pr` command), and invokes monorel with the requested command.

## Installation

Add the action to a workflow:

```yaml
- uses: disaresta-org/monorel/ci/github@v0.1.2
  with:
    command: release
```

::: tip Pre-1.0 pinning
monorel hasn't shipped a moving major-track tag yet (no `@v0` or `@v1` ref). Pin to an exact patch (`@v0.1.2`) until that ships. Bump deliberately when a new monorel patch lands.
:::

## Inputs

| Input | Default | Description |
|-------|---------|-------------|
| `command` | required | `pr` (run `monorel preview --upsert` to maintain the release PR) or `release` (run the local→push→publish pipeline). |
| `version` | `latest` | Pin a specific monorel version, e.g. `v0.1.2`. |
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

The `pr` command stages the configured head branch (default `monorel/release`) before invoking the orchestrator. It force-creates that branch from `origin/<default-branch>` plus one empty marker commit, then force-pushes, so GitHub's PR-create API accepts the always-open release PR. The branch's diff stays empty by design — the rendered plan goes in the PR body, not in a code diff.

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
    # Skip on the release PR's own merge commit (subject begins with
    # "chore(release):" by monorel convention) so the workflow doesn't
    # churn the just-merged PR.
    if: ${{ !startsWith(github.event.head_commit.message, 'chore(release):') }}
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
      - uses: disaresta-org/monorel/ci/github@v0.1.2
        with:
          command: pr
```

On every push to `main` (except the release-PR merge), the action computes the plan and either:

- Creates / updates the release PR if there are pending changesets.
- Closes the release PR if there are no pending changesets.

### `release.yml`: publish on release-PR merge

The minimal shape — release-only — is:

```yaml
name: release
on:
  push:
    branches: [main]
  workflow_dispatch:

permissions:
  contents: write

jobs:
  release:
    if: github.event_name == 'workflow_dispatch' || startsWith(github.event.head_commit.message, 'chore(release):')
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
      - uses: disaresta-org/monorel/ci/github@v0.1.2
        with:
          command: release
```

The `if:` filter is `startsWith(...)`, not `contains(...)`. monorel's release commit subject is exactly `chore(release): ...`; the prefix check is precise and matches the marker commit's subject inheritance behavior described under [Branch protection](#branch-protection). Use `workflow_dispatch` for the bootstrap path before monorel-driven releases are wired up (see the [bootstrap recipe](/bootstrap)).

### Chaining downstream workflows (deploy-docs, build-binaries, etc.)

GitHub's anti-recursion rule suppresses `release: published` and `push: tags` events when those events are caused by a workflow using `secrets.GITHUB_TOKEN`. monorel's `publish` step creates the GitHub Release and `git push --follow-tags` pushes the tag using `GITHUB_TOKEN`, so any workflow you'd expect to fire on `release: published` or on `push: tags: 'v*'` after a monorel-driven release will silently *not* fire.

The supported sidestep is to chain those workflows from `release.yml` via `workflow_call`. The pattern:

```yaml
# release.yml — extended with chained downstream workflows
jobs:
  release:
    # … same as above, but expose the released root tag as an output …
    outputs:
      root_tag: ${{ steps.root_tag.outputs.root_tag }}
    steps:
      - uses: actions/checkout@v4
        with: { fetch-depth: 0 }
      - uses: disaresta-org/monorel/ci/github@v0.1.2
        with:
          command: release
      - name: Capture root tag
        id: root_tag
        run: |
          root_tag=$(git tag --points-at HEAD | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' | sort -V | tail -1 || true)
          echo "root_tag=${root_tag}" >> "$GITHUB_OUTPUT"

  deploy-docs:
    needs: release
    if: ${{ needs.release.result == 'success' }}
    uses: ./.github/workflows/docs.yml
    permissions:
      contents: read
      pages: write
      id-token: write

  build-binaries:
    needs: release
    if: ${{ needs.release.result == 'success' && needs.release.outputs.root_tag != '' }}
    uses: ./.github/workflows/build-release-binaries.yml
    with:
      tag: ${{ needs.release.outputs.root_tag }}
    permissions:
      contents: write

  build-image:
    needs: release
    if: ${{ needs.release.result == 'success' && needs.release.outputs.root_tag != '' }}
    uses: ./.github/workflows/build-image.yml
    with:
      tag: ${{ needs.release.outputs.root_tag }}
    permissions:
      contents: read
      packages: write
```

The chained workflows must declare `workflow_call` in their `on:` block and accept whatever inputs they need (e.g. a `tag` input for build workflows). The natural `push: tags` and `release: published` triggers can stay alongside `workflow_call` so manual tag pushes and externally-created Releases still fire the downstream chain.

The `root_tag` capture is what lets `build-binaries` and `build-image` skip themselves when the release was sub-module-only (no `vX.Y.Z` root tag created). For docs deploy this isn't needed — every release should redeploy the docs.

## Branch protection

Recommended settings for the default branch:

- Require PR review.
- Require status checks (CI) to pass before merge.
- Allow squash-merge for non-release PRs; allow merge-commit (or rebase) for the release PR.

The release PR is special: monorel's `chore(release):` commit subject must reach `main` for `release.yml` to trigger. Squash-merging the release PR produces a single `chore(release): ...` commit; rebase-merging produces the same. Both work; just don't squash-merge into a different subject.

::: warning Squash-merge subject inheritance
By default, the marker commit on the staged head branch carries the subject `chore(release): always-open release PR marker`. When you squash-merge the release PR, GitHub picks the squash subject from one of two sources depending on repo settings:

- **"Default to pull request title"**: the squash subject is the orchestrator-set PR title (e.g. `chore(release): transports/blank v1.6.2`). This is the cleaner outcome.
- **"Default commit message"** (legacy): the squash subject is the marker commit's subject (e.g. `chore(release): always-open release PR marker`). Functionally equivalent — both start with `chore(release):` so `release.yml` fires either way — but visually less informative.

Set the repo's squash-merge default to "pull request title" to get the better-named merge commits. Either setting works for the release pipeline itself.
:::

## Troubleshooting

### "tag already exists" on release

monorel aborts before any mutation if a planned tag is already present on the remote. This usually means a previous release run partially succeeded (created the tag) but failed before pushing the commit. Investigate the remote state, delete the stale tag if appropriate, and re-run.

### The release PR doesn't update

Check the `release-pr.yml` run on the latest push to `main`. Common causes:

- The workflow lacks `pull-requests: write` permission.
- The `token` input doesn't have access to PRs.
- A path filter (if you added one) excluded the change.
- The `if:` filter skipped the run because the head commit's subject starts with `chore(release):`. That's expected behavior for the release PR's merge commit; if it's hitting non-release commits, check the filter.

### Tag-triggered downstream workflows don't fire

Symptom: a release lands, the tag exists on origin and a GitHub Release is created, but workflows you expected to fire on `push: tags: 'v*'` (e.g. binary builds, image pushes) didn't run.

Cause: GitHub's anti-recursion rule suppresses `push: tags` events when the tag was pushed via `GITHUB_TOKEN` from another workflow. The fix is to chain those workflows from `release.yml` via `workflow_call`; see [Chaining downstream workflows](#chaining-downstream-workflows-deploy-docs-build-binaries-etc). The natural `push: tags` trigger still works for direct `git push --tags` flows; the chain covers the monorel-driven path.

### `monorel publish` fails partway through

monorel reports `Created N/M releases before failing.` Re-running publishes the remaining tags (each `CreateRelease` is idempotent on the tag name; the forge returns an error for duplicates, which the partial-success path surfaces). Tags from the prior `release` step are already in place.

### "422 Field:head Code:invalid" on release-pr

If you see this with a monorel version older than v0.1.1, upgrade to `@v0.1.2` or later. v0.1.1 added the head-branch staging step the orchestrator needs; v0.1.0 didn't push the head branch, so GitHub's PR-create API rejected the always-open PR.
