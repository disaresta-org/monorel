---
title: Gitea / Forgejo
description: "Wire up monorel against a Gitea or Forgejo repository: configuration, tokens, CI options."
---

# Gitea / Forgejo

monorel's Gitea provider talks to any Gitea or Forgejo instance via the standard Gitea REST API. [Forgejo](https://forgejo.org/) is a Gitea fork that maintains API compatibility, so the same provider implementation covers both; point `provider.host` at whichever instance you're targeting. Tested against Gitea 1.23 via the `code.gitea.io/sdk/gitea` SDK.

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

1. **Token wiring.** Gitea Actions auto-injects the `secrets.GITHUB_TOKEN` secret for GitHub-Actions YAML compatibility. The composite action exports whatever you pass as `with: token:` into both the `GITHUB_TOKEN` and `GITEA_TOKEN` env vars internally, so monorel reads whichever name matches the configured provider. Pass the auto-injected token for the basic case (see the workflow YAML below); pass a PAT for required-status-check repos (see [Tokens and required status checks](#tokens-and-required-status-checks) below).
2. **`provider.host` must be set** in `monorel.toml` to your Gitea instance.

One workflow file drives the entire lifecycle. On every push to the default branch, the wrapper runs [`monorel auto`](/cli-reference#monorel-auto), which detects whether HEAD is the merge of monorel's release PR and dispatches accordingly:

- **Feature commit** (the common case): stage the always-open release PR's diff via `monorel apply` + `monorel preview --upsert`. If the planner has nothing to apply, any open release PR is closed.
- **Release-PR merge**: run `monorel tag` + `git push --follow-tags` + `monorel publish` to create per-package tags and one Release per tag.

Detection uses HEAD's `monorel-Release:` commit-body trailer OR an API lookup that finds the PR whose merge produced HEAD and confirms its source branch is `monorel/release`. Either signal alone is sufficient, so the dispatch works regardless of merge strategy.

`.gitea/workflows/release.yml`:

<!--@include: ../_partials/gitea-release-yml.md-->

The auto-injected `secrets.GITHUB_TOKEN` is enough for the basic case (the action wrapper exports it under both the `GITHUB_TOKEN` and `GITEA_TOKEN` env vars internally). The PAT escalation for required-status-check repos is documented under [Tokens and required status checks](#tokens-and-required-status-checks) below.

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

### Skipping CI on chore(release) commits

The release commit `chore(release): ...` (created when the always-open release PR is merged) updates module `go.mod` files to require new in-plan sibling versions. monorel's release workflow creates and pushes the matching tags on the same push. Any *other* workflow that fires on that push and resolves Go module versions (lint, test, deploy) races the tag push and may transiently fail with:

```
go: example.com/foo/v2: reading example.com/foo/go.mod at revision v2.1.0: unknown revision v2.1.0
```

The release succeeds and the tags get pushed; the racing workflow's red mark stays in the UI. Gitea Actions is GitHub Actions-compatible, so the same `if:` clause works:

```yaml
jobs:
  test:
    if: github.event_name == 'pull_request' || !startsWith(github.event.head_commit.message, 'chore(release):')
    # ... rest of job ...
```

monorel's own release workflow does NOT need this filter. On a `chore(release):` push it's the workflow doing the tagging; on every other push `monorel auto` falls through to the upsert path.

## Branch protection

`monorel auto` force-pushes to `monorel/release` on every feature-branch run. By default Gitea allows this for unprotected branches; if you've added branch protection rules covering `monorel/release`, allow force-push for that ref (or for the bot account that owns the workflow).

### Merge strategy

`monorel auto` detects a release-PR merge using two independent signals OR'd together:

- **Trailer signal** (fast path): HEAD's commit body contains a `monorel-Release:` trailer. Hits when squash- or rebase-merge propagated the source commit's body.
- **API signal** (network): monorel asks the host to identify the PR whose merge produced HEAD; if its source branch is `monorel/release`, this is a release-PR merge. Hits when the trailer is missing for any reason.

Either signal alone is enough. Squash, rebase, and merge-commit are all fine merge strategies for the release PR; pick whichever matches the rest of your repo's convention. `monorel tag` (run on the release path) reads the trailer when present and falls back to the universal trailers source (a `<!-- monorel-trailers ... -->` HTML comment in the merged PR's body) when it isn't, so tag creation also works regardless of merge strategy.

## Tokens and required status checks

PRs created via Gitea Actions' auto-injected `secrets.GITHUB_TOKEN` are subject to the same anti-recursion rule GitHub has: workflows configured to run on `pull_request` won't fire for those PRs. If your Gitea instance enforces required status checks on the default branch, the always-open release PR will sit on "Some checks haven't completed yet" with no path to merge.

Fix: generate a personal access token at `/-/user/settings/applications` (or your Forgejo instance's equivalent) with `repository: write` + `user: read`. Add it as a repo secret (e.g. `MONOREL_GITEA_TOKEN`) and pass it to the action wrapper instead of the auto-injected token:

```yaml
      - uses: disaresta-org/monorel/ci/github@v1.0.0
        env:
          GITEA_TOKEN: ${{ secrets.MONOREL_GITEA_TOKEN }}
```

Same trade-off as on GitHub: PATs are tied to a user; a service-account user is the durable shape. See the [GitHub page's Tokens and required status checks](/integrations/github#tokens-and-required-status-checks) for the full PAT / GitHub App / bypass discussion: the Gitea equivalents (Gitea API tokens, Gitea OAuth Apps, branch-protection bypass lists) follow the same shape with provider-renamed UI labels.

## Troubleshooting

### `provider: unknown provider "gitea"`

monorel binary doesn't recognize the `gitea` provider name. Upgrade with `go install monorel.disaresta.com/cmd/monorel@latest` or pin the action wrapper to `@v1.0.0` or later.

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
