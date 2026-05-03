---
title: Changesets
description: "Changeset file format, naming, multi-package shape, and pre-release mode interaction."
---

# Changesets

A **changeset** is a `.changeset/<name>.md` file that declares which packages a PR affects and at what bump level. monorel reads pending changesets at release time, computes the next version per affected package, and writes the body of each changeset into the per-package CHANGELOG.

## File format

```markdown
---
"package-a": minor
"package-b": patch
---

Body text becomes the changelog entry for these packages.

Multi-paragraph bodies are supported; CommonMark indentation rules
keep the entry as one bullet under the rendered ## [vX.Y.Z] section.
```

The frontmatter is YAML; the body is markdown. Package names are quoted because they typically contain `/` or `.` (TOML/YAML map keys with special chars need escaping; quoting universally keeps the format simple).

Bump levels are `major`, `minor`, or `patch`. `none` is rejected: a changeset that doesn't affect a package shouldn't list it.

## Naming

monorel auto-generates names as `<adjective>-<noun>` (`quick-otter`, `dapper-koala`, ...) from a 70 × 80 wordlist. Override with `--name`:

```sh
monorel add --package transports/zerolog:minor --message "..." --name "rc1-coverage"
```

Names must match `[a-z0-9]+(-[a-z0-9]+)*` (lowercase letters, digits, hyphens; no leading/trailing/consecutive hyphens).

## Multi-package changesets

A single changeset can target multiple packages with different bump levels:

```markdown
---
"transports/zerolog": major
"github.com/acme/widget": patch
---

Reshape the zerolog Config; pass-through fix in the root.
```

In the rendered CHANGELOGs, this entry appears under `### Major Changes` for `transports/zerolog` and under `### Patch Changes` for `github.com/acme/widget`. The body is the same in both.

::: tip One changeset per coherent change
If a PR contains two unrelated changes (a feature in zerolog and a fix in zap), write two changesets. The release notes will be cleaner for users skimming the CHANGELOG.
:::

## Combining changesets

When multiple changesets target the same package, monorel uses the **strongest** bump level across them. Three changesets all bumping `transports/zerolog` (one `patch`, one `minor`, one `major`) produce a single `major` release. All three bodies appear in the rendered CHANGELOG, bucketed by the level THAT changeset requested for THIS package.

## Pre-release mode

`monorel pre enter <channel>` writes `.changeset/pre.json` with the channel name, an empty per-package counter map, and a `schemaVersion` (currently `1`) so future on-disk format changes are detected rather than silently misread. While pre-release mode is active:

- `monorel release` appends `-<channel>.N` to each released version (e.g. `v1.7.0-rc.0`).
- `.changeset/*.md` files are NOT deleted between releases.
- CHANGELOG entries are NOT written between releases.
- Per-package counters in `pre.json` increment.

Each pre-release sees the cumulative bump from ALL pending changesets, so escalating-severity changes are reflected in subsequent rcs.

`monorel pre exit` removes `pre.json`. The next stable release applies the accumulated changesets cumulatively, deletes them, and writes a single CHANGELOG entry per affected package.

::: warning Pre-mode keeps changesets
If you run `monorel pre enter rc` and then `monorel release` ten times without `pre exit`, the changeset files stay in `.changeset/` the entire time. They get consumed only at the next stable release.
:::

## Reserved filenames

Files in `.changeset/` that are NOT changesets:

- `pre.json`: pre-release-mode state.
- `README.md`: optional human-facing README.
- `.gitkeep`: empty-directory marker.
- `config.json`: reserved for future config.

`monorel` skips these when loading changesets.

## Authoring workflow

1. Make your code change as usual on a feature branch.
2. Author the changeset:
   - **Interactive** (multi-select packages, per-package level prompt, in-place body field): `monorel add`.
   - **Editor-driven body** (multi-paragraph markdown in `$VISUAL` / `$EDITOR`): `monorel add --editor`. Pair with `--package` to skip the multi-select: `monorel add -p PKG:LEVEL --editor`.
   - **Fully scripted** (CI generators, automated bumps): `monorel add -p PKG:LEVEL -m "..."`.
3. Commit the changeset along with your code change.
4. Open the PR.
5. CI runs `monorel preview` to update the always-open release PR.
6. When your PR merges, the next release PR includes your changeset's bodies.

A change that doesn't need a release entry (typo fix in a comment, refactoring nobody can observe) doesn't need a changeset. monorel doesn't enforce this; it's a convention.
