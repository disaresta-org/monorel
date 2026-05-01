# monorel

A changesets-style release tool for multi-module Go monorepos.

[![CI](https://github.com/disaresta-org/monorel/actions/workflows/ci.yml/badge.svg)](https://github.com/disaresta-org/monorel/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

`monorel` manages per-package versions, tags, and changelogs in a Go monorepo using explicit `.changeset/*.md` files instead of inferring releases from commit messages. Pair it with the [GitHub Action](#github-action) to drive an always-open release PR.

> **Status: pre-v0.1.0.** Under active development. Not yet ready for external use.

## Why monorel?

The Go ecosystem has a real gap: no battle-tested off-the-shelf release tool fits "main module at repo root with bare `vX.Y.Z` tags + sub-modules with `<path>/vX.Y.Z` tags" cleanly.

- **release-please** works with friction (`Release-As:` footers leak across packages, full-history scans cause initial-release surprises, squash-merge strips footers).
- **Knope** doesn't support per-package tag-prefix overrides (so it can't do bare-tag root + prefixed sub-modules).
- **changesets** is JS-native and needs synthetic `package.json` files in every Go module.

`monorel` fills that gap. Read [the introduction](https://disaresta-org.github.io/monorel/introduction) for the full comparison.

## Quickstart

```sh
# 1. Install
go install github.com/disaresta-org/monorel/cmd/monorel@latest

# 2. Scaffold config in your repo
monorel init

# 3. Add a changeset describing what's about to release
monorel add

# 4. Preview what will release
monorel plan

# 5. Apply (writes CHANGELOGs, tags, pushes)
monorel release
```

## Documentation

- [Introduction](https://disaresta-org.github.io/monorel/introduction) — why monorel, design tradeoffs.
- [Getting Started](https://disaresta-org.github.io/monorel/getting-started) — install, init, first release.
- [Configuration](https://disaresta-org.github.io/monorel/configuration) — `monorel.toml` reference.
- [CLI](https://disaresta-org.github.io/monorel/cli-reference) — every command and flag.
- [Changesets](https://disaresta-org.github.io/monorel/changesets) — file format, conventions.
- [GitHub Action](https://disaresta-org.github.io/monorel/github-action) — always-open PR setup.

## GitHub Action

```yaml
- uses: disaresta-org/monorel-action@v1
  with:
    command: pr      # or 'release' on release-PR merge
    token: ${{ secrets.GITHUB_TOKEN }}
```

Full setup in the [GitHub Action docs](https://disaresta-org.github.io/monorel/github-action).

## Development

```sh
git clone https://github.com/disaresta-org/monorel
cd monorel

bun install        # commit-msg linter + docs deps
make hooks         # install git hooks via lefthook
make build         # builds ./monorel
make test-race     # full test suite under -race
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for details.

## License

[MIT](LICENSE)
