---
"monorel.disaresta.com": patch
---

Fail the tidy pre-flight with a precise error when a released sub-module requires a monorel-managed sibling that is not in the release plan and has no existing tag. The rewriter cannot pin such a sibling, so its placeholder require would otherwise reach the offline tidy (GOPROXY=off) unresolvable and fail with a generic message.
