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
      - uses: actions/checkout@v4
        with:
          # Full history so doctor's git-log scan sees every prior
          # chore(release): commit. The default shallow clone
          # (fetch-depth: 1) would miss them and turn doctor into a
          # no-op.
          fetch-depth: 0
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
      - name: Install monorel
        run: go install monorel.disaresta.com/cmd/monorel@latest
      - name: Run doctor
        run: monorel doctor --config monorel.toml
```
