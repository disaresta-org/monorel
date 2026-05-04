---
title: "monorel: changesets-style releases for Go monorepos"
description: A release tool for single-module and multi-module Go repos. Per-PR intent via .changeset files, native Go tag conventions, always-open release PR.

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
    details: One .changeset/{name}.md per PR names the affected packages and bump levels. Squash-merges can't strip footers because monorel doesn't read commit messages.
  - title: Native Go tag conventions
    details: Bare vX.Y.Z at the root, {path}/vX.Y.Z for sub-modules. What go install expects, configurable per package.
  - title: Clean go.mod + tidy go.sum at release
    details: Strips dev replace directives, pins sibling versions, runs offline go mod tidy. main stays clean for proxy consumers.
  - title: Always-open release PR
    details: Each release stages on a monorel/release branch. The PR's diff IS the file changes the release will produce.
  - title: Pre-release support
    details: pre enter rc for release-candidate mode; pre exit for stable. Per-package counters, multi-channel.
  - title: GitHub, GitLab, Gitea / Forgejo
    details: One workflow file, one step. monorel auto opens release PRs, creates tags, publishes Releases.
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
