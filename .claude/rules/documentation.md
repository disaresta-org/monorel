# Documentation Rules

## Prose Style

### No em dashes. Ever.

Use the right replacement based on what the em dash was doing. **A bare comma is almost always wrong** because it usually creates a comma splice (two independent clauses joined by a comma).

| Em-dash pattern | Replacement |
|-----------------|-------------|
| `X — Y` where Y is a noun phrase that defines/elaborates X | colon: `X: Y` |
| `X — Y` where Y is a complete clause | period: `X. Y.` |
| `X — Y` where X and Y are tightly related independent clauses | semicolon: `X; Y` |
| `X — Y, Z, A — B` (mid-sentence parenthetical aside) | parens: `X (Y, Z, A) B` |
| `## Heading X — Y` (heading split into two parts) | colon or parens: `## Heading X: Y` or `## Heading X (Y)` |
| Bullet list separator: `- name — description` | colon: `- name: description` |

```markdown
✅ "Changesets are caller intent: package names, bump levels, body."  (colon for elaboration)
✅ "monorel deletes consumed changesets. Pre-release mode keeps them."  (period for two clauses)
✅ "WithFields merges; WithoutFields removes by key."  (semicolon for tight pair)
✅ "Use --json (or pipe to jq) to parse the plan."  (parens for aside)
✅ "## Pre-release mode: enter / exit / status"  (heading colon)
✅ "- patch: backwards-compatible fixes."  (bullet colon)

❌ "Changesets are caller intent, package names, bump levels."  (comma splice / list confusion)
❌ "monorel deletes consumed changesets, pre-release mode keeps them."  (comma splice)
```

When in doubt: split into two sentences. Period is always safe.

### Heading patterns

- Two-part headings use a colon: `## Pre-release Mode: enter / exit`, not `## Pre-release Mode — enter / exit` and not `## Pre-release Mode, enter / exit`.
- A heading should not look like a comma-separated list unless it actually is one.

### Frontmatter gotcha

If a frontmatter `description:` value contains a `:`, the value must be quoted, otherwise the YAML parser fails:

```yaml
✅ description: "Per-package release config: tag_prefix, path, changelog."
❌ description: Per-package release config: tag_prefix, path, changelog.
```

### Cross-reference link text

When linking to a sub-heading, use just the heading title:

```markdown
✅ See [Pre-release Mode](/cli-reference#pre-release-mode).
❌ See [CLI Reference, Pre-release Mode](/cli-reference#pre-release-mode).
❌ See [CLI Reference — Pre-release Mode](/cli-reference#pre-release-mode).
```

### General rules

- Lead with the conclusion, then explain.
- Default to short sentences; one idea per sentence.
- Don't write paragraphs that exist only to introduce the next paragraph.
- Avoid "let's", "we'll", "you'll find that" filler.

This applies to every doc page, README, code comment, commit message, PR description, and chat response.

### Density: prefer sub-sections to dense bullets

If a section is a bulleted list where each bullet is its own multi-sentence concept, break it into named `###` sub-sections. The reader should be able to scan headings to find the strategy that matches their situation. Heuristic: 2+ bullets that each combine a name, a technique, caveats, and a link → split.

Single-sentence bullets are fine and don't need this treatment.

### Long enumerative paragraphs: split into bullets

If a paragraph is structurally an enumeration (a list of distinct items strung together with commas / semicolons / "then" clauses), break it into a bulleted list. Bullets give each item its own line and let the reader skim instead of parse.

Heuristics:

- 3+ comma-separated noun phrases that could each stand alone (e.g. "the orchestrator finds an existing PR by head ref, opens one if missing, updates the title, updates the body, closes it on empty plans").
- 3+ "then X, then Y" clauses describing a sequence of steps.
- A paragraph that's wider than ~80 words and carries no narrative connective tissue between items.

The complement of the rule above this one: dense bullets become sub-sections; dense prose enumerations become bullets.

```markdown
❌ The `pr` command stages a fresh `monorel/release` branch off the default branch, runs the speculative apply, force-pushes the result, opens or updates the always-open release PR with the rendered plan in its body, and closes any open release PR if the planner has nothing to apply.

✅ The `pr` command:
   - Stages a fresh `monorel/release` branch off the default branch.
   - Runs `monorel apply` (writes CHANGELOG entries, deletes consumed changesets, makes one commit).
   - Force-pushes the result.
   - Opens or updates the always-open release PR with the rendered plan in its body.
   - Closes any open release PR if the planner has nothing to apply.
```

