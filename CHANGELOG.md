# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

From `v0.1.1` onward, this file is maintained automatically by monorel itself
via changesets in `.changeset/*.md`. The `v0.1.0` entry below is hand-written
as the one-time bootstrap.

## [0.1.1] - 2026-05-01

### Patch Changes

- Fix release-pr workflows by staging the head branch before opening
  the always-open release PR.

  GitHub's PR-create API rejects PRs whose head branch doesn't exist
  on the remote (`422 Validation Failed [Field:head Code:invalid]`).
  Both `ci/github/action.yml` and the self-hosted `release-pr.yml`
  now create the configured head branch (default `monorel/release`)
  as a fast-forward of the default branch plus one empty marker
  commit, then force-push, before invoking `monorel preview --upsert`.

  The branch's diff stays empty by design; the rendered plan goes in
  the PR body, not in a code diff. At merge time, the squash commit
  inherits the orchestrator-set PR title (`chore(release): ...`),
  which `release.yml` filters on to trigger the post-merge release
  pipeline. So the marker commit's own subject is irrelevant after
  squash and there is no two-`chore(release):`-commit churn on main.

  Surfaced by loglayer-go's first monorel-driven release attempt,
  where the smoke-test PR's `release-pr` workflow run failed with
  the 422 error on PR creation.

  Branch staging in the orchestrator (Go code) is still on the
  roadmap; this CI-wrapper fix unblocks consumers in the meantime
  and is the right layer per the `Run` doc-comment ("delegated to
  the CI wrapper because it's a thin shell-out to git").

## [0.1.0] - 2026-04-30

Initial release. monorel is a changesets-style release tool for multi-module
Go monorepos: per-PR intent files declare which packages release at what bump
level, instead of inferring releases from commit messages.

### Major Changes

- **Pure-function planner** (`internal/plan`). Takes the parsed
  `monorel.toml`, the pending changesets, the current git tags, and (optionally)
  pre-release state; returns the next-release plan. No I/O; exhaustive
  table-driven tests for the version-math matrix.
- **`monorel.toml` config** with per-package `tag_prefix`: bare `vX.Y.Z` for
  the root module, `<path>/vX.Y.Z` for sub-modules — the formats Go's
  toolchain expects.
- **CLI surface**: `add`, `status`, `plan`, `release`, `publish`, `preview`,
  `pre {enter|exit|status}`, `init`. The release pipeline splits local
  application (`release`), push (`git push --follow-tags`), and forge publish
  (`publish`) so tags exist on the remote before the forge validates them.
- **Pre-release mode**: `monorel pre enter <channel>` switches the repo into
  rc / beta / alpha mode; subsequent releases tag `vX.Y.Z-channel.N` and
  increment per-package counters. `pre exit` returns to stable.
- **Always-open release PR pattern** via the orchestrator. CI calls
  `monorel preview --upsert` on every push to the default branch; the release
  PR auto-creates, updates, or closes based on the plan state.
- **Provider-neutral forge seam** (`internal/forge`). GitHub today; GitLab /
  Gitea / Bitbucket / Forgejo plug in as new subpackages plus one factory
  case.
- **Keep-a-Changelog writer** with idempotent insertion above the first `## `
  heading. Existing release-please / changesets-bot content is preserved
  verbatim — migrations are forward-only, no rewrites of historical entries.
- **GitHub Action wrapper** at `disaresta-org/monorel/ci/github@v1`. Composite
  action; downloads the runner-matched binary and invokes monorel with the
  requested command.
- **Docker image** at `ghcr.io/disaresta-org/monorel:0.1.0` for `linux/amd64`
  + `linux/arm64`. macOS / Windows users can sidestep the unsigned-binary OS
  prompts by running monorel inside the container.
- **Vanity import path**: `go install monorel.disaresta.com/cmd/monorel@v0.1.0`.
  The docs site serves the `go-import` + `go-source` meta tags pointing at
  the GitHub repo.

### Documentation

Full VitePress documentation site at <https://monorel.disaresta.com>:
introduction, getting-started, configuration, CLI reference, changesets,
GitHub Action, design, bootstrap, Docker, plus recipes for migrating from
release-please and a worked example for `loglayer-go`.

[0.1.0]: https://github.com/disaresta-org/monorel/releases/tag/v0.1.0
