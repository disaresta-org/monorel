---
"monorel.disaresta.com": patch
---

Deploy docs only on release, not on every push to main.

Previously `docs.yml` deployed to GitHub Pages on every push to `main`,
which meant any merge — even a typo fix in a Go file unrelated to
docs — would re-deploy the site. Switch the deploy gate to match
the loglayer-go pattern:

- `pull_request` (paths-filtered to `docs/**`): build-only, for
  verification that the site still builds.
- `release: published`: deploy on manually-created GitHub Releases.
- `workflow_dispatch`: manual escape hatch.
- `workflow_call`: invoked from `release.yml` after a monorel-driven
  release is cut. Sidesteps GitHub's anti-recursion rule (Releases
  created via `GITHUB_TOKEN` don't propagate `release: published`
  events to other workflows).

`release.yml` chains `docs.yml` via `workflow_call` alongside the
existing `build-binaries` and `build-image` chains, so every
monorel-driven release deploys the docs that go with it. No
deployment fires for non-release commits to main, even if they
touch `docs/**`.
