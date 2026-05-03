---
"monorel.disaresta.com": minor
---

`monorel apply` now runs `go mod tidy` (offline, against a seeded local module cache) in every released sub-module that requires an in-plan sibling, so the release commit's `go.sum` and `go.mod` are canonically clean across the repo. Closes #46.

Before: pinning a sub-module's sibling-require version to `vX.Y.Z` (#42, #44) shifted the `go.sum` drift problem onto consumers — `main` was not `go mod tidy`-clean immediately after a release, and contributors with pre-push tidy hooks tripped on every release pull.

After: the apply step seeds the developer's Go module cache with the freshly-built release artifacts (`.info`, `.mod`, `.zip`, `.ziphash`), then execs `go mod tidy` with `GOPROXY=off GOSUMDB=off GOWORK=off GOFLAGS=` in each affected sub-module. Tidy resolves the seeded versions from the cache, walks the full transitive closure using the developer's existing cache, and writes correct `go.sum` entries (and any new `// indirect` lines) into the release commit. The cache seeds are removed via deferred cleanup whether the apply succeeds or fails.

The pre-flight check surfaces a precise error before tidy runs if an out-of-plan managed sibling (the smarter-rewriter case from #44) isn't in the developer's cache.

Pre-release mode (`monorel pre`) is unaffected; that path doesn't rewrite `go.mod` and so doesn't drift.

New direct dependencies (promoted from indirect via existing `golang.org/x/mod`):

- `golang.org/x/mod/zip` for proxy-compatible zip construction.
- `golang.org/x/mod/sumdb/dirhash` for the `h1:` hash.
- `golang.org/x/mod/module` for path / version escaping.

`go` must be on `PATH` at apply time (already required by every existing release-pipeline runner).
