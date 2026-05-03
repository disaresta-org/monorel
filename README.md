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

`monorel` manages per-package versions, tags, and CHANGELOG entries in a Go monorepo using explicit `.changeset/*.md` files instead of inferring releases from commit messages. Pair it with CI on your provider (GitHub, Gitea / Forgejo, GitLab, or Bitbucket Cloud) to drive an always-open release PR whose diff IS the actual file changes the next release will produce.

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

# 3. Wire up CI: copy the provider-specific workflow files from
#    examples/{github,gitea,gitlab}/ into your repo

# 4. Author a changeset on a feature branch
monorel add
git commit && gh pr create

# 5. Merge the PR. The release-pr workflow opens (or updates) an
#    always-open release PR. Merge it when ready to ship.
```

Reference setups for each provider live in [`examples/`](examples/) (GitHub, Gitea / Forgejo, GitLab, Bitbucket). Copy the files you need; the [Getting Started](https://monorel.disaresta.com/getting-started) walkthrough explains the full lifecycle.

## Documentation

- [Introduction](https://monorel.disaresta.com/introduction): why monorel + comparison vs release-please / changesets / Knope.
- [Getting Started](https://monorel.disaresta.com/getting-started): install, init, wire up CI, ship the first release.
- [Workflows](https://monorel.disaresta.com/workflows): ASCII diagrams of the daily flow, release cuts, pre-release cycles.
- [Cheat Sheet](https://monorel.disaresta.com/cheat-sheet): at-a-glance command map, common one-liners, files monorel reads and writes.
- [Configuration](https://monorel.disaresta.com/configuration): `monorel.toml` reference.
- [CLI](https://monorel.disaresta.com/cli-reference): every command and flag.
- [Changesets](https://monorel.disaresta.com/changesets): file format and authoring conventions.
- Integration guides: [GitHub](https://monorel.disaresta.com/integrations/github), [Gitea / Forgejo](https://monorel.disaresta.com/integrations/gitea), [GitLab](https://monorel.disaresta.com/integrations/gitlab), [Bitbucket](https://monorel.disaresta.com/integrations/bitbucket).
- [FAQ](https://monorel.disaresta.com/faq): the questions that come up after the first release.
- [Use with AI / LLMs](https://monorel.disaresta.com/llms): paste-ready `llms.txt` and `llms-full.txt` for coding assistants.
- [Glossary](https://monorel.disaresta.com/glossary): canonical definitions of monorel terminology.
- [Library API](https://monorel.disaresta.com/api): Go packages exposed for programmatic use.

## CI integration

monorel ships a composite GitHub Action wrapper plus first-class support for Gitea / Forgejo, GitLab CI, and Bitbucket Pipelines. Each provider has a working reference setup under [`examples/`](examples/):

| Provider | Example | Notes |
|---|---|---|
| GitHub | [`examples/github/`](examples/github/) | Composite action wrapper + two workflow files. |
| Gitea / Forgejo | [`examples/gitea/`](examples/gitea/) | Same wrapper on Gitea Actions; `provider.host` set; covers Forgejo via API compatibility. |
| GitLab | [`examples/gitlab/`](examples/gitlab/) | Single `.gitlab-ci.yml` using `ghcr.io/disaresta-org/monorel`. Project must use fast-forward merge. |
| Bitbucket Cloud | [`examples/bitbucket/`](examples/bitbucket/) | Single `bitbucket-pipelines.yml` using `ghcr.io/disaresta-org/monorel`. Cloud-only; needs the [workspace plan acceptance](https://monorel.disaresta.com/integrations/bitbucket#workspace-plan-acceptance) step. |

The GitHub flow looks like this:

```yaml
- uses: actions/setup-go@v5
  with:
    go-version-file: go.mod
- uses: disaresta-org/monorel/ci/github@v0.14.0
  with:
    command: pr      # or 'release' for the post-merge tag + push + publish step
```

`actions/setup-go` is required because monorel's apply step runs `go mod tidy` against a seeded local cache so the release commit's `go.sum` is canonically clean. See the [GitHub integration page](https://monorel.disaresta.com/integrations/github) for the full workflow.

If your repo enforces required status checks on the default branch, the bot-created release PR's checks won't fire on the auto-injected token (anti-recursion). Switch to a PAT or GitHub App token. See [Tokens and required status checks](https://monorel.disaresta.com/integrations/github#tokens-and-required-status-checks).

Full setup per provider in the integration guides linked above.

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
