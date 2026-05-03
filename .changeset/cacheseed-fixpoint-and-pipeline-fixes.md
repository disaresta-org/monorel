---
"monorel.disaresta.com": minor
---

Fix `cacheseed` writing the wrong h1: hash for released sub-modules
(would silently produce broken go.sum entries on every release; see
[`loglayer/loglayer-go`'s v2.1.0 incident](https://github.com/loglayer/loglayer-go/pull/76)).
Reorder `applyStable` so all working-tree mutations happen before
the seed step, and replace the single-pass seed-and-tidy with
iterate-to-fixpoint to handle cross-sibling dep chains.

Add a `go mod download` priming step before offline tidy so fresh
CI runners (with empty `GOMODCACHE`) can resolve third-party deps.
The `GOPROXY=off` invariant during tidy is preserved.

Document the `actions/setup-go` prerequisite (sub-modules with
`go 1.25.0` directives need a 1.25+ runner since `GOTOOLCHAIN=local`
during tidy blocks auto-download) and the `chore(release):`-commit
skip filter recipe. See [issue #54](https://github.com/disaresta-org/monorel/issues/54).
