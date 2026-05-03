---
title: Bitbucket
description: "Wire up monorel against a Bitbucket Cloud repository: configuration, two-env-var auth, workspace plan acceptance, Bitbucket Pipelines workflow."
---

# Bitbucket

monorel's Bitbucket provider talks to [Bitbucket Cloud](https://bitbucket.org) via the standard Bitbucket REST API v2. The implementation is hand-rolled against `net/http` (no SDK), so the provider has no extra direct dependencies and works on any platform Go does.

::: info Available in v0.14+
The Bitbucket provider landed in monorel v0.14.0. Earlier versions support `github`, `gitea`, and `gitlab` only.
:::

::: warning Bitbucket Cloud only
Only Bitbucket Cloud (`bitbucket.org`) is supported. Bitbucket Data Center / Server (the self-hosted product) uses a different REST surface and is not covered. The package layout is namespace-ready for a future `bitbucket/datacenter/` sibling, but that work is not on the roadmap.
:::

::: tip Example
Working reference setup at [`examples/bitbucket/`](https://github.com/disaresta-org/monorel/tree/main/examples/bitbucket). Copy the files you need.
:::

## Configuration

`monorel.toml`:

```toml
[provider]
name  = "bitbucket"
owner = "your-workspace"   # bitbucket workspace slug
repo  = "your-repo"
```

| Field | Notes |
|-------|-------|
| `name` | Must be `"bitbucket"`. The default (`"github"`) won't work against Bitbucket. |
| `owner` | The Bitbucket workspace slug. The provider treats `owner` as the workspace and joins it with `repo` to form the `{workspace}/{repo_slug}` path the REST API expects. |
| `repo` | The repository slug (the URL component, not the display name). |
| `host` | Not used. Bitbucket Cloud is the only host. The package layout is namespace-ready for a future Data Center variant; today the field is ignored. |

Run `monorel validate` to confirm the config loads cleanly.

## Auth: two environment variables

Bitbucket Cloud no longer accepts username + app-password as a long-term auth scheme; the supported path is an Atlassian API token bound to your Atlassian account. monorel reads two env vars:

| Env var | Meaning |
|---------|---------|
| `BITBUCKET_EMAIL` | The email address on the Atlassian account that owns the token. Used as the username on the Basic-auth pair. |
| `BITBUCKET_TOKEN` | The Atlassian API token string itself. |

Both must be set; missing either fails the constructor with a clear error.

### Creating an API token with Bitbucket scopes

1. Go to [`id.atlassian.com/manage-profile/security/api-tokens`](https://id.atlassian.com/manage-profile/security/api-tokens) (the Atlassian-account API tokens page; works for any user with a Bitbucket workspace).
2. Click **Create API token with scopes**. The plain "Create API token" button generates a token without Bitbucket scopes; that token will return HTTP 401 against every Bitbucket REST call. The scoped flow is the one you want.
3. Pick **Bitbucket** as the app, then select the workspace.
4. Grant at minimum:
   - `read:repository:bitbucket` and `write:repository:bitbucket` (for tag creation and metadata reads)
   - `read:pullrequest:bitbucket` and `write:pullrequest:bitbucket` (for the always-open release PR)
   - `read:account` (so monorel can probe the username on `/2.0/user`; see below)
5. Copy the token and store it as a secret in your CI provider (`BITBUCKET_TOKEN`). The token is shown once.

::: warning Tokens without Bitbucket scopes return HTTP 401
The "Create API token" button (no scopes) at `id.atlassian.com` produces a token that the rest of Atlassian Cloud accepts but Bitbucket rejects. If your first push or `monorel preview --upsert` returns `401 Unauthorized` despite the token being non-empty, regenerate the token via "Create API token with scopes" and pick the Bitbucket app explicitly.
:::

### Username probing

Bitbucket's git-over-HTTPS push expects the workspace's Bitbucket username, not the email on the API token. monorel probes `GET /2.0/user` once at constructor time with the email + token, reads the `username` field, and caches it on the client. You don't supply a username; it's resolved automatically from the token's identity.

This is why the API token MUST have `read:account`: without it, the probe fails and the provider can't construct.

## Workspace plan acceptance

Bitbucket Cloud workspaces created (or upgraded) since the September 2024 plan migration require an explicit workspace-plan acceptance step before push, MR, and release-tag operations succeed. The symptom is HTTP 402 (`Payment Required`) on the first push from CI, even on free-tier workspaces, even when billing is current.

Resolution: a workspace admin visits `https://bitbucket.org/<workspace>/workspace/settings/plans` and clicks the plan-acceptance button (the page presents it the first time the workspace is loaded after migration). It's a one-time action per workspace and unrelated to billing.

If you see `402 Payment Required` from a `monorel preview --upsert` or `monorel tag` step, hit that URL first and re-run the workflow. monorel surfaces the upstream HTTP status verbatim so the error is easy to recognize.

## Workflows

monorel doesn't ship a Bitbucket-specific CI wrapper. The simplest setup uses Bitbucket Pipelines with the published Docker image (`ghcr.io/disaresta-org/monorel`). One pipeline that branches on whether the just-landed commit is a `chore(release):` merge: non-release commits run `monorel apply` + `monorel preview --upsert`; release commits run `monorel tag` + `git push --follow-tags` + `monorel publish`.

`bitbucket-pipelines.yml`:

<!--@include: ../_partials/bitbucket-pipelines-yml.md-->

Setup:

1. **Add `BITBUCKET_TOKEN` and `BB_USER` as repository secrets** under Repository settings → Repository variables. `BITBUCKET_TOKEN` is the Atlassian API token; `BB_USER` is the Bitbucket username (the same one monorel probes from `/2.0/user`; you can copy it from your Bitbucket profile URL). The pipeline uses both to authenticate the git push over HTTPS.
2. **Pick a merge strategy that preserves the release commit body.** Fast-forward and merge-commit both work; squash-merge rewrites the body but the universal trailers fallback recovers (see below). Configure under Repository settings → Branch restrictions / Merge strategies.
3. **Push the `bitbucket-pipelines.yml` to the default branch.** The pipeline fires on every push to `main`; it runs `monorel apply` + `monorel preview --upsert` and opens the always-open release PR once you have a `.changeset/<name>.md`.

::: info Token revocation
To revoke or rotate, return to `id.atlassian.com/manage-profile/security/api-tokens` and delete the token. Update `BITBUCKET_TOKEN` in the repo's pipeline variables and re-run.
:::

### Local CLI (no CI)

Same shape as the [Working without CI](/getting-started#working-without-ci) section of Getting Started, with the two Bitbucket env vars:

```sh
BITBUCKET_EMAIL=... BITBUCKET_TOKEN=... monorel release
git push --follow-tags
BITBUCKET_EMAIL=... BITBUCKET_TOKEN=... monorel publish
```

`monorel publish` is effectively a no-op on Bitbucket (see below); the command exits cleanly without making any API calls. The release commit and tags still land via `monorel release` + `git push --follow-tags`.

### External CI

monorel is a single static binary. Any CI that can run a shell command can run it. Pattern is the same as the local flow: download the binary (or use the Docker image), set `BITBUCKET_EMAIL` + `BITBUCKET_TOKEN`, run `monorel release` + `git push --follow-tags` + `monorel publish`.

## Merge strategy: any of the three works

The release PR's commit body carries `monorel-Release:` trailers that `monorel tag` reads post-merge. Three Bitbucket merge strategies are available:

- **Fast-forward**: preserves the head commit verbatim. Trailers reach `main` unchanged. Recommended.
- **Merge commit**: creates a merge commit with the head commit as a parent. `monorel tag` reads the parent's body trailers. Works.
- **Squash merge**: rewrites the body, dropping the trailers from the merge commit. The universal trailers fallback recovers: monorel walks back to the merged PR (via `FindPRByMergeCommit`) and reads the trailers from a `<!-- monorel-trailers ... -->` HTML comment that `monorel preview` writes into the PR body. The release still completes.

Squash merge is supported but not preferred; the fallback adds an extra round-trip to the API and a small risk that a contributor edited the PR body and removed the comment block. Pick fast-forward or merge-commit when you have the choice.

## No native releases: CHANGELOG.md is canonical

Bitbucket Cloud has no first-class "Release" concept analogous to GitHub Releases or GitLab Releases (a tag-attached object with a body, assets, and a pre-release flag). The closest equivalent is the tag itself plus the per-package `CHANGELOG.md` entry that `monorel apply` writes alongside.

Consequence: `monorel publish` does nothing on Bitbucket. It's safe to run (the command exits successfully without making any API calls), but the canonical record of what changed in each release is the `CHANGELOG.md` entry. The pipeline above includes `monorel publish` for symmetry with the other providers; you can omit it without loss.

::: warning `monorel publish` still requires the auth env vars
The publish command constructs the provider client before noticing it has nothing to do, so `BITBUCKET_EMAIL` and `BITBUCKET_TOKEN` must still be set in the environment when it runs. Constructor validation (including the `/2.0/user` username probe) fires before the no-op check; missing or invalid credentials surface as a constructor error, not a clean exit.
:::

If you need release notes shown in a UI, link the per-package CHANGELOG file from the repo's README or your project's docs site.

## Branch protection

Bitbucket calls these "branch restrictions." Two points:

- **`monorel/release` should NOT have a `Prevent rewriting branch history` restriction.** The pipeline force-pushes to that branch on every release-pr run. Either don't add a restriction covering `monorel/release`, or exempt the bot user / token.
- **The default branch's merge strategy should preserve the trailers** (fast-forward or merge-commit; see above). Squash-merge works via the fallback but adds a network hop and one failure mode (edited PR body).

## Troubleshooting

### `provider: unknown provider "bitbucket"`

monorel binary is older than v0.14.0 (when the Bitbucket provider landed). Upgrade with `go install monorel.disaresta.com/cmd/monorel@latest` or use a newer Docker image tag.

### `bitbucket: probe username: 401 Unauthorized`

The API token doesn't have Bitbucket scopes (or doesn't have `read:account`). Regenerate via the **Create API token with scopes** flow at `id.atlassian.com/manage-profile/security/api-tokens` and grant the scopes listed under [Auth](#auth-two-environment-variables).

### `bitbucket: ...: 402 Payment Required`

The workspace plan hasn't been accepted yet. Visit `https://bitbucket.org/<workspace>/workspace/settings/plans` as a workspace admin and click through the plan-acceptance prompt. See [Workspace plan acceptance](#workspace-plan-acceptance).

### `bitbucket: ...: 403 Forbidden`

The token's scopes are insufficient for the operation. The most common gap is `write:pullrequest:bitbucket` (needed to open / update the always-open release PR). Regenerate the token with all scopes listed under [Auth](#auth-two-environment-variables).

### `monorel tag` returns `ErrNoReleaseCommit` despite the merge happening

HEAD's commit body has no `monorel-Release:` trailers AND the merged PR's body has no `<!-- monorel-trailers ... -->` comment block (so the universal fallback also missed). The fallback fails when both sources are gone, which typically means a contributor manually edited the release-PR body and removed the comment block before merge. Recovery: hand-create the tags pointing at the merge commit, push, then re-run. See the [tag-recovery FAQ](/faq#a-release-pr-merged-without-monorel-release-body-trailers-and-monorel-tag-returned-errnoreleasecommit-what-now) for the exact commands.
