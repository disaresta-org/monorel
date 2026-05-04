---
"monorel.disaresta.com": patch
---

**Fix multi-module `go mod tidy` failing on a clean Go module cache.**

`offlineTidyEnv` (and `primeCacheEnv`) now resolve `GOMODCACHE` via `go env GOMODCACHE` under the host's full env and pin it explicitly in the subprocess env. Previously, on systems where `GOMODCACHE` is derived from `GOPATH` (notably `golang:alpine` images, where `GOPATH=/go` but no explicit `GOMODCACHE` is set), the restricted-env tidy subprocess defaulted to `~/go/pkg/mod` while `seedModuleCache` had written into `/go/pkg/mod` — surfacing as `module lookup disabled by GOPROXY=off` on the in-plan sibling.

Affects multi-module repos with sibling-`require`s on cold-cache CI runners and inside the published Docker image. Surfaced by the new `e2e_tidy`-tagged regression test (`tests/e2e/tidy_isolated_test.go`) running `monorel apply` inside `golang:1.26-alpine`; the test now runs unconditionally.
