---
title: Bitbucket
description: "Wire up monorel against a Bitbucket Cloud repository: configuration, two-env-var auth, workspace plan acceptance, Bitbucket Pipelines workflow."
---

# Bitbucket

::: danger Disabled in this build
The Bitbucket provider is **disabled**. `provider.name = "bitbucket"` is rejected by the validator with `"is not recognized"`, and the factory does not dispatch to it. The provider implementation, the example pipeline, and the Atlassian API client code remain in-tree at `internal/provider/bitbucket/` for future re-enablement, but the workflow has not been verified end-to-end against a live Bitbucket Pipelines runner. Re-enabling requires:

1. Successful end-to-end Pipelines runs of the canonical example (the maintainer hit a workspace 2FA enforcement that blocks the API enable path; verification is pending).
2. Adding `ProviderBitbucket` back to `config.KnownProviders` and uncommenting the case in `internal/provider/factory/factory.go`.

This page is preserved as a reference for the eventual re-enablement and for users who choose to compile against the in-tree provider directly. **Do not use the configuration shapes below in production until the provider is re-enabled.**
:::

monorel's Bitbucket provider talks to [Bitbucket Cloud](https://bitbucket.org) via the standard Bitbucket REST API v2. The implementation is hand-rolled against `net/http` (no SDK), so the provider has no extra direct dependencies and works on any platform Go does.

::: warning Bitbucket Cloud only (when re-enabled)
Only Bitbucket Cloud (`bitbucket.org`) was targeted. Bitbucket Data Center / Server (the self-hosted product) uses a different REST surface and is not covered.
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

