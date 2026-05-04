# Examples

Reference monorel setups for each supported provider. Each subdirectory is a minimal working configuration: `monorel.toml` + workflow files + `.changeset/README.md`. Copy the files you need into your repo.

| Provider | Path | Notes |
|---|---|---|
| GitHub | [`github/`](github/) | Uses the `disaresta-org/monorel/ci/github` composite action. Single workflow file. |
| Gitea / Forgejo | [`gitea/`](gitea/) | Reuses the GitHub action wrapper on Gitea Actions. Single workflow file plus `provider.host` set in `monorel.toml`. |
| GitLab | [`gitlab/`](gitlab/) | Single `.gitlab-ci.yml` using the published Docker image (`ghcr.io/disaresta-org/monorel`). |

Each example targets a hypothetical two-package monorepo: a root module plus one sub-module under `transports/foo/`. Adapt the package names, paths, and tag prefixes to your own repo. The single workflow / pipeline file in each example runs [`monorel auto`](https://monorel.disaresta.com/cli-reference#monorel-auto), which detects whether HEAD is the merge of the always-open release PR and dispatches accordingly.

For the full walkthrough see [Getting Started](https://monorel.disaresta.com/getting-started); for provider-specific setup details see the integration pages: [GitHub](https://monorel.disaresta.com/integrations/github), [Gitea / Forgejo](https://monorel.disaresta.com/integrations/gitea), [GitLab](https://monorel.disaresta.com/integrations/gitlab).

The `bitbucket/` directory contains a Bitbucket Cloud reference setup the provider code supports but which we have not been able to verify end-to-end against a live Pipelines runner. See [`bitbucket/README.md`](bitbucket/README.md) for the caveats.
