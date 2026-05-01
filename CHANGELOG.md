# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

From `v0.1.1` onward, this file is maintained automatically by monorel itself
via changesets in `.changeset/*.md`. The `v0.1.0` entry below is hand-written
as the one-time bootstrap.

## [0.3.0] - 2026-05-01

### Minor Changes

- **Breaking:** rename the user-visible "forge" terminology to "provider".

  Reasoning: "forge" is industry jargon (short for "software forge,"
  inherited from SourceForge-era naming) that's opaque to anyone who
  hasn't been exposed to it. monorel's existing config field was already
  called `provider`; the section header was inconsistent. Standardizing
  on "provider" everywhere user-visible removes the duplication and the
  "what is a forge?" lookup.

  ### `monorel.toml` migration

  ```diff
  -[forge]
  -provider = "github"
  +[provider]
  +name = "github"
   owner = "acme"
   repo  = "widget"
   host  = ""
  ```

  Section name: `[forge]` → `[provider]`. Inner field: `provider` →
  `name`. The `owner`, `repo`, and `host` fields are unchanged. v0.3
  rejects legacy `[forge]` configs via the existing unknown-keys parse
  error: existing configs surface as a clear "unknown keys: [forge ...]"
  on the next run. Pre-1.0 hard break, no migration helper.

  ### Public Go API migration

  | Before | After |
  |--------|-------|
  | `config.ForgeConfig` | `config.ProviderConfig` |
  | `config.Config.Forge` (field) | `config.Config.Provider` (field) |
  | `config.ForgeConfig.Provider` (field) | `config.ProviderConfig.Name` (field) |

  Constants were already named correctly (`config.ProviderGitHub`,
  `config.KnownProviders`, `config.ResolveProvider`, `config.IsKnownProvider`).

  ### Internal package rename

  `internal/forge/` is now `internal/provider/` (package name `provider`,
  type `provider.Client`). Consumers don't import this — it's
  internal-only — but anyone reading the codebase will see the rename.
  The `internal/forge/factory/` and `internal/forge/github/` subpackages
  moved correspondingly.

  ### Error messages

  | Before | After |
  |--------|-------|
  | `forge.owner is required` | `provider.owner is required` |
  | `forge.repo is required` | `provider.repo is required` |
  | `forge.provider %q is not recognized` | `provider.name %q is not recognized` |

  ### Docs

  Configuration, getting-started, design, api, github-action, docker,
  cli-reference, recipes, AGENTS.md, README.md, CONTRIBUTING.md, and
  the `.claude/rules/*.md` agent guides all swept. The recipes'
  `[forge]` example blocks now use `[provider]`.

## [0.2.1] - 2026-05-01

### Patch Changes

