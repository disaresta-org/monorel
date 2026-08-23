---
"monorel.disaresta.com": patch
---

Align each package's planned release version with the major its Go module path requires (e.g. a /v3 module releases at v3.x.y, not v2.0.0). config.Load derives the module major from each package's go.mod directive; the planner clamps :major/:minor/:patch bumps (and first releases) to it. Fixes releases failing at cache-seed with 'invalid version: should be v3, not v2' for sub-modules whose module path major changed without a matching tag history.
