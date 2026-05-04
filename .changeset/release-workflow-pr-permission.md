---
"monorel.disaresta.com": patch
---

**Fix `release.yml` 403 on every non-release push to main.**

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