- Drop `monorel publish` from monorel's own release.yml.

  v0.2.0 surfaced a fatal interaction between `monorel publish` and
  goreleaser when both run in the same release pipeline:

  1. `monorel publish` creates the GitHub Release for the new tag,
     with a body sourced from the rendered CHANGELOG entry.
  2. `build-binaries` runs goreleaser via `workflow_call`. Goreleaser
     sees the existing release, attempts to PATCH it to attach the
     built binaries, and (in recent goreleaser versions hitting
     GitHub's new immutable-release feature) flips the release to
     `immutable: true` as a side effect of the PATCH.
  3. Subsequent uploads fail with `422 Cannot upload assets to an
     immutable release`.

  Once a tag has been used by an immutable release, GitHub permanently
  retains the tag name even if the release is deleted: no new release
  can be created with the same tag. v0.2.0 is therefore stuck without
  binaries; consumers should pin to v0.2.1 or later for the action
  wrapper.

  Fix: monorel's own release.yml lets goreleaser own release creation.
  The release job runs `monorel release` + `git push --follow-tags`
  only; goreleaser (via build-binaries) creates the release with its
  binary uploads in one step.

  Trade-off: monorel's own GitHub Release bodies are goreleaser's
  auto-generated commit list rather than the curated CHANGELOG entry.
  The per-package CHANGELOG.md still contains the curated entry; this
  only affects the body shown on the GitHub Release UI.

  This fix is monorel-self-hosting-specific. Other consumers
  (loglayer-go and similar) call the action wrapper, which still runs
  `monorel publish`. Those repos keep producing curated release bodies
  because they don't run goreleaser as part of their release flow.

## [0.2.0] - 2026-05-01

### Minor Changes

- Promote six pure-function packages from `internal/` to the top-level
  public API. From v0.2.0, external consumers can import:

  - `monorel.disaresta.com/config` — `monorel.toml` schema, `Config.Load`,
    `Config.Validate`, package iteration helpers.
  - `monorel.disaresta.com/changeset` — `.changeset/<name>.md` parse
    and write, frontmatter shape check, name generation.
  - `monorel.disaresta.com/plan` — pure-function planner: takes
    config + changesets + tags + pre-release state, returns the
    release plan.
  - `monorel.disaresta.com/semver` — bump-level abstraction (Major /
    Minor / Patch / None), version application, initial-release
    rules, pre-release suffixing.
  - `monorel.disaresta.com/validate` — fault-tolerant static checks
    against a monorel.toml + the changeset directory.
  - `monorel.disaresta.com/changelog` — Keep-a-Changelog renderer
    with non-destructive insertion above the existing version
    history.

  Each package now ships a runnable Example (visible on pkg.go.dev)
  covering the canonical entry point. Package-level GoDoc was
  tightened where it leaked monorel-internal context.

  The side-effect-bearing packages stay in `internal/` deliberately:
  `release` (writes files / commits / tags), `orchestrator` (forge-
  coupled), `forge` (provider-specific), `git` (shell-out), `cli`
  (Cobra wiring). These bake in monorel's opinions about side-effect
  ordering and should not be public commitments.

  This is a non-breaking move for callers within monorel: import
  paths updated from `monorel.disaresta.com/internal/<pkg>` to
  `monorel.disaresta.com/<pkg>`. External consumers (none yet)
  gain a stable API surface from v0.2.0 onward.
- Add `monorel validate` — a static-checks subcommand that walks
  `monorel.toml`, the changeset directory, and (opt-in) the local
  tag namespace, surfacing every issue in one pass.

  Unlike the existing commands' fail-fast loaders (`config.Load`
  returns the first violation; `changeset.LoadAll` bails on the
  first malformed file), `validate` is fault-tolerant by design:
  schema, filesystem, changeset, and tag findings are all
  collected and reported together so authors fix them in one
  round-trip.

  Checks:

  - **Schema**: forge fields, package fields, no duplicate tag
    prefixes. Delegates to existing `Config.Validate()`.
  - **Filesystem**: every package's `path` exists, no two packages
    share a path, every changelog's parent directory exists.
  - **Changesets**: every `.changeset/*.md` parses cleanly and
    only names packages declared in `monorel.toml`. Unknown
    package key is the most common authoring typo; surfaced as
    an error.
  - **Tags** (opt-in via `--check-tags`): every tag matching a
    package's prefix has a parseable semver version. Non-semver
    tags surface as warnings.

  Output: human-readable by default, `--json` for machine-readable
  (field shape is the public `Finding` type's encoding). Exit codes:
  `0` clean, `1` errors, `2` warnings only when `--strict`.

  Designed for three use sites: ad-hoc by maintainers after editing
  the config, pre-commit hook (e.g. `lefthook.yml: monorel validate
  --json`), and CI gates.

  Implementation lives in `internal/validate/` for now; promotion
  to a public package is queued as a follow-up alongside the
  broader library-API design.

## [0.1.2] - 2026-05-01

### Patch Changes

- Chain `build-release-binaries` and `build-image` into `release.yml`
  via `workflow_call` so they fire after every monorel-driven release.

  When `release.yml` pushes a tag using `secrets.GITHUB_TOKEN`, GitHub's
  anti-recursion rule suppresses the resulting `push: tags` event for
  other workflows. That meant downstream tag-triggered workflows (the
  binary builder and the container builder) silently didn't fire on
  real releases — only on the v0.1.0 bootstrap, which was cut via
  `workflow_dispatch` (a user-initiated run that doesn't trip the
  anti-recursion rule).

  Surfaced by monorel's own v0.1.1 release: the tag and GitHub Release
  appeared, but no binaries were attached and no image was pushed to
  GHCR. v0.1.1 is consequently usable only via `go install`, not via
  the action wrapper or `docker pull`.

  Mirrors the same `workflow_call` workaround `docs.yml` already uses
  (in loglayer-go's release.yml — monorel's docs.yml deploys on every
  push to main, so it doesn't need this).

  Both build workflows now accept a `tag` input via `workflow_call` and
  `workflow_dispatch`. The tag-push trigger is preserved so manual
  `git push <tag>` flows still work. `release.yml`'s release job
  captures the released root tag (`vX.Y.Z`) and passes it to both build
  workflows; the chain skips when no root tag was created (sub-module-
  only release; not yet possible for monorel itself, but forward-looking).

  v0.1.1's missing assets are not backfilled by this change; users on
  v0.1.1 should bump to v0.1.2.
- Deploy docs only on release, not on every push to main.

  Previously `docs.yml` deployed to GitHub Pages on every push to `main`,
  which meant any merge — even a typo fix in a Go file unrelated to
  docs — would re-deploy the site. Switch the deploy gate to match
  the loglayer-go pattern:

  - `pull_request` (paths-filtered to `docs/**`): build-only, for
    verification that the site still builds.
  - `release: published`: deploy on manually-created GitHub Releases.
  - `workflow_dispatch`: manual escape hatch.
  - `workflow_call`: invoked from `release.yml` after a monorel-driven
    release is cut. Sidesteps GitHub's anti-recursion rule (Releases
    created via `GITHUB_TOKEN` don't propagate `release: published`
    events to other workflows).

  `release.yml` chains `docs.yml` via `workflow_call` alongside the
  existing `build-binaries` and `build-image` chains, so every
  monorel-driven release deploys the docs that go with it. No
  deployment fires for non-release commits to main, even if they
  touch `docs/**`.

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
