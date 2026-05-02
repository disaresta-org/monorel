---
title: Gitea / Forgejo
description: "Wire up monorel against a Gitea or Forgejo repository: configuration, tokens, CI options."
---

# Gitea / Forgejo

monorel's Gitea provider talks to any Gitea or Forgejo instance via the standard Gitea REST API. [Forgejo](https://forgejo.org/) is a Gitea fork that maintains API compatibility, so the same provider implementation covers both; point `provider.host` at whichever instance you're targeting. Tested against Gitea 1.23 via the `code.gitea.io/sdk/gitea` SDK.

::: info Available in v0.6+
The Gitea provider landed in monorel v0.6.0. Earlier versions only support GitHub.
:::

::: tip Example
Working reference setup at [`examples/gitea/`](https://github.com/disaresta-org/monorel/tree/main/examples/gitea). Copy the files you need.
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
| `name` | Must be `"gitea"`. Set explicitly: the default (`"github"`) won't work against a Gitea instance. The same value is used for Forgejo. |
| `host` | Required. Gitea / Forgejo have no canonical public host equivalent to `api.github.com`. Accepts a bare hostname (`gitea.example.com`, defaults to `https://`) or a fully-qualified URL (`http://localhost:3000` for dev, `https://gitea.example.com` for prod). |
| `owner`, `repo` | Same as GitHub: the user / org and the repository name. |

Run `monorel validate` to confirm the config loads cleanly.

## Token

monorel reads the auth token from the `GITEA_TOKEN` environment variable. Generate one in Gitea under **Settings → Applications → Generate New Token** (or hit `/-/user/settings/applications`). Required scopes:

- `repository: write` (create / edit / close PRs, create releases)
- `user: read` (identify the bot account)

Set `GITEA_TOKEN` in your shell or CI environment before running `monorel preview --upsert`, `monorel publish`, or any command that talks to the API.

## Workflows

monorel doesn't ship a Gitea-specific CI wrapper. Three viable approaches:

### Gitea Actions (recommended)

Gitea Actions (since Gitea 1.21, Forgejo 1.21) implements GitHub Actions' workflow YAML format. Two things to know before reusing the GitHub workflow shape:

1. **Token env var.** monorel reads `GITEA_TOKEN`, not `GITHUB_TOKEN`. Gitea Actions keeps the auto-injected token under `secrets.GITHUB_TOKEN` for compatibility with GitHub Actions' YAML; map it to the env var monorel reads.
2. **`provider.host` must be set** in `monorel.toml` to your Gitea instance.

`.gitea/workflows/release-pr.yml`:

<!--@include: ../_partials/gitea-release-pr-yml.md-->

`.gitea/workflows/release.yml`:

<!--@include: ../_partials/gitea-release-yml.md-->


The auto-injected `GITHUB_TOKEN` is enough for the basic case (open / update / close the release PR; create tags; create Releases). The PAT escalation for required-status-check repos is documented under [Tokens and required status checks](#tokens-and-required-status-checks) below.

`.gitea/workflows/doctor.yml` (recommended pre-merge sanity check; mirrors the GitHub setup):

<!--@include: ../_partials/gitea-doctor-yml.md-->

`fetch-depth: 0` is required so doctor's git-log scan sees prior `chore(release):` commits. See [`monorel doctor`](/cli-reference#monorel-doctor) for what the check covers.

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

## Branch protection

The `pr` command force-pushes to `monorel/release` on every release-pr workflow run. By default Gitea allows this for unprotected branches; if you've added branch protection rules covering `monorel/release`, allow force-push for that ref (or for the bot account that owns the workflow).

The release PR's commit body carries `monorel-Release:` trailers that `monorel tag` reads post-merge. The merge strategy must preserve the body. Use **rebase merge** (recommended) or **create a merge commit** (which preserves the head commit verbatim as the parent and lets `monorel tag` find trailers via the merged commit). Squash merge with default settings on Gitea collapses the body; verify on a test PR before relying on it.

## Tokens and required status checks

PRs created via Gitea Actions' auto-injected `secrets.GITHUB_TOKEN` are subject to the same anti-recursion rule GitHub has: workflows configured to run on `pull_request` won't fire for those PRs. If your Gitea instance enforces required status checks on the default branch, the always-open release PR will sit on "Some checks haven't completed yet" with no path to merge.

Fix: generate a personal access token at `/-/user/settings/applications` (or your Forgejo instance's equivalent) with `repository: write` + `user: read`. Add it as a repo secret (e.g. `MONOREL_GITEA_TOKEN`) and pass it to the action wrapper instead of the auto-injected token:

```yaml
      - uses: disaresta-org/monorel/ci/github@v0.6.0
        with:
          command: pr
        env:
          GITEA_TOKEN: ${{ secrets.MONOREL_GITEA_TOKEN }}
```

Same trade-off as on GitHub: PATs are tied to a user; a service-account user is the durable shape. See the [GitHub page's Tokens and required status checks](/integrations/github#tokens-and-required-status-checks) for the full PAT / GitHub App / bypass discussion: the Gitea equivalents (Gitea API tokens, Gitea OAuth Apps, branch-protection bypass lists) follow the same shape with provider-renamed UI labels.

## Troubleshooting

### `provider: unknown provider "gitea"`

monorel binary is older than v0.6.0 (when the Gitea provider landed). Upgrade with `go install monorel.disaresta.com/cmd/monorel@latest` or pin the action wrapper to `@v0.6.0` or later.

The error surfaces from the validator (`config.Validate`) when reading `monorel.toml`, so it'll fire on every command, not just network-touching ones.

### `gitea: connect <host>: ...`

The `New` constructor performs a server-version handshake against the configured host. A connect error means the host is unreachable from the runner. Check:

- Spelling of the host value in `monorel.toml`.
- Whether you need `http://` (not `https://`) for a dev container or unencrypted instance.
- Whether the runner can reach the host at all (firewall, DNS, VPN).

### Per-call deadlines don't fire

The Gitea SDK anchors every request on the context passed to `NewClient`, not per-method ctx arguments. monorel's call sites use `context.Background()` for construction; per-call deadlines passed via `ctx` are silently ignored. This differs from the GitHub provider (which threads ctx through every call). Invisible to current users (monorel doesn't expose per-call deadlines today), but worth noting if a future flag would have you assuming consistent timeout behavior across providers.

### Force-push rejected on `monorel/release`

Gitea branch protection blocks force-push by default for protected branches. Either remove `monorel/release` from the protection rule, allow force-push for the bot's account, or remove the protection entirely (the branch is bot-managed and doesn't need protection).
