---
"monorel.disaresta.com": minor
---

**Breaking:** rename the user-visible "forge" terminology to "provider".

Reasoning: "forge" is industry jargon (short for "software forge,"
inherited from SourceForge-era naming) that's opaque to anyone who
hasn't been exposed to it. monorel's existing config field was already
called `provider`; the section header was inconsistent. Standardizing
on "provider" everywhere user-visible removes the duplication and the
"what is a forge?" lookup.

### `monorel.toml` migration

```diff
-[forge]
-provider = "github"
+[provider]
+name = "github"
 owner = "acme"
 repo  = "widget"
 host  = ""
```

Section name: `[forge]` → `[provider]`. Inner field: `provider` →
`name`. The `owner`, `repo`, and `host` fields are unchanged. v0.3
rejects legacy `[forge]` configs via the existing unknown-keys parse
error: existing configs surface as a clear "unknown keys: [forge ...]"
on the next run. Pre-1.0 hard break, no migration helper.

### Public Go API migration

| Before | After |
|--------|-------|
| `config.ForgeConfig` | `config.ProviderConfig` |
| `config.Config.Forge` (field) | `config.Config.Provider` (field) |
| `config.ForgeConfig.Provider` (field) | `config.ProviderConfig.Name` (field) |

Constants were already named correctly (`config.ProviderGitHub`,
`config.KnownProviders`, `config.ResolveProvider`, `config.IsKnownProvider`).

### Internal package rename

`internal/forge/` is now `internal/provider/` (package name `provider`,
type `provider.Client`). Consumers don't import this — it's
internal-only — but anyone reading the codebase will see the rename.
The `internal/forge/factory/` and `internal/forge/github/` subpackages
moved correspondingly.

### Error messages

| Before | After |
|--------|-------|
| `forge.owner is required` | `provider.owner is required` |
| `forge.repo is required` | `provider.repo is required` |
| `forge.provider %q is not recognized` | `provider.name %q is not recognized` |

### Docs

Configuration, getting-started, design, api, github-action, docker,
cli-reference, recipes, AGENTS.md, README.md, CONTRIBUTING.md, and
the `.claude/rules/*.md` agent guides all swept. The recipes'
`[forge]` example blocks now use `[provider]`.
