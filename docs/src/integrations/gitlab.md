---
title: GitLab
description: "Wire up monorel against a GitLab project: configuration, tokens, GitLab CI workflow, branch protection."
---

# GitLab

monorel's GitLab provider talks to GitLab.com or any self-hosted GitLab Community / Enterprise Edition instance via the standard GitLab REST API. Tested against GitLab.com via `gitlab.com/gitlab-org/api/client-go` (the official Go SDK).

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

::: info Sub-groups and `monorel init` auto-detection
`monorel init` auto-detects `owner` and `repo` from `git config remote.origin.url`. For sub-group projects, the auto-detected split is leading-component: a URL like `https://gitlab.com/team/platform/widget.git` becomes `owner = "team"`, `repo = "platform/widget"`. Both shapes work because the provider concatenates `owner + "/" + repo` to form the project path; pick whichever you find easier to read in your `monorel.toml`. The example in [`examples/gitlab/`](https://github.com/disaresta-org/monorel/tree/main/examples/gitlab) uses the canonical form (`owner = "team/platform"`, `repo = "widget"`).
:::

Run `monorel validate` to confirm the config loads cleanly.

## Token

monorel reads the auth token from the `GITLAB_TOKEN` environment variable (falls back to `CI_JOB_TOKEN` if `GITLAB_TOKEN` is empty, useful in pipelines).

Generate a personal access token under **User Settings → Access Tokens** with the `api` scope. The narrower `read_api` / `write_repository` scopes don't compose cleanly for monorel's set of operations (MR write + Releases write + project read all want `api`).

For self-hosted instances or sub-group projects, use a project access token or group access token instead of a personal one. Same scope (`api`).

## Workflows

monorel doesn't ship a GitLab-specific CI wrapper. The simplest setup uses GitLab CI with the published Docker image (`ghcr.io/disaresta-org/monorel`). One job drives the entire lifecycle: on every push to the default branch, it runs [`monorel auto`](/cli-reference#monorel-auto), which detects whether HEAD is the merge of monorel's release MR and dispatches accordingly:

- **Feature commit** (the common case): stage the always-open release MR's diff via `monorel apply` + `monorel preview --upsert`. If the planner has nothing to apply, any open release MR is closed.
- **Release-MR merge**: run `monorel tag` + `git push --follow-tags` + `monorel publish` to create per-package tags and one Release per tag.

Detection uses HEAD's `monorel-Release:` commit-body trailer OR an API lookup that finds the PR whose merge produced HEAD and confirms its source branch is `monorel/release`. Either signal alone is sufficient, so the dispatch works regardless of merge method.

`.gitlab-ci.yml`:

<!--@include: ../_partials/gitlab-ci-yml.md-->

Setup:

1. **Add `MONOREL_GITLAB_TOKEN` as a CI/CD variable** under Settings → CI/CD → Variables. Use a personal or project access token with `api` scope. Mark it Masked but NOT Protected (so it's available on the bot-managed `monorel/release` branch too).
2. **Pick a merge method that suits your repo's convention.** Detection is API-based; fast-forward, merge-commit, and rebase all work. (Fast-forward preserves the trailer in the merge commit body and avoids the universal-fallback round-trip; the others rely on the fallback.)
3. **Push the `.gitlab-ci.yml` to the default branch.** The first push that includes the file runs `monorel auto`; once you have a `.changeset/<name>.md`, the always-open MR opens.

Run `monorel doctor` as a separate pre-merge check on every MR if you want the stale-branch + squash-merge revival diagnostic; it's a separate concern from `monorel auto`. See [`monorel doctor`](/cli-reference#monorel-doctor).

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

### Skipping CI on chore(release) commits

The release commit `chore(release): ...` (created when the always-open release MR is merged) updates module `go.mod` files to require new in-plan sibling versions. monorel's release pipeline creates and pushes the matching tags on the same push. Any *other* job that fires on that push and resolves Go module versions (lint, test, deploy) races the tag push and may transiently fail with:

```
go: example.com/foo/v2: reading example.com/foo/go.mod at revision v2.1.0: unknown revision v2.1.0
```

The release succeeds and the tags get pushed; the racing job's red mark stays in the UI. Skip the racing job on `chore(release):` commits with a `rules:` clause:

```yaml
test:
  rules:
    - if: '$CI_PIPELINE_SOURCE == "merge_request_event"'
    - if: '$CI_COMMIT_TITLE =~ /^chore\(release\):/'
      when: never
    - when: on_success
  script:
    - go test ./...
```

The two-clause shape: allow merge-request runs through unconditionally, then explicitly drop `chore(release):` push runs on the default branch, then default to running.

monorel's own release pipeline does NOT need this filter. On a `chore(release):` push it's the pipeline doing the tagging; on every other push `monorel auto` falls through to the upsert path.

## Branch protection

`monorel auto` force-pushes to `monorel/release` on every feature-branch run. Two GitLab-specific points:

- **`monorel/release` should NOT be a protected branch.** GitLab's Protected Branches feature blocks force-push by default. Either don't protect the branch, or add a wildcard rule (`monorel/release` excluded) under Settings → Repository → Protected branches.
- **Pick whichever merge method matches your repo's convention.** Detection is API-based; `merge`, `merge_train`, `rebase_merge`, and fast-forward all work. `monorel tag` falls back to the universal trailers source (a `<!-- monorel-trailers ... -->` HTML comment in the merged MR's body) when the merge method drops the commit-body trailer, so tag creation works regardless of method.

## Tokens and required status checks

GitLab doesn't have an exact analog of GitHub's `pull_request`-trigger anti-recursion rule, but the equivalent comes up via "Pipelines for merged results" / "Required status checks" features and via group-level / project-level merge approval rules.

If your project uses **merge approvals** that block merging the release MR until human approval, you can:

- Use a **CODEOWNERS** rule that auto-approves `.changeset/*.md` and `CHANGELOG.md` by listing a service-account user on those paths.
- Configure the **Project access token** (used in `MONOREL_GITLAB_TOKEN`) to be associated with a Maintainer-role bot account so the token can self-approve via GitLab's API.

For "Pipelines must succeed" rules, the bot-created MR's pipeline runs the same as any other MR's. If you find your release MR sitting on pending pipelines, check the project's CI/CD Pipelines page (usually a missing or non-running pipeline rather than an actual block).

For complex setups (cross-project pipelines, dynamic environments), consider replacing `MONOREL_GITLAB_TOKEN` with a [GitLab App token](https://docs.gitlab.com/api/oauth2/) issued to a dedicated service account. Same shape as GitHub's PAT-vs-App story, [GitHub page](/integrations/github#tokens-and-required-status-checks).

## Troubleshooting

### `provider: unknown provider "gitlab"`

monorel binary doesn't recognize the `gitlab` provider name. Upgrade with `go install monorel.disaresta.com/cmd/monorel@latest` or use a newer Docker image tag.

### `gitlab: build client: ...`

The SDK's `NewClient` rejected the supplied options. The most common cause is a malformed `host` value (`provider.host` in `monorel.toml`). The SDK doesn't make a network call at construction; this error is purely option-application.

### `gitlab: get project ...: ...` (or any other method-prefixed gitlab error)

The first network call against the configured host failed. Likely cause: the host is unreachable from the runner. Check:

- Spelling of `host` in `monorel.toml`.
- Whether the runner can reach the host (firewall, DNS).
- For self-hosted: whether the API path is at the default `/api/v4/` (the SDK assumes so).

### `monorel tag` returns `ErrNoReleaseCommit`

The merge commit on the default branch doesn't have `monorel-Release:` trailers in its body AND the merged MR's body has no `<!-- monorel-trailers ... -->` HTML comment (so the universal fallback also missed). Either someone edited the comment out before merge, or the release MR was opened by an older monorel that didn't write the comment. Recovery: hand-create the tags pointing at the merge commit and run `monorel publish` against the pushed tags.

### Force-push to `monorel/release` is rejected

`monorel/release` is set as a protected branch. Either remove the protection or exclude this ref via a wildcard rule.

### MR sits on "Pipelines must succeed" forever

GitLab CI doesn't have GitHub Actions' anti-recursion rule, but pipelines on the bot-created MR can still appear to hang if:

- The CI runner is offline or oversubscribed.
- The project has [Pipelines for Merged Results](https://docs.gitlab.com/ci/pipelines/merged_results_pipelines/) enabled and the merge result fails to compute (rare).

Check the project's CI/CD → Pipelines page for the actual status; usually a transient runner issue.

### `tidy in <sub-module>: exit status 1` with `module lookup disabled by GOPROXY=off`

Indicates a stale `monorel` binary. Earlier builds did not pin `GOMODCACHE` for the offline-tidy subprocess, so on systems where `GOMODCACHE` is derived from `GOPATH` (notably `golang:alpine` images and cold-cache CI runners) the seed and the read pointed at different paths and tidy missed the cached in-plan sibling. Upgrade `monorel` to the latest release.

### `403: You are not allowed to push code to this project`

The runner is using `CI_JOB_TOKEN` (read-only) on the `origin` remote. The example `.gitlab-ci.yml` includes a `git remote set-url origin` step that swaps in `MONOREL_GITLAB_TOKEN`; if you adapted the example and dropped that step, add it back. A typical `monorel auto` run pushes both `monorel/release` and per-package tags, which all need write access through the same remote.
