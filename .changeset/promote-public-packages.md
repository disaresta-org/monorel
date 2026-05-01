---
"monorel.disaresta.com": minor
---

Promote six pure-function packages from `internal/` to the top-level
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
