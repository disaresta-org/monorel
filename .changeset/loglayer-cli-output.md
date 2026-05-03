---
"monorel.disaresta.com": minor
---

`monorel` now drives all CLI output through a structured logger. Three new persistent flags compose the output:

- `--color=auto|always|never` controls ANSI color (auto detects whether stdout is a TTY).
- `-v` / `-vv` increase verbosity: `-v` enables debug messages, `-vv` also appends key/value fields after each line.
- `-q` / `--quiet` suppresses info and warn output, leaving only errors.

Status and plan tables now render via the logger's table support, so column alignment stays consistent across terminals and pipes.
