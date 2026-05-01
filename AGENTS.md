# monorel: Agent Guidelines

> **Note:** Always-applicable rules (documentation, code patterns) live in `.claude/rules/`.
> This file is high-level project context.

## Project Overview

monorel is a changesets-style release tool for multi-module Go monorepos.
Born out of the `loglayer-go` repo's release-please saga: per-PR intent
declared in `.changeset/<name>.md` files instead of inferred from commit
messages. Closes the entire class of "release tool got confused"
failure modes (footer leaks, squash-merge stripping, full-history
attribution scans) by construction.

**Module path:** `monorel.disaresta.com` (vanity URL via go-import meta tag)
**GitHub:** `github.com/disaresta-org/monorel`
**Docs:** VitePress site under `docs/`

## Project Structure

```
monorel/
├── cmd/monorel/main.go           Thin entrypoint
├── internal/
│   ├── cli/                      Cobra commands: add, plan, status, release, preview, pre, init
│   ├── config/                   monorel.toml schema + Load + Validate
│   ├── changeset/                .changeset/*.md format: parse, write, name-gen, pre.json
│   ├── semver/                   Bump levels + Apply / InitialFromBump / ApplyPrerelease
│   ├── git/                      Repo interface + shell-out impl + in-memory fake
│   ├── git/testutil/             On-disk git repo helper for integration tests
│   ├── plan/                     Pure-function planner: (config, changesets, tags, pre) -> ReleasePlan
│   ├── changelog/                Keep-a-Changelog writer (insert above first H2)
│   ├── release/                  Apply ReleasePlan: write changelogs, tag, commit, publish
│   ├── provider/                 Provider-neutral host API seam (PR upsert + release create)
│   │   ├── factory/              Dispatch by config.ProviderConfig.Name
│   │   └── github/               go-github implementation
│   └── action/                   (Phase 9, not yet built) bot orchestrator
├── docs/                         VitePress documentation site
│   ├── .vitepress/config.ts      Sidebar + OG meta
│   └── src/                      Markdown source
├── ci/                           (Phase 10, not yet built) per-CI-system wrappers
│   └── github/action.yml         Composite GitHub Action
├── .changeset/                   Self-hosted changesets
├── monorel.toml                  Self-hosted config
├── lefthook.yml                  Git hooks
└── Makefile
```

## Key Design Decisions

- **Changeset files are the source of truth** for what should release. YAML
  frontmatter maps package names to bump levels; markdown body becomes the
  changelog entry. No commit-message parsing, no path attribution, no
  `Release-As:` footers, no `bootstrap-sha`.
- **Tag format configurable per package**: bare `vX.Y.Z` for the main
  module at the repo root, `<path>/vX.Y.Z` for sub-modules. The convention
  Go's toolchain expects.
- **Always-open release PR pattern**: the bot orchestrator force-pushes a
  speculative-version branch and upserts a PR. Reviewable, mergeable.
- **No GitHub App. CLI + reusable Action.** No hosted infrastructure.
- **Self-hosted from day one**: monorel releases itself with monorel.
- **No linked releases ever** (per user direction). Each package's
  version evolves independently.
- **Hard cut to Keep-a-Changelog** on migrated repos: monorel writes
  Keep-a-Changelog entries from now forward and preserves prior content
  (e.g. release-please-style entries) verbatim.
- **Pre-release support**: `monorel pre enter <channel>` writes
  `.changeset/pre.json`; subsequent releases append `-<channel>.N` to
  versions and increment per-package counters. `pre exit` returns to
  stable, applying accumulated changesets cumulatively.
- **Provider-neutral host seam**: `internal/provider.Client` abstracts
  GitHub today, GitLab/Gitea/Bitbucket/Forgejo by adding a subpackage
  + factory case + KnownProviders entry.
- **Pure-function planner**: `plan.Plan` takes static inputs and
  returns a ReleasePlan. No I/O. Exhaustively table-tested. From
  v0.2.0 it lives at `monorel.disaresta.com/plan` (public API)
  alongside `config`, `changeset`, `semver`, `validate`, and
  `changelog`. Side-effect-bearing packages (`release`,
  `orchestrator`, `provider`, `git`, `cli`) stay in `internal/`.

## Verification

After any code change:

