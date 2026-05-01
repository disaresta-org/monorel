---
"monorel.disaresta.com": patch
---

Fix `release-pr` workflow regression where `git push --force-with-lease`
rejected the staged release branch with "stale info". The previous
`monorel apply` PR replaced `git push -f` with `--force-with-lease`
on review feedback, but `--force-with-lease` requires a previously-
fetched value of the remote ref to compare against — and the
speculative-apply step builds the local `monorel/release` from
`origin/main` without ever fetching the remote `monorel/release`,
so the lease has no expected value and the push is rejected.

Reverted to plain `git push -f` in both `.github/workflows/release-pr.yml`
and `ci/github/action.yml`. The `monorel/release` branch is
bot-exclusive (the workflow is its only writer), so blind force-push
is the intended behavior. Comments updated to spell out why.
