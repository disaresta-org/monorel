---
"monorel.disaresta.com": minor
---

`monorel add` now uses a [huh](https://github.com/charmbracelet/huh)
form when stdin is an interactive terminal: arrow-key multi-select
for packages, per-package bump-level select, and a multi-line text
field for the changelog body.

Non-TTY stdin (piped input, redirected files, scripted use, tests)
falls back to the existing line-based bufio prompt — the contract
that `printf '1\nminor\nFix.\n\n' | monorel add` works is preserved.
The auto-detected TTY check uses `golang.org/x/term.IsTerminal`.

This was in the original v1 plan as a direct dependency but never
landed; the form is now what the plan called for.
