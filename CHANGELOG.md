# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

From `v0.1.1` onward, this file is maintained automatically by monorel itself
via changesets in `.changeset/*.md`. The `v0.1.0` entry below is hand-written
as the one-time bootstrap.

## [1.0.0] - 2026-05-04

### Major Changes

- Initial release.

## [0.15.1] - 2026-05-04

### Patch Changes

- **Fix `examples/gitlab/.gitlab-ci.yml` failing on the `docker+machine` executor.**

  The published `ghcr.io/disaresta-org/monorel` image is an entrypoint-binary container (its entrypoint is `monorel` itself). GitLab's `docker+machine` executor wraps every script as `sh -c '...'` and passes that to the container's entrypoint, producing `monorel sh -c '...'` and failing with `unknown command "sh"`.

  The example pipeline + the partial it includes from now use the long-form `image:` block with `entrypoint: [""]` to clear the container's entrypoint so the runner's shell wrapper takes over. Surfaced by an end-to-end test against a real GitLab runner.

  Also documents an outstanding known issue in `docs/src/integrations/gitlab.md`: multi-module Go monorepos with sibling `require`s hit a separate offline-tidy bug on cold-cache CI runners. Tracked separately from this fix; single-module repos and multi-module repos without in-plan sibling requires are unaffected. A new build-tag-gated test (`tests/e2e/tidy_isolated_test.go` under `e2e_tidy`) reproduces the bug deterministically using a `golang:1.26-alpine` testcontainer for use during the eventual fix.

## [0.15.0] - 2026-05-04

### Minor Changes

- **Provider-API release detection.**

  Two new monorel subcommands replace the previous text-pattern release-detection:

  - `monorel detect-release` reports whether HEAD is the merge of monorel's release PR. Exit 0 yes, 1 no, 2 error.
  - `monorel auto` is the one-stop CI command. It detects, then runs the release pipeline (tag + push + publish) or the feature pipeline (apply + push + preview --upsert) accordingly.

  The action wrapper at `disaresta-org/monorel/ci/github` simplifies to a single auto step. The `command: pr`, `command: release`, and `command: doctor` inputs are removed. Each provider's example workflow / pipeline file collapses to one file with one step that runs `monorel auto`. The `monorel doctor` workflows install monorel directly and run the command as a standalone step.

  Detection uses two signals OR'd together: the `monorel-Release:` trailer in HEAD's commit body (fast path; squash + rebase) and the provider's `FindPRByMergeCommit` returning a PR whose source branch is `monorel/release` (network signal; covers merge-commit and Bitbucket squash). Either signal alone is sufficient.

  Migration from v0.14:

  - Replace `command: pr` and `command: release` workflow steps with a single step (no `command:` input) that runs the action wrapper. The wrapper runs `monorel auto` internally.
  - `command: doctor` users invoke `monorel doctor` as their own step (install monorel via `go install monorel.disaresta.com/cmd/monorel@latest` first).
  - Custom CI scripts that text-grep `chore(release):` or `monorel-Release:` from commit messages should switch to running `monorel detect-release` and branching on its exit code.

### Patch Changes

- **Fix `detect-release` false-positive on prose mentions of the trailer marker.**

  `detect.IsReleaseMerge`'s trailer fast path used `strings.Contains(headBody, "monorel-Release:")`, which matched anywhere in the body, including prose mentions of the marker (e.g., docs commits that explain how the trailer works, or squash-merge bodies that aggregate sub-commit messages discussing release tooling).

  The CI symptom was a contradiction: `monorel detect-release` reported "release commit detected (source: trailer)" on a non-release commit, then the next pipeline step `monorel tag` correctly rejected HEAD with `ErrNoReleaseCommit`. The release workflow exited non-zero on every push to main whose squash body coincidentally contained the literal text `monorel-Release:`.

  The fix line-anchors the match to mirror the canonical parser at `release.parseReleaseTrailers`: the marker must appear at the start of a (whitespace-trimmed) line. Detect and tag now agree on what counts as a real trailer.
