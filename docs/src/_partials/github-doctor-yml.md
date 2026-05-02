```yaml
name: doctor
on:
  pull_request:
    branches: [main]

permissions:
  contents: read

jobs:
  doctor:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v5
        with:
          # Full history so doctor's git-log scan sees every prior
          # chore(release): commit. The default shallow clone
          # (fetch-depth: 1) would miss them and turn doctor into a
          # no-op.
          fetch-depth: 0
      - uses: disaresta-org/monorel/ci/github@v0.8.0
        with:
          command: doctor
```
