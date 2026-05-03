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
    # Skip on the release PR's own merge commit so the workflow
    # doesn't churn the just-merged PR. Require BOTH the
    # chore(release): subject prefix AND the monorel-Release: trailer
    # in the body: a feature PR titled `chore(release): X` would also
    # match a subject-only check, leaving the release PR un-upserted.
    if: ${{ !(startsWith(github.event.head_commit.message, 'chore(release):') && contains(github.event.head_commit.message, 'monorel-Release:')) }}
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with: { fetch-depth: 0 }
      # `monorel apply` runs `go mod tidy` (offline, against a seeded
      # local cache) so the release commit's go.sum is canonically
      # clean. Pin Go via the released module's directive.
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
      - uses: disaresta-org/monorel/ci/github@v1.0.0
        with:
          command: pr
        env:
          GITEA_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```
