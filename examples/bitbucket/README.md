# Bitbucket example

Minimal monorel setup for a Bitbucket Cloud-hosted project with two packages (root + one sub-module under `transports/foo/`). Uses Bitbucket Pipelines with the published Docker image (`ghcr.io/disaresta-org/monorel`).

```
bitbucket/
├── monorel.toml
├── .changeset/README.md
└── bitbucket-pipelines.yml
```

Copy these files into your repo, replace `your-workspace` / `your-repo` with your workspace slug and repo slug, then:

1. Generate an Atlassian API token **with Bitbucket scopes** at [`id.atlassian.com/manage-profile/security/api-tokens`](https://id.atlassian.com/manage-profile/security/api-tokens) (the plain "Create API token" button generates a token without Bitbucket scopes; use **Create API token with scopes** instead). Required scopes: `read:repository:bitbucket`, `write:repository:bitbucket`, `read:pullrequest:bitbucket`, `write:pullrequest:bitbucket`, `read:account`.
2. Add `BITBUCKET_EMAIL` (the Atlassian-account email that owns the token) and `BITBUCKET_TOKEN` (the token string) under Repository settings → Repository variables. monorel reads both from the environment to construct the Bitbucket API client.
3. **Accept the workspace plan** at `https://bitbucket.org/<workspace>/workspace/settings/plans` if you haven't already. Workspaces created or migrated since September 2024 require this one-time acceptance step or every push will return HTTP 402 Payment Required.
4. Push `bitbucket-pipelines.yml` to your default branch.

The pipeline runs `monorel auto` on every push to the default branch. `monorel auto` queries Bitbucket's REST API to detect whether HEAD is the merge of monorel's release PR; if so it runs `monorel tag` + `git push --follow-tags` + `monorel publish`, otherwise it runs the feature path (`monorel apply` + `git push -f` + `monorel preview --upsert`).

Bitbucket Cloud has no first-class Release concept, so `monorel publish` is a no-op on Bitbucket; per-package `CHANGELOG.md` is the canonical release-notes source.

See [Getting Started](https://monorel.disaresta.com/getting-started) and the [Bitbucket integration page](https://monorel.disaresta.com/integrations/bitbucket) for the full walkthrough.
