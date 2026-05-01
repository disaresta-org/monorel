---
title: Gitea / Forgejo
description: "Wire up monorel against a Gitea or Forgejo repository: configuration, tokens, CI options."
---

# Gitea / Forgejo

monorel's Gitea provider talks to any Gitea or Forgejo instance via the standard Gitea REST API. Forgejo is a Gitea fork that maintains API compatibility, so the same provider implementation covers both; point `provider.host` at whichever instance you're targeting.

::: info Available in v0.6+
The Gitea provider landed in monorel v0.6.0. Earlier versions only support GitHub.
:::

## Configuration

`monorel.toml`:

```toml
[provider]
name  = "gitea"
host  = "gitea.example.com"
owner = "acme"
repo  = "widget"
```

| Field | Notes |
|-------|-------|
| `name` | Must be `"gitea"`. Set explicitly: the default (`"github"`) won't work against a Gitea instance. |
| `host` | Required. Gitea / Forgejo have no canonical public host equivalent to `api.github.com`. Accepts a bare hostname (`gitea.example.com`, defaults to `https://`) or a fully-qualified URL (`http://localhost:3000` for dev, `https://gitea.example.com` for prod). |
| `owner`, `repo` | Same as GitHub: the user / org and the repository name. |

Run `monorel validate` to confirm the config loads cleanly.

## Token

monorel reads the auth token from the `GITEA_TOKEN` environment variable. Generate one in Gitea under **Settings → Applications → Generate New Token** (or hit `/-/user/settings/applications`). Required scopes:

- `repository: write` (create / edit / close PRs, create releases)
- `user: read` (identify the bot account)

Set `GITEA_TOKEN` in your shell or CI environment before running `monorel preview --upsert`, `monorel publish`, or any command that talks to the API.

## CI options

monorel doesn't ship a Gitea-specific CI wrapper today. Three viable approaches:

### Gitea Actions (recommended)

Gitea Actions (since Gitea 1.21, Forgejo 1.21) implements GitHub Actions' workflow YAML format. The same workflow shape that drives the GitHub flow works here, with two adjustments:

1. The `disaresta-org/monorel/ci/github` action wrapper passes `secrets.GITHUB_TOKEN` as `GITHUB_TOKEN` to the binary by default. monorel reads `GITEA_TOKEN`. Set `GITEA_TOKEN` explicitly in the step's `env`.
2. The `host` field in `monorel.toml` must be set to your Gitea instance.

Example `.gitea/workflows/release-pr.yml`:

```yaml
name: release-pr
on:
  push:
    branches: [main]
permissions:
  contents: write
  pull-requests: write
jobs:
  release-pr:
    if: ${{ !startsWith(github.event.head_commit.message, 'chore(release):') }}
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with: { fetch-depth: 0 }
      - uses: disaresta-org/monorel/ci/github@v0.6.0
        with:
          command: pr
        env:
          GITEA_TOKEN: ${{ secrets.MONOREL_GITEA_TOKEN }}
```

The `MONOREL_GITEA_TOKEN` secret is one you create explicitly in your repo's secrets; the auto-injected `GITEA_TOKEN` in Gitea Actions is restricted to the running workflow's repo, which is enough for the speculative-apply push and PR upsert.

`release.yml` mirrors the GitHub setup, gated on the `chore(release):` commit subject. The same `command: release` step does `monorel tag` + `git push --follow-tags` + `monorel publish`.

### Local CLI (no CI)

Same shape as the [Working without CI](/getting-started#working-without-ci) section of Getting Started, with `GITEA_TOKEN` instead of `GITHUB_TOKEN`:

```sh
GITEA_TOKEN=... monorel release
git push --follow-tags
GITEA_TOKEN=... monorel publish
```

Works on a contributor's laptop or any non-Gitea-Actions CI (Drone, Woodpecker, plain shell). The release commit, tags, and Releases all land the same way.

### External CI (Drone, Woodpecker, etc.)

monorel is a single static binary; any CI that can run a shell command can run it. The pattern is the same as the local CLI flow: download the binary, set `GITEA_TOKEN`, run `monorel release` + `git push --follow-tags` + `monorel publish`. There's no monorel-specific machinery to install.

## Force-push to the staging branch

The `pr` command force-pushes to `monorel/release` on every release-pr workflow run. By default Gitea allows this for unprotected branches; if you've added branch protection rules covering `monorel/release`, allow force-push for that ref (or the bot account that owns the workflow).

## Forgejo notes

[Forgejo](https://forgejo.org/) is a Gitea fork with stated API compatibility. The Gitea provider works against Forgejo unchanged: set `provider.host` to your Forgejo instance and use a Forgejo-issued access token in `GITEA_TOKEN`. The token UI lives at `/user/settings/applications` (same path as Gitea).

If you hit an API call that behaves differently between Gitea and Forgejo, file an issue: the SDK monorel uses (`code.gitea.io/sdk/gitea`) is supposed to handle both. We've tested the implementation against Gitea 1.23.

## Troubleshooting

### `provider.name "gitea" is not recognized`

monorel binary is older than v0.6.0. Upgrade with `go install monorel.disaresta.com/cmd/monorel@latest` or pin the action wrapper to `@v0.6.0` or later.

### `gitea: connect <host>: ...`

The `New` constructor performs a server-version handshake against the configured host. If `gitea.New` fails with a connect error, the host is unreachable from the runner: check spelling, whether you need `http://` (not `https://`) for a dev container, and whether the runner can reach the host at all (firewall, DNS).

### Per-call deadlines don't fire

The Gitea SDK anchors every request on the context passed to `NewClient`, not per-method ctx arguments. monorel's call sites use `context.Background()` for construction; per-call deadlines passed via `ctx` are silently ignored. This differs from the GitHub provider (which threads ctx through every call). Practical impact: the `--timeout` flag (when added) won't behave identically across providers. File an issue if this matters for your setup.

### Force-push rejected on `monorel/release`

Gitea branch protection blocks force-push by default for protected branches. Either remove `monorel/release` from the protection rule, allow force-push for the bot's account, or remove the protection entirely (the branch is bot-managed and doesn't need protection).
