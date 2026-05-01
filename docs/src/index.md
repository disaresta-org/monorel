---
layout: home

hero:
  name: monorel
  text: Changesets-style releases for Go monorepos.
  tagline: Explicit per-PR intent. Bare main tags + prefixed sub-module tags. No commit-message inference, no leaked footers.
  actions:
    - theme: brand
      text: Get Started
      link: /getting-started
    - theme: alt
      text: View on GitHub
      link: https://github.com/disaresta-org/monorel

features:
  - title: Changeset files, not commit messages
    details: Every release-affecting PR includes a `.changeset/<name>.md` file naming affected packages and bump levels. No path-attribution leaks, no `Release-As:` footers stripped by squash-merges.
  - title: Native Go tag conventions
    details: Bare `vX.Y.Z` for the main module, `<path>/vX.Y.Z` for sub-modules. The format `go get` actually expects, configurable per package.
  - title: Always-open release PR
    details: Reviewable, mergeable. Pair with the GitHub Action for the always-open PR pattern; the binary works standalone for local dry-runs.
  - title: Pre-release support
    details: `monorel pre enter rc` switches the repo into release-candidate mode; `pre exit` returns to stable. Per-package counters, multi-channel.
---
