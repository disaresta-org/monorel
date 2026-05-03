# monorel GitHub Action

Composite action that downloads the monorel binary for the runner's OS+arch and invokes it.

## Inputs

| Input | Default | Description |
|-------|---------|-------------|
| `command` | required | `pr` (upsert / close the always-open release PR via `monorel preview --upsert`), `release` (run the full local→push→publish pipeline), or `doctor` (run `monorel doctor`; exits non-zero on error-severity findings). |
| `version` | `latest` | monorel version to run (e.g. `v1.2.3`). |
| `config` | `monorel.toml` | Path to the config file. |
| `token` | the workflow's `GITHUB_TOKEN` | Token for provider API calls. Needs `contents: write` and `pull-requests: write`. The `doctor` command needs no token; `contents: read` alone is sufficient. |

The `release` command runs three monorel invocations in order:

1. `monorel release` — local file mutations + commit + tag.
2. `git push --follow-tags` — publish commits and tags to the remote.
3. `monorel publish` — create one provider release per tag at HEAD; body sourced from each package's CHANGELOG entry.

The split exists because most providers validate that the tag exists on the remote before allowing a release to be created against it. Bundling 1 and 3 in one process would race that validation.

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
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
      - uses: disaresta-org/monorel/ci/github@v1
        with:
          command: pr
```

### `release.yml`: cut a release

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
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
      - uses: disaresta-org/monorel/ci/github@v1
        with:
          command: release
```

## Requirements

- **`go` on `PATH`.** monorel's `apply` step runs `go mod tidy` (offline, against a seeded local cache) in every released sub-module that requires an in-plan sibling, so the release commit's `go.sum` is canonically clean for the proxy-published state. The runner needs a `go` binary whose version satisfies every released module's `go` directive — use `actions/setup-go@v5` with `go-version-file: go.mod` (or pin `go-version` explicitly) to install the right version. GitHub-hosted runners include a recent Go by default, but pinning is safer than relying on the runner's pre-installed version, especially when modules use a recent `go` directive.

## Notes

- The action runs as a composite action that shells out to `monorel`. Calling it via `disaresta-org/monorel/ci/github@vX.Y.Z` pins both the action and the binary version (the action resolves `inputs.version` via the GitHub Releases API).
- For self-hosted GitHub Enterprise installations, set `monorel.toml`'s `[provider].host` field; the action passes `GITHUB_TOKEN` through and the binary picks up the host from config.
- Windows runners are supported. The download step matches the binary by name (`monorel` or `monorel.exe`) and installs to `$RUNNER_TEMP` with a `$GITHUB_PATH` entry rather than the Linux/macOS `/usr/local/bin` path.
- The `pr` command currently runs `monorel preview --upsert` only. It does not yet stage CHANGELOG edits on a release branch; the upserted PR body shows the rendered plan but does not include file diffs. The action will gain branch staging in a follow-up release.