::: info `BITBUCKET_EMAIL` is for the API client only
`BITBUCKET_EMAIL` is what the monorel binary uses as the Basic-auth username when calling Bitbucket's REST API. Manual `git push` from a workstation over HTTPS uses Bitbucket's account *username* (visible in the repo's HTTPS clone URL) as the Basic-auth user, NOT the Atlassian email. The two are different fields on a Bitbucket account. This only affects operator-driven pushes from a laptop; pushes inside Bitbucket Pipelines use auto-provided runner credentials and don't need either form configured manually.
:::

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

When monorel needs the Bitbucket workspace username (not the email on the API token), it probes `GET /2.0/user` lazily on the first call that needs it, reads the `username` field, and caches the result on the client. You don't supply a username; it's resolved automatically from the token's identity.

This is why the API token needs `read:account`: without it, the probe fails when monorel reaches a code path that needs the username.

## Workspace plan acceptance

Bitbucket Cloud workspaces created (or upgraded) since the September 2024 plan migration require an explicit workspace-plan acceptance step before push, MR, and release-tag operations succeed. The symptom is HTTP 402 (`Payment Required`) on the first push from CI, even on free-tier workspaces, even when billing is current.

Resolution: a workspace admin visits `https://bitbucket.org/<workspace>/workspace/settings/plans` and clicks the plan-acceptance button (the page presents it the first time the workspace is loaded after migration). It's a one-time action per workspace and unrelated to billing.

If you see `402 Payment Required` from a `monorel preview --upsert` or `monorel tag` step, hit that URL first and re-run the workflow. monorel surfaces the upstream HTTP status verbatim so the error is easy to recognize.

## Enable Pipelines on the repo

Bitbucket Cloud disables Pipelines on every repo by default. Before the example workflow can fire, enable Pipelines under Repository settings → Pipelines → Settings → On.

::: warning Workspace 2FA enforces the API path
If your workspace requires 2FA on the user account, the API call (`PUT /2.0/repositories/<ws>/<slug>/pipelines_config {"enabled": true}`) returns `403` with `account-service.user.2fa-required`. The Bitbucket UI path doesn't have this gate; click through it once in the browser and the repo is good to go. Repository variables (`BITBUCKET_EMAIL` / `BITBUCKET_TOKEN`) can be set via API regardless of 2FA status.
:::

## Workflows

monorel doesn't ship a Bitbucket-specific CI wrapper. The simplest setup uses Bitbucket Pipelines with the published Docker image (`ghcr.io/disaresta-org/monorel`). One pipeline drives the entire lifecycle: on every push to the default branch, it runs [`monorel auto`](/cli-reference#monorel-auto), which detects whether HEAD is the merge of monorel's release PR and dispatches accordingly:

- **Feature commit** (the common case): stage the always-open release PR's diff via `monorel apply` + `monorel preview --upsert`. If the planner has nothing to apply, any open release PR is closed.
- **Release-PR merge**: run `monorel tag` + `git push --follow-tags` + `monorel publish` to create per-package tags. (`monorel publish` is a no-op on Bitbucket Cloud; see [No native releases](#no-native-releases-changelog-md-is-canonical) below.)

Detection uses HEAD's `monorel-Release:` commit-body trailer OR the provider's `FindPRByMergeCommit` API returning a PR whose source branch is `monorel/release`. Either signal alone is sufficient, so the dispatch works regardless of merge strategy.

`bitbucket-pipelines.yml`:

<!--@include: ../_partials/bitbucket-pipelines-yml.md-->

Setup:

1. **Add `BITBUCKET_EMAIL` and `BITBUCKET_TOKEN` as repository variables** under Repository settings → Repository variables. `BITBUCKET_EMAIL` is the Atlassian-account email that owns the token; `BITBUCKET_TOKEN` is the API token string. monorel reads both from the environment when constructing the Bitbucket API client. Bitbucket Pipelines exports repository variables to the runner's environment automatically, so no explicit `env:` mapping in the pipeline is needed. Pipelines also injects git credentials for pushes back to the same repo.
2. **Pick a merge strategy.** Squash, fast-forward, and merge-commit all work via the API signal; pick whichever matches your repo's convention. Configure under Repository settings → Branch restrictions / Merge strategies.
3. **Push the `bitbucket-pipelines.yml` to the default branch.** The pipeline fires on every push to `main`; it runs `monorel auto` and opens the always-open release PR once you have a `.changeset/<name>.md`.

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

`monorel auto` detects a release-PR merge using two independent signals OR'd together:

- **Trailer signal** (fast path): HEAD's commit body contains a `monorel-Release:` trailer. Hits when fast-forward or merge-commit propagated the source body.
- **API signal** (network): provider's `FindPRByMergeCommit` returns a PR whose source branch is `monorel/release`. Hits when the trailer is missing for any reason.

Either signal alone is enough. So all three Bitbucket merge strategies work for the release PR; pick whichever matches your repo's convention:

- **Fast-forward**: preserves the head commit verbatim. Trailers reach `main` unchanged. Both signals hit.
- **Merge commit**: creates a merge commit with the head commit as a parent. The trailer reaches HEAD via the merged commit; the API signal also hits. Both signals hit.
- **Squash merge**: rewrites the body, dropping the trailers from the merge commit. The trailer signal misses; the API signal hits. `monorel tag` falls back to the universal trailers source (a `<!-- monorel-trailers ... -->` HTML comment that `monorel preview` writes into the PR body) so tag derivation still works.

Squash-merge adds an API round-trip for the trailers fallback and a small risk that a contributor edited the PR body and removed the comment block. Fast-forward and merge-commit avoid both. Pick whichever matches your team's habit; detection itself is robust to all three.

## No native releases: CHANGELOG.md is canonical

Bitbucket Cloud has no first-class "Release" concept analogous to GitHub Releases or GitLab Releases (a tag-attached object with a body, assets, and a pre-release flag). The closest equivalent is the tag itself plus the per-package `CHANGELOG.md` entry that `monorel apply` writes alongside.

Consequence: `monorel publish` does nothing on Bitbucket. It's safe to run (the command exits successfully without making any API calls), but the canonical record of what changed in each release is the `CHANGELOG.md` entry. `monorel auto` invokes `publish` on the release path for symmetry with the other providers; on Bitbucket it's just a no-op step.

::: warning `monorel publish` still requires the auth env vars
The publish command constructs the provider client before noticing it has nothing to do, so `BITBUCKET_EMAIL` and `BITBUCKET_TOKEN` must still be set in the environment when it runs. Constructor validation (including the `/2.0/user` username probe) fires before the no-op check; missing or invalid credentials surface as a constructor error, not a clean exit.
:::

If you need release notes shown in a UI, link the per-package CHANGELOG file from the repo's README or your project's docs site.

## Branch protection

Bitbucket calls these "branch restrictions." Two points:

- **`monorel/release` should NOT have a `Prevent rewriting branch history` restriction.** The pipeline force-pushes to that branch on every feature-branch run. Either don't add a restriction covering `monorel/release`, or exempt the bot user / token.
- **Any merge strategy works for the default branch's release PR** (see [Merge strategy](#merge-strategy-any-of-the-three-works)). Squash-merge adds a network hop for the trailers fallback; fast-forward and merge-commit avoid it.

## Tokens and required status checks

Bitbucket Cloud has no analogue to GitHub's required-status-check anti-recursion rule. Pipelines triggered by the always-open release PR run with the same Pipelines OAuth token as any other pipeline; there's no "PRs created by the auto-injected token don't trigger checks" pitfall to escalate around. If your repo enforces required reviews on the default branch, the standard PR-review approval flow applies; no special PAT escalation is needed for monorel itself.

## Troubleshooting

### `provider: unknown provider "bitbucket"`

monorel binary doesn't recognize the `bitbucket` provider name. Upgrade with `go install monorel.disaresta.com/cmd/monorel@latest` or use a newer Docker image tag.

### `bitbucket: probe username: 401 Unauthorized`

The API token doesn't have Bitbucket scopes (or doesn't have `read:account`). Regenerate via the **Create API token with scopes** flow at `id.atlassian.com/manage-profile/security/api-tokens` and grant the scopes listed under [Auth](#auth-two-environment-variables).

### `bitbucket: ...: 402 Payment Required`

The workspace plan hasn't been accepted yet. Visit `https://bitbucket.org/<workspace>/workspace/settings/plans` as a workspace admin and click through the plan-acceptance prompt. See [Workspace plan acceptance](#workspace-plan-acceptance).

### `bitbucket: ...: 403 Forbidden`

The token's scopes are insufficient for the operation. The most common gap is `write:pullrequest:bitbucket` (needed to open / update the always-open release PR). Regenerate the token with all scopes listed under [Auth](#auth-two-environment-variables).

### `monorel tag` returns `ErrNoReleaseCommit` despite the merge happening

HEAD's commit body has no `monorel-Release:` trailers AND the merged PR's body has no `<!-- monorel-trailers ... -->` comment block (so the universal fallback also missed). The fallback fails when both sources are gone, which typically means a contributor manually edited the release-PR body and removed the comment block before merge. Recovery: hand-create the tags pointing at the merge commit, push, then re-run. See the [tag-recovery FAQ](/faq#a-release-pr-merged-without-monorel-release-body-trailers-and-monorel-tag-returned-errnoreleasecommit-what-now) for the exact commands.