A short list of two items in prose is fine ("CHANGELOG entries and changeset deletions"). The trigger is when the comma list is doing the work a list ought to do.

### Casual users vs implementers

Pages aimed at *callers* (getting-started, configuration, cli-reference, changesets) should not bleed implementation details that only a provider author needs. That includes:

- The `provider.Client` interface shape.
- The factory dispatch mechanism.
- Internal package layout.

Implementer-only material lives in `design.md` and the per-provider creator material (none yet). Casual-user pages can mention that those creator pages exist (one-liner pointer is fine), but should not paraphrase their content.

## README Requirements

The repo root `README.md` should be minimal:

1. Project name and one-line description.
2. Install command (`go install github.com/disaresta-org/monorel/cmd/monorel@latest` or the GitHub Action ref).
3. Minimal usage example (init + add changeset + plan + release).
4. Link to the docs site for everything else.

Reserve deeper details for the VitePress site. Do not duplicate full reference content in the README.

## Site Documentation Structure

Docs live under `docs/src/`. Layout:

```
docs/src/
├── index.md                    Homepage (hero + Quick Example + sidebar entry list)
├── introduction.md             "Why monorel?" Why not release-please / changesets / knope?
├── getting-started.md          Install + init + first release end-to-end
├── design.md                   Design principles + tradeoffs
├── configuration.md            monorel.toml reference
├── cli-reference.md            Per-command reference with flags
├── changesets.md               File format, naming, multi-package changesets, pre-mode interaction
├── github-action.md            Action setup, workflow recipes, troubleshooting
└── recipes/
    ├── migration-from-release-please.md
    └── loglayer-go.md          Worked example: 25-module migration
```

Required elements per page:

- Frontmatter with `title` and `description` (used for OG meta).
- One H1 matching the title (or a `# Heading` derived from it).
- Concrete code examples in language-tagged code blocks (` ```go `, ` ```toml `, ` ```sh `).

## Custom Containers (VitePress)

```markdown
::: info Title
Informational note.
:::

::: tip Title
Recommended approach or shortcut.
:::

::: warning Title
Behavior to be aware of; default workflow still works.
:::

::: danger Title
MUST-type instructions or behavior that breaks expectations.
:::

::: details Title
Collapsible block for optional lengthy information.
:::
```

Use `:::danger` sparingly. Reserve it for cases like "this command rewrites tags" or "this flag bypasses the idempotency check."

## VitePress + GitHub Actions / GitLab CI syntax

VitePress treats `{{ ... }}` as Vue template interpolation, even inside backtick code spans at the start of the span. Writing `${{ secrets.GITHUB_TOKEN }}` in inline prose breaks the build with `Cannot read properties of undefined (reading 'GITHUB_TOKEN')`.

Two safe patterns:

- **Move the literal into a fenced code block.** ` ```yaml ` + the YAML + ` ``` ` always renders verbatim.
- **Rephrase to avoid the literal in inline prose.** "Pass the auto-injected token via `with: token:`" does the same job without the `${{ }}` literal.

Don't try to escape the braces in markdown. `\{\{` doesn't help, and HTML entities (`&#123;&#123;`) read terribly inside backticks. Pick one of the two patterns above.

## Don't pin runner-specific file names in conceptual docs

Workflow file names (`release.yml`, `release-pr.yml`, `.gitlab-ci.yml`, `bitbucket-pipelines.yml`) belong on the per-provider integration pages. In CLI reference, cheat-sheet, workflows / design docs, and the recipes, refer to commands and abstractions (`monorel auto`, "the auto feature path") rather than specific files.

Why: when the workflow shape changes (the `monorel auto` consolidation, the `command: pr|release` removal), the integration pages get updated as part of the change. Conceptual docs that name workflow files independently drift out of sync, often invisibly.

```markdown
✅ "`monorel apply` is the speculative-apply primitive invoked by `monorel auto`'s feature path on the staging branch."
❌ "`monorel apply` is used by `release-pr.yml` (CI)."
```

The same rule applies to action wrapper inputs. If you reference `command: pr` or `with: token:` in conceptual docs, that reference will go stale when the wrapper's input shape changes. Keep wrapper specifics on `ci/github/README.md` and the integration pages; refer to monorel commands by name elsewhere.

## Code-verifiable claims need code-verification

Doc claims of the form "the validator rejects X", "the parser requires Y", "the planner deduplicates Z", "the provider client probes once at constructor time" are not prose decoration; they are factual claims about runtime behavior. Each one must be either:

1. Linked to (or co-located with) a test that proves it, or
2. Verified against the relevant code at write time (read the function; confirm the claim).

Two recent failures the audit caught:

- `configuration.md` and `faq.md` claimed `tag_prefix` was rejected by the validator. `validate/`'s `Validate` doesn't check it; omit decodes to `""`. Doc lied; nothing enforced the claim.
- `integrations/bitbucket.md` claimed the username probe runs "once at constructor time." `internal/provider/bitbucket/identity.go` probes lazily on first call. Doc was written from the spec, not the code.

When in doubt, soften the claim ("monorel reads `tag_prefix` from `[packages]` blocks; the empty string produces bare tags") rather than assert specific validator / parser behavior the doc author hasn't verified.

## Pitfalls / traps: inline, not centralized

Don't create a "Common Pitfalls" or similar page that aggregates every footgun. Embed warnings inline on the page that owns the API the trap relates to, using a `::: warning` callout near the relevant API description. Keep the callout short: name the trap, show ❌ and ✅ side-by-side if helpful, link to a deeper page only if there's already a good target.

Why: a centralized pitfalls page bit-rots, gets read once and forgotten, and means readers have to round-trip when they hit a snag. Inline warnings show up exactly when the reader is making the call that could trigger the trap.

Confirmed pitfalls to inline as monorel matures:

- Pre-release mode does NOT delete changesets → `cli-reference.md`, `changesets.md`.
- Bare-tag root requires `tag_prefix = ""` (not omitting the field) → `configuration.md`.
- `monorel release` does NOT push tags → `cli-reference.md`.
- `--publish` requires tags already pushed → `cli-reference.md`.

When you ship a new trap, add a `::: warning` (or `:::danger` for breakage) on the owning page.

## When You Add a New Feature, CLI Flag, or Config Field

Update all of these in the same change:

1. **Relevant doc page** (`configuration.md` for a Config field, `cli-reference.md` for a flag, `changesets.md` for a frontmatter shape).
2. **`docs/src/cheat-sheet.md`** if the change adds a command, file, or env var (the cheat sheet is the at-a-glance index; new surface should appear there).
3. **`docs/src/public/llms.txt`**: concise LLM-facing reference. Add a bullet, table row, or short snippet for the new surface.
4. **`docs/src/public/llms-full.txt`**: comprehensive LLM-facing reference. Add a section, table row, or snippet describing the new surface in full.
5. **`docs/src/index.md`** if the change is a real selling point (rare).
6. **`.changeset/<name>.md`**: ship the change via `monorel add` (or hand-roll the file). The root `CHANGELOG.md` is written from changesets by `monorel release`; do not edit it by hand.
7. **`AGENTS.md`** "Key Design Decisions" if the change introduces a new concept (rare).

For a brand-new provider, also follow `AGENTS.md` "Adding a New Provider".

The two `llms.*` files matter because external users feed them directly into LLM coding assistants; stale entries become wrong answers in third-party tools. They live at `docs/src/public/`, served by VitePress at `/llms.txt` and `/llms-full.txt`.

## When You Make a CLI Output Change

If the change affects what a user sees (table layout, messages, exit codes):

1. Update `cli-reference.md` examples.
2. Verify the integration tests in `internal/cli/*_test.go` still pin the
   right shape; add a test if the output is new.

## Words to Avoid

- "first-party" / "First-party": every transport, plugin, integration, and
  provider in the monorel module is part of the same module; calling
  them "first-party" implies a tier that doesn't exist. Use "built-in"
  if a qualifier is needed at all, or drop the qualifier entirely.
- Em dashes anywhere.
- "Simply", "just", "easily": empty intensifiers.

## Code Examples in Docs

Prefer `monorel.toml` as the canonical config name in examples. Show the call shape correctly:

```sh
# ✅ correct
monorel add --package transports/zerolog:minor --message "Adds X."

# ❌ wrong: --message is a single body, not a list
monorel add --package transports/zerolog:minor --message X --message Y
```

Use real package names from realistic monorepo layouts (`transports/zerolog`, `plugins/redact`, `internal/store`) rather than placeholder names like `pkg-a`, except when the example specifically demonstrates a multi-package shape where placeholders read clearer.

## Configuration Tables

When documenting a config struct, prefer a table for quick scanning:

```markdown
| Field | Type | Default | Description |
|-------|------|---------|-------------|
```

Use code blocks for the full struct shape only when the type/default columns would push line length too far.

## Versioning Note

The repo is currently a single Go module. There is no per-package
`CHANGELOG.md`; the root `CHANGELOG.md` is the authoritative log.
monorel itself maintains it via the same process this tool implements.
