---
title: Bootstrapping monorel
description: "How to ship monorel's first release before monorel exists. One-time workflow; subsequent releases are monorel-driven."
---

# Bootstrapping monorel

monorel releases itself with monorel from `v0.1.1` onward. The first release (`v0.1.0`) has to happen without monorel because the binary doesn't exist yet. This page documents that one-time bootstrap.

::: warning One-time only
After `v0.1.0` ships, every subsequent release goes through `monorel release --publish` (locally or via the GitHub Action). Do not re-run this bootstrap on a healthy repo.
:::

## What ships in v0.1.0

- A GitHub Release for `v0.1.0` with the rendered Keep-a-Changelog body.
- One archive per OS+arch attached to the Release: `monorel_0.1.0_linux_amd64.tar.gz`, `monorel_0.1.0_darwin_arm64.tar.gz`, etc.
- The repo's first git tag at `v0.1.0`.

## Steps

### 1. Verify the working tree

```sh
git status                  # clean
go test ./...               # green
cd docs && bun run docs:build  # clean
```

### 2. Hand-write the v0.1.0 entry in `CHANGELOG.md`

monorel's normal flow writes CHANGELOG entries from changesets. For the bootstrap, edit `CHANGELOG.md` directly:

```markdown
# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.0] - 2026-04-30

Initial release.

### Major Changes

- ...
```

Commit:

```sh
git add CHANGELOG.md
git commit -m "chore(release): v0.1.0"
```

### 3. Tag and push

```sh
git tag -a v0.1.0 -m "monorel v0.1.0"
git push origin main
git push origin v0.1.0
```

### 4. The binary-build workflow runs

`.github/workflows/build-release-binaries.yml` triggers on the tag push. It runs GoReleaser, which:

- Builds the per-OS+arch binaries.
- Creates the GitHub Release for `v0.1.0` (mode `append` creates the Release if it doesn't exist).
- Attaches the archives + a checksums file.

### 5. Edit the Release body

GoReleaser's auto-generated body is empty (`changelog.disable: true`). Paste the `## [0.1.0]` section from `CHANGELOG.md` into the GitHub Release body via the UI.

(For `v0.1.1` onward, `monorel release --publish` writes the body automatically; this manual step is only for the bootstrap.)

### 6. Verify the install path

```sh
go install github.com/disaresta-org/monorel/cmd/monorel@v0.1.0
monorel --version
# v0.1.0
```

And the GitHub Action wrapper:

```yaml
- uses: disaresta-org/monorel/ci/github@v0.1.0
  with:
    command: pr
```

## After bootstrap

From `v0.1.1` on:

1. Author changesets via `monorel add`.
2. CI runs `monorel preview --upsert` on push to main → release PR opens / updates.
3. Merging the release PR runs `monorel release --publish` on the merge commit → tags pushed → binary-build workflow → archives attached.

The bootstrap workflow file (`build-release-binaries.yml`) is the only piece that survives the bootstrap; everything else converges on the `monorel`-driven flow.
