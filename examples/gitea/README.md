# Gitea / Forgejo example

Minimal monorel setup for a Gitea or Forgejo project with two packages (root + one sub-module under `transports/foo/`). Uses Gitea Actions (Gitea 1.21+ / Forgejo 1.21+) which is GitHub Actions-compatible.

```
gitea/
├── monorel.toml
├── .changeset/README.md
└── .gitea/workflows/
    └── release.yml
```

Copy these files into your repo, set `provider.host` to your instance (or `codeberg.org`, etc.), replace `acme/widget` with your owner/repo, then commit and push to `main`. The single workflow runs `monorel auto` on every push, which detects whether HEAD is the merge of the always-open release PR and dispatches accordingly.

The action wrapper takes the token via the `with: token:` input. The example wires it to `${{ secrets.GITHUB_TOKEN }}`, which Gitea Actions auto-injects for GitHub-Actions YAML compatibility. The action then exports the token under both `GITHUB_TOKEN` and `GITEA_TOKEN` env vars for the underlying `monorel auto` invocation, so monorel reads the name matching the configured Gitea provider.

For Forgejo, the same files work: Forgejo maintains API compatibility with Gitea.

See [Getting Started](https://monorel.disaresta.com/getting-started) and the [Gitea / Forgejo integration page](https://monorel.disaresta.com/integrations/gitea) for the full walkthrough.
