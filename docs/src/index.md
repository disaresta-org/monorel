---
title: "monorel: changesets-style releases for Go monorepos"
description: A release tool for multi-module Go monorepos. Per-PR intent via .changeset files, native Go tag conventions, always-open release PR.

layout: home

hero:
  name: monorel
  text: Releases for Go monorepos
  tagline: Explicit per-PR changesets. Native Go tag conventions. Single and multi-module support.
  image:
    src: /logo-v2.webp
    alt: "monorel logo: many incoming rails merging into one"
  actions:
    - theme: brand
      text: Why monorel?
      link: /introduction
    - theme: alt
      text: Quickstart
      link: /getting-started
    - theme: alt
      text: Coming from release-please?
      link: /recipes/migration-from-release-please

features:
  - title: Changesets, not commit messages
    details: One .changeset/{name}.md per release-affecting PR, naming the affected packages, bump levels, and changelog body. No commit-message inference — squash-merges can't strip footers, and stray Release-As lines from old commits can't leak into a newly-registered package's first release.
  - title: Native Go tag conventions
    details: Bare vX.Y.Z for the root module, {path}/vX.Y.Z for sub-modules. The format go install expects, configurable per package.
  - title: Clean go.mod + tidy go.sum at release
    details: Strips dev replace directives, pins sibling require versions to the planned release, runs offline go mod tidy in every released sub-module. main stays canonically clean for proxy consumers on the next pull.
  - title: Always-open release PR
    details: monorel stages each release on a monorel/release branch and force-pushes after every change. The PR's diff is the actual file changes — CHANGELOGs, go.mod updates, the chore(release) commit — so reviewers see real content, not a body summary.
  - title: Pre-release support
    details: monorel pre enter rc switches the repo into release-candidate mode; pre exit returns to stable. Per-package counters; multi-channel.
  - title: GitHub, GitLab, Gitea / Forgejo
    details: One CI step on any provider. monorel auto opens release PRs / MRs, creates per-package tags, and publishes a Release per tag. Single workflow file; no provider-specific orchestration in your repo.
---

## Quick Example

```toml
# monorel.toml
[provider]
name = "github"
owner = "acme"
repo = "widget"

[packages."github.com/acme/widget"]
tag_prefix = ""
path       = "."
changelog  = "CHANGELOG.md"

[packages."transports/zerolog"]
tag_prefix = "transports/zerolog"
path       = "transports/zerolog"
changelog  = "transports/zerolog/CHANGELOG.md"
```

```sh
# Author a changeset describing this PR (or run `monorel add` with no
# flags for an interactive picker — useful when bumping multiple
# packages in one changeset).
monorel add --package "transports/zerolog:minor" --message "Adds Lazy() helper."

# Preview the next release locally
monorel plan
# PACKAGE             FROM     BUMP   TO       TAG
# transports/zerolog  v1.6.1   minor  v1.7.0   transports/zerolog/v1.7.0
#
# 1 package(s) to release; 1 changeset(s) consumed.

# Apply: write CHANGELOGs, delete consumed changesets, commit, tag
monorel release
git push --follow-tags
```

That's the loop. The CI workflow (one file, one step on any of the three supported providers) drives the same flow as an always-open release PR; merging the PR runs the release path and publishes one Release per tag.

monorel runs in production on [loglayer-go](https://github.com/loglayer/loglayer-go) (multi-module Go monorepo) and on monorel itself. Pair it with [GoReleaser](https://goreleaser.com) for binary builds and Docker image distribution; monorel handles versions, tags, and CHANGELOGs.

---

monorel is made with ❤️ by [Theo Gravity](https://suteki.nu) / [Disaresta](https://disaresta.com).
