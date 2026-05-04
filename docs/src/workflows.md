---
title: Workflows
description: "Common monorel workflows as ASCII sequence diagrams: daily contributor flow, cutting a release, pre-release cycles."
---

# Workflows

The diagrams below cover the three lifecycles every monorel user touches: opening a feature PR, cutting a release, and running a pre-release window. Plus a short sequence for the one-time first setup. For the full command reference see [CLI Reference](/cli-reference).

## First-time setup

```
1. monorel init
   └─> writes monorel.toml + .changeset/README.md

2. monorel validate
   └─> sanity-checks the generated config (paths exist, no
       duplicate tag prefixes, etc.)

3. Add .github/workflows/release.yml
   (copy from Getting Started, or fork github.com/disaresta-org/monorel-example)

4. Commit and push to main
   └─> release.yml runs `monorel auto`, finds no changesets, no
       release PR opens (this is the expected steady state until
       your first changeset)
```

After this, every release-affecting PR includes a `.changeset/<name>.md` and the release PR opens automatically. See [Getting Started](/getting-started) for the full walkthrough.

## Daily contributor flow

What happens when a contributor opens a PR with a changeset and merges it.

```
┌─────────────────────────────────────────────────────────┐
│ 1. Feature PR                                           │
│                                                         │
│    Branch carries:                                      │
│      - the code change                                  │
│      - .changeset/<name>.md naming affected packages    │
│        and bump levels (created by `monorel add`)       │
└────────────────────────┬────────────────────────────────┘
                         │  merge to main
                         ▼
┌─────────────────────────────────────────────────────────┐
│ 2. main has the new commit                              │
│    release.yml fires; runs `monorel auto`               │
│                                                         │
│    auto's detect step: HEAD is a feature commit         │
│    (no monorel-Release: trailer, no PR matches)         │
│    → dispatch to feature path                           │
└────────────────────────┬────────────────────────────────┘
                         │  feature path on a fresh
                         │  monorel/release branch:
                         │    monorel apply
                         │    git push --force
                         │    monorel preview --upsert
                         ▼
┌─────────────────────────────────────────────────────────┐
│ 3. Always-open release PR (opens or updates)            │
│                                                         │
│    The PR's diff IS the file changes the next release   │
│    will produce:                                        │
│      - new CHANGELOG entries per affected package       │
│      - deleted .changeset/*.md files                    │
│      - tidied go.mod / go.sum across released modules   │
│      - one chore(release): commit with                  │
│        monorel-Release: trailers in the body            │
└─────────────────────────────────────────────────────────┘
```

Subsequent feature PRs with their own changesets re-trigger the workflow. The release PR's diff updates each time, accumulating the staged changes. Closing the PR without merging cancels that release window.

## Cutting a release

What happens when a maintainer merges the always-open release PR.

```
┌─────────────────────────────────────────────────────────┐
│ 1. Release PR contains all the changesets to ship       │
│                                                         │
│    Reviewer reads the rendered CHANGELOG diff,          │
│    confirms version bumps, approves.                    │
└────────────────────────┬────────────────────────────────┘
                         │  squash-merge the release PR
                         │  (or rebase / merge-commit;
                         │   body trailers MUST be preserved)
                         ▼
┌─────────────────────────────────────────────────────────┐
│ 2. main has the chore(release): merge commit            │
│    release.yml fires; runs `monorel auto`               │
│                                                         │
│    auto's detect step: HEAD is the release-PR merge     │
│    (trailer in body OR API: head ref == monorel/release)│
│    → dispatch to release path                           │
└────────────────────────┬────────────────────────────────┘
                         │  release path:
                         │    monorel tag                ──┐
                         │    git push --follow-tags     │ in this order
                         │    monorel publish            ──┘
                         ▼
┌─────────────────────────────────────────────────────────┐
│ 3. Tags and Releases are live                           │
│                                                         │
│    For each released package:                           │
│      - annotated git tag at the merge commit            │
│        (bare vX.Y.Z for root, prefix/vX.Y.Z for subs)   │
│      - GitHub Release with the CHANGELOG entry as body  │
│                                                         │
│    Consumers can now: go get <module>@vX.Y.Z            │
└─────────────────────────────────────────────────────────┘
```

`monorel tag` reads the merge commit's `monorel-Release:` body trailers to learn which tags to create. `git push --follow-tags` must come before `monorel publish` because GitHub validates that the tag exists on the remote before allowing a Release to be created against it.

## Pre-release cycle

How a beta / rc window works. Multiple pre-release cuts accumulate changes; a single stable release at the end consumes them all.

```
┌─────────────────────────────────────────────────────────┐
│ 1. monorel pre enter rc                                 │
│    └─> writes .changeset/pre.json                       │
│        (channel = "rc", per-package counter starts at 0)│
└────────────────────────┬────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────┐
│ 2. Feature PR(s) merge as usual                         │
│    Each adds a .changeset/<name>.md                     │
│                                                         │
│    The release PR shows pre-release versions:           │
│      e.g. transports/foo: v1.6.0  ->  v1.7.0-rc.0       │
└────────────────────────┬────────────────────────────────┘
                         │  merge release PR
                         ▼
┌─────────────────────────────────────────────────────────┐
│ 3. monorel tag creates v1.7.0-rc.0                      │
│                                                         │
│    Important deltas from a stable release:              │
│      - .changeset/*.md files NOT deleted                │
│      - CHANGELOG entries NOT written                    │
│      - pre.json counter increments                      │
│        (next rc will be -rc.1)                          │
└────────────────────────┬────────────────────────────────┘
                         │  loop back to 2 for the next rc;
                         │  or proceed to 4 when ready to
                         │  ship stable
                         ▼
┌─────────────────────────────────────────────────────────┐
│ 4. monorel pre exit                                     │
│    └─> removes .changeset/pre.json                      │
└────────────────────────┬────────────────────────────────┘
                         │  push to main; `monorel auto`
                         │  refreshes the release PR with
                         │  stable versions
                         ▼
┌─────────────────────────────────────────────────────────┐
│ 5. Stable release: merge release PR                     │
│                                                         │
│    THIS time:                                           │
│      - tag created as transports/foo/v1.7.0             │
│        (no -rc suffix)                                  │
│      - all accumulated .changeset/*.md files DELETED    │
│      - one CHANGELOG entry per affected package,        │
│        containing every changeset body since pre enter  │
└─────────────────────────────────────────────────────────┘
```

