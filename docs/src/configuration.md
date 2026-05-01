---
title: Configuration
description: "monorel.toml reference: provider, per-package tag_prefix, path, changelog."
---

# Configuration

`monorel.toml` lives at the repo root. It's the single source of truth for which packages exist, where their CHANGELOGs live, and how their tags are formatted.

## File shape

```toml
[provider]
name  = "github"  # optional, default "github"
owner = "acme"
repo  = "widget"
host  = ""        # optional, for self-hosted (e.g. "github.example.com")

[packages."<name>"]
tag_prefix = "..."
path       = "..."
changelog  = "..."
```

At least one `[packages."<name>"]` block is required.

## `[provider]`

Identifies the version-control host that owns the repo. monorel is host-agnostic; the `[provider]` block selects which implementation to use and identifies the specific repository on that host.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `name` | string | `"github"` | Provider implementation: currently only `"github"`. Future: `"gitlab"`, `"gitea"`, `"bitbucket"`, `"forgejo"`. |
| `owner` | string | required | The user or org that owns the repo. On GitLab maps to namespace; on Bitbucket to workspace. |
| `repo` | string | required | The repository name. |
| `host` | string | `""` | API host for self-hosted installations. Empty means the provider's default public host. |

## `[packages."<name>"]`

`<name>` is an arbitrary identifier used as the changeset frontmatter key. Convention:

- Use the import path for the main module (`"github.com/acme/widget"`).
- Use the relative directory for sub-modules (`"transports/zerolog"`).

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `tag_prefix` | string | required | Prefix prepended (with `/`) to the version when forming the git tag. Empty `""` produces bare `vX.Y.Z`. |
| `path` | string | required | Package directory relative to the repo root. Used to locate `go.mod` for v2+ major-version updates. |
| `changelog` | string | required | Path (relative to repo root) of the per-package `CHANGELOG.md` monorel writes new entries to. |

::: warning Bare-tag root requires explicit empty string
The main module at the repo root needs `tag_prefix = ""` (the empty string) for monorel to produce bare `vX.Y.Z` tags. Omitting the field is currently rejected by the validator; setting it to `""` is the supported path.
:::

## Example: single-package repo

```toml
[provider]
owner = "acme"
repo  = "widget"

[packages."github.com/acme/widget"]
tag_prefix = ""
path       = "."
changelog  = "CHANGELOG.md"
```

Tags emerge as bare `vX.Y.Z` (`v1.0.0`, `v1.1.0`, ...). This is what `go install github.com/acme/widget@vX.Y.Z` expects.

## Example: monorepo with sub-modules

```toml
[provider]
owner = "loglayer"
repo  = "loglayer-go"

[packages."go.loglayer.dev"]
tag_prefix = ""
path       = "."
changelog  = "CHANGELOG.md"

[packages."transports/zerolog"]
tag_prefix = "transports/zerolog"
path       = "transports/zerolog"
changelog  = "transports/zerolog/CHANGELOG.md"

[packages."transports/zap"]
tag_prefix = "transports/zap"
path       = "transports/zap"
changelog  = "transports/zap/CHANGELOG.md"
```

Tags: `v1.6.0` (root), `transports/zerolog/v1.6.0`, `transports/zap/v1.6.0`. Each package versions independently.

## Validation

`monorel` rejects misconfigurations at load time:

- **`provider.owner is required`** / **`provider.repo is required`**: required fields.
- **`provider.name %q is not recognized`**: provider name is not in `config.KnownProviders`. Today only `"github"` is supported.
- **`no packages declared`**: at least one `[packages."<name>"]` block is required.
- **`packages.X: path is required`** / **`changelog is required`**: per-package required fields.
- **`packages.X and packages.Y share tag_prefix Z`**: two packages mapping to the same tag namespace would silently collide. Pick distinct prefixes.
- **`unknown keys: ...`**: unknown TOML keys fail loud, so config typos surface immediately.

## Self-hosted providers

Set `host` to the API hostname:

```toml
[provider]
name  = "github"
host  = "github.example.com"  # GitHub Enterprise
owner = "acme"
repo  = "widget"
```

The provider implementation maps `host` to its host-specific URL shape (GitHub Enterprise uses `https://<host>/api/v3/`).

