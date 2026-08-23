---
"monorel.disaresta.com": patch
---

Document the tidy pre-flight failure modes in the GitHub Action troubleshooting page and the FAQ: an untagged out-of-plan sibling cannot be pinned by the rewriter and must be released first or included in the plan; a cold CI runner's module cache can also block the offline tidy ("module lookup disabled by GOPROXY=off"), which `go mod download all` fixes.