- **Fix `release.yml` 403 on every non-release push to main.**

  When PR #64 consolidated `release-pr.yml` into `release.yml`, the
  `pull-requests: write` permission was lost. The release path (tag +
  push) only needs `contents: write`, but the feature path (`monorel
  auto` against the always-open release PR) needs to PATCH the PR via
  GitHub's REST API, which requires `pull-requests: write`.

  Symptom: every push to `main` whose HEAD is NOT a release commit
  fails the `release` workflow with `403 Resource not accessible by
  integration` from `PATCH /repos/.../pulls/<n>`.

  This is a self-host-only fix; the example workflows and docs partials
  already document the correct two-permission shape.

## [0.14.0] - 2026-05-03

### Minor Changes

- monorel now supports Bitbucket Cloud (`provider.name = "bitbucket"`) alongside GitHub, Gitea / Forgejo, and GitLab. The `internal/provider/bitbucket/` package implements the `provider.Client` interface against Bitbucket's REST API v2 (hand-rolled `net/http`; no new direct deps).

  Auth uses two environment variables: `BITBUCKET_EMAIL` (Atlassian account email) and `BITBUCKET_TOKEN` (Atlassian API token with Bitbucket scopes). The Bitbucket username for git over HTTPS is probed from `/2.0/user` and cached on the client.

  Bitbucket Cloud has no first-class Release concept, so `monorel publish` is a no-op on Bitbucket; per-package `CHANGELOG.md` is the canonical release-notes source.

  Plus a defensive recovery mechanism that benefits every provider: `monorel preview` now appends a `<!-- monorel-trailers ... -->` HTML comment to the PR body. `monorel tag` falls back to that block when the merge commit body lacks `monorel-Release:` trailers (e.g. because of a squash-merge that rewrote the body). The fallback uses the new `provider.Client.FindPRByMergeCommit` method, implemented by every provider.

  See [Bitbucket integration](/integrations/bitbucket).
