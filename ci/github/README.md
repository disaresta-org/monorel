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

1. `monorel release`: local file mutations, commit, and tag.
2. `git push --follow-tags`: publish commits and tags to the remote.
3. `monorel publish`: create one provider release per tag at HEAD; body sourced from each package's CHANGELOG entry.

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
      # monorel runs `go mod tidy` with GOTOOLCHAIN=local during release,
      # so Go must already be installed at a version satisfying every
      # released sub-module's `go` directive (the highest one wins).
      # `go-version-file: go.mod` reads the root module's go.mod; if a
      # sub-module declares a higher floor, pin `go-version` explicitly.
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
      - uses: disaresta-org/monorel/ci/github@v1.0.0
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
      # monorel runs `go mod tidy` with GOTOOLCHAIN=local during release,
      # so Go must already be installed at a version satisfying every
      # released sub-module's `go` directive (the highest one wins).
      # `go-version-file: go.mod` reads the root module's go.mod; if a
      # sub-module declares a higher floor, pin `go-version` explicitly.
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
      - uses: disaresta-org/monorel/ci/github@v1.0.0
        with:
          command: release
```

## Requirements

- **`go` on `PATH`.** monorel's `apply` step runs `go mod tidy` (offline, against a seeded local cache) in every released sub-module that requires an in-plan sibling, so the release commit's `go.sum` is canonically clean for the proxy-published state. The runner needs a `go` binary whose version satisfies every released module's `go` directive; use `actions/setup-go@v5` with `go-version-file: go.mod` (or pin `go-version` explicitly) to install the right version. GitHub-hosted runners include a recent Go by default, but pinning is safer than relying on the runner's pre-installed version, especially when modules use a recent `go` directive.

  If the runner's Go is older than the highest sub-module's `go` directive, tidy fails with `go.mod requires go >= X.Y; running Z; GOTOOLCHAIN=local`. The fix is always to bump the runner's Go (raise the `go-version`), not to remove `GOTOOLCHAIN=local` from monorel's tidy step (the env var is part of monorel's offline-tidy determinism guarantee).

  See [Avoiding the chore(release) CI race](../../docs/src/workflows.md#avoiding-the-chorerelease-ci-race) for the related skip-filter pattern other workflows on the same branch should apply.

## Notes

- The action runs as a composite action that shells out to `monorel`. Calling it via `disaresta-org/monorel/ci/github@vX.Y.Z` pins both the action and the binary version (the action resolves `inputs.version` via the GitHub Releases API).
- For self-hosted GitHub Enterprise installations, set `monorel.toml`'s `[provider].host` field; the action passes `GITHUB_TOKEN` through and the binary picks up the host from config.
- Windows runners are supported. The download step matches the binary by name (`monorel` or `monorel.exe`) and installs to `$RUNNER_TEMP` with a `$GITHUB_PATH` entry rather than the Linux/macOS `/usr/local/bin` path.
- The `pr` command currently runs `monorel preview --upsert` only. It does not yet stage CHANGELOG edits on a release branch; the upserted PR body shows the rendered plan but does not include file diffs. The action will gain branch staging in a follow-up release.

## Recipes

### Skipping CI on chore(release) commits

The release commit `chore(release): ...` is created by the always-open release PR's merge. On the same push event, `release.yml` (using this action with `command: release`) creates and pushes per-package tags. Any *other* workflow that runs on the same push and resolves Go module versions will race the tag-push and may transiently fail with:

```
go: example.com/foo/v2: reading example.com/foo/go.mod at revision v2.1.0: unknown revision v2.1.0
```

To avoid the phantom failure, skip the workflow on `chore(release):` commits. The skip filter:

```yaml
jobs:
  test:
    if: github.event_name == 'pull_request' || !startsWith(github.event.head_commit.message, 'chore(release):')
    # ... rest of job ...
```

Apply the filter to every job (`test`, `staticcheck`, `govulncheck`, etc.) that runs `go mod tidy` or anything else that resolves the new versions. Pull-request triggers stay always-on; only push-to-main runs are skipped, and only when the head commit is the release-PR merge.

The `release.yml` workflow that runs the actual release pipeline does NOT need this filter; its own `if:` clause is the *opposite* shape (only run on `chore(release):` commits), so it's already mutually exclusive with the skip pattern above.

For non-GitHub-Actions CI systems, see the [universal recipe](../../docs/src/workflows.md#avoiding-the-chorerelease-ci-race) covering GitHub / GitLab / Gitea filter syntax.
