---
title: CLI Reference
description: "Per-command reference for monorel: add, plan, status, validate, apply, tag, release, preview, publish, pre, init, doctor."
---

# CLI Reference

monorel ships as a single binary with the following subcommands. All commands take a `--config <path>` flag (default `monorel.toml`).

## Global Flags

These persistent flags apply to every subcommand:

| Flag | Type | Description |
|------|------|-------------|
| `--config <path>` | string | Path to `monorel.toml`. Default `monorel.toml`. The repository root is its parent directory. |
| `--color auto\|always\|never` | string | ANSI color in CLI output. `auto` (default) detects whether stdout is a TTY. `always` keeps color when piping into a paginator that handles ANSI; `never` disables it for log files and `grep` consumers. |
| `-v`, `-vv` | count | Increase verbosity. `-v` adds debug-level messages; `-vv` also appends structured fields to each line. Default level shows info, warn, and error. |
| `-q`, `--quiet` | bool | Suppress info and warn chatter (progress hints, "nothing to do" messages, etc.), leaving only error and fatal. Each command's primary result still prints: `validate` and `doctor` print their findings, the release-pipeline commands (`apply`, `release`, `tag`, `publish`) print their headline output for the GitHub Action wrapper to grep, and so on. |

## `monorel add`

Create a new changeset describing pending package changes.

```sh
# Non-interactive
monorel add \
  --package "transports/zerolog:minor" \
  --package "github.com/acme/widget:patch" \
  --message "Adds Lazy() helper; pass-through fix in the root."

# Interactive (prompts for packages, levels, body)
monorel add
```

| Flag | Type | Description |
|------|------|-------------|
| `-p, --package <name>:<level>` | string (repeatable) | Package and bump level. Triggers non-interactive mode when given. |
| `-m, --message <text>` | string | Changeset body (the changelog entry). May be empty. |
| `--name <name>` | string | Override the auto-generated changeset filename (default: random `<adj>-<noun>`). Lowercase letters, digits, hyphens only. |

Validates: package exists in `monorel.toml`, bump level is `major|minor|patch`, no duplicate package across `--package` flags.

Writes to `.changeset/<name>.md`.

## `monorel plan`

Compute the proposed releases without applying them. Reads `.changeset/*.md`, the configured packages, and the latest matching git tag per package.

```sh
monorel plan
# PACKAGE             FROM     BUMP   TO       TAG                          CHANGESETS
# transports/zerolog  v1.6.1   minor  v1.7.0   transports/zerolog/v1.7.0    quick-otter
#
# 1 package(s) to release; 1 changeset(s) consumed.

monorel plan --json
# {"releases": [...], "consumed": [...]}
```

| Flag | Type | Description |
|------|------|-------------|
| `--json` | bool | Emit the plan as JSON. Stable schema across versions. |

## `monorel status`

Print pending changesets and which packages they affect. Doesn't compute bumps.

```sh
monorel status
# CHANGESET    PACKAGE             BUMP    SUMMARY
# quick-otter  transports/zerolog  minor   Adds Lazy() helper.
#
# 1 changeset(s) pending.
```

## `monorel validate`

Run static checks against `monorel.toml` + `.changeset/*.md` and report findings. Doesn't mutate anything; doesn't compute the release plan; doesn't make network calls. Surfaces every issue in one pass instead of bailing on the first.

```sh
monorel validate
# No findings. monorel.toml + .changeset/*.md look valid.

monorel validate --json
# {"findings": [], "errors": 0, "warnings": 0}

# Example with errors:
monorel validate
#
# ERRORS:
#   - path_missing: packages."transports/foo".path "transports/foo" does not exist [transports/foo]
#   - changeset_unknown_package: changeset references package "widgett" which is not declared in monorel.toml [.changeset/typo.md]
#
# 2 error(s), 0 warning(s).
# (exits 1)
```

| Flag | Type | Description |
|------|------|-------------|
| `--json` | bool | Emit findings as JSON. Stable schema across versions. |
| `--strict` | bool | Treat warnings as failures (exit 2 instead of 0). |
| `--check-tags` | bool | Also walk the local tag namespace and warn on tags whose version part isn't valid semver. Requires a git repo at the config's parent directory. |

### What it checks

- **Schema**: provider fields, package fields, no two packages share a `tag_prefix`. Same checks `monorel plan` runs lazily; this command runs them eagerly and aggregates.
- **Filesystem**: every package's `path` exists relative to the config and is a directory; no two packages share a `path`; every `changelog`'s parent directory exists (the file itself can be absent — first release creates it).
- **Changesets**: every `.changeset/*.md` parses cleanly (frontmatter shape, body present, recognized bump levels) and only names packages declared in `monorel.toml`. An unknown package key is the most common authoring typo and is surfaced as an error.
- **Tags** (opt-in via `--check-tags`): for each package, every tag matching its prefix has a parseable semver version. Non-semver tags surface as warnings; the planner already ignores them, but `validate` lets operators clean up tag noise.

### Exit codes

