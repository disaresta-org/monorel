---
title: Why monorel?
description: "monorel exists because the Go ecosystem doesn't have a release tool that handles bare-tag root + prefixed sub-module tags cleanly. release-please works with friction; Knope can't do bare-tag root; changesets needs a JS bridge for Go."
---

# Why monorel?

The Go ecosystem doesn't have a release tool that handles the canonical Go monorepo layout cleanly. The layout in question:

```
my-repo/                        ← main module (root)
├── go.mod                        module example.com/widget
├── widget.go
├── transports/
│   └── foo/                    ← sub-module
│       ├── go.mod                module example.com/widget/transports/foo
│       └── foo.go
└── plugins/
    └── bar/                    ← sub-module
        ├── go.mod                module example.com/widget/plugins/bar
        └── bar.go

tags:  v1.2.3                   ← root: BARE tag (required by go install)
       transports/foo/v0.5.1    ← sub-module: PATH-PREFIXED tag (required by go get)
       plugins/bar/v2.0.0
```

- **Main module** at the repo root takes bare `vX.Y.Z` tags. `go install <module>@v1.2.3` requires this.
- **Sub-modules** in subdirectories take `<path>/vX.Y.Z` tags. `go get <module>/transports/foo@v1.2.3` requires this.

The off-the-shelf options each have a sharp edge for this layout.

## At a glance

| Capability | release-please | changesets | Knope | monorel |
|------------|:---:|:---:|:---:|:---:|
| Per-package versioning | ✅ | ✅ | ✅ | ✅ |
| Auto-generated CHANGELOG | ✅ | ✅ | ✅ | ✅ |
| Pre-release / RC support | ✅ | ✅ | ✅ | ✅ |
| Always-open release PR | ✅ | ✅ via changesets-bot | ⚠️ via custom workflow | ✅ |
| Local CLI (works off-CI) | ⚠️ via npm package | ✅ | ✅ | ✅ |
| Bare-tag root (`vX.Y.Z`) | ✅ | n/a (JS layout) | ❌ prefixes mandatory | ✅ |
| Path-prefixed sub-module tags | ✅ | n/a (JS layout) | ✅ | ✅ |
| Cleans `go.mod` for proxy publish | ❌ | n/a (JS) | ❌ | ✅ |
| Tidies sub-module `go.sum` at release | ❌ | n/a (JS) | ❌ | ✅ |
| Source of truth | commit footers (`Release-As:`) | `.changeset/*.md` | configurable (commits or files) | `.changeset/*.md` |
| Native to | TypeScript | TypeScript | Rust | Go |
| Multi-provider | GitHub | GitHub (bot); CLI host-agnostic | GitHub / GitLab / Gitea | GitHub + Gitea / Forgejo + GitLab |
| Polyglot / non-language-specific | ✅ | ⚠️ JS-shaped (`package.json` per package) | ✅ | ❌ Go-only by design |

The first three rows are the common ground: every tool in this category will manage independent per-package versions, write a per-package CHANGELOG, and support pre-release windows. The friction shows up below those rows: how releases are *triggered* (commit messages vs explicit files), what tag shapes are supported, and which language ecosystem the tool is native to.

The per-tool sections below dive into each tool's specific friction point for the Go-monorepo layout.

### release-please

Works, with friction. The friction lives in three sharp edges:

- **Squash-merge strips Conventional Commits footers.** GitHub's squash uses the PR title as the subject and the PR body as the message body. Per-commit `Release-As:` or `BREAKING CHANGE:` footers don't survive. Workarounds (put it in the PR body) are easy to forget.
- **Full-history scans on new packages leak footers.** A `Release-As:` footer in any commit in repo history can apply to a newly-registered package because release-please has no "first release" boundary to stop the scan. We hit this on `loglayer-go`: an old `Release-As: 1.1.0` on an unrelated change set the initial version of a new sub-module to `v1.1.0`.
- **`exclude-paths` doesn't catch path-attribution leaks for everything.** A docs-only PR can still bump the main module if the path-attribution rules don't cover the directory. The list grows over time and is easy to forget.

These are all recoverable, but recovery means manual `release-as` cleanup, manual tag deletes, manual manifest fixes. The tool's mental model is "infer intent from commit history"; the failures are all variations of "the inference got confused."

### Knope

Doesn't fit the bare-tag root convention. Knope per-package tag prefixes are mandatory; you can't get bare `vX.Y.Z` for the root module. That's a Go-specific requirement Knope wasn't built for.

### changesets

Works conceptually but is JS-native. Adapting it to Go requires a synthetic `package.json` per Go module, a manual bridge between npm version and git tag, and a JS toolchain in the release path.

## What monorel does

monorel takes the changesets *idea* (per-PR intent files, named affected packages, bump levels) and ships it as a Go-native CLI:

- **Changeset files are the source of truth.** Each release-affecting PR includes `.changeset/<name>.md` with YAML frontmatter mapping package names to bump levels and a markdown body that becomes the changelog entry. No commit-message parsing, no path attribution, no `Release-As:` footers. The class of "release tool got confused" failures becomes structurally impossible.
- **Tag format is per-package.** `tag_prefix = ""` for the main module yields bare `vX.Y.Z`; `tag_prefix = "transports/foo"` for a sub-module yields `transports/foo/vX.Y.Z`. Both work in the same repo.
- **Always-open release PR.** The bot orchestrator force-pushes a speculative-version branch and upserts a PR. Merging the PR runs `monorel release` on the merge commit, pushes tags, and publishes per-tag releases.
- **Pre-release support.** `monorel pre enter rc` switches the repo to release-candidate mode; subsequent releases append `-rc.N` to the next stable version and increment a per-package counter. `pre exit` returns to stable.
- **Provider-neutral.** GitHub, Gitea / Forgejo, and GitLab are all wired up. The orchestrator never sees provider-specific types; add a subpackage to support a host that isn't covered.
- **Clean `go.mod` at release time.** Sub-modules carry dev `replace` directives and placeholder `require` versions for local cross-module work; monorel strips and pins them in the release commit so downstream consumers' `go mod tidy` resolves the published versions.
- **Tidy sub-module `go.sum` at release time.** Pinning sibling requires shifts the `go.sum` drift problem onto consumers. monorel runs `go mod tidy` (offline, against a seeded local cache) in every released sub-module that requires an in-plan sibling, so the release commit's `go.sum` is canonically clean. No proxy roundtrip; main is `go mod tidy`-clean for every consumer on the next pull.

## When monorel is overkill

monorel works fine on single-module repos (it's the trivial N=1 case of the multi-module config), but its overhead — `monorel.toml`, a release workflow, `.changeset/*.md` per PR — only pays off above a certain threshold. If your repo is a single Go module with infrequent releases and no plans to grow, `git tag` plus a hand-written CHANGELOG is a reasonable choice. monorel's value shows up when you have:

- two or more packages that version independently,
- a public API where downstream users `go get` specific sub-modules at specific tags,
- a release cadence frequent enough that hand-crafting changelogs is friction.

Below that threshold, the tool's overhead (a `monorel.toml`, `.changeset/*.md` per PR, a release workflow) costs more than it saves.

## Where monorel sits

monorel is a CLI plus a thin GitHub Action wrapper. There is no GitHub App, no hosted service, no per-repo install beyond a workflow file. The same binary runs in CI and on your laptop.

The next page walks through wiring it up: [Getting Started](/getting-started).
