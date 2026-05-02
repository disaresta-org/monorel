---
"monorel.disaresta.com": patch
---

Auto-truncate the always-open release PR body when it would exceed the provider's body limit. The orchestrator now falls back through three forms: full rendering with per-package release notes (default), compact rendering with the version table only (when the full body exceeds 65,536 chars), or hard byte-truncation with a trailing marker (last-resort safety net for releases with hundreds of packages where even the table blows past the limit).

Fixes [#37](https://github.com/disaresta-org/monorel/issues/37): the loglayer-go v2 cascade triggered GitHub's `422 Validation Failed: body is too long` because 27 packages × a single multi-package changeset body × table overhead pushed the rendered PR body past 65,536 chars.
