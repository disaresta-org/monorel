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
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
      - uses: disaresta-org/monorel/ci/github@v1.0.0
        with:
          # Gitea Actions auto-injects secrets.GITHUB_TOKEN for
          # GitHub-Actions YAML compatibility. The action exports
          # whatever you pass as `token:` into both GITHUB_TOKEN
          # and GITEA_TOKEN env vars, so monorel reads the right
          # one for the configured Gitea provider.
          token: ${{ secrets.GITHUB_TOKEN }}
```