| Code | Meaning |
|------|---------|
| 0 | No findings, or warnings without `--strict`. |
| 1 | One or more errors. |
| 2 | Warnings only AND `--strict` was passed. |

### Use as a pre-commit hook

```yaml
# .lefthook.yml or .pre-commit-config.yaml equivalent
pre-commit:
  commands:
    monorel-validate:
      run: monorel validate --json
```

`--json` makes the output machine-readable for IDE integrations and CI parsers; the field shape is the public `Finding` type's encoding.

## `monorel apply`

Apply pending changesets to the working tree: write CHANGELOG entries, delete the consumed `.changeset/*.md` files, and create one `chore(release): ...` commit. Does NOT create tags.

```sh
monorel apply
# Applied 1 release(s) at a4f77ab:
#   staged: transports/zerolog/v1.7.0
# Run `monorel tag` (typically post-merge) to create tags from this commit's trailers.
```

The commit body carries machine-readable trailers (`monorel-Release: <name> <version>`, `monorel-PreRelease: <bool>`) so a later `monorel tag` can derive the per-package tags without re-reading changesets:

```
chore(release): transports/zerolog v1.7.0

monorel-Release: transports/zerolog v1.7.0
monorel-PreRelease: false
```

`apply` is the speculative-apply primitive used by the GitHub Action wrapper's `pr` command. See [GitHub Action](/integrations/github) for how it's wired.

In pre-release mode (`.changeset/pre.json` present), `apply` increments per-package counters in `pre.json` instead of writing CHANGELOGs and keeps the `.changeset/*.md` files. Tags carry the channel suffix (e.g. `v1.7.0-rc.0`).

## `monorel tag`

Read HEAD's commit trailers and create the corresponding annotated git tags at HEAD. Does NOT mutate files. Used post-merge by the GitHub Action wrapper's `release` command.

```sh
monorel tag
# Tagged 1 release(s) at a4f77ab:
#   transports/zerolog/v1.7.0
# Run `git push --follow-tags && monorel publish` to publish.
```

Errors:

- `ErrNoReleaseCommit`: HEAD has no `monorel-Release:` trailers. The release-pipeline workflow filters on the `chore(release):` subject, so this only fires if the filter is misconfigured.
- `ErrUnknownReleasedPackage`: a trailer names a package not declared in `monorel.toml`. The config drifted between `apply` and `tag`.
- `ErrTagExists`: a derived tag is already present (preflight check). Investigate (probably a partial prior run) and delete the stale tag before re-running.

::: warning Partial-tag failure mode
If `tag` fails on the Nth tag of a multi-package release, tags 1..N-1 exist and N..end don't. A naive re-run hits `ErrTagExists` on tag #1 and aborts. Recovery: `git tag -d <stale-tag>` for each partial tag, then re-run.
:::

## `monorel release`

One-shot local release: `apply` + `tag` in sequence with a single preflight pass. Equivalent to `monorel apply && monorel tag` for trees you don't intend to review through a PR. Idempotency: aborts if any planned tag already exists.

```sh
monorel release
# Released 1 package(s) at a4f77ab:
#   transports/zerolog/v1.7.0
# Run `git push --follow-tags && monorel publish` to publish.
```

