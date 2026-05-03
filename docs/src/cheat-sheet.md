---
title: Cheat Sheet
description: "One-page reference: the command map across the release lifecycle, common one-liners, and the files monorel reads and writes."
---

# Cheat Sheet

One-page reference. For prose explanations of each command's flags and behavior, see the [CLI reference](/cli-reference); for the lifecycle diagrams, see [Workflows](/workflows).

## Command map

Every monorel subcommand, indexed by lifecycle phase + runner + purpose.

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
| Diagnostic | `monorel plan` | Anyone (local) | Print the release plan that would be applied right now. Pure read; no mutation. |
| Diagnostic | `monorel status` | Anyone (local) | Summarize pending changesets. |

The `disaresta-org/monorel/ci/github` action groups these:

- `command: pr` runs `monorel apply` → `git push -f` → `monorel preview --upsert`.
- `command: release` runs `monorel tag` → `git push --follow-tags` → `monorel publish`.

## Common one-liners

Daily authoring:

```sh
# Interactive: pick packages, levels, write body in-place
monorel add

# Editor-driven body (multi-paragraph markdown)
monorel add --editor

# Fully scripted (CI generators, automated bumps)
monorel add --package "transports/foo:minor" --message "Added X."
```

Local inspection:

```sh
monorel plan                # what would the next release ship?
monorel status              # what changesets are pending?
monorel validate            # is the config still loadable?
monorel doctor              # diagnose stale-branch / revival issues
```

Local one-shot release (no CI):

```sh
monorel release             # apply + tag in one process
git push --follow-tags
monorel publish             # create provider releases at the new tags
```

Pre-release window:

```sh
monorel pre enter rc        # next releases append -rc.N
# ...feature PRs and releases happen as usual...
monorel pre exit            # the following release is stable
```

## Files monorel reads and writes

| Path | Read by | Written by | Purpose |
|---|---|---|---|
| `monorel.toml` | every command | `monorel init` | Per-package config (path, tag prefix, changelog file). |
| `.changeset/<name>.md` | `apply`, `release`, `plan`, `status`, `preview` | `monorel add` (and removed by `monorel apply` after a stable release). | Pending release intent: which packages, what bump level, the changelog body. |
| `.changeset/pre.json` | every command | `monorel pre enter` (created), `monorel apply` (counter increments), `monorel pre exit` (removed). | Pre-release-mode state. |
| `<package>/CHANGELOG.md` | `monorel apply` (read-and-prepend) | `monorel apply` | Per-package changelog. New entry inserted above the latest existing entry; older entries preserved verbatim. |
| `<package>/go.mod` | `monorel apply` | `monorel apply` | Sub-modules: dev `replace` directives stripped, sibling requires pinned to the planned version. |
| `<package>/go.sum` | `monorel apply` | `monorel apply` | Sub-modules: tidied via offline `go mod tidy` so the release commit is canonically clean. |

## Env vars

| Var | Read by | Effect |
|---|---|---|
| `GITHUB_TOKEN` / `GH_TOKEN` | `preview`, `publish` | Auth for the GitHub provider. |
| `GITEA_TOKEN` | `preview`, `publish` | Auth for the Gitea / Forgejo provider. |
| `GITLAB_TOKEN` | `preview`, `publish` | Auth for the GitLab provider. |
| `VISUAL` / `EDITOR` | `monorel add --editor` | Editor to launch for the body. Falls back to `vi` / `nano` (Unix) or `notepad` (Windows). |
| `GOMODCACHE`, `GOCACHE` | `monorel apply` | Inherited by the offline `go mod tidy` invocation; pre-populated by your normal dev / `go test ./...` runs. |
