# Changesets

This directory holds pending release intents for [monorel](https://monorel.disaresta.com).

A `.changeset/<name>.md` file declares which packages should release at what bump level when the next release lands, plus the changelog body to use for each release.

```markdown
---
"<package-name>": minor
---

What changed and why.
```

Run `monorel add` for the interactive flow, or write the file directly. See [the docs](https://monorel.disaresta.com/changesets) for details.