The split between `release` (local) and `apply` + `tag` (CI) is explained in [Always-open release PR](/design#always-open-release-pr-speculative-apply).

::: warning Push is the caller's job
The applier creates the commit and tags locally. Run `git push --follow-tags` (or use the GitHub Action) to publish.
:::

### Pre-release mode

When `.changeset/pre.json` exists (i.e. after `monorel pre enter`), `release` behaves differently:

- CHANGELOG entries are NOT written.
- `.changeset/*.md` files are NOT deleted.
- Per-package counters in `pre.json` are incremented.
- Tags carry the suffix (e.g. `transports/zerolog/v1.7.0-rc.0`).

Accumulated changes are emitted to CHANGELOGs at the next stable release after `pre exit`.

## `monorel preview`

Render the release plan as markdown and (with `--upsert`) open or update the always-open release PR's body. Used by the GitHub Action wrapper's `pr` command after `monorel apply` stages the file changes.

```sh
monorel preview
# Markdown plan rendered to stdout.

monorel preview --upsert
# Created release PR #42: https://github.com/acme/widget/pull/42
```

| Flag | Type | Description |
|------|------|-------------|
| `--upsert` | bool | Open or update the always-open release PR. Requires `pull-requests: write` and a configured provider token. Closes any open release PR if the plan is empty. |

## `monorel publish`

Read tags pointing at HEAD, match each to a configured package, and create a provider release using the matching CHANGELOG entry as the release notes. Pre-release tags are flagged as such on the provider.

```sh
monorel publish
# Published 1 release(s):
#   transports/zerolog/v1.7.0
```

Splitting `publish` from `tag` (and from `release`) ensures the tag is on the remote before the provider validates it when creating the Release. The post-merge release pipeline is:

```sh
monorel tag           # create tags from HEAD's release-commit trailers
git push --follow-tags
monorel publish       # create one provider release per tag
```

Requires the configured provider's auth token in the environment (e.g. `GITHUB_TOKEN` for GitHub). The error message names the expected env var.

If `publish` fails partway through, it reports `Created N/M releases before failing.` and re-running picks up the remainder (each `CreateRelease` is idempotent on the tag name; the provider returns an error on duplicates, which the partial-success path handles).

## `monorel pre`

Manage pre-release mode (rc / beta / alpha). Pre-release mode causes `release` to append a `-<channel>.N` suffix to next versions and increment N per release.

### `monorel pre enter <channel>`

Start pre-release mode with the given channel name (`rc`, `beta`, `alpha`, or any non-empty SemVer-2.0-compatible identifier).

```sh
monorel pre enter rc
# entered pre-release mode (channel "rc"). Subsequent releases will be tagged vX.Y.Z-rc.N.
```

Errors if pre-release mode is already active. To switch channels, run `pre exit` first.

### `monorel pre exit`

Return to stable releases. Removes `.changeset/pre.json`. Idempotent.

```sh
monorel pre exit
# exited pre-release mode (was channel "rc"). Next release is a stable version.
```

The next stable release does NOT re-bump from the pre version; it bumps from the last stable tag, applying every changeset accumulated since.

### `monorel pre status`

Print the current pre-release state, if any.

```sh
monorel pre status
# pre-release mode: channel="rc"
#   transports/zerolog: counter=2
```

## `monorel init`

Scaffold a `monorel.toml` and `.changeset/` directory for a fresh repo. Walks every `go.mod` under the working directory (skipping `vendor/`, `node_modules/`, and hidden directories) and writes one `[packages]` block per Go module. Infers `provider`, `owner`, and `repo` from the git origin remote.

```sh
monorel init
# Wrote monorel.toml with 2 package(s):
#   github.com/acme/widget (path: ., tag prefix: "")
#   sub/foo (path: sub/foo, tag prefix: "sub/foo")
# Created .changeset/ with a README.
# Next steps:
#   monorel validate     # confirm the config
#   monorel add          # write your first changeset
```

| Flag | Type | Description |
|------|------|-------------|
| `--provider <name>` | string | Version-control host. Default `github`. Used as the `provider.name` value. |
| `--owner <name>` | string | Repo owner. Auto-detected from `git config remote.origin.url` if empty. |
| `--repo <name>` | string | Repo name. Auto-detected from `git config remote.origin.url` if empty. |
| `--force` | bool | Overwrite an existing `monorel.toml` (otherwise the command refuses). |

Refuses to run without at least one `go.mod`. Existing `.changeset/README.md` is preserved; only created when missing.

## `monorel doctor`

Diagnose repository state issues monorel's planner won't catch on its own. Today the only built-in check is `revived-changeset`: a `.changeset/*.md` file currently on disk that a previous `chore(release):` commit deleted, indicating a stale-branch + squash-merge revival. The next release would re-ship the same content under a new version.

```sh
monorel doctor
# No findings. Repository state looks healthy.

monorel doctor --json
# { "findings": [...], "errors": 0, "warnings": 0 }
```

When something's wrong:

```sh
monorel doctor
# ERRORS:
#   - revived-changeset: .changeset/foo.md was deleted by a previous
#     chore(release) commit but is back on disk; likely cause:
#     stale-branch + squash-merge revived it. Delete the file and
#     the next release plan will re-evaluate. [.changeset/foo.md]
#
# 1 error(s), 0 warning(s).
echo $?
# 1
```

| Flag | Type | Description |
|------|------|-------------|
| `--json` | bool | Emit findings as JSON. Schema: `{ findings: [{severity, check_name, path, message}], errors, warnings }`. |

Exit codes:

- `0`: no findings.
- `1`: one or more error-severity findings.

Mechanically, doctor walks `git log --diff-filter=D --grep='chore(release):'` to build the set of changeset filenames previous releases consumed, then intersects with the live `.changeset/` directory. The check costs one `git log` invocation and is cheap to run on every PR.

::: tip Wire it into CI
The check is most useful as a pre-merge gate: CI checks out fresh against the actual base, so the git-log scan always reflects current main. Each integration page documents the workflow file:

- [GitHub](/integrations/github#doctor-yml-pre-merge-sanity-check-recommended)
- [Gitea / Forgejo](/integrations/gitea) (under Workflows)
- [GitLab](/integrations/gitlab#workflows) (the `doctor` stage in the canonical `.gitlab-ci.yml`)

Local pre-commit / pre-push hooks are NOT a good fit: the bug class doctor catches arises from GitHub's squash-merge taking a stale branch tree, which the local branch state can't observe directly. CI on PRs is the right gate.
:::

The same logic is exposed as a Go library at [`monorel.disaresta.com/doctor`](/api#doctor) for callers who want to embed the check in custom CI without shelling out.

## Persistent flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--config <path>` | string | `monorel.toml` | Path to the config file. The repo root is its parent directory. |

## Exit codes

- `0`: success.
- non-zero: an error occurred. The CLI prints the error to stderr; commands that do partial work (e.g. `release --publish` failing on the second provider release) print a "created N/M" line before the error.
