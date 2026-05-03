---
"monorel.disaresta.com": minor
---

`.changeset/pre.json` now carries a `schemaVersion` field (currently `1`). Files written by older monorel builds omit the field and load as version 1, so existing pre-release windows are unaffected. A file whose `schemaVersion` is higher than the current build supports is rejected with a clear `"upgrade monorel"` error rather than being silently misread.

The on-disk shape is otherwise unchanged. Library callers that construct `*changeset.PreState` directly don't need to set the field; `PreState.Write` stamps `schemaVersion: 1` automatically when the field is zero.

Constants exposed: `changeset.PreStateCurrentSchemaVersion`. Bump it (and add a migration in `LoadPreState`) the next time the on-disk shape changes incompatibly.

Pre-v1.0 housekeeping: future-proofs the pre-release-state file format ahead of the v1.0 stability commitment.
