---
"monorel.disaresta.com": patch
---

**Fix multi-module `go mod tidy` failing on a clean Go module cache, plus a GitLab CI doc gap surfaced verifying the fix end-to-end.**

`offlineTidyEnv` (and `primeCacheEnv`) now resolve `GOMODCACHE` via `go env GOMODCACHE` under the host's full env and pin it explicitly in the subprocess env. Previously, on systems where `GOMODCACHE` is derived from `GOPATH` (notably `golang:alpine` images, where `GOPATH=/go` but no explicit `GOMODCACHE` is set), the restricted-env tidy subprocess defaulted to `~/go/pkg/mod` while `seedModuleCache` had written into `/go/pkg/mod`. The mismatch surfaced as `module lookup disabled by GOPROXY=off` on the in-plan sibling. Affects multi-module repos with sibling-`require`s on cold-cache CI runners and inside the published Docker image.

The example `.gitlab-ci.yml` and its docs partial now include a `git remote set-url origin` step that rewrites the runner's auto-cloned remote to use `MONOREL_GITLAB_TOKEN` instead of the read-only `CI_JOB_TOKEN`. Without it, `monorel auto`'s push to `monorel/release` (and the subsequent tag push on the release-PR merge) fail with `403: You are not allowed to push code to this project`. The integration page gains a matching `:::warning` callout and a troubleshooting entry.

Surfaced by the new `e2e_tidy`-tagged regression test (`tests/e2e/tidy_isolated_test.go`, runs unconditionally now) and a real-runner GitLab.com pipeline test against the fix-branch SHA, which produced an opened release MR with `replace` stripped and `require example.com/widget v1.0.0` pinned in every sub-module.
