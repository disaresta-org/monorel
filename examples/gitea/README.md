# Gitea / Forgejo example

Minimal monorel setup for a Gitea or Forgejo project with two packages (root + one sub-module under `transports/foo/`). Uses Gitea Actions (Gitea 1.21+ / Forgejo 1.21+) which is GitHub Actions-compatible.

```
gitea/
├── monorel.toml
├── .changeset/README.md
└── .gitea/workflows/
    ├── release-pr.yml
    └── release.yml
```

Copy these files into your repo, set `provider.host` to your instance (or `codeberg.org`, etc.), replace `acme/widget` with your owner/repo, then commit and push to `main`. The `release-pr` workflow opens / updates the always-open release MR; merging it triggers `release.yml`.

The `env: GITEA_TOKEN: ${{ secrets.GITHUB_TOKEN }}` mapping is necessary because monorel reads `GITEA_TOKEN` and Gitea Actions auto-injects its token under the GHA-compatible `GITHUB_TOKEN` name.

For Forgejo, the same files work: Forgejo maintains API compatibility with Gitea.

See [Getting Started](https://monorel.disaresta.com/getting-started) and the [Gitea / Forgejo integration page](https://monorel.disaresta.com/integrations/gitea) for the full walkthrough.
