---
"monorel.disaresta.com": minor
---

Add Gitea provider (also covers Forgejo).

`provider.name = "gitea"` is now a recognized value in `monorel.toml`.
The implementation lives in `internal/provider/gitea` and wraps
[`code.gitea.io/sdk/gitea`](https://gitea.com/gitea/go-sdk).

Forgejo (a Gitea fork that maintains API compatibility) works with
the same provider; point `provider.host` at the Forgejo instance.

Configuration:

```toml
[provider]
name  = "gitea"
host  = "gitea.example.com"
owner = "acme"
repo  = "widget"
```

`provider.host` is required for Gitea/Forgejo because there's no
canonical public instance. The token comes from the `GITEA_TOKEN`
environment variable.

Validates the provider seam: this is the second provider
implementation, the first non-GitHub one. The factory at
`internal/provider/factory/factory.go` documents the three-step
recipe for adding more providers.

A `//go:build livetest` test suite at
`internal/provider/gitea/livetest_test.go` validates the
implementation against a real Gitea instance. Run locally with:

```sh
docker run -d --name monorel-gitea-test -p 3000:3000 gitea/gitea:1.23
# ...complete install wizard, create user, generate token, create repo...
export MONOREL_GITEA_HOST=localhost:3000
export MONOREL_GITEA_TOKEN=<token>
export MONOREL_GITEA_OWNER=<user>
export MONOREL_GITEA_REPO=<repo>
go test -tags=livetest ./internal/provider/gitea
```
