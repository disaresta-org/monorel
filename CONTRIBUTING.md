# Contributing to monorel

Thanks for taking a look. This is the on-ramp for a local dev loop.

## Prerequisites

| Tool | Why | Install |
|------|-----|---------|
| Go 1.23+ | Build, test, run. | <https://go.dev/dl/> |
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

## Workflow

- Branch off `main` as `<type>/<short-slug>` (e.g. `feat/planner`, `fix/changeset-name-collision`).
- Use [Conventional Commits](https://www.conventionalcommits.org/) for the commit message. The `commit-msg` hook lints with `@conventional-commits/parser`.
- Add a changeset to `.changeset/` describing what's changing. (`monorel` uses itself for releases.)
- Hooks run on every commit and push. Fix failures rather than `--no-verify` past them.

## Project layout

```
monorel/
├── cmd/monorel/                CLI entrypoint
├── internal/
│   ├── cli/                    cobra commands
│   ├── config/                 monorel.toml schema
│   ├── changeset/              .changeset/*.md format + pre.json
│   ├── semver/                 bump levels + initial-release rules
│   ├── git/                    interface + shell-out impl + fake + testutil
│   ├── plan/                   pure-function planner (the core)
│   ├── changelog/              Keep-a-Changelog generator
│   ├── release/                applies a ReleasePlan; renders preview markdown
│   ├── provider/               provider-neutral host API seam
│   │   ├── factory/            dispatch by config.ForgeConfig.Provider
│   │   └── github/             go-github implementation
│   └── orchestrator/           drives the always-open PR pattern
├── ci/                         per-CI-system action wrappers
│   └── github/action.yml       composite GitHub Action
├── docs/                       VitePress docs site
└── .changeset/                 self-hosted changesets
```

The pure-function planner (`internal/plan`) is the heart. Most logic lives there as `(config, []Changeset, []Tag, *PreState) -> ReleasePlan`; everything else is plumbing around it. Tests for `Plan` cover the version-math matrix exhaustively.

Adding a new provider: see [`AGENTS.md`](AGENTS.md) "Adding a New Provider". The interface is `provider.Client` (six methods); each provider lives in `internal/provider/<name>/` and the factory dispatches by config.

## License

By contributing, you agree that your contributions are licensed under the [MIT License](LICENSE).
