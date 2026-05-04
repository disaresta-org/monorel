---
title: GitHub
description: "Wire up monorel against a GitHub repository: action wrapper, workflows, branch protection, tokens."
---

# GitHub

The canonical monorel-on-GitHub setup: a composite action wrapper plus one workflow file that drives the always-open release PR lifecycle. Set `provider.name = "github"` (the default) in `monorel.toml` and the action wrapper takes care of the rest.

::: tip Example
Working reference setup at [`examples/github/`](https://github.com/disaresta-org/monorel/tree/main/examples/github). Copy the files you need.
:::

## Configuration

`monorel.toml`:

```toml
[provider]
name  = "github"  # optional; default
owner = "acme"
repo  = "widget"
host  = ""        # optional; set for GitHub Enterprise
```

| Field | Notes |
|-------|-------|
| `name` | `"github"` (the default; can be omitted). |
| `owner`, `repo` | The user / org that owns the repo and the repository name. |
| `host` | API host for GitHub Enterprise (e.g. `github.example.com`). Empty for github.com. |

## Token

The action wrapper passes the workflow's auto-generated `GITHUB_TOKEN` to the binary by default. Required workflow permissions: `contents: write`, `pull-requests: write`. Override with the `token` input when you need a personal access token or GitHub App token; see [Tokens and required status checks](#tokens-and-required-status-checks) below for the escalation path.

## Workflows

One workflow file drives the entire lifecycle. On every push to `main`, the wrapper runs [`monorel auto`](/cli-reference#monorel-auto), which detects whether HEAD is the merge of monorel's release PR and dispatches accordingly:

- **Feature commit** (the common case): stage the always-open release PR's diff via `monorel apply` + `monorel preview --upsert`. If the planner has nothing to apply, any open release PR is closed.
- **Release-PR merge** (HEAD is the squash / rebase / merge-commit of `monorel/release`): run `monorel tag` + `git push --follow-tags` + `monorel publish` to create per-package tags and one GitHub Release per tag.

Detection uses two signals OR'd together: HEAD's `monorel-Release:` commit-body trailer (fast path, no network), and the provider's `FindPRByMergeCommit` API returning a PR whose source branch is `monorel/release` (covers cases where the trailer is lost). Either signal alone is sufficient, so the dispatch works regardless of merge strategy.

The `disaresta-org/monorel/ci/github` composite action wrapper:

- Downloads the monorel binary for the runner OS + arch.
- Configures the git author for the `apply` commit on `monorel/release`.
- Runs `monorel auto` against the configured `monorel.toml`.

::: tip Pinning the action wrapper
monorel doesn't auto-publish a moving major-track tag (no `@v1` ref). Pin to an exact patch (`@v1.0.0` or whichever you've validated). Bump deliberately when a new monorel release lands.
:::

<!--@include: ../_partials/github-release-yml.md-->

### `doctor.yml`: pre-merge sanity check (recommended)

A separate workflow that runs `monorel doctor` against every PR. Today's only built-in check catches stale-branch + squash-merge changeset revivals; the workflow exits non-zero on any error-severity finding so the PR can't merge until cleaned up. See [`monorel doctor`](/cli-reference#monorel-doctor) for the diagnostic itself.

<!--@include: ../_partials/github-doctor-yml.md-->

`fetch-depth: 0` is required: doctor's `git log --grep='chore(release):'` scan needs full history to see prior release commits. The default shallow checkout would miss them and turn the check into a no-op.

### Action wrapper inputs

| Input | Default | Description |
|-------|---------|-------------|
| `version` | `latest` | Pin a specific monorel version, e.g. `v1.0.0`. |
| `token` | the workflow's auto-generated `GITHUB_TOKEN` | Token used for GitHub API calls. Needs `contents: write` and `pull-requests: write` permissions on the workflow. Override with a PAT or App token via `secrets.<name>` syntax. |
| `config` | `monorel.toml` | Path to the config file. |

### Chaining downstream workflows (deploy-docs, build-binaries, etc.)

GitHub's anti-recursion rule suppresses `release: published` and `push: tags` events when those events are caused by a workflow using `secrets.GITHUB_TOKEN`. monorel's `publish` step creates the GitHub Release and `git push --follow-tags` pushes the tag using `GITHUB_TOKEN`, so any workflow you'd expect to fire on `release: published` or on `push: tags: 'v*'` after a monorel-driven release will silently *not* fire.

The supported sidestep is to chain those workflows from the monorel workflow via `workflow_call`. The pattern:

```yaml
# release.yml: extended with chained downstream workflows
jobs:
  monorel:
    # … same as above, but expose the released root tag as an output …
    outputs:
      root_tag: ${{ steps.root_tag.outputs.root_tag }}
    steps:
      - uses: actions/checkout@v4
        with: { fetch-depth: 0 }
      - uses: disaresta-org/monorel/ci/github@v1.0.0
      - name: Capture root tag
        id: root_tag
        run: |
          root_tag=$(git tag --points-at HEAD | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' | sort -V | tail -1 || true)
          echo "root_tag=${root_tag}" >> "$GITHUB_OUTPUT"

  deploy-docs:
    needs: monorel
    if: ${{ needs.monorel.result == 'success' }}
    uses: ./.github/workflows/docs.yml
    permissions:
      contents: read
      pages: write
      id-token: write

  build-binaries:
    needs: monorel
    if: ${{ needs.monorel.result == 'success' && needs.monorel.outputs.root_tag != '' }}
    uses: ./.github/workflows/build-release-binaries.yml
    with:
      tag: ${{ needs.monorel.outputs.root_tag }}
    permissions:
      contents: write

  build-image:
    needs: monorel
    if: ${{ needs.monorel.result == 'success' && needs.monorel.outputs.root_tag != '' }}
    uses: ./.github/workflows/build-image.yml
    with:
      tag: ${{ needs.monorel.outputs.root_tag }}
    permissions:
      contents: read
      packages: write
```

`monorel auto` exits cleanly on every push, including non-release commits where it just keeps the release PR up to date. The chained jobs run on every push but their own no-op gates (the `root_tag` check, or the docs job's `release.result == 'success'`) keep them cheap.

The chained workflows must declare `workflow_call` in their `on:` block and accept whatever inputs they need (e.g. a `tag` input for build workflows). The natural `push: tags` and `release: published` triggers can stay alongside `workflow_call` so manual tag pushes and externally-created Releases still fire the downstream chain.

The `root_tag` capture is what lets `build-binaries` and `build-image` skip themselves when the release was sub-module-only (no `vX.Y.Z` root tag created). For docs deploy this isn't needed; every release should redeploy the docs.

## Branch protection

Recommended settings for the default branch:

- Require PR review.
- Require status checks (CI) to pass before merge.
- Any of squash, rebase, or merge-commit work for the release PR. Detection is API-based; the merge strategy doesn't change which signal monorel uses.

### Merge strategy

`monorel auto` detects a release-PR merge using two independent signals OR'd together:

- **Trailer signal** (fast path): HEAD's commit body contains a `monorel-Release:` trailer. Hits when squash- or rebase-merge propagated the source commit's body.
- **API signal** (network): provider's `FindPRByMergeCommit` returns a PR whose source branch is `monorel/release`. Hits when the trailer is missing for any reason (a squash-merge configuration that strips the body, a merge-commit body that ignores the parent, etc.).

Either signal alone is enough. So squash, rebase, and merge-commit are all fine merge strategies for the release PR; pick whichever matches the rest of your repo's convention.

::: tip The trailer is still useful when present
`monorel tag` (run by `monorel auto` on the release path) reads the trailer for fast tag derivation. When the trailer is missing, `monorel tag` falls back to the universal trailers source (a `<!-- monorel-trailers ... -->` HTML comment that `monorel preview --upsert` writes into the PR body), so tag creation also works regardless of merge strategy. The fallback adds one API round-trip; the trailer path is direct.
:::

If a release PR merged without producing tags despite the API detecting the merge, the most likely cause is the trailers HTML comment was edited out of the PR body before merge. Recovery: hand-create the tags (`git tag -a <prefix>/v<X.Y.Z> <merge-sha> -m 'Release ...'` for each package) and push them, then run `monorel publish` against the pushed tags.

## Tokens and required status checks

By default the action uses the workflow's auto-generated `GITHUB_TOKEN`. This works for most monorel operations: opening / updating the release PR, creating tags, publishing GitHub Releases. It has one significant limitation: **PRs created by `GITHUB_TOKEN` don't trigger other workflows** (GitHub's [anti-recursion rule](https://docs.github.com/en/actions/security-guides/automatic-token-authentication#using-the-github_token-in-a-workflow)).

This bites monorel specifically when:

- Branch protection requires status checks (e.g. `lint`, `test`) to pass before merging.
- `monorel auto` (running in the release workflow on every push to `main`) opens or updates the always-open release PR.
- Those required checks never fire on the release PR (because `pull_request` events for `GITHUB_TOKEN`-created PRs are suppressed).
- The release PR sits forever with "Some checks haven't completed yet" and can't be merged through standard branch protection.

The fix is to use a token whose author identity isn't `github-actions[bot]`. Three options:

### PAT (personal access token)

Simplest path. Create a fine-grained PAT scoped to the target repo with these permissions:

- **Pull requests**: Read and write.
- **Contents**: Read and write.

Add it as a repo secret (e.g. `MONOREL_PR_TOKEN`) and pass it to the action's `token` input:

```yaml
- uses: disaresta-org/monorel/ci/github@v1.0.0
  with:
    token: ${{ secrets.MONOREL_PR_TOKEN }}
```

PRs the action creates with this token are authored by your user, which means GitHub treats them as ordinary PRs and fires the workflows you'd expect.

::: warning PAT lifecycle
The PAT is tied to the user who minted it. If that user leaves the org or rotates credentials, the token must be regenerated. Use a service-account user if your org allows them; otherwise plan for the rotation.
:::

### GitHub App

More robust for org-managed repos. [Create a GitHub App](https://docs.github.com/en/apps/creating-github-apps) with these repository permissions:

- **Pull requests**: Read and write.
- **Contents**: Read and write.

Install it on the target repo. Save the App ID as a repo variable (`MONOREL_APP_ID`) and the private key as a repo secret (`MONOREL_APP_PRIVATE_KEY`).

In the workflow, exchange the App's private key for a short-lived installation token via [`actions/create-github-app-token`](https://github.com/actions/create-github-app-token):

```yaml
jobs:
  monorel:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
      - uses: actions/create-github-app-token@v3
        id: app-token
        with:
          app-id: ${{ vars.MONOREL_APP_ID }}
          private-key: ${{ secrets.MONOREL_APP_PRIVATE_KEY }}
      - uses: disaresta-org/monorel/ci/github@v1.0.0
        with:
          token: ${{ steps.app-token.outputs.token }}
```

PRs created with the App's token are authored by the App's bot user (e.g. `monorel-bot[bot]`). They trigger workflows normally and aren't tied to any individual user. Tokens are short-lived (~1 hour) and minted per workflow run.

### Bypass branch protection

Last resort. In repo Settings → Rules → Rulesets, add `github-actions[bot]` (or whichever identity the workflow runs as) to the **Bypass list** of the ruleset enforcing the required checks. The release PR can then merge without the checks firing.

::: warning Trade-off
The release PR's diff IS the actual file changes the release will produce (CHANGELOG entries, changeset deletions). Running CI on it validates the staged result. Bypass means you trust the changeset content sight-unseen. Recommend only when CI on the release PR doesn't catch anything CI on the source PRs already caught.
:::

### Which to pick

| Repo size | Use case | Recommended |
|-----------|----------|-------------|
| Small / personal | One maintainer, infrequent rotation | PAT |
| Org-managed | Multiple maintainers, secret hygiene rules | GitHub App |
| Solo experiments | Don't care about CI on the release PR | Bypass |

The same token applies regardless of whether `monorel auto` lands on the feature branch (open / update the release PR) or the release branch (tag + push + publish). The anti-recursion concern only matters on the feature branch (where the action opens or updates a PR); on the release branch the token is just used for `monorel publish`'s API calls and `GITHUB_TOKEN` is sufficient there.

## Troubleshooting

### Release PR is stuck on "Some checks haven't completed yet"

Symptom: the always-open release PR shows required checks (`lint`, `test`, etc.) as "Expected — Waiting for status to be reported" indefinitely. The merge button is disabled.

Cause: GitHub's anti-recursion rule suppresses `pull_request` triggers on PRs created by `secrets.GITHUB_TOKEN`. The monorel workflow opened the PR using the default token, so your CI workflows didn't fire on it.

Fix: switch the workflow's `token` input to a PAT or GitHub App token. See [Tokens and required status checks](#tokens-and-required-status-checks).

### "tag already exists" on release

monorel aborts before any mutation if a planned tag is already present on the remote. This usually means a previous release run partially succeeded (created the tag) but failed before pushing the commit. Investigate the remote state, delete the stale tag if appropriate, and re-run.

### The release PR doesn't update

Check the monorel workflow run on the latest push to `main`. Common causes:

- The workflow lacks `pull-requests: write` permission.
- The `token` input doesn't have access to PRs.
- A path filter (if you added one) excluded the change.
- The push was the release-PR merge itself; `monorel auto` correctly took the release branch (tag + publish) and didn't touch the PR. The next non-release push reconciles.

### Tag-triggered downstream workflows don't fire

Symptom: a release lands, the tag exists on origin and a GitHub Release is created, but workflows you expected to fire on `push: tags: 'v*'` (e.g. binary builds, image pushes) didn't run.

Cause: GitHub's anti-recursion rule suppresses `push: tags` events when the tag was pushed via `GITHUB_TOKEN` from another workflow. The fix is to chain those workflows from the monorel workflow via `workflow_call`; see [Chaining downstream workflows](#chaining-downstream-workflows-deploy-docs-build-binaries-etc). The natural `push: tags` trigger still works for direct `git push --tags` flows; the chain covers the monorel-driven path.

### `monorel publish` fails partway through

monorel reports `Created N/M releases before failing.` Re-running publishes the remaining tags (each `CreateRelease` is idempotent on the tag name; the provider returns an error for duplicates, which the partial-success path surfaces). Tags from the prior run are already in place.

### "422 Field:head Code:invalid" on the release PR upsert

GitHub's PR-create API requires the head branch to exist on the remote with at least one commit between head and base. `monorel auto`'s feature-branch path creates `monorel/release` from the default branch and force-pushes the `monorel apply` commit, so the head exists by the time the orchestrator runs `monorel preview --upsert`. If you see this 422, the staging push failed silently. Check the action's run log for a `git push` error before the `monorel preview` invocation.

### `monorel tag` returns `ErrNoReleaseCommit`

The merge commit on `main` doesn't have `monorel-Release:` trailers in its body AND the merged PR's body doesn't contain the universal-fallback `<!-- monorel-trailers ... -->` HTML comment. Either someone edited the comment out before merge, or the release PR was opened by an older monorel that didn't write the comment. See [Merge strategy](#merge-strategy) for the fallback mechanics; recovery is to hand-create the tags pointing at the merge commit and run `monorel publish` against the pushed tags.

### `monorel tag` returns `ErrTagExists`

A tag the trailers ask for already exists on the remote, usually because a previous workflow run partially completed. See [Partial-tag failure mode](/cli-reference#monorel-tag) for recovery; the gist is `git tag -d <name>` locally plus `git push origin :refs/tags/<name>` to remove from the remote, then re-run.

### `monorel tag` returns `ErrUnknownReleasedPackage`

A trailer names a package not declared in `monorel.toml`. The config drifted between when the release PR was opened (when `monorel apply` ran) and when it was merged. Restore the missing entry in `monorel.toml`, or delete and recreate the release PR.
