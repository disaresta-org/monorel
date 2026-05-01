---
"monorel.disaresta.com": patch
---

Documents the GitHub `GITHUB_TOKEN` anti-recursion limitation and
the three workarounds (PAT, GitHub App, ruleset bypass) under a new
`Tokens and required status checks` section in
`docs/src/github-action.md`. This is the recurring pain point that
bites every consumer with branch-protection-required status checks.

Adds a matching troubleshooting entry ("Release PR is stuck on
'Some checks haven't completed'") that points at the new section.
