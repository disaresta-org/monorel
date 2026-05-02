---
title: GitLab
description: "Wire up monorel against a GitLab project: configuration, tokens, GitLab CI workflow, branch protection."
---

# GitLab

monorel's GitLab provider talks to GitLab.com or any self-hosted GitLab Community / Enterprise Edition instance via the standard GitLab REST API. Tested against GitLab.com via `gitlab.com/gitlab-org/api/client-go` (the official Go SDK).

::: info Available in v0.7+
The GitLab provider landed in monorel v0.7.0. Earlier versions support `github` and `gitea` only.
:::

::: tip Example
Working reference setup at [`examples/gitlab/`](https://github.com/disaresta-org/monorel/tree/main/examples/gitlab). Copy the files you need.
:::

## Configuration

`monorel.toml`:

```toml
[provider]
name  = "gitlab"
host  = "gitlab.com"           # or your self-hosted instance
owner = "team/platform"        # may contain slashes for sub-groups
repo  = "widget"
```

| Field | Notes |
|-------|-------|
| `name` | Must be `"gitlab"`. Set explicitly: the default (`"github"`) won't work against a GitLab instance. |
| `host` | Optional. Defaults to `gitlab.com`. Accepts a bare hostname (`gitlab.example.com`) or a fully-qualified URL (`https://gitlab.example.com`, `http://localhost:8080`). |
| `owner` | The user, group, or sub-group path that owns the project. May contain slashes for nested sub-groups (e.g. `team/platform`). |
| `repo` | The project's path component (the last segment of the project's full path). |

Run `monorel validate` to confirm the config loads cleanly.

## Token

monorel reads the auth token from the `GITLAB_TOKEN` environment variable (falls back to `CI_JOB_TOKEN` if `GITLAB_TOKEN` is empty, useful in pipelines).

Generate a personal access token under **User Settings → Access Tokens** with the `api` scope. The narrower `read_api` / `write_repository` scopes don't compose cleanly for monorel's set of operations (MR write + Releases write + project read all want `api`).

For self-hosted instances or sub-group projects, use a project access token or group access token instead of a personal one. Same scope (`api`).

## Workflows

monorel doesn't ship a GitLab-specific CI wrapper. The simplest setup uses GitLab CI with the published Docker image (`ghcr.io/disaresta-org/monorel`). Two-stage pipeline: `release-pr` keeps the always-open MR up to date on every push to the default branch; `release` cuts the release once the MR is merged.

`.gitlab-ci.yml`:

<!--@include: ../_partials/gitlab-ci-yml.md-->

Setup:

1. **Add `MONOREL_GITLAB_TOKEN` as a CI/CD variable** under Settings → CI/CD → Variables. Use a personal or project access token with `api` scope. Mark it Masked but NOT Protected (so it's available on the bot-managed `monorel/release` branch too).
2. **Set the project's merge method to Fast-forward** under Settings → Merge requests → Merge method. The release MR's commit body carries `monorel-Release:` trailers that `monorel tag` reads post-merge; fast-forward preserves the commit verbatim. The default `merge` method creates a merge commit whose body wouldn't carry the trailers, and `monorel tag` would return `ErrNoReleaseCommit`.
3. **Push the `.gitlab-ci.yml` to the default branch.** The first push that includes the file triggers the `release-pr` job; once you have a `.changeset/<name>.md`, the always-open MR opens.

### Local CLI (no CI)

Same shape as the [Working without CI](/getting-started#working-without-ci) section of Getting Started, with `GITLAB_TOKEN` instead of `GITHUB_TOKEN`:

```sh
GITLAB_TOKEN=... monorel release
git push --follow-tags
GITLAB_TOKEN=... monorel publish
```

Works on a contributor's laptop or any non-GitLab-CI runner. The release commit, tags, and Releases all land the same way.

### External CI

monorel is a single static binary; any CI that can run a shell command can run it. Pattern is the same as the local CLI flow: download the binary (or use the Docker image), set `GITLAB_TOKEN`, run `monorel release` + `git push --follow-tags` + `monorel publish`. There's no monorel-specific machinery to install.

## Branch protection

The `release-pr` job force-pushes to `monorel/release` on every run. Two GitLab-specific points:

- **`monorel/release` should NOT be a protected branch.** GitLab's Protected Branches feature blocks force-push by default. Either don't protect the branch, or add a wildcard rule (`monorel/release` excluded) under Settings → Repository → Protected branches.
- **The default branch's merge method must be Fast-forward** (see Workflows step 2 above). Other merge methods (`merge` / `merge_train` / `rebase_merge`) collapse the trailers into a merge commit body that `monorel tag` doesn't see.

The `release-pr` job's rules also exclude commits whose subject starts with `chore(release):` (the merge commit) so the workflow doesn't churn the just-merged MR.

## Tokens and required status checks

GitLab doesn't have an exact analog of GitHub's `pull_request`-trigger anti-recursion rule, but the equivalent comes up via "Pipelines for merged results" / "Required status checks" features and via group-level / project-level merge approval rules.

If your project uses **merge approvals** that block merging the release MR until human approval, you can:

- Use a **CODEOWNERS** rule that auto-approves `.changeset/*.md` and `CHANGELOG.md` by listing a service-account user on those paths.
- Configure the **Project access token** (used in `MONOREL_GITLAB_TOKEN`) to be associated with a Maintainer-role bot account so the token can self-approve via GitLab's API.

For "Pipelines must succeed" rules, the bot-created MR's pipeline runs the same as any other MR's. If you find your release MR sitting on pending pipelines, check the project's CI/CD Pipelines page (usually a missing or non-running pipeline rather than an actual block).

For complex setups (cross-project pipelines, dynamic environments), consider replacing `MONOREL_GITLAB_TOKEN` with a [GitLab App token](https://docs.gitlab.com/api/oauth2/) issued to a dedicated service account. Same shape as GitHub's PAT-vs-App story, [GitHub page](/integrations/github#tokens-and-required-status-checks).

## Troubleshooting

### `provider: unknown provider "gitlab"`

monorel binary is older than v0.7.0 (when the GitLab provider landed). Upgrade with `go install monorel.disaresta.com/cmd/monorel@latest` or use a newer Docker image tag.

### `gitlab: connect: ...`

The GitLab SDK's first API call failed. Likely cause: the host is unreachable from the runner. Check:

- Spelling of `host` in `monorel.toml`.
- Whether the runner can reach the host (firewall, DNS).
- For self-hosted: whether the API path is at the default `/api/v4/` (the SDK assumes so).

### `monorel tag` returns `ErrNoReleaseCommit`

The merge commit on the default branch doesn't have `monorel-Release:` trailers in its body. Most likely cause: the project's merge method isn't Fast-forward, so the trailers are on the parent commit, not HEAD. Switch under Settings → Merge requests → Merge method, then re-run the release pipeline.

### Force-push to `monorel/release` is rejected

`monorel/release` is set as a protected branch. Either remove the protection or exclude this ref via a wildcard rule.

### MR sits on "Pipelines must succeed" forever

GitLab CI doesn't have GitHub Actions' anti-recursion rule, but pipelines on the bot-created MR can still appear to hang if:

- The CI runner is offline or oversubscribed.
- The project has [Pipelines for Merged Results](https://docs.gitlab.com/ci/pipelines/merged_results_pipelines/) enabled and the merge result fails to compute (rare).

Check the project's CI/CD → Pipelines page for the actual status; usually a transient runner issue.
