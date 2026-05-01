---
"monorel.disaresta.com": minor
---

Add `monorel validate` — a static-checks subcommand that walks
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
