---
"monorel.disaresta.com": minor
---

**Provider-API release detection.**

Two new monorel subcommands replace the previous text-pattern release-detection:

- `monorel detect-release` reports whether HEAD is the merge of monorel's release PR. Exit 0 yes, 1 no, 2 error.
- `monorel auto` is the one-stop CI command. It detects, then runs the release pipeline (tag + push + publish) or the feature pipeline (apply + push + preview --upsert) accordingly.

The action wrapper at `disaresta-org/monorel/ci/github` simplifies to a single auto step. The `command: pr`, `command: release`, and `command: doctor` inputs are removed. Each provider's example workflow / pipeline file collapses to one file with one step that runs `monorel auto`. The `monorel doctor` workflows install monorel directly and run the command as a standalone step.

Detection uses two signals OR'd together: the `monorel-Release:` trailer in HEAD's commit body (fast path; squash + rebase) and the provider's `FindPRByMergeCommit` returning a PR whose source branch is `monorel/release` (network signal; covers merge-commit and Bitbucket squash). Either signal alone is sufficient.

Migration from v0.14:

- Replace `command: pr` and `command: release` workflow steps with a single step (no `command:` input) that runs the action wrapper. The wrapper runs `monorel auto` internally.
- `command: doctor` users invoke `monorel doctor` as their own step (install monorel via `go install monorel.disaresta.com/cmd/monorel@latest` first).
- Custom CI scripts that text-grep `chore(release):` or `monorel-Release:` from commit messages should switch to running `monorel detect-release` and branching on its exit code.
