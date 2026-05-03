---
title: Design
description: "monorel's design principles and trade-offs: changeset files as truth, native Go tag conventions, always-open PR pattern, provider-neutral seam."
---

# Design

This page captures monorel's design choices and why they were made. Most are reactions to specific failures observed on `loglayer-go` during the release-please era.

## Changeset files are the source of truth

Every release-affecting PR includes a `.changeset/<name>.md` file. The file names the affected packages, the bump level for each, and carries the changelog body. monorel reads pending changesets at release time and computes the version transition; commit messages are not parsed.

**Why:** The class of "the release tool got confused" failures collapses by construction. A few we hit on `loglayer-go`:

- **Squash-merge stripping `Release-As:` footers.** GitHub squash uses the PR title as the subject and the PR body as the body. Per-commit footers don't survive.
- **Full-history scan leaks.** A new sub-module's manifest entry being `0.0.0` (never released) caused release-please to scan the entire repo history for `Release-As:` footers; a footer from an unrelated commit then leaked into the new package's initial version.
- **Path attribution.** PRs that touched only `docs/` could still bump the main module if `exclude-paths` didn't cover the directory.

With explicit changeset files, none of these are reachable. The author writes a file naming exactly which packages they're claiming to affect.

## Native Go tag conventions

The main module at the repo root uses bare `vX.Y.Z` tags. Sub-modules use `<path>/vX.Y.Z`. monorel produces both in the same repo via per-package `tag_prefix` (empty string for the root, the path for sub-modules).

**Why:** It's what `go install <module>@vX.Y.Z` expects. Knope made per-package prefixes mandatory and we couldn't get bare tags for the root; that's a Go-specific requirement Knope wasn't built for.

## Always-open release PR (speculative apply)

The bot orchestrator stages a `monorel/release` branch off the default branch and runs `monorel apply` on it. `apply` writes per-package CHANGELOG entries (or `pre.json` increments in pre-release mode), deletes the consumed `.changeset/*.md` files, and creates one `chore(release): ...` commit with `monorel-Release:` trailers in the body. The orchestrator force-pushes the result. The release PR's diff is the actual file changes the release will produce, not a body summary. Merging the PR runs `monorel tag` on the merge commit, which reads the trailers and creates per-package tags.

**Why:** Release PRs are reviewable, mergeable, and visible. A maintainer can see exactly what's about to ship before approving, including the rendered CHANGELOG diff. The pattern was popularized by release-please and changesets-bot; monorel reuses it because there's nothing better for the public-facing release-cadence-control problem.

**Why two phases (`apply` / `tag`):** Tags can't be created on the staging branch without leaving them behind if the PR is closed without merging. Splitting file mutations from tag creation lets the staging step be repeatable (force-push the staging branch as many times as the planner output changes) while tags only ever land at merge time, against the merge commit. The trailer block in the commit body is the contract between the two phases: `monorel apply` writes it; `monorel tag` reads it.

## Pure-function planner

[`plan.Plan(cfg, changesets, tags, pre)`](https://pkg.go.dev/monorel.disaresta.com/plan#Plan) is a pure function: no I/O, no side effects, deterministic output for identical inputs. The CLI, the orchestrator, and the release applier all consume its output; none of them re-derive versions from changesets independently. The package is part of monorel's public API surface so external consumers (custom orchestrators, IDE plugins, audit tools) can call it directly.

**Why:** Exhaustive table-driven tests are cheap. The planner is the load-bearing logic of the tool, and "pure function over plain data" is the cheapest way to give it the test coverage that matches its blast radius.

## Provider-neutral host seam

`internal/provider.Client` is a seven-method interface (`GetDefaultBranch`, `FindOpenReleasePR`, `FindPRByMergeCommit`, `CreatePR`, `UpdatePR`, `ClosePR`, `CreateRelease`). The factory dispatches by `config.ProviderConfig.Name` to a per-provider subpackage. GitHub, GitLab, Gitea / Forgejo, and Bitbucket Cloud are all built in; add a subpackage to support a host that isn't covered.

**Why:** The interface is genuinely the slice of host operations monorel uses. Naming the abstraction `provider` (rather than `github`) is a small upfront cost that pays off the first time someone wants GitLab.

Adding a new provider requires:

- One new subpackage under `internal/provider/<name>/` implementing `provider.Client`.
- One case in `internal/provider/factory/factory.go`.
- One entry in `config.KnownProviders`.
- One entry in `provider.TokenEnvVars` (if the provider uses an env-var token).

The recipe is documented at the top of `factory.go`.

::: tip CI wrappers are NOT abstracted in Go
The orchestration layer is provider-neutral, but the **CI wrapper** (the `action.yml` for GitHub, the `.gitlab-ci.yml` for GitLab, etc.) is per-CI-system YAML. There's no shared schema across providers' CI systems; each wrapper is a thin shim that downloads the monorel binary and runs it. They live under `ci/<provider>/` at the repo root.
:::

## Hard cut to Keep-a-Changelog

When monorel migrates a repo from another tool (release-please, changesets-bot), it preserves the existing `CHANGELOG.md` content verbatim and inserts new entries above the first `## ` heading. New entries are Keep-a-Changelog format; old entries stay in whatever format they were written in.

**Why:** Rewriting historical entries is risky (tools differ in subtle markdown shapes, and the historical record loses fidelity). Forward-only formatting gets us the new format on every new release without disturbing the old.

## No linked releases

Each package versions independently. Two packages with one changeset bumping both get separate version transitions and separate tags; there's no "they ship together" mode.

**Why:** Linked releases sound nice in concept and create coupling problems in practice. monorel was specified explicitly without them per direction during planning. If the demand emerges, it can be added without breaking single-package configs.

## No commit-message fallback

monorel will not read commit messages to infer bump levels. A PR without a changeset doesn't trigger a release.

**Why:** Mixing two signal sources (changesets + commit messages) re-introduces the inference failures changesets were designed to avoid. If a PR doesn't need a release, it doesn't need a changeset.

## Single static binary

monorel is a single Go binary with no runtime dependencies. The CLI is the same binary that runs in CI.

**Why:** Easier to install, easier to debug. No daemon, no GitHub App, no hosted state.

## Self-hosted from day one

monorel releases itself with monorel. The `monorel.toml` in the monorel repo declares a single package at the root with `tag_prefix = ""`.

**Why:** Dogfooding is the cheapest way to surface UX bugs. Anything that pinches in monorel's own release cadence is going to pinch every user.

## Out of scope

- **Polyglot support.** Go-only. The semver math, tag conventions, and changelog format aren't language-neutral, and pretending they are creates the same edge cases changesets-the-JS-library has when targeting non-JS projects.
- **GitHub App / Bot.** The Action wrapper is enough; a hosted bot adds an authority-to-act problem (which repos can it write to? which PRs can it merge?) that the per-repo Action sidesteps.
- **Cross-repo coordination.** Single-repo only. If your release coordinates across multiple repos, you're outside the use case monorel solves.
- **Linked releases.** Each package versions independently.
