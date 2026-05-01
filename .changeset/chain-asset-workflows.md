---
"monorel.disaresta.com": patch
---

Chain `build-release-binaries` and `build-image` into `release.yml`
via `workflow_call` so they fire after every monorel-driven release.

When `release.yml` pushes a tag using `secrets.GITHUB_TOKEN`, GitHub's
anti-recursion rule suppresses the resulting `push: tags` event for
other workflows. That meant downstream tag-triggered workflows (the
binary builder and the container builder) silently didn't fire on
real releases — only on the v0.1.0 bootstrap, which was cut via
`workflow_dispatch` (a user-initiated run that doesn't trip the
anti-recursion rule).

Surfaced by monorel's own v0.1.1 release: the tag and GitHub Release
appeared, but no binaries were attached and no image was pushed to
GHCR. v0.1.1 is consequently usable only via `go install`, not via
the action wrapper or `docker pull`.

Mirrors the same `workflow_call` workaround `docs.yml` already uses
(in loglayer-go's release.yml — monorel's docs.yml deploys on every
push to main, so it doesn't need this).

Both build workflows now accept a `tag` input via `workflow_call` and
`workflow_dispatch`. The tag-push trigger is preserved so manual
`git push <tag>` flows still work. `release.yml`'s release job
captures the released root tag (`vX.Y.Z`) and passes it to both build
workflows; the chain skips when no root tag was created (sub-module-
only release; not yet possible for monorel itself, but forward-looking).

v0.1.1's missing assets are not backfilled by this change; users on
v0.1.1 should bump to v0.1.2.
