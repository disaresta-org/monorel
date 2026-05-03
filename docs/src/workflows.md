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

3. Add .github/workflows/release-pr.yml + release.yml
   (copy from Getting Started, or fork github.com/disaresta-org/monorel-example)

4. Commit and push to main
   └─> release-pr.yml runs, finds no changesets, no release PR opens
       (this is the expected steady state until your first changeset)
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
│    release-pr.yml fires automatically                   │
└────────────────────────┬────────────────────────────────┘
                         │  on a fresh monorel/release branch:
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
│    release.yml fires (matched by commit subject)        │
└────────────────────────┬────────────────────────────────┘
                         │  inside the action wrapper:
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
                         │  push to main; release-pr fires
                         │  release PR now shows stable versions
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

## Command map

Every command shown in the diagrams above, indexed by phase and runner. The diagrams describe what happens; this table answers "which command, when, by whom?":

| Phase | Command | Run by | Purpose |
|---|---|---|---|
| First-time setup | `monorel init` | Maintainer (local) | Scaffold `monorel.toml` from go.mod files + git origin. |
| First-time setup | `monorel validate` | Maintainer (local) or PR-time CI | Confirm the config loads and paths exist. |
| Daily contributor flow | `monorel add` | Contributor (local) | Author a `.changeset/<name>.md` for the current PR. |
| Daily contributor flow | `monorel doctor` | PR-time CI (optional) | Diagnose stale-branch + revived-changeset issues; non-zero exit fails the PR. |
| Daily contributor flow | `monorel apply` | `release-pr.yml` (CI) | Stage the file changes the next release will produce (CHANGELOGs, go.mod / go.sum tidies, consumed-changeset removals) into a single commit on the staging branch. |
| Daily contributor flow | `monorel preview --upsert` | `release-pr.yml` (CI) | Open or update the always-open release PR with the rendered plan. |
| Cutting a release | `monorel tag` | `release.yml` (CI) | Read the merge commit's `monorel-Release:` trailers, create one annotated tag per released package. |
| Cutting a release | `monorel publish` | `release.yml` (CI) | Create one provider release per tag at HEAD; body sourced from each package's CHANGELOG entry. |
| Pre-release cycle | `monorel pre enter <channel>` | Maintainer (local) | Switch into pre-release mode for the named channel. |
| Pre-release cycle | `monorel pre exit` | Maintainer (local) | Leave pre-release mode; the next release is stable. |
| Local one-shot release | `monorel release` | Maintainer (local) | `monorel apply` + `monorel tag` in one process. Skips the always-open-PR pattern; useful for local releases without CI. |

Inside the `disaresta-org/monorel/ci/github` action, `command: pr` runs the `monorel apply` + `git push -f` + `monorel preview --upsert` sequence; `command: release` runs `monorel tag` + `git push --follow-tags` + `monorel publish`. The action wrapper exists so workflow files don't have to spell out each step.

## See also

- [Changesets](/changesets) for the `.changeset/<name>.md` file format.
- [GitHub Action](/integrations/github) for the workflow YAML and token setup.
- [FAQ](/faq) for the questions that come up after the first release.
