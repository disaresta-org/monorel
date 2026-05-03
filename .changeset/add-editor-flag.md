---
"monorel.disaresta.com": minor
---

`monorel add` now accepts `-e` / `--editor` to write the changelog body in `$EDITOR` (or `$VISUAL`) instead of the in-place text-area prompt. Mirrors `git commit`'s editor flow: the temp file is pre-seeded with a commented prompt block; lines beginning with `#` are stripped on save; surrounding whitespace is trimmed.

Editor resolution order: `$VISUAL`, then `$EDITOR`, then `vi` / `nano` (Unix) or `notepad` (Windows) when neither env var is set. `--editor` and `--message` are mutually exclusive (passing both is an error).

Useful when:

- The body is more than a one-liner and the in-place text area feels cramped.
- You want syntax highlighting for the markdown.
- You're building a non-interactive package selection (`--package`) but still want a real editor for the body (`monorel add -p foo:minor --editor`).

The default behavior (in-place text prompt via `huh`) is unchanged when `--editor` is not passed.
