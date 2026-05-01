---
"monorel.disaresta.com": minor
---

Replace the empty marker-commit pattern with real speculative apply
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
