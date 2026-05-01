---
title: GitHub Action
description: "Wire up the monorel GitHub Action for the always-open release PR pattern."
---

# GitHub Action

The `disaresta-org/monorel/ci/github` composite action wraps the monorel binary for use in GitHub Actions. It downloads the right binary for the runner OS+arch, sets up git, stages the release branch (for the `pr` command), and invokes monorel with the requested command.

## Installation

Add the action to a workflow:

```yaml
- uses: disaresta-org/monorel/ci/github@v0.4.0
  with:
    command: release
```

::: tip Pre-1.0 pinning
monorel hasn't shipped a moving major-track tag yet (no `@v0` or `@v1` ref). Pin to an exact patch (`@v0.4.0` or whichever you've validated) until that ships. Bump deliberately when a new monorel release lands.
:::

## Inputs

| Input | Default | Description |
|-------|---------|-------------|
| `command` | required | `pr` (stage the release PR's diff via `monorel apply` + `monorel preview --upsert`) or `release` (post-merge: `monorel tag` + push + `monorel publish`). |
| `version` | `latest` | Pin a specific monorel version, e.g. `v0.1.2`. |
| `token` | the workflow's auto-generated `GITHUB_TOKEN` | Token used for GitHub API calls. Needs `contents: write` and `pull-requests: write` permissions on the workflow. |
| `config` | `monorel.toml` | Path to the config file. |

::: tip Token override
If you need a different token (e.g. a personal access token to bypass branch protection), pass it via the `token` input using GitHub Actions context syntax. The action uses the workflow's default token when the input is unset.
:::

The `release` command runs three monorel invocations in order on the merge commit:

1. `monorel tag` — read HEAD's `monorel-Release:` commit-body trailers (written upstream by `monorel apply`) and create per-package annotated tags. The merge already brought the file changes in via the release PR; only the tags still need creating.
2. `git push --follow-tags` — push the new tags to the remote.
3. `monorel publish` — create one GitHub Release per tag at HEAD; body sourced from each package's CHANGELOG entry.

The split exists because GitHub validates that the tag is already on the remote before allowing a Release to be created against it.

The `pr` command implements **speculative apply**: it stages a fresh `monorel/release` branch off the default branch, runs `monorel apply` on it (writes per-package CHANGELOG entries / `pre.json` increments, deletes consumed `.changeset/*.md` files, creates one `chore(release): ...` commit), and force-pushes. The release PR's diff IS the file changes the release will produce. The orchestrator (`monorel preview --upsert`) then opens or updates the always-open release PR with the rendered plan in its body.

If the planner has nothing to apply (no pending changesets), `monorel apply` exits with `Nothing to apply.` and the `pr` command skips the force-push; the orchestrator closes any open release PR.

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
      - uses: disaresta-org/monorel/ci/github@v0.4.0
        with:
          command: pr
```

On every push to `main` (except the release-PR merge), the action stages the release PR's diff via speculative apply, then either:

- Creates / updates the release PR if there are pending changesets (the PR's diff is the actual CHANGELOG entries + changeset deletions).
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
      - uses: disaresta-org/monorel/ci/github@v0.4.0
        with:
          command: release
```

The `if:` filter is `startsWith(...)`, not `contains(...)`. monorel's release commit subject is exactly `chore(release): <pkg> <ver>` (or a comma-joined list for multi-package releases) — the prefix check is precise. Use `workflow_dispatch` for the bootstrap path before monorel-driven releases are wired up (see the [bootstrap recipe](/bootstrap)).

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
      - uses: disaresta-org/monorel/ci/github@v0.4.0
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

The release PR is special: monorel's `chore(release): <pkg> <ver>` commit subject AND the `monorel-Release:` body trailers must both reach `main` for `release.yml` to trigger and `monorel tag` to derive the right tags.

::: warning Preserve the staged commit body
The staging step (speculative apply) creates a real commit on `monorel/release` whose body carries the `monorel-Release:` trailers `monorel tag` reads. The merge commit on `main` MUST keep that body. Configure the squash subject + body via repo Settings → General → Pull Requests → "Default commit message for squash merging":

- **`Default message`** (legacy) — for single-commit PRs (which the release PR always is) the subject and body come straight from the head commit. Trailers preserved verbatim. Safe default.
- **`Pull request title and commit details`** — subject is the PR title, body lists the commit subjects and includes their bodies. The parser tolerates leading whitespace from any indentation, so trailers remain matchable.

What NOT to use:

- **`Pull request title`** — body is empty. Trailers lost. `monorel tag` returns `ErrNoReleaseCommit`.
- **`Pull request title and description`** — body is the PR description (the rendered plan that `monorel preview --upsert` writes), which doesn't contain the trailers. Trailers lost.

Rebase-merge and merge-commit both preserve the staged commit verbatim, so neither needs configuration.

If a release PR merged without trailers, the recovery is to manually create the tags (`git tag -a <prefix>/v<X.Y.Z> <merge-sha> -m 'Release ...'` for each package) and push them, then run `monorel publish` against the pushed tags.
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

monorel reports `Created N/M releases before failing.` Re-running publishes the remaining tags (each `CreateRelease` is idempotent on the tag name; the provider returns an error for duplicates, which the partial-success path surfaces). Tags from the prior `release` step are already in place.

### "422 Field:head Code:invalid" on release-pr

GitHub's PR-create API requires the head branch to exist on the remote with at least one commit between head and base. The `pr` command's speculative-apply step creates `monorel/release` from the default branch and force-pushes the `monorel apply` commit, so the head exists by the time the orchestrator runs `monorel preview --upsert`. If you see this 422, the staging push failed silently — check the `Run monorel pr` step's log for a `git push` error before the `monorel preview` invocation.

### `monorel tag` returns `ErrNoReleaseCommit`

The merge commit on `main` doesn't have `monorel-Release:` trailers in its body. Most likely cause: the squash-merge setting stripped the staged commit's body. See the [Branch protection](#branch-protection) section's `Preserve the staged commit body` warning for which settings work and how to recover.

### `monorel tag` returns `ErrTagExists`

A tag the trailers ask for already exists on the remote. Most often this means a previous `release` workflow run partially completed (created some tags before failing). Investigate, delete the partial tags (`git tag -d <name>` locally + `git push origin :refs/tags/<name>` to remove from the remote), and re-run.

### `monorel tag` returns `ErrUnknownReleasedPackage`

A trailer names a package not declared in `monorel.toml`. Indicates the config drifted between when the release PR was opened (when `monorel apply` ran) and when it was merged (when `monorel tag` runs). Restore the missing entry in `monorel.toml`, or delete and recreate the release PR.