```sh
go build ./...
go test ./...
```

For docs:

```sh
cd docs && bun run docs:build
```

For the full lint pass (matches CI + pre-commit hook):

```sh
gofmt -l .
go vet ./...
staticcheck ./...
```

## Git Hooks (lefthook)

Pre-commit and pre-push hooks are managed by [lefthook](https://github.com/evilmartians/lefthook).
Config lives in `lefthook.yml` at the repo root.

Install once after cloning:

```sh
go install github.com/evilmartians/lefthook@latest
lefthook install
```

What runs:

- **commit-msg**: lints the commit message against the conventional-commits
  parser (same one release-please uses). Hard-fails if `bun` isn't on PATH
  or `node_modules` is missing; install bun (https://bun.sh) and run
  `bun install`.
- **pre-commit** (parallel): `gofmt -l` on staged Go files, `go vet ./...`,
  `staticcheck ./...`. Hard-fails if `staticcheck` isn't on PATH; install
  with `go install honnef.co/go/tools/cmd/staticcheck@latest`.
- **pre-push**: `go test -race -count=1 ./...`.

Skip a hook for one command with `git commit --no-verify` or
`git push --no-verify`.

## Versioning and Changelog

monorel releases itself with monorel. The self-hosted `monorel.toml`
declares a single package at the repo root with `tag_prefix = ""` so
tags emerge as bare `vX.Y.Z`.

`CHANGELOG.md` at the repo root is maintained by monorel itself.
Format: Keep-a-Changelog (`## [X.Y.Z] - YYYY-MM-DD` + `### Major /
Minor / Patch Changes`).

To cut a release:

1. Land changes in `main` with at least one `.changeset/*.md` file.
2. Wait for the release PR to update (or run `monorel preview` locally).
3. Merge the release PR.
4. The release workflow runs `monorel release`, pushes tags, and
   publishes a GitHub Release per tag.

## Adding a New Provider

The provider seam is `internal/provider.Client`, the abstract host
interface monorel uses for PR / Release operations. To add `<name>`:

1. Create `internal/provider/<name>/<name>.go` implementing the six
   methods on `provider.Client`. Return `provider.Client` from
   `New(opts)`, keep the concrete type unexported.
2. Add a case to `internal/provider/factory.New` that constructs your
   provider from `config.ProviderConfig`.
3. Append your provider name to `config.KnownProviders` (public) and
   add a case to `provider.TokenEnvVars` (internal) if the provider
   uses an env-var token.
4. Add tests against `provider.NewFake` for any orchestration that
   uses provider-specific behavior. The provider's own impl typically
   doesn't need unit tests beyond constructor validation; live-API
   tests are out of scope for the main test suite.

The CI wrapper layer (under `ci/<name>/`) is separate and per-CI-system;
each wrapper is a thin shim that downloads the monorel binary and runs
it with the right env vars.

## CI / Release Workflows

`.github/workflows/`:

- **ci.yml**: build, gofmt, vet, test -race, staticcheck.
- **docs.yml**: build VitePress docs on PR (verify clean), deploy to
  GitHub Pages on release.
- **release-pr.yml**: `monorel preview` on push to `main`. Maintains
  the always-open release PR.
- **release.yml**: `monorel release --publish` on release-PR merge.
- **pr-title.yml**: validates PR titles follow conventional commits.

To trigger a release: merge the release PR. The release workflow
runs `monorel release --publish`, pushes tags, and creates one
GitHub Release per tag.

## Thread Safety

- **Pure-function planner**: `plan.Plan` is concurrency-safe (no
  shared state).
- **`provider.Fake`**: protected by an internal mutex; safe for parallel
  test runs.
- **`git.Exec`**: NOT designed for concurrent use against the same
  working tree. Each test/CLI invocation owns its own working tree.

## Currently Out of Scope

- Polyglot support (Go-only).
- GitHub App / Bot model (the reusable Action is enough).
- Conventional-commit fallback (changesets are the only signal).
- Cross-repo coordination (single-repo only).
- Linked releases (per user direction; each package versions
  independently).
- Bitbucket: lacks a first-class Release; impl can no-op CreateRelease.
- Gerrit: doesn't fit the always-open PR model.
- Homebrew/apt/Scoop packaging (until users ask).
