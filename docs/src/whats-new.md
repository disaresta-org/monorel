---
title: What's new
description: "User-facing release notes for monorel."
---

# What's new

User-visible changes by release. The full per-commit record lives in the [`CHANGELOG.md`](https://github.com/disaresta-org/monorel/blob/main/CHANGELOG.md) on GitHub.

## May 03, 2026

`v1.0.0`:

**monorel reaches 1.0.** First stable release. The public Go library API surface (`config`, `changeset`, `plan`, `semver`, `validate`, `changelog`, `doctor`) is now SemVer-committed: additive changes are minor bumps; breaking changes are major bumps and ship as `monorel.disaresta.com/v2/...` per Go module convention.

What's in the box at 1.0:

- Four providers wired up out of the box: GitHub, Gitea / Forgejo, GitLab, Bitbucket Cloud.
- Always-open release PR pattern with speculative-version branches.
- Per-package CHANGELOG via `.changeset/*.md` files (no commit-message inference, no path attribution).
- Bare-tag root (`vX.Y.Z`) and path-prefixed sub-module tags (`<path>/vX.Y.Z`) in the same repo.
- Pre-release support (`monorel pre enter`/`exit` with per-package counters).
- `go.mod` cleanup at release (strips dev `replace` directives, pins sibling versions).
- `go.sum` tidy at release (offline, against a seeded local cache).
- Universal PR-body trailers fallback: `monorel tag` recovers from squash-merges that strip `monorel-Release:` commit trailers.
- `monorel doctor` for repository-state diagnostics.

The 1.0 release contains no functional changes from v0.14.0; 1.0 is a stability commitment, not new code. Self-hosted (monorel releases itself with monorel) and validated in production by [loglayer-go](https://go.loglayer.dev), a 26-sub-module Go monorepo.

See the [`CHANGELOG.md`](https://github.com/disaresta-org/monorel/blob/main/CHANGELOG.md) for the per-version history of every prior release.
