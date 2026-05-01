---
"monorel.disaresta.com": patch
---

Refresh the repo README to reflect current state:

- Drop the "pre-v0.1.0, not yet ready for external use" status
  banner (monorel just shipped v0.5.0).
- Quickstart rewritten around the canonical Action-driven flow:
  `monorel init` instead of hand-writing `monorel.toml`, plus the
  `add → PR → release-pr workflow → merge release PR` lifecycle.
- Reference [`disaresta-org/monorel-example`](https://github.com/disaresta-org/monorel-example)
  as the working starter to fork.
- Documentation list updated: added Workflows, FAQ, Glossary; fixed
  the broken `/bootstrap` link (page moved to
  `/recipes/bootstrapping-monorel`); split CLI / Library API.
- GitHub Action version pin bumped from `@v0.1.2` to `@v0.5.0`.
- Added a one-liner pointer to the token-guidance section for
  branch-protection users.
