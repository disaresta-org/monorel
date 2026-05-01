---
"monorel.disaresta.com": minor
---

Implement `monorel init`. Walks every `go.mod` under the working
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