Switching channels (e.g. `rc` → `beta`) requires `monorel pre exit` first; entering a new channel while one is active is rejected. The next stable release applies all accumulated changesets cumulatively, so escalating-severity changes during the pre-release window are reflected correctly.

## See also

- [Cheat Sheet](/cheat-sheet) for an at-a-glance index of every command by lifecycle phase + runner, common one-liners, and the files monorel reads and writes.
- [Changesets](/changesets) for the `.changeset/<name>.md` file format.
- [GitHub Action](/integrations/github) for the workflow YAML and token setup.
- [FAQ](/faq) for the questions that come up after the first release.

## CI environment requirements

Any CI system invoking `monorel auto` (GitHub Actions, GitLab CI, Gitea Actions, Bitbucket Pipelines, CircleCI, Drone, self-hosted runners) must provide the following:

- **Go installed at a version compatible with every released sub-module's `go` directive.** monorel runs `go mod tidy` with `GOTOOLCHAIN=local` during release; the env var is intentional and part of the offline-tidy determinism guarantee. Auto-toolchain-download is blocked. Pin the runner's Go to the highest floor explicitly (or use `go-version-file: go.mod` if the root module's `go` directive matches the highest sub-module floor).
- **`GOPROXY` set to a real proxy or `direct`.** monorel's release pipeline includes a "prime cache" step that runs `go mod download` (with the inherited `GOPROXY`) to populate the local module cache for third-party deps. The offline tidy that follows uses `GOPROXY=off` regardless of this setting; only the priming step honors `GOPROXY`. If `GOPROXY` is empty or missing, `go mod download` falls back to its default (`https://proxy.golang.org,direct`), which is what most CI systems already provide implicitly.
- **Push permissions for tags + the `monorel/release` branch.** `monorel auto`'s release path runs `git push --follow-tags`; the feature path force-pushes `monorel/release`. The runner's git config needs commit + push credentials.
- **Provider API token.** `monorel auto` reads the provider token to maintain the always-open release PR (feature path) and to create per-tag releases (release path). The token needs `contents: write` and `pull-requests: write` (GitHub naming; equivalent on GitLab / Gitea / Bitbucket).

For GitHub Actions, see [`ci/github/README.md`](../../ci/github/README.md) for the canonical workflow examples that satisfy these requirements.

For GitLab CI, the equivalent setup is a `before_script:` block that installs Go (e.g., via the `golang:1.26` image or a `gimme` install) and a `rules:` clause on the job that resolves Go modules (see "Avoiding the chore(release) CI race" below). The token requirement maps to `CI_JOB_TOKEN` for read-only access plus a project access token for push/release operations.

For Gitea Actions, the syntax matches GitHub Actions (Gitea Actions is API-compatible). Substitute `disaresta-org/monorel/ci/github@<version>` references with whatever path your Gitea instance uses for the same action.

## Avoiding the chore(release) CI race

The release commit `chore(release): ...` (created when the always-open release PR is merged) updates module `go.mod` files to require new in-plan sibling versions. The matching tags are created and pushed by the workflow running `monorel release` on the same push. Any *other* workflow that fires on the same push and resolves Go module versions will race the tag push and may transiently fail with:

```
go: example.com/foo/v2: reading example.com/foo/go.mod at revision v2.1.0: unknown revision v2.1.0
```

The release succeeds and the tags get pushed, but the racing workflow's red mark stays in the UI. The fix is to skip the racing workflow when the head commit subject begins with `chore(release):`. The principle is universal; the syntax varies per CI system.

### GitHub Actions / Gitea Actions

Same syntax: an `if:` clause on each job that runs Go module resolution.

```yaml
jobs:
  test:
    if: github.event_name == 'pull_request' || !startsWith(github.event.head_commit.message, 'chore(release):')
    # ... rest of job ...
```

See [`ci/github/README.md`](../../ci/github/README.md#skipping-ci-on-chorerelease-commits) for the full GitHub Actions snippet.

### GitLab CI

Use a `rules:` clause on each job:

```yaml
test:
  rules:
    - if: '$CI_PIPELINE_SOURCE == "merge_request_event"'
    - if: '$CI_COMMIT_TITLE =~ /^chore\(release\):/'
      when: never
    - when: on_success
  script:
    - go test ./...
```

The two-clause `rules` first allows merge-request runs through unconditionally, then explicitly drops `chore(release):` push runs on the default branch, then defaults to running.

### Other CI systems

The principle is universal: skip the workflow when the head commit subject starts with `chore(release):`. Most CI systems support a per-job filter on commit message; the exact key varies (`commit.message`, `CI_COMMIT_MESSAGE`, `BUILDKITE_MESSAGE`, etc.). Apply the same `^chore(release):` regex check.

The release-pipeline workflow itself (the one running `monorel auto`) does NOT need this filter. `monorel auto` runs on every push to `main` and dispatches internally based on detection, so the only commits whose tag-resolution can race are the ones an *unrelated* CI workflow (lint, test, deploy) processes after the `chore(release):` push. Apply the skip there.
