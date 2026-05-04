# GitLab example

Minimal monorel setup for a GitLab.com or self-hosted GitLab project with two packages (root + one sub-module under `transports/foo/`). Uses GitLab CI with the published Docker image (`ghcr.io/disaresta-org/monorel`).

```
gitlab/
├── monorel.toml
├── .changeset/README.md
└── .gitlab-ci.yml
```

Copy these files into your repo, replace `team/platform/widget` with your project path, then:

1. Add `MONOREL_GITLAB_TOKEN` as a CI/CD variable under Settings → CI/CD → Variables. Use a personal or project access token with `api` scope.
2. Push the `.gitlab-ci.yml` to your default branch.

The single `monorel` job runs `monorel auto` on every push to the default branch. `monorel auto` detects whether HEAD is the merge of the always-open release MR (via the GitLab API) and dispatches: a feature commit refreshes the release MR; a release-MR merge cuts tags and publishes GitLab Releases. Squash, merge-commit, and fast-forward merge strategies all work because the API signal sees through any of them.

See [Getting Started](https://monorel.disaresta.com/getting-started) and the [GitLab integration page](https://monorel.disaresta.com/integrations/gitlab) for the full walkthrough.
