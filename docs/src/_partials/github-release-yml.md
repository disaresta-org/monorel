```yaml
name: release
on:
  push:
    branches: [main]
  workflow_dispatch:

permissions:
  contents: write

jobs:
  release:
    # Require BOTH the chore(release): subject prefix AND a
    # monorel-Release: trailer in the body. Subject prefix alone is
    # not sufficient: a feature PR titled `chore(release): X` also
    # starts with the prefix but doesn't carry the trailer monorel
    # tag needs to know which tags to create.
    if: github.event_name == 'workflow_dispatch' || (startsWith(github.event.head_commit.message, 'chore(release):') && contains(github.event.head_commit.message, 'monorel-Release:'))
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
      # Required so monorel's apply step can run `go mod tidy`. See
      # release-pr.yml for the rationale.
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
      - uses: disaresta-org/monorel/ci/github@v1.0.0
        with:
          command: release
```
