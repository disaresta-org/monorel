# Contributing to monorel

Thanks for taking a look. This is the on-ramp for a local dev loop.

## Prerequisites

| Tool | Why | Install |
|------|-----|---------|
| Go 1.26+ | Build, test, run. | <https://go.dev/dl/> |
| lefthook | pre-commit / commit-msg / pre-push hooks. | `go install github.com/evilmartians/lefthook@latest` |
| staticcheck | Pre-commit lint that mirrors CI. | `go install honnef.co/go/tools/cmd/staticcheck@latest` |
| Bun | Commit-msg linter + docs site. | <https://bun.sh/> |

`go install` puts binaries in `$(go env GOPATH)/bin` (default `~/go/bin`). Make sure that directory is on your `PATH`, otherwise the git hooks can't find `lefthook` / `staticcheck`.

## One-time setup

```sh
git clone https://github.com/disaresta-org/monorel
cd monorel

make hooks       # wire up git hooks via lefthook
bun install      # install commit-msg lint deps
make build       # build the monorel binary
```

## Common make targets

`make help` lists everything. The ones you'll use most:

| Target | What it does |
|--------|--------------|
| `make build` | Build the `monorel` binary into `./monorel`. |
| `make test` | Fast unit tests. |
| `make test-race` | Race-detector tests across the repo. Mirrors pre-push. |
| `make lint` | vet + gofmt-check + staticcheck. |
| `make fmt` | gofmt every Go file in place. |
| `make tidy` | `go mod tidy`. |
| `make ci` | Full CI gauntlet. Run before pushing. |
| `make docs` | Build the VitePress docs site. |
| `make docs-dev` | Run the docs dev server with live reload. |

## Running the e2e suites

`make ci` (and the pre-push hook) covers unit tests + race detection across the repo. The build-tag-gated end-to-end suites under `tests/e2e/` are not in the default test run; invoke them explicitly when you're touching the release pipeline, the provider seam, or the offline-tidy / cacheseed paths. Both require Docker (testcontainers-go starts disposable containers and tears them down at the end of the run).

```sh
# Forgejo lifecycle scenarios (~27 tests, ~75s wall clock):
go test -tags=e2e -count=1 -timeout 15m ./tests/e2e/...

# Clean-cache regression test for multi-module offline tidy
# (`monorel apply` inside a fresh golang:1.26-alpine, ~15s):
go test -tags=e2e_tidy -count=1 -timeout 5m ./tests/e2e/... -run TestApply_MultiModule_CleanCache
```

CI (`.github/workflows/e2e-gitea.yml`) runs the Forgejo suite on every PR. The `e2e_tidy` reproducer is opt-in locally; consider running it whenever you change `internal/release/cacheseed.go`, `internal/release/tidy.go`, or anything in the seed-and-tidy fixpoint loop.

## Workflow

- Branch off `main` as `<type>/<short-slug>` (e.g. `feat/planner`, `fix/changeset-name-collision`).
- Use [Conventional Commits](https://www.conventionalcommits.org/) for the commit message. The `commit-msg` hook lints with `@conventional-commits/parser`.
- Add a changeset to `.changeset/` describing what's changing. (`monorel` uses itself for releases.)
- Hooks run on every commit and push. Fix failures rather than `--no-verify` past them.

## Project layout

```
monorel/
├── cmd/monorel/                CLI entrypoint
├── config/                     monorel.toml schema (public)
├── changeset/                  .changeset/*.md format + pre.json (public)
├── semver/                     bump levels + initial-release rules (public)
├── plan/                       pure-function planner: the core (public)
├── changelog/                  Keep-a-Changelog generator (public)
├── validate/                   configuration + changeset validator (public)
├── doctor/                     repository-state diagnostics (public)
├── internal/
│   ├── cli/                    cobra commands
│   ├── git/                    interface + shell-out impl + fake + testutil
│   ├── detect/                 release-merge detection (trailer + provider-API signal)
│   ├── orchestrator/           `monorel auto` dispatch: release vs feature path
│   ├── release/                applies a ReleasePlan; renders preview markdown
│   └── provider/               provider-neutral host API seam
│       ├── factory/            dispatch by config.ProviderConfig.Name
│       ├── github/             go-github implementation
│       ├── gitea/              Gitea / Forgejo implementation
│       ├── gitlab/             GitLab implementation
│       └── bitbucket/          Bitbucket Cloud implementation
├── ci/                         per-CI-system action wrappers
│   └── github/action.yml       composite GitHub Action (runs `monorel auto`)
├── tests/e2e/                  live-Forgejo integration suite (build tag `e2e`)
│                                  + clean-cache reproducer in `golang:1.26-alpine` (build tag `e2e_tidy`)
├── docs/                       VitePress docs site
└── .changeset/                 self-hosted changesets
```

The pure-function planner (`plan`) is the heart. Most logic lives there as `(config, []Changeset, []Tag, *PreState) -> ReleasePlan`; everything else is plumbing around it. Tests for `Plan` cover the version-math matrix exhaustively. The top-level `config`, `changeset`, `semver`, `plan`, `changelog`, `validate`, and `doctor` packages are public Go API; everything under `internal/` is implementation detail.

Adding a new provider: see [`AGENTS.md`](AGENTS.md) "Adding a New Provider". The interface is `provider.Client` (seven methods); each provider lives in `internal/provider/<name>/` and the factory dispatches by config.

## License

By contributing, you agree that your contributions are licensed under the [MIT License](LICENSE).
