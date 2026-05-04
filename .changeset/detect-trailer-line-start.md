---
"monorel.disaresta.com": patch
---

**Fix `detect-release` false-positive on prose mentions of the trailer marker.**

`detect.IsReleaseMerge`'s trailer fast path used `strings.Contains(headBody, "monorel-Release:")`, which matched anywhere in the body, including prose mentions of the marker (e.g., docs commits that explain how the trailer works, or squash-merge bodies that aggregate sub-commit messages discussing release tooling).

The CI symptom was a contradiction: `monorel detect-release` reported "release commit detected (source: trailer)" on a non-release commit, then the next pipeline step `monorel tag` correctly rejected HEAD with `ErrNoReleaseCommit`. The release workflow exited non-zero on every push to main whose squash body coincidentally contained the literal text `monorel-Release:`.

The fix line-anchors the match to mirror the canonical parser at `release.parseReleaseTrailers`: the marker must appear at the start of a (whitespace-trimmed) line. Detect and tag now agree on what counts as a real trailer.
