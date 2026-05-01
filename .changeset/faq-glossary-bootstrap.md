---
"monorel.disaresta.com": patch
---

Three structural docs improvements based on a comparison against
how changesets / release-please / Knope organize their sites:

- New `docs/src/faq.md`: ~20 entries grouped by Authoring,
  Release PR, Tags and versions, Pre-release mode, Recovery, and
  Boundaries. Covers the questions that don't fit into reference
  docs ("Can I edit a changeset?", "What if I forget to add a
  changeset?", "Can I downgrade a published version?",
  "Recovery from `ErrTagExists`?").

- New `docs/src/glossary.md`: ~20 canonical definitions of terms
  used across the docs (changeset, speculative apply, tag prefix,
  bare-tag root, trailer block, pre.json, etc.). Resolves the
  ambiguity of overloaded terms that previously had no single
  authoritative definition.

- `docs/src/bootstrap.md` moved to
  `docs/src/recipes/bootstrapping-monorel.md`. The page documents
  monorel's one-time self-hosted bootstrap (a recipe for the next
  maintainer if the tool ever forks); the previous top-level
  location implied users needed to do this in their own repo. Now
  groups with the other recipes, sidebar entry renamed to
  "Bootstrapping monorel itself" for clarity.

Sidebar updated:
- Glossary added under Reference.
- New "Help" group containing the FAQ entry.
- Bootstrap recipe moved + renamed.

The single inbound link to `/bootstrap` (in `github-action.md`)
updated to point at the new recipe URL.
