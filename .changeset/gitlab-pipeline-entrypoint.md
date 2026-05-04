---
"monorel.disaresta.com": patch
---

**Fix `examples/gitlab/.gitlab-ci.yml` failing on the `docker+machine` executor.**

The published `ghcr.io/disaresta-org/monorel` image is an entrypoint-binary container (its entrypoint is `monorel` itself). GitLab's `docker+machine` executor wraps every script as `sh -c '...'` and passes that to the container's entrypoint, producing `monorel sh -c '...'` and failing with `unknown command "sh"`.

The example pipeline + the partial it includes from now use the long-form `image:` block with `entrypoint: [""]` to clear the container's entrypoint so the runner's shell wrapper takes over. Surfaced by an end-to-end test against a real GitLab runner.

Also documents an outstanding known issue in `docs/src/integrations/gitlab.md`: multi-module Go monorepos with sibling `require`s hit a separate offline-tidy bug on cold-cache CI runners. Tracked separately from this fix; single-module repos and multi-module repos without in-plan sibling requires are unaffected. A new build-tag-gated test (`tests/e2e/tidy_isolated_test.go` under `e2e_tidy`) reproduces the bug deterministically using a `golang:1.26-alpine` testcontainer for use during the eventual fix.
