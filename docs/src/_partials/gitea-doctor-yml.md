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
        with: { fetch-depth: 0 }
      - uses: disaresta-org/monorel/ci/github@v1.0.0
        with:
          command: doctor
```
