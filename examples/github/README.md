# GitHub example

Minimal monorel setup for a GitHub-hosted Go monorepo with two packages (root + one sub-module under `transports/foo/`).

```
github/
├── monorel.toml
├── .changeset/README.md
└── .github/workflows/
    ├── release-pr.yml
    └── release.yml
```

Copy these files into your repo, replace `acme/widget` with your owner/repo and the package keys with your own (`monorel init` will scaffold this from your `go.mod` files), then commit and push to `main`. The `release-pr` workflow opens / updates the always-open release PR; merging it triggers `release.yml` which creates tags and publishes GitHub Releases.

See [Getting Started](https://monorel.disaresta.com/getting-started) and the [GitHub integration page](https://monorel.disaresta.com/integrations/github) for the full walkthrough.

## Branch protection

If your default branch enforces required status checks, the bot-created release PR won't trigger them (GitHub's anti-recursion rule). Switch the workflow's token to a PAT or GitHub App token. See [Tokens and required status checks](https://monorel.disaresta.com/integrations/github#tokens-and-required-status-checks).
