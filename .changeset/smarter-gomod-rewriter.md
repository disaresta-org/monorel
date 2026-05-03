---
"monorel.disaresta.com": minor
---

The release-time `go.mod` rewriter now pins sibling requires for managed packages outside the current release plan, not just packages being released.

Before: releasing a single sub-module that required another monorel-managed sibling would leave the sibling's require at whatever the dev `go.mod` specified (typically a placeholder pseudo-version), because the rewriter only built its sibling map from packages in the current plan. This forced contributors to include the root module in every recovery release just to seed the sibling map (see [loglayer/loglayer-go#70](https://github.com/loglayer/loglayer-go/pull/70)).

After: the rewriter walks every package declared in `monorel.toml`. In-plan packages pin to their planned version; out-of-plan packages pin to their latest existing stable tag (resolved through `plan.LatestStableTagVersion`, newly exported). Out-of-plan siblings with no existing tag (or only pre-release tags) leave the require alone instead of failing the release.

Closes #43.
