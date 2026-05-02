---
"monorel.disaresta.com": minor
---

Add GitLab provider (third provider after GitHub and Gitea/Forgejo).

`provider.name = "gitlab"` is now a recognized value in
`monorel.toml`. The implementation lives in
`internal/provider/gitlab` and wraps
[`gitlab.com/gitlab-org/api/client-go`](https://gitlab.com/gitlab-org/api/client-go)
(the official GitLab Go SDK).

Configuration:

```toml
[provider]
name  = "gitlab"
host  = "gitlab.com"          # or your self-hosted instance
owner = "team/platform"       # may contain slashes for sub-groups
repo  = "widget"
```

`provider.host` defaults to `gitlab.com`. The `Owner` field accepts
nested sub-group paths (e.g. `team/platform`); the SDK URL-encodes
them automatically. Token comes from `GITLAB_TOKEN` (falls back to
`CI_JOB_TOKEN` in pipelines); needs `api` scope.

A `//go:build livetest` test suite at
`internal/provider/gitlab/livetest_test.go` validates the
implementation against a real GitLab project. Tested end-to-end on
gitlab.com with the full pipeline (init → add → apply → push →
preview --upsert → merge MR → tag → push tags → publish).

GitLab specifics worth knowing:

- **Project merge method must be Fast-forward** for `monorel tag`
  to find the `monorel-Release:` trailers post-merge. The default
  `merge` method creates a merge commit that strips the body.
- **GitLab Releases have no first-class prerelease flag**. The
  SemVer pre-release suffix on the tag (e.g. `-rc.0`) is the only
  signal; monorel's tag naming already encodes it.
- **Sub-groups are supported** via the `Owner` field accepting
  slashes.

Hard cut: no backward-compat shims (pre-1.0). Existing GitHub and
Gitea consumers are unaffected.

Examples directory:

The `examples/` directory in the monorel repo now has minimal
reference setups for each provider (`examples/github/`,
`examples/gitea/`, `examples/gitlab/`). Each contains a
`monorel.toml` + workflow files + `.changeset/README.md` that
users can copy into their own repo. Replaces the previous
disaresta-org/monorel-example external repo as the canonical
"working example" reference.
