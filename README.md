# monorel

<p align="center">
  <a href="https://monorel.disaresta.com" title="monorel"><img src="docs/src/public/logo-v2.webp" alt="monorel" width="220" /></a>
</p>

<p align="center">
  <a href="https://github.com/disaresta-org/monorel/actions/workflows/ci.yml"><img src="https://github.com/disaresta-org/monorel/actions/workflows/ci.yml/badge.svg" alt="CI" /></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/License-MIT-blue.svg" alt="License: MIT" /></a>
</p>

A changesets-style release tool for multi-module Go monorepos.

`monorel` manages per-package versions, tags, and changelogs in a Go monorepo using explicit `.changeset/*.md` files instead of inferring releases from commit messages. Pair it with the [GitHub Action](#github-action) to drive an always-open release PR.

> **Status: pre-v0.1.0.** Under active development. Not yet ready for external use.

## Why monorel?

The Go ecosystem has a real gap: no battle-tested release tool fits "main module at repo root with bare `vX.Y.Z` tags + sub-modules with `<path>/vX.Y.Z` tags" cleanly.

- **release-please** works with friction (`Release-As:` footers leak across packages, full-history scans cause initial-release surprises, squash-merge strips footers).
- **Knope** doesn't support per-package tag-prefix overrides, so it can't do bare-tag root + prefixed sub-modules.
- **changesets** is JS-native and needs synthetic `package.json` files in every Go module.

`monorel` fills that gap. Read [the introduction](https://monorel.disaresta.com/introduction) for the full comparison.

## Quickstart

```sh
# 1. Install
go install monorel.disaresta.com/cmd/monorel@latest

# 2. Author a changeset describing this PR
monorel add --package "transports/zerolog:minor" --message "Adds Lazy() helper."

# 3. Preview the next release
monorel plan

# 4. Apply locally (writes CHANGELOGs, deletes consumed changesets,
#    creates the release commit and tags). Push is your job.
monorel release
git push --follow-tags
```

For a `monorel.toml` example and full walkthrough, see [Getting Started](https://monorel.disaresta.com/getting-started).

## Documentation

- [Introduction](https://monorel.disaresta.com/introduction): why monorel, design tradeoffs.
- [Getting Started](https://monorel.disaresta.com/getting-started): install, init, first release.
- [Configuration](https://monorel.disaresta.com/configuration): `monorel.toml` reference.
- [CLI](https://monorel.disaresta.com/cli-reference): every command and flag.
- [Changesets](https://monorel.disaresta.com/changesets): file format, conventions.
- [GitHub Action](https://monorel.disaresta.com/github-action): always-open PR setup.
- [Bootstrapping](https://monorel.disaresta.com/bootstrap): one-time procedure for the first release.

## GitHub Action

```yaml
- uses: disaresta-org/monorel/ci/github@v1
  with:
    command: pr      # or 'release' on the release-PR merge / dispatch
```

Full setup in the [GitHub Action docs](https://monorel.disaresta.com/github-action).

## Development

```sh
git clone https://github.com/disaresta-org/monorel
cd monorel

bun install        # commit-msg linter + docs deps
make hooks         # install git hooks via lefthook
make build         # builds ./monorel
make test-race     # full test suite under -race
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for the dev-loop details.

## License

[MIT](LICENSE)
