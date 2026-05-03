---
title: Running monorel in Docker
description: "Use the monorel container image when you can't or don't want to install the binary directly. Useful on macOS and Windows where the unsigned releases trigger Gatekeeper / SmartScreen warnings."
---

# Running monorel in Docker

monorel ships an OCI image at `ghcr.io/disaresta-org/monorel` for `linux/amd64` and `linux/arm64`. Use it when:

- You're on macOS or Windows and don't want to deal with the unsigned-binary warnings (Gatekeeper / SmartScreen).
- You don't have Go installed and don't want to.
- You want a hermetic, reproducible monorel run pinned to a specific version.

The container ships the same monorel binary as the platform releases, just inside Linux.

::: warning Mac / Windows code signing
Distributing signed binaries for macOS and Windows requires paid developer certificates (Apple Developer Program at $99/year; Windows EV at $300+/year). monorel is a small open-source tool and doesn't carry those — the platform releases are unsigned, which is why your OS shows a security prompt the first time you run them. Docker sidesteps the signing question entirely.
:::

## Pull the image

```sh
docker pull ghcr.io/disaresta-org/monorel:latest
# or pin a version
docker pull ghcr.io/disaresta-org/monorel:0.11.0
```

## Run a command

The container's `WORKDIR` is `/workspace`. Mount your repo at that path:

```sh
docker run --rm \
  -v "$PWD:/workspace" \
  ghcr.io/disaresta-org/monorel:latest plan
```

For commands that need a provider token (`monorel publish`, `monorel preview --upsert`), pass it via an environment variable. The shorthand `-e GITHUB_TOKEN` forwards the host's variable of the same name without retyping its value:

```sh
export GITHUB_TOKEN=ghp_…   # (or use your shell's secret store)
docker run --rm \
  -v "$PWD:/workspace" \
  -e GITHUB_TOKEN \
  ghcr.io/disaresta-org/monorel:latest publish
```

## Authoring a changeset

`monorel add` defaults to interactive prompts. Add `-it` so the container has a TTY:

```sh
docker run --rm -it \
  -v "$PWD:/workspace" \
  ghcr.io/disaresta-org/monorel:latest add
```

Or stay non-interactive with explicit flags (preferred for muscle memory):

```sh
docker run --rm \
  -v "$PWD:/workspace" \
  ghcr.io/disaresta-org/monorel:latest \
  add --package "transports/zerolog:minor" --message "Adds Lazy() helper."
```

## Shell alias

To make the `docker run` shell less typing, drop one of these into your shell config:

```sh
# bash / zsh
monorel() {
  docker run --rm -it \
    -v "$PWD:/workspace" \
    -e GITHUB_TOKEN \
    -e GH_TOKEN \
    ghcr.io/disaresta-org/monorel:latest "$@"
}
```

Then `monorel plan`, `monorel add`, etc. work as if it were installed directly.

## Notes

- The image runs as root by default. The files monorel creates (`.changeset/*.md`, `CHANGELOG.md` updates, the release commit) end up owned by uid 0 in your working tree. On Linux that's annoying; on macOS / Windows with Docker Desktop, the file-sharing layer handles ownership translation automatically.
- The image bundles `git` and the Go toolchain. `git` is needed because `monorel release` commits and tags. The Go toolchain is needed because the `apply` step runs `go mod tidy` (offline, against a seeded local cache) so the release commit's `go.sum` is canonically clean for the proxy-published state. The image's Go version pins what tidy can resolve; use a release whose Go version satisfies your modules' `go` directive.
- `monorel release` creates the commit + tag inside the container. Use `git push --follow-tags` from the host afterward — that's not the container's job.
