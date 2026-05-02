# Examples

Reference monorel setups for each supported provider. Each subdirectory is a minimal working configuration: `monorel.toml` + workflow files + `.changeset/README.md`. Copy the files you need into your repo.

| Provider | Path | Notes |
|---|---|---|
| GitHub | [`github/`](github/) | Uses the `disaresta-org/monorel/ci/github` composite action. Two workflow files. |
| Gitea / Forgejo | [`gitea/`](gitea/) | Reuses the GitHub action wrapper on Gitea Actions. Same two workflow files plus `provider.host` set in `monorel.toml`. |
| GitLab | [`gitlab/`](gitlab/) | Single `.gitlab-ci.yml` using the published Docker image (`ghcr.io/disaresta-org/monorel`). Project must be set to fast-forward merge. |

Each example targets a hypothetical two-package monorepo: a root module plus one sub-module under `transports/foo/`. Adapt the package names, paths, and tag prefixes to your own repo.

For the full walkthrough see [Getting Started](https://monorel.disaresta.com/getting-started); for provider-specific setup details see the integration pages: [GitHub](https://monorel.disaresta.com/integrations/github), [Gitea / Forgejo](https://monorel.disaresta.com/integrations/gitea), [GitLab](https://monorel.disaresta.com/integrations/gitlab).
