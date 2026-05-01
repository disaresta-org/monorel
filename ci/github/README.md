# monorel GitHub Action

Composite action that downloads the monorel binary for the runner's OS+arch and invokes it.

## Inputs

| Input | Default | Description |
|-------|---------|-------------|
| `command` | required | `pr` (upsert / close the always-open release PR via `monorel preview --upsert`) or `release` (cut a release via `monorel release [--publish]` and push tags). |
| `version` | `latest` | monorel version to run (e.g. `v1.2.3`). |
| `config` | `monorel.toml` | Path to the config file. |
| `token` | the workflow's `GITHUB_TOKEN` | Token for forge API calls. Needs `contents: write` and `pull-requests: write`. |
| `publish` | `true` | When `command=release`, also create one GitHub Release per tag. Requires tags to be pushed (the action does this automatically). |

## Workflows

### `release-pr.yml`: maintain the always-open release PR

```yaml
name: release-pr
on:
  push:
    branches: [main]

permissions:
  contents: write
  pull-requests: write

jobs:
  release-pr:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
      - uses: disaresta-org/monorel/ci/github@v1
        with:
          command: pr
```

### `release.yml`: cut a release

This workflow runs on a manual dispatch. v1 of the action ships the simpler "release on demand" flow; the always-open PR's auto-merge wiring is a follow-up.

```yaml
name: release
on:
  workflow_dispatch:

permissions:
  contents: write

jobs:
  release:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
      - uses: disaresta-org/monorel/ci/github@v1
        with:
          command: release
```

## Notes

- The action runs as a composite action that shells out to `monorel`. Calling it via `disaresta-org/monorel/ci/github@vX.Y.Z` pins both the action and the binary version (the action resolves `inputs.version` via the GitHub Releases API).
- For self-hosted GitHub Enterprise installations, set `monorel.toml`'s `[forge].host` field; the action auto-passes `GITHUB_TOKEN` and the binary picks up the host from config.
- The `pr` command currently runs `monorel preview --upsert` only. It does not yet stage CHANGELOG edits on a release branch; the upserted PR body shows the rendered plan as a preview but does not include file diffs. The action will gain branch staging in a follow-up release.
