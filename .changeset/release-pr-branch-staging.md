---
"monorel.disaresta.com": patch
---

Fix release-pr workflows by staging the head branch before opening
the always-open release PR.

GitHub's PR-create API rejects PRs whose head branch doesn't exist
on the remote (`422 Validation Failed [Field:head Code:invalid]`).
Both `ci/github/action.yml` and the self-hosted `release-pr.yml`
now create the configured head branch (default `monorel/release`)
as a fast-forward of the default branch plus one empty marker
commit, then force-push, before invoking `monorel preview --upsert`.

The branch's diff stays empty by design; the rendered plan goes in
the PR body, not in a code diff. At merge time, the squash commit
inherits the orchestrator-set PR title (`chore(release): ...`),
which `release.yml` filters on to trigger the post-merge release
pipeline. So the marker commit's own subject is irrelevant after
squash and there is no two-`chore(release):`-commit churn on main.

Surfaced by loglayer-go's first monorel-driven release attempt,
where the smoke-test PR's `release-pr` workflow run failed with
the 422 error on PR creation.

Branch staging in the orchestrator (Go code) is still on the
roadmap; this CI-wrapper fix unblocks consumers in the meantime
and is the right layer per the `Run` doc-comment ("delegated to
the CI wrapper because it's a thin shell-out to git").
