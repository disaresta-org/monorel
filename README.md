# monorel

<p align="center">
  <a href="https://monorel.disaresta.com" title="monorel"><img src="docs/src/public/logo-v2.webp" alt="monorel" width="220" /></a>
</p>

<p align="center">
  <a href="https://github.com/disaresta-org/monorel/releases"><img src="https://img.shields.io/github/v/tag/disaresta-org/monorel?filter=v*&sort=date&label=version&color=blue" alt="Latest version" /></a>
  <a href="https://pkg.go.dev/monorel.disaresta.com"><img src="https://pkg.go.dev/badge/monorel.disaresta.com.svg" alt="Go Reference" /></a>
  <a href="https://github.com/disaresta-org/monorel/actions/workflows/ci.yml"><img src="https://github.com/disaresta-org/monorel/actions/workflows/ci.yml/badge.svg" alt="CI" /></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/License-MIT-blue.svg" alt="License: MIT" /></a>
</p>

A changesets-style release tool for multi-module Go monorepos.

`monorel` manages per-package versions, tags, and CHANGELOG entries in a Go monorepo using explicit `.changeset/*.md` files instead of inferring releases from commit messages. Pair it with the [GitHub Action](#github-action) to drive an always-open release PR whose diff IS the actual file changes the next release will produce.

## Why monorel?

The Go ecosystem has a real gap: no battle-tested release tool fits "main module at repo root with bare `vX.Y.Z` tags + sub-modules with `<path>/vX.Y.Z` tags" cleanly.

- **release-please** works with friction (`Release-As:` footers leak across packages, full-history scans cause initial-release surprises, squash-merge strips footers).
- **Knope** doesn't support per-package tag-prefix overrides, so it can't do bare-tag root + prefixed sub-modules.
- **changesets** is JS-native and needs synthetic `package.json` files in every Go module.

`monorel` fills that gap. See [the introduction](https://monorel.disaresta.com/introduction) for the side-by-side comparison table.

## Quickstart

In a Go repo with at least one `go.mod`:

```sh
# 1. Install
go install monorel.disaresta.com/cmd/monorel@latest

# 2. Scaffold monorel.toml + .changeset/ from your repo
monorel init

# 3. Wire up CI: copy .github/workflows/{release-pr,release}.yml from
#    https://monorel.disaresta.com/getting-started

# 4. Author a changeset on a feature branch
monorel add --package "transports/foo:minor" --message "Adds Lazy() helper."
git commit && gh pr create

# 5. Merge the PR. The release-pr workflow opens (or updates) an
#    always-open release PR. Merge it when ready to ship.
```

Reference setups for each provider live in [`examples/`](examples/) (GitHub, Gitea / Forgejo, GitLab). Copy the files you need; the [Getting Started](https://monorel.disaresta.com/getting-started) walkthrough explains the full lifecycle.

## Documentation

- [Introduction](https://monorel.disaresta.com/introduction): why monorel + comparison vs release-please / changesets / Knope.
- [Getting Started](https://monorel.disaresta.com/getting-started): install, init, wire up CI, ship the first release.
- [Workflows](https://monorel.disaresta.com/workflows): ASCII diagrams of the daily flow, release cuts, pre-release cycles.
- [Configuration](https://monorel.disaresta.com/configuration): `monorel.toml` reference.
- [CLI](https://monorel.disaresta.com/cli-reference): every command and flag.
- [Changesets](https://monorel.disaresta.com/changesets): file format and authoring conventions.
- [GitHub Action](https://monorel.disaresta.com/github-action): action wrapper inputs, branch protection, token setup.
- [FAQ](https://monorel.disaresta.com/faq): the questions that come up after the first release.
- [Glossary](https://monorel.disaresta.com/glossary): canonical definitions of monorel terminology.
- [Library API](https://monorel.disaresta.com/api): Go packages exposed for programmatic use.

## GitHub Action

```yaml
- uses: disaresta-org/monorel/ci/github@v0.5.0
  with:
    command: pr      # or 'release' for the post-merge tag + push + publish step
```

If your repo has required status checks on the default branch, set the `token` input to a PAT or GitHub App token instead of the default `GITHUB_TOKEN`. See [Tokens and required status checks](https://monorel.disaresta.com/github-action#tokens-and-required-status-checks).

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

See [CONTRIBUTING.md](CONTRIBUTING.md) for the full dev-loop reference.

## License

[MIT](LICENSE)
