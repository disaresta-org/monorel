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
2. Set the project's merge method to **Fast-forward** under Settings → Merge requests → Merge method. Required so the `monorel-Release:` body trailers reach `main` post-merge.
3. Push the `.gitlab-ci.yml` to your default branch.

The `release-pr` job opens / updates the always-open release MR; merging it triggers the `release` job which creates tags and publishes Releases.

See [Getting Started](https://monorel.disaresta.com/getting-started) and the [GitLab integration page](https://monorel.disaresta.com/integrations/gitlab) for the full walkthrough.