- Fix `cacheseed` writing the wrong h1: hash for released sub-modules
  (would silently produce broken go.sum entries on every release; see
  [`loglayer/loglayer-go`'s v2.1.0 incident](https://github.com/loglayer/loglayer-go/pull/76)).
  Reorder `applyStable` so all working-tree mutations happen before
  the seed step, and replace the single-pass seed-and-tidy with
  iterate-to-fixpoint to handle cross-sibling dep chains.

  Add a `go mod download` priming step before offline tidy so fresh
  CI runners (with empty `GOMODCACHE`) can resolve third-party deps.
  The `GOPROXY=off` invariant during tidy is preserved.

  Document the `actions/setup-go` prerequisite (sub-modules with
  `go 1.25.0` directives need a 1.25+ runner since `GOTOOLCHAIN=local`
  during tidy blocks auto-download) and the `chore(release):`-commit
  skip filter recipe. See [issue #54](https://github.com/disaresta-org/monorel/issues/54).

## [0.13.0] - 2026-05-03

### Minor Changes

- `.changeset/pre.json` now carries a `schemaVersion` field (currently `1`). Files written by older monorel builds omit the field and load as version 1, so existing pre-release windows are unaffected. A file whose `schemaVersion` is higher than the current build supports is rejected with a clear `"upgrade monorel"` error rather than being silently misread.

  The on-disk shape is otherwise unchanged. Library callers that construct `*changeset.PreState` directly don't need to set the field; `PreState.Write` stamps `schemaVersion: 1` automatically when the field is zero.

  Constants exposed: `changeset.PreStateCurrentSchemaVersion`. Bump it (and add a migration in `LoadPreState`) the next time the on-disk shape changes incompatibly.

  Pre-v1.0 housekeeping: future-proofs the pre-release-state file format ahead of the v1.0 stability commitment.

## [0.12.0] - 2026-05-03

### Minor Changes

- `monorel add` now accepts `-e` / `--editor` to write the changelog body in `$EDITOR` (or `$VISUAL`) instead of the in-place text-area prompt. Mirrors `git commit`'s editor flow: the temp file is pre-seeded with a commented prompt block; lines beginning with `#` are stripped on save; surrounding whitespace is trimmed.

  Editor resolution order: `$VISUAL`, then `$EDITOR`, then `vi` / `nano` (Unix) or `notepad` (Windows) when neither env var is set. `--editor` and `--message` are mutually exclusive (passing both is an error).

  Useful when:

  - The body is more than a one-liner and the in-place text area feels cramped.
  - You want syntax highlighting for the markdown.
  - You're building a non-interactive package selection (`--package`) but still want a real editor for the body (`monorel add -p foo:minor --editor`).

  The default behavior (in-place text prompt via `huh`) is unchanged when `--editor` is not passed.

## [0.11.0] - 2026-05-03

### Minor Changes

- `monorel apply` now runs `go mod tidy` (offline, against a seeded local module cache) in every released sub-module that requires an in-plan sibling, so the release commit's `go.sum` and `go.mod` are canonically clean across the repo. Closes #46.

  Before: pinning a sub-module's sibling-require version to `vX.Y.Z` (#42, #44) shifted the `go.sum` drift problem onto consumers. `main` was not `go mod tidy`-clean immediately after a release, and contributors with pre-push tidy hooks tripped on every release pull.

  After: the apply step seeds the developer's Go module cache with the freshly-built release artifacts (`.info`, `.mod`, `.zip`, `.ziphash`), then execs `go mod tidy` with `GOPROXY=off GOSUMDB=off GOWORK=off GOFLAGS=` in each affected sub-module. Tidy resolves the seeded versions from the cache, walks the full transitive closure using the developer's existing cache, and writes correct `go.sum` entries (and any new `// indirect` lines) into the release commit. The cache seeds are removed via deferred cleanup whether the apply succeeds or fails.

  The pre-flight check surfaces a precise error before tidy runs if an out-of-plan managed sibling (the smarter-rewriter case from #44) isn't in the developer's cache.

  Pre-release mode (`monorel pre`) is unaffected; that path doesn't rewrite `go.mod` and so doesn't drift.

  New direct dependencies (promoted from indirect via existing `golang.org/x/mod`):

  - `golang.org/x/mod/zip` for proxy-compatible zip construction.
  - `golang.org/x/mod/sumdb/dirhash` for the `h1:` hash.
  - `golang.org/x/mod/module` for path / version escaping.

  **Pipeline-side note:** `go` must be on `PATH` at apply time.

  - **GitHub Actions / Gitea Actions**: add `actions/setup-go@v5` with `go-version-file: go.mod` ahead of the `disaresta-org/monorel/ci/github` action. GitHub-hosted runners include a recent Go binary, but pinning is safer than relying on the runner's pre-installed version. The example workflows in the README and under `examples/{github,gitea}/` have been updated.
  - **GitLab CI (Docker image)**: the `ghcr.io/disaresta-org/monorel` image's runtime base switched from `alpine:3.20` to `golang:1.26-alpine` so `go` is on `PATH` inside the container. The image grows from roughly 10MB compressed to roughly 250MB; pull once per runner and amortize over releases. Pin a Go version compatible with your modules' `go` directive by choosing the matching monorel image tag.

## [0.10.0] - 2026-05-03

### Minor Changes

- `monorel` now drives all CLI output through a structured logger. Three new persistent flags compose the output:

  - `--color=auto|always|never` controls ANSI color (auto detects whether stdout is a TTY).
  - `-v` / `-vv` increase verbosity: `-v` enables debug messages, `-vv` also appends key/value fields after each line.
  - `-q` / `--quiet` suppresses info and warn output, leaving only errors.

  Status and plan tables now render via the logger's table support, so column alignment stays consistent across terminals and pipes.
- The release-time `go.mod` rewriter now pins sibling requires for managed packages outside the current release plan, not just packages being released.

  Before: releasing a single sub-module that required another monorel-managed sibling would leave the sibling's require at whatever the dev `go.mod` specified (typically a placeholder pseudo-version), because the rewriter only built its sibling map from packages in the current plan. This forced contributors to include the root module in every recovery release just to seed the sibling map (see [loglayer/loglayer-go#70](https://github.com/loglayer/loglayer-go/pull/70)).

  After: the rewriter walks every package declared in `monorel.toml`. In-plan packages pin to their planned version; out-of-plan packages pin to their latest existing stable tag (resolved through `plan.LatestStableTagVersion`, newly exported). Out-of-plan siblings with no existing tag (or only pre-release tags) leave the require alone instead of failing the release.

  Closes #43.

## [0.9.0] - 2026-05-02

### Minor Changes

- `monorel apply` now rewrites each released sub-module's `go.mod` before staging the release commit:

  1. Drops dev-only `replace` directives whose target is a sibling package in the same release plan AND whose source is a relative filesystem path (the `replace go.loglayer.dev/<sibling> => ../<sibling>` convention from monorel's monorepo template). External replaces (forking a third-party dep, etc.) are preserved.
  2. Pins each sibling `require` line to the planned release version, replacing the placeholder pseudo-version (`v0.0.0-00010101000000-000000000000`) sub-modules carry during development.

  Without this rewrite, the dev-only state shipped to the module proxy and downstream consumers' `go mod tidy` returned 404 on the placeholder pseudo-version.

  Fixes [#41](https://github.com/disaresta-org/monorel/issues/41).

### Patch Changes

- Auto-truncate the always-open release PR body when it would exceed the provider's body limit. The orchestrator now falls back through three forms: full rendering with per-package release notes (default), compact rendering with the version table only (when the full body exceeds 65,536 chars), or hard byte-truncation with a trailing marker (last-resort safety net for releases with hundreds of packages where even the table blows past the limit).

  Fixes [#37](https://github.com/disaresta-org/monorel/issues/37): the loglayer-go v2 cascade triggered GitHub's `422 Validation Failed: body is too long` because 27 packages × a single multi-package changeset body × table overhead pushed the rendered PR body past 65,536 chars.

## [0.8.0] - 2026-05-02

### Minor Changes

- Add `monorel doctor` command and public `doctor` package.

  The new diagnostic catches a stale-branch + squash-merge revival of a
  previously-consumed `.changeset/*.md` file: a contributor branches
  from `main` BEFORE a release commit lands, and their PR is later
  squash-merged. GitHub's squash-merge re-introduces the file the
  release commit deleted; the next release plan re-ships the same
  content under a new version. monorel's planner does what its spec
  says (changesets on main = stuff to release); the input is the bug.
  doctor catches the bad input.

  CLI:

  ```sh
  monorel doctor          # text output
  monorel doctor --json   # machine-readable
  ```

  Exits non-zero on any error-severity finding so CI can gate on it.

  Library:

  ```go
  findings, err := doctor.Run(doctor.Options{
      RepoDir: ".",
      GitLog:  repo.DeletedFilesInCommitsMatching,
  })
  ```

  `doctor.GitLog` is a function value; any git library works as the
  backing store. The check itself walks `git log --diff-filter=D
  --grep='chore(release):'` to build the set of previously-deleted
  changeset filenames, then intersects with the live `.changeset/`
  directory.

  Built as a check-runner internally so future checks (orphan
  changesets, malformed frontmatter, etc.) drop in without
  re-architecting; today only `revived-changeset` ships.

  Verified end-to-end against the real revival incident on the monorel
  repo (PR #29 changeset that PR #30 deleted but PRs #31 + #32
  revived via stale-branch + squash-merge): doctor flags it as a
  SeverityError with check name `revived-changeset`, the prior PR
  hand-cleanup (#34) is no longer needed.

## [0.7.0] - 2026-05-02

### Minor Changes

- Add GitLab provider (third provider after GitHub and Gitea/Forgejo).

  `provider.name = "gitlab"` is now a recognized value in
  `monorel.toml`. The implementation lives in
  `internal/provider/gitlab` and wraps
  [`gitlab.com/gitlab-org/api/client-go`](https://gitlab.com/gitlab-org/api/client-go)
  (the official GitLab Go SDK).

  Configuration:

  ```toml
  [provider]
  name  = "gitlab"
  host  = "gitlab.com"          # or your self-hosted instance
  owner = "team/platform"       # may contain slashes for sub-groups
  repo  = "widget"
  ```

  `provider.host` defaults to `gitlab.com`. The `Owner` field accepts
  nested sub-group paths (e.g. `team/platform`); the SDK URL-encodes
  them automatically. Token comes from `GITLAB_TOKEN` (falls back to
  `CI_JOB_TOKEN` in pipelines); needs `api` scope.

  A `//go:build livetest` test suite at
  `internal/provider/gitlab/livetest_test.go` validates the
  implementation against a real GitLab project. Tested end-to-end on
  gitlab.com with the full pipeline (init → add → apply → push →
  preview --upsert → merge MR → tag → push tags → publish).

  GitLab specifics worth knowing:

  - **Project merge method must be Fast-forward** for `monorel tag`
    to find the `monorel-Release:` trailers post-merge. The default
    `merge` method creates a merge commit that strips the body.
  - **GitLab Releases have no first-class prerelease flag**. The
    SemVer pre-release suffix on the tag (e.g. `-rc.0`) is the only
    signal; monorel's tag naming already encodes it.
  - **Sub-groups are supported** via the `Owner` field accepting
    slashes.

  Hard cut: no backward-compat shims (pre-1.0). Existing GitHub and
  Gitea consumers are unaffected.

  Examples directory:

  The `examples/` directory in the monorel repo now has minimal
  reference setups for each provider (`examples/github/`,
  `examples/gitea/`, `examples/gitlab/`). Each contains a
  `monorel.toml` + workflow files + `.changeset/README.md` that
  users can copy into their own repo. Replaces the previous
  disaresta-org/monorel-example external repo as the canonical
  "working example" reference.

## [0.6.0] - 2026-05-01

### Minor Changes

- Add Gitea provider (also covers Forgejo).

  `provider.name = "gitea"` is now a recognized value in `monorel.toml`.
  The implementation lives in `internal/provider/gitea` and wraps
  [`code.gitea.io/sdk/gitea`](https://gitea.com/gitea/go-sdk).

  Forgejo (a Gitea fork that maintains API compatibility) works with
  the same provider; point `provider.host` at the Forgejo instance.

  Configuration:

  ```toml
  [provider]
  name  = "gitea"
  host  = "gitea.example.com"
  owner = "acme"
  repo  = "widget"
  ```

  `provider.host` is required for Gitea/Forgejo because there's no
  canonical public instance. The token comes from the `GITEA_TOKEN`
  environment variable.

  Validates the provider seam: this is the second provider
  implementation, the first non-GitHub one. The factory at
  `internal/provider/factory/factory.go` documents the three-step
  recipe for adding more providers.

  A `//go:build livetest` test suite at
  `internal/provider/gitea/livetest_test.go` validates the
  implementation against a real Gitea instance. Run locally with:

  ```sh
  docker run -d --name monorel-gitea-test -p 3000:3000 gitea/gitea:1.23
  # ...complete install wizard, create user, generate token, create repo...
  export MONOREL_GITEA_HOST=localhost:3000
  export MONOREL_GITEA_TOKEN=<token>
  export MONOREL_GITEA_OWNER=<user>
  export MONOREL_GITEA_REPO=<repo>
  go test -tags=livetest ./internal/provider/gitea
  ```

## [0.5.1] - 2026-05-01

### Patch Changes

- Refresh the repo README to reflect current state:

  - Drop the "pre-v0.1.0, not yet ready for external use" status
    banner (monorel just shipped v0.5.0).
  - Quickstart rewritten around the canonical Action-driven flow:
    `monorel init` instead of hand-writing `monorel.toml`, plus the
    `add → PR → release-pr workflow → merge release PR` lifecycle.
  - Reference [`disaresta-org/monorel-example`](https://github.com/disaresta-org/monorel-example)
    as the working starter to fork.
  - Documentation list updated: added Workflows, FAQ, Glossary; fixed
    the broken `/bootstrap` link (page moved to
    `/recipes/bootstrapping-monorel`); split CLI / Library API.
  - GitHub Action version pin bumped from `@v0.1.2` to `@v0.5.0`.
  - Added a one-liner pointer to the token-guidance section for
    branch-protection users.

## [0.5.0] - 2026-05-01

### Minor Changes

- Implement `monorel init`. Walks every `go.mod` under the working
  directory (skipping `vendor/`, `node_modules/`, and hidden directories),
  infers `provider`, `owner`, and `repo` from `git config remote.origin.url`,
  and writes a starter `monorel.toml` with one `[packages]` block per
  detected Go module plus a `.changeset/README.md`.

  Flags: `--provider`, `--owner`, `--repo` (overrides for auto-detection),
  `--force` (overwrite existing `monorel.toml`).

  Refuses to run without at least one `go.mod`. Existing
  `.changeset/README.md` is preserved when present.

  Removes the "(Planned.)" placeholder from `docs/src/cli-reference.md`
  and replaces it with the real reference.

### Patch Changes

- Three structural docs improvements based on a comparison against
  how changesets / release-please / Knope organize their sites:

  - New `docs/src/faq.md`: ~20 entries grouped by Authoring,
    Release PR, Tags and versions, Pre-release mode, Recovery, and
    Boundaries. Covers the questions that don't fit into reference
    docs ("Can I edit a changeset?", "What if I forget to add a
    changeset?", "Can I downgrade a published version?",
    "Recovery from `ErrTagExists`?").

  - New `docs/src/glossary.md`: ~20 canonical definitions of terms
    used across the docs (changeset, speculative apply, tag prefix,
    bare-tag root, trailer block, pre.json, etc.). Resolves the
    ambiguity of overloaded terms that previously had no single
    authoritative definition.

  - `docs/src/bootstrap.md` moved to
    `docs/src/recipes/bootstrapping-monorel.md`. The page documents
    monorel's one-time self-hosted bootstrap (a recipe for the next
    maintainer if the tool ever forks); the previous top-level
    location implied users needed to do this in their own repo. Now
    groups with the other recipes, sidebar entry renamed to
    "Bootstrapping monorel itself" for clarity.

  Sidebar updated:
  - Glossary added under Reference.
  - New "Help" group containing the FAQ entry.
  - Bootstrap recipe moved + renamed.

  The single inbound link to `/bootstrap` (in `github-action.md`)
  updated to point at the new recipe URL.
- Documents the GitHub `GITHUB_TOKEN` anti-recursion limitation and
  the three workarounds (PAT, GitHub App, ruleset bypass) under a new
  `Tokens and required status checks` section in
  `docs/src/github-action.md`. This is the recurring pain point that
  bites every consumer with branch-protection-required status checks.

  Adds a matching troubleshooting entry ("Release PR is stuck on
  'Some checks haven't completed'") that points at the new section.

## [0.4.1] - 2026-05-01

### Patch Changes

- Update docs for the v0.4.0 speculative-apply design:

  - **`docs/src/cli-reference.md`**: New sections for `monorel apply`,
    `monorel tag`, and `monorel publish`. Updated `monorel release` to
    describe it as the local one-shot wrapper around apply+tag.
    Replaced the stale "Planned for Phase 9" placeholder for
    `monorel preview` with the real reference. Frontmatter description
    updated to list every command.

  - **`docs/src/github-action.md`**: Replaced the empty marker-commit
    description with the speculative-apply explanation throughout.
    Action wrapper version pins bumped from `@v0.1.2` examples to
    `@v0.4.0`. The Branch Protection warning was rewritten: the v0.1.x
    squash-subject-inheritance footgun no longer applies (the staged
    commit is now a real `chore(release): <pkg> <ver>`). A NEW concern
    is documented: the `monorel-Release:` body trailers must survive
    the merge for `monorel tag` to work, which means avoiding the
    squash settings that strip the body. Troubleshooting tail rewritten
    with the new error symptoms (`ErrNoReleaseCommit`,
    `ErrUnknownReleasedPackage`, `ErrTagExists`).

  - **`docs/src/design.md`**: The "Always-open release PR" section now
    describes the two-phase apply/tag split and what each phase does.
    Added a "Why two phases" sub-paragraph naming the contract (the
    `monorel-Release:` trailer block) that bridges them.

  - **`docs/src/getting-started.md`**: Action wrapper pin bumped from
    the non-existent `@v1` to `@v0.4.0`.

  - **`docs/src/recipes/loglayer-go.md` removed.** The page narrated
    monorel's own v0.1.x bug-fix saga during the loglayer-go
    migration. Useful as historical context for monorel maintainers,
    not for someone integrating monorel into their own repo. Sidebar
    entry and the inbound link from
    `docs/src/recipes/migration-from-release-please.md` removed too.

  `docs/src/bootstrap.md` is left as-is; its `v0.1.0` references are
  accurate as the one-time bootstrap context.

## [0.4.0] - 2026-05-01

### Minor Changes

- `monorel add` now uses a [huh](https://github.com/charmbracelet/huh)
  form when stdin is an interactive terminal: arrow-key multi-select
  for packages, per-package bump-level select, and a multi-line text
  field for the changelog body.

  Non-TTY stdin (piped input, redirected files, scripted use, tests)
  falls back to the existing line-based bufio prompt — the contract
  that `printf '1\nminor\nFix.\n\n' | monorel add` works is preserved.
  The auto-detected TTY check uses `golang.org/x/term.IsTerminal`.

  This was in the original v1 plan as a direct dependency but never
  landed; the form is now what the plan called for.

### Patch Changes

- Fix `release-pr` workflow regression where `git push --force-with-lease`
  rejected the staged release branch with "stale info". The previous
  `monorel apply` PR replaced `git push -f` with `--force-with-lease`
  on review feedback, but `--force-with-lease` requires a previously-
  fetched value of the remote ref to compare against — and the
  speculative-apply step builds the local `monorel/release` from
  `origin/main` without ever fetching the remote `monorel/release`,
  so the lease has no expected value and the push is rejected.

  Reverted to plain `git push -f` in both `.github/workflows/release-pr.yml`
  and `ci/github/action.yml`. The `monorel/release` branch is
  bot-exclusive (the workflow is its only writer), so blind force-push
  is the intended behavior. Comments updated to spell out why.

## [0.3.0] - 2026-05-01

### Minor Changes

- Replace the empty marker-commit pattern with real speculative apply
  on `monorel/release`. The always-open release PR's diff is now the
  actual file changes the release will produce (CHANGELOG entries,
  changeset deletions), not an empty marker.

  ### New CLI commands

  - **`monorel apply`** — writes per-package CHANGELOG entries (stable
    mode) or increments `pre.json` counters (pre-release mode), deletes
    the consumed `.changeset/*.md` files, and creates a single commit.
    The commit body carries machine-readable trailers
    (`monorel-Release: <name> <version>`) so a later `monorel tag` can
    derive the per-package tags. Does NOT create tags.

  - **`monorel tag`** — reads HEAD's commit trailers, looks each
    released package up in `monorel.toml`, and creates the
    corresponding annotated git tags at HEAD. Does NOT mutate files
    or create commits.

  - **`monorel release`** keeps its existing one-shot semantics
    (apply + tag) for local releases. Internally it now calls
    `release.ApplyAndTag` which is `release.Apply` followed by
    `release.Tag`.

  ### CI wrapper changes

  The `disaresta-org/monorel/ci/github` action now runs:

  - `command: pr` — `monorel apply` on a fresh `monorel/release` branch
    off the default branch, force-push, then `monorel preview --upsert`.
    The release PR's diff is the actual release.
  - `command: release` — `monorel tag` (file changes already in main
    from the merge), `git push --follow-tags`, `monorel publish`.

  monorel's own `release-pr.yml` and `release.yml` were updated to
  match.

  ### Why

  The marker commit was a workaround for GitHub's PR-create API
  requiring a non-empty head branch. The orchestrator's own GoDoc
  documented this as a v0.x gap to be closed: a "fully orchestrated"
  flow stages the speculative changes on the head branch so the PR is
  genuinely reviewable as a diff. This PR closes that gap.

  Side benefits:

  - The squash-merge subject inheritance footgun (the marker commit's
    generic subject overriding the orchestrator's PR title under some
    repo settings) is gone — the staged commit is a real
    `chore(release): <pkg> <version>` from `monorel apply`.
  - The "two `chore(release):` commits in a row on main after a
    release" issue is gone.
  - Reviewers see the actual file diff, not a body summary.

  ### Implementation

  `internal/release/release.Apply` no longer creates tags; that's now
  `release.Tag`'s job. `release.Tag` reads HEAD's commit message via a
  new `git.Repo.HeadCommitMessage()` method, parses `monorel-Release:`
  and `monorel-PreRelease:` trailers, and creates the corresponding
  tags. `release.ApplyAndTag` chains the two for one-shot use.

  The trailer format is the contract that bridges the two phases:

  ```
  chore(release): transports/zerolog v1.7.0

  monorel-Release: transports/zerolog v1.7.0
  monorel-PreRelease: false
  ```

  Multi-package releases produce one `monorel-Release:` per package in
  plan order; `monorel-PreRelease:` appears once at the end.

  ### Hard cut

  No backward compatibility shims (pre-1.0). Action wrapper at
  `@v0.4` requires monorel binary v0.4 (action auto-coordinates via
  its binary download). Existing consumers on `@v0.3` keep the marker
  pattern; bumping the wrapper pin to `@v0.4` opts in to speculative
  apply.

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
