---
title: Glossary
description: "Canonical definitions of monorel's terminology: changeset, release PR, speculative apply, tag prefix, trailer block, and other terms used across the docs."
---

# Glossary

Terms used throughout the docs, defined once. Many overlap with general SemVer / Go-modules terminology; entries below note where monorel uses a term in a specific way.

## Always-open release PR

The single, persistent pull request the bot orchestrator maintains on `monorel/release` (the head branch). On every push to `main`, monorel either updates this PR's contents (if there are pending changesets), creates it (if it doesn't exist), or closes it (if there's nothing to release). Distinct from a feature PR: contributors don't commit to it directly; the bot owns the branch.

## Apply (speculative apply)

Two senses worth keeping straight:

- **`apply` the command**: `monorel apply` writes per-package CHANGELOG entries (or `pre.json` increments in pre-release mode), deletes the consumed `.changeset/*.md` files, and creates one `chore(release): ...` commit. The file-mutation half of the release flow.
- **The concept**: called "speculative" when `monorel apply` runs against the `monorel/release` staging branch in CI (the changes preview the next release without yet being merged); called "real" when it runs as part of `monorel release` locally.

## Bare-tag root

The convention of tagging the root module with bare version tags (`v1.0.0`, `v1.1.0`) instead of prefixed ones (`root/v1.0.0`). Required by `go install <module>@v1.2.3` and the canonical Go-modules layout. In monorel, set `tag_prefix = ""` (the empty string) on the root package's config block.

## Bump level

How much a release advances the version: `major` (breaking changes), `minor` (backward-compatible features), `patch` (backward-compatible fixes). When multiple changesets target the same package, the **strongest** bump wins for that release.

## Changelog (CHANGELOG.md)

The per-package markdown file documenting release history, in [Keep a Changelog](https://keepachangelog.com/) format. monorel writes new entries above the first existing `##` heading; existing content (including pre-monorel entries) is preserved verbatim.

## Changelog entry

A single rendered section in `CHANGELOG.md` for one release: `## [X.Y.Z] - YYYY-MM-DD` heading, plus `### Major Changes` / `### Minor Changes` / `### Patch Changes` subsections containing the bodies of every consumed changeset that targeted that package at that bump level.

## Changeset

Two senses: the **file** (`.changeset/<name>.md`) and the **system** (the changesets-style release model monorel implements).

The file: a markdown file with YAML frontmatter mapping package names to bump levels, plus a body that becomes the changelog entry. Authored on a feature branch; consumed at release time.

The system: a release model where each release-affecting PR includes an explicit intent file, in contrast to commit-message-based inference.

## Changeset frontmatter

The YAML block at the top of a `.changeset/<name>.md` file:

```yaml
---
"package-a": minor
"package-b": patch
---
```

Keys are package names from `monorel.toml`. Values are bump levels. Multi-package changesets use multiple keys. Empty frontmatter is rejected.

## monorel.toml

The single file at the repo root that declares which packages exist, their tag prefixes, paths, and changelog locations. The source of truth for everything except the per-PR release intent (which lives in `.changeset/*.md` files).

## Package

In monorel parlance: a unit of independent versioning, declared in `monorel.toml` as a `[packages."<name>"]` block. Often (but not always) corresponds 1:1 with a Go module. The root module is one package; each sub-module is another. Distinct from "Go package" (a directory with `.go` files).

## Plan / Planner

The pure-function step that takes (config, changesets, current tags) and produces the list of (package → next version) tuples. Implemented by `plan.Plan`. Called by `monorel plan` for inspection and internally by `apply` / `release` / `preview` to drive their work.

## pre.json

The state file (`.changeset/pre.json`) created by `monorel pre enter <channel>`. Tracks the active pre-release channel name and per-package counter. Removed by `monorel pre exit`. While present, releases append `-<channel>.N` to versions and don't consume changesets.

## Pre-release mode

The state when `.changeset/pre.json` exists. In this mode, `monorel release` produces suffixed versions (`v1.7.0-rc.0`, `-rc.1`, ...) and preserves changeset files across releases (they accumulate). The next stable release after `monorel pre exit` consumes everything and writes a single CHANGELOG entry per affected package.

## Provider

The version-control host (GitHub, GitLab, Gitea, Bitbucket, Forgejo). monorel's `internal/provider` interface abstracts the host-specific operations (find PR, create PR, create release, etc.). Selected by `provider.name` in `monorel.toml`. As of v0.4, only `github` is wired up; the seam exists for others.

## Release PR

The pull request the always-open release PR pattern produces. Its diff IS the file changes the next release will produce (CHANGELOG entries + changeset deletions, written by speculative apply). Its merge triggers `monorel tag` post-merge to create the per-package tags.

## SemVer / Semantic Versioning

[`MAJOR.MINOR.PATCH`](https://semver.org/), with optional `-<channel>.N` pre-release suffix. monorel applies SemVer 2.0.0 rules: any breaking change bumps MAJOR, additive changes bump MINOR, backward-compatible fixes bump PATCH. Pre-release versions (`-rc.0`, `-beta.1`) sort before the corresponding stable version.

## Speculative apply

See [Apply](#apply-speculative-apply).

## Tag

Two senses worth keeping straight:

- **`tag` the command**: `monorel tag` reads HEAD's commit-body trailers and creates per-package annotated git tags. The post-merge step of the release flow.
- **A git tag**: the annotated reference monorel creates, named per the package's tag prefix and version (e.g. `transports/foo/v1.7.0` or bare `v1.7.0`).

## Tag prefix

The string prepended (with `/`) to the version when forming a tag. Configured per package in `monorel.toml` as `tag_prefix`. Empty string for the root module's bare tags; the package's relative path for sub-modules. Two packages can't share a tag prefix (validator rejects).

## Trailer / Trailer block

The machine-readable `Key: Value` block at the bottom of a release commit's body. Written by `monorel apply`, read by `monorel tag`. The contract bridges the two phases: `apply` declares what was staged; `tag` derives which tags to create.

```
chore(release): foo v1.7.0

monorel-Release: foo v1.7.0
monorel-PreRelease: false
```

Multi-package commits emit one `monorel-Release:` per package in plan order. The squash-merge setting must preserve the commit body for `monorel tag` to find these.

## Validate

The static-check pass `monorel validate` runs against `monorel.toml` and `.changeset/*.md`. Surfaces every issue in one pass (loadable config, declared paths exist, changeset frontmatter parses, no shared tag prefixes, no unknown TOML keys). Uses no network, no git mutation; safe to run in a pre-commit hook.
