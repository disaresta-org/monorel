---
"monorel.disaresta.com": patch
---

Drop `monorel publish` from monorel's own release.yml.

v0.2.0 surfaced a fatal interaction between `monorel publish` and
goreleaser when both run in the same release pipeline:

1. `monorel publish` creates the GitHub Release for the new tag,
   with a body sourced from the rendered CHANGELOG entry.
2. `build-binaries` runs goreleaser via `workflow_call`. Goreleaser
   sees the existing release, attempts to PATCH it to attach the
   built binaries, and (in recent goreleaser versions hitting
   GitHub's new immutable-release feature) flips the release to
   `immutable: true` as a side effect of the PATCH.
3. Subsequent uploads fail with `422 Cannot upload assets to an
   immutable release`.

Once a tag has been used by an immutable release, GitHub permanently
retains the tag name even if the release is deleted: no new release
can be created with the same tag. v0.2.0 is therefore stuck without
binaries; consumers should pin to v0.2.1 or later for the action
wrapper.

Fix: monorel's own release.yml lets goreleaser own release creation.
The release job runs `monorel release` + `git push --follow-tags`
only; goreleaser (via build-binaries) creates the release with its
binary uploads in one step.

Trade-off: monorel's own GitHub Release bodies are goreleaser's
auto-generated commit list rather than the curated CHANGELOG entry.
The per-package CHANGELOG.md still contains the curated entry; this
only affects the body shown on the GitHub Release UI.

This fix is monorel-self-hosting-specific. Other consumers
(loglayer-go and similar) call the action wrapper, which still runs
`monorel publish`. Those repos keep producing curated release bodies
because they don't run goreleaser as part of their release flow.
