---
title: Library API
description: "monorel's Go library API surface for programmatic use: pure-function packages for parsing config, planning releases, validating, and rendering changelog entries."
---

# Library API

monorel ships a Go library API alongside the CLI. The following pure-function packages are public, SemVer-committed from v1.0.0 onward, and have been stable in shape since v0.2.0:

| Package | Purpose | Entry points |
|---------|---------|--------------|
| [`config`](https://pkg.go.dev/monorel.disaresta.com/config) | Parse and validate `monorel.toml`. | `Load`, `Config.Validate`, `IsKnownProvider` |
| [`changeset`](https://pkg.go.dev/monorel.disaresta.com/changeset) | Read and write `.changeset/*.md` files. | `LoadAll`, `Parse`, `Changeset.WriteFile`, `RandomName` |
| [`plan`](https://pkg.go.dev/monorel.disaresta.com/plan) | Pure-function planner: config + changesets + tags → release plan. | `Plan` |
| [`semver`](https://pkg.go.dev/monorel.disaresta.com/semver) | Bump-level abstraction, version application, initial-release rules. | `Apply`, `InitialFromBump`, `Max`, `ParseBumpLevel`, `IsValid`, `Compare` |
| [`validate`](https://pkg.go.dev/monorel.disaresta.com/validate) | Fault-tolerant static checks against a config + changeset directory. | `Run`, `HasErrors`, `HasWarnings` |
| [`changelog`](https://pkg.go.dev/monorel.disaresta.com/changelog) | Keep-a-Changelog renderer with non-destructive insertion. | `Insert`, `WriteFile`, `Today` |
| [`doctor`](https://pkg.go.dev/monorel.disaresta.com/doctor) | Repository state diagnostics the planner can't catch (e.g. revived changesets). | `Run`, `Finding`, `Severity`, `GitLog` |

Module path: `monorel.disaresta.com`. Install:

```sh
go get monorel.disaresta.com@latest
```

Full GoDoc with runnable examples lives on [pkg.go.dev](https://pkg.go.dev/monorel.disaresta.com).

## When to reach for the library

- **Custom orchestrator.** You're running monorel under a different CI system, or you want to modify the always-open release PR pattern. Use `plan.Plan` to compute the next-release plan and drive your own publish step.
- **IDE / editor plugin.** You want "is this changeset frontmatter valid?" or "does this package key exist in monorel.toml?" feedback. Use `changeset.Parse` + `validate.Run`.
- **Repo audit tool.** You want to surface monorel-shaped state across many repos. Use `config.Load` + `validate.Run`.
- **Custom changeset authoring.** A bot or workflow that produces `.changeset/*.md` files programmatically. Use `changeset.Changeset` + `Changeset.WriteFile`.
- **Repo health diagnostics.** A pre-merge or pre-release CI step that fails closed when a previously-shipped changeset is back on disk (the stale-branch + squash-merge revival pattern). Use `doctor.Run` with a `GitLog` adapter over your existing git library.

## doctor

The `doctor` package wraps a small, extensible check-runner. Today it ships a single check, `revived-changeset`, that catches a common failure mode: when a contributor branches from `main` BEFORE a release commit and squash-merges later, GitHub re-introduces the changeset files the release commit deleted. The next release plan re-ships the same content under a new version.

```go
package main

import (
	"fmt"
	"os/exec"
	"sort"
	"strings"

	"monorel.disaresta.com/doctor"
)

func main() {
	findings, err := doctor.Run(doctor.Options{
		RepoDir: ".",
		GitLog:  shellGitLog("."),
	})
	if err != nil {
		panic(err)
	}
	for _, f := range findings {
		fmt.Printf("%s [%s] %s: %s\n", f.Severity, f.CheckName, f.Path, f.Message)
	}
}

// shellGitLog adapts `git log` to doctor.GitLog. Returns every file
// path deleted by a commit whose message contains messageGrep as a
// case-sensitive literal substring, deduped and sorted.
func shellGitLog(dir string) doctor.GitLog {
	return func(messageGrep string) ([]string, error) {
		cmd := exec.Command("git", "log",
			"--diff-filter=D", "--name-only", "--format=",
			"--fixed-strings", "--grep="+messageGrep)
		cmd.Dir = dir
		out, err := cmd.Output()
		if err != nil {
			return nil, err
		}
		seen := map[string]struct{}{}
		var paths []string
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if line == "" {
				continue
			}
			if _, dup := seen[line]; dup {
				continue
			}
			seen[line] = struct{}{}
			paths = append(paths, line)
		}
		sort.Strings(paths)
		return paths, nil
	}
}
```

`doctor.GitLog` is a function value, not an interface, so callers can adapt any git library (go-git, libgit2 bindings, an in-memory cache) without conforming to a struct shape. The `os/exec` form above is the simplest adapter; replace it with a call into your existing git library if you have one. The contract: `func(messageGrep string) (deletedPaths []string, err error)` returning every path deleted by a commit whose subject or body contains the literal substring.

## What's NOT public

These packages stay in `internal/` deliberately and are not part of the SemVer commitment:

| Internal package | Why it stays private |
|------------------|----------------------|
| `release` | Writes files, creates commits, creates tags. Promoting locks side-effect ordering. |
| `orchestrator` | Provider-coupled (calls `provider.Client`); not useful without it. |
| `provider` | Host-abstraction interface; promoting locks every interface change as breaking. |
| `git` | Shell-out implementation detail. The doctor package's `GitLog` function-value seam is how callers get at git history without depending on it. |
| `cli` | Cobra wiring. The library is the API; the CLI is one consumer. |

Side-effect-bearing packages should never be public commitments — every refactor would be a SemVer break.

## Stability

From v1.0.0 onward, the public packages are SemVer-committed. Additive changes (new types, new functions, new fields on existing types) are minor bumps; breaking changes (renames, removals, signature changes, field type changes) are major bumps that ship as `monorel.disaresta.com/v2/...` per Go module convention.

## Quick example

```go
package main

import (
	"fmt"
	"log"

	"monorel.disaresta.com/changeset"
	"monorel.disaresta.com/config"
	"monorel.disaresta.com/plan"
)

func main() {
	cfg, err := config.Load("monorel.toml")
	if err != nil {
		log.Fatal(err)
	}

	cs, err := changeset.LoadAll(".changeset")
	if err != nil {
		log.Fatal(err)
	}

	// In a real tool, fetch tags from git or your provider's API. Here we use
	// a hardcoded list for illustration.
	tags := []string{"v1.6.1", "transports/zerolog/v1.6.1"}

	p, err := plan.Plan(cfg, cs, tags, nil)
	if err != nil {
		log.Fatal(err)
	}

	for _, r := range p.Releases {
		fmt.Printf("%s: %s -> %s (tag %s)\n", r.Name, r.From, r.To, r.Tag)
	}
}
```

`plan.Plan` is a pure function: same inputs, same output, no I/O. Drop in test fixtures or an in-memory tag list and the planner runs unchanged.
