---
title: "monorel: changesets-style releases for Go monorepos"
description: A release tool for multi-module Go monorepos. Per-PR intent via .changeset files, native Go tag conventions, always-open release PR.

layout: home

hero:
  name: monorel
  text: Releases for Go monorepos
  tagline: Explicit per-PR changesets. Native Go tag conventions.
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
      text: GitHub (MIT Licensed)
      link: https://github.com/disaresta-org/monorel

features:
  - title: Changeset files, not commit messages
    details: Every release-affecting PR includes a .changeset/{name}.md file naming affected packages and bump levels. No path-attribution leaks, no Release-As footers stripped by squash-merges.
  - title: Native Go tag conventions
    details: Bare vX.Y.Z for the main module, {path}/vX.Y.Z for sub-modules. The format go install actually expects, configurable per package.
  - title: Always-open release PR
    details: The bot orchestrator force-pushes a speculative-version branch and upserts a PR. Reviewable, mergeable. The CLI also works standalone for local dry-runs.
  - title: Pre-release support
    details: monorel pre enter rc switches the repo into release-candidate mode; pre exit returns to stable. Per-package counters; multi-channel.
  - title: Provider-neutral host seam
    details: Three providers wired up out of the box (GitHub, GitLab, Gitea / Forgejo); add a subpackage to support others. Renames, hosts, and auth shapes are encapsulated in the provider, not the orchestrator.
  - title: Self-hosted from day one
    details: monorel releases itself with monorel. The same binary that ships your library cuts its own tags.
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
# Author a changeset describing this PR
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

That's the loop. The [GitHub Action](/integrations/github) drives the same flow as an always-open release PR; merging the PR runs `monorel release` on the merge commit and publishes one GitHub Release per tag.
