# monorel GitHub Action

Composite action that downloads the monorel binary for the runner's OS+arch and runs `monorel auto` against the current repo.

`monorel auto` detects whether `HEAD` is a release-PR merge (via the `monorel-Release:` trailer the orchestrator wrote, with a provider-API fallback) and dispatches to either the post-merge release pipeline (tag, push, publish) or the pre-merge maintenance pipeline (apply changesets onto a staging branch, force-push, upsert the always-open release PR). One workflow, one step, no `command` input.

## Inputs

| Input | Default | Description |
|-------|---------|-------------|
| `version` | `latest` | monorel version to run (e.g. `v1.2.3`). |
| `config` | `monorel.toml` | Path to the config file. |
| `token` | the workflow's `GITHUB_TOKEN` | Token for provider API calls. Needs `contents: write` and `pull-requests: write`. |

## Workflow

A single workflow is enough. It triggers on every push to `main` and lets `monorel auto` decide what to do.

```yaml
name: monorel
on:
  push:
    branches: [main]

permissions:
  contents: write
  pull-requests: write

jobs:
  monorel:
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
```

On a regular merge, `monorel auto` applies pending changesets onto a staging branch and upserts the always-open release PR. On the merge of that release PR, the same step instead tags, pushes, and publishes.

## `monorel doctor` is a separate step

`monorel doctor` is a pre-merge diagnostic. It belongs on PR builds, not on the post-merge release pipeline this action wraps. Invoke it as its own job step in the PR workflow rather than through this action:

```yaml
name: pr-checks
on:
  pull_request:
    branches: [main]

permissions:
  contents: read

jobs:
  doctor:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
      - run: monorel doctor --config monorel.toml
```

(Install monorel however your PR workflow already installs tools; the action wrapper is only for the release-side flow.)

## Requirements

- **`go` on `PATH`.** monorel's `apply` step runs `go mod tidy` (offline, against a seeded local cache) in every released sub-module that requires an in-plan sibling, so the release commit's `go.sum` is canonically clean for the proxy-published state. The runner needs a `go` binary whose version satisfies every released module's `go` directive; use `actions/setup-go@v5` with `go-version-file: go.mod` (or pin `go-version` explicitly) to install the right version. GitHub-hosted runners include a recent Go by default, but pinning is safer than relying on the runner's pre-installed version, especially when modules use a recent `go` directive.

  If the runner's Go is older than the highest sub-module's `go` directive, tidy fails with `go.mod requires go >= X.Y; running Z; GOTOOLCHAIN=local`. The fix is always to bump the runner's Go (raise the `go-version`), not to remove `GOTOOLCHAIN=local` from monorel's tidy step (the env var is part of monorel's offline-tidy determinism guarantee).

  See [Avoiding the chore(release) CI race](../../docs/src/workflows.md#avoiding-the-chorerelease-ci-race) for the related skip-filter pattern other workflows on the same branch should apply.

## Notes

- The action runs as a composite action that shells out to `monorel`. Calling it via `disaresta-org/monorel/ci/github@vX.Y.Z` pins both the action and the binary version (the action resolves `inputs.version` via the GitHub Releases API).
- For self-hosted GitHub Enterprise installations, set `monorel.toml`'s `[provider].host` field; the action passes `GITHUB_TOKEN` through and the binary picks up the host from config.
- Windows runners are supported. The download step matches the binary by name (`monorel` or `monorel.exe`) and installs to `$RUNNER_TEMP` with a `$GITHUB_PATH` entry rather than the Linux/macOS `/usr/local/bin` path.

## Recipes

### Skipping CI on chore(release) commits

The release commit `chore(release): ...` is created by the always-open release PR's merge. On the same push event, this action runs `monorel auto`, which detects the release merge and creates and pushes per-package tags. Any *other* workflow that runs on the same push and resolves Go module versions will race the tag-push and may transiently fail with:

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

The monorel workflow itself does NOT need this filter; on a `chore(release):` push it's the workflow doing the tagging, and on every other push `monorel auto` falls through to the upsert path.

For non-GitHub-Actions CI systems, see the [universal recipe](../../docs/src/workflows.md#avoiding-the-chorerelease-ci-race) covering GitHub / GitLab / Gitea filter syntax.
