---
"monorel.disaresta.com": minor
---

`monorel apply` now rewrites each released sub-module's `go.mod` before staging the release commit:

1. Drops dev-only `replace` directives whose target is a sibling package in the same release plan AND whose source is a relative filesystem path (the `replace go.loglayer.dev/<sibling> => ../<sibling>` convention from monorel's monorepo template). External replaces (forking a third-party dep, etc.) are preserved.
2. Pins each sibling `require` line to the planned release version, replacing the placeholder pseudo-version (`v0.0.0-00010101000000-000000000000`) sub-modules carry during development.

Without this rewrite, the dev-only state shipped to the module proxy and downstream consumers' `go mod tidy` returned 404 on the placeholder pseudo-version.

Fixes [#41](https://github.com/disaresta-org/monorel/issues/41).
