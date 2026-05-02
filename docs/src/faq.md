---
title: FAQ
description: "Answers to the questions that come up after you've read the rest of the docs: editing changesets, multi-PR coordination, recovery, and the boundaries of what monorel handles."
---

# FAQ

Common questions about authoring, releasing, and operating monorel. Reference docs answer "what does this command do"; this page answers "what should I do when..."

## Authoring changesets

### Can I edit a changeset after I've written it?

Yes, until it's consumed at release time. A changeset file is just markdown; edit `.changeset/<name>.md` directly and commit the change. If the changeset is on a feature branch that's already merged, open a follow-up PR with the edit; the always-open release PR will pick up the new content on the next push to `main`.

### Can I delete a changeset to skip a release?

Yes. Delete the file from `main` (open a PR that removes it). The next `release-pr` workflow run will recompute the plan without that changeset. If the changeset was the only one pending, the always-open release PR will close itself.

### What if I forget to add a changeset?

Open a follow-up PR adding the changeset. monorel doesn't enforce "PR must have a changeset"; that's a convention, optionally enforced by a separate CI check. If you want to enforce it, see the changesets-style ["Are changesets required?"](https://github.com/changesets/changesets/blob/main/docs/intro-to-using-changesets.md) check pattern; monorel doesn't ship one because what counts as "release-affecting" is repo-specific.

### Can a PR include multiple changeset files?

Yes. monorel reads every `.changeset/*.md` file regardless of which PR added it; a PR can include zero, one, or many. Three patterns come up:

- **One file, one package.** The simple case. `monorel add --package "foo:minor" --message "..."` writes one file targeting one package.
- **One file, multiple packages** (a multi-package changeset). When a single coordinated change touches several packages (e.g. a feature in package A that requires a pass-through patch in package B), name them all in the same changeset's frontmatter. The body becomes the changelog entry for every named package, bucketed at each package's bump level.
- **Multiple files in one PR.** When a PR contains genuinely independent changes (a feature in package A and an unrelated fix in package B), write one changeset file per change. The bodies appear separately in the rendered CHANGELOG, which reads cleaner than one merged entry.

Rule of thumb: write one changeset per **coherent change**, not one per PR. A PR with N independent changes deserves N changeset files; a PR with one change spanning N packages deserves one multi-package changeset.

### Can multiple PRs target the same package in the same release window?

Yes. Multiple changesets stacking on one package combine: the **strongest bump level** wins, and every changeset's body appears in the rendered CHANGELOG entry. Three PRs each adding a `patch` changeset to `transports/foo` plus one PR adding a `minor` produce a single `minor` release containing all four entries.

## The release PR

### Does the release PR auto-rebase?

No, it's force-pushed from a fresh branch off `main` on every workflow run. The release PR's branch (`monorel/release` by default) is bot-managed and doesn't preserve history; each push to `main` rebuilds it from scratch via speculative apply. Reviewers always see a diff against current `main`.

### Can I have multiple release PRs open at once?

No. monorel maintains exactly one always-open release PR per repo. The orchestrator finds an existing open PR by head ref (`monorel/release`) and updates it; if no open PR matches, it creates one. Closing the release PR without merging "cancels" the current release window; the next push to `main` with pending changesets will create a new one.

### What if I want to ship one package but not others?

You can't selectively merge a release PR. Two ways to handle this:

1. **Keep only the changesets you want in this window.** Delete the changesets for packages you don't want to ship yet. They'll be released in a future window.
2. **Two separate release windows.** Merge the release PR with the current changeset set, then the next push to `main` opens a new release PR with whatever's pending.

There's intentionally no "release subset" mode: separating the decision ("which packages ship") from the timing ("when to merge") would multiply the failure modes.

### What happens if I push a commit directly to `main` while the release PR is open?

The next `release-pr` workflow run rebuilds the staging branch off the new `main`. The release PR's diff updates to include the new commit's effect (a new changeset, if any). If the commit is unrelated to releases (no new changeset), the release PR's diff stays the same, just rebased.

## Tags and versions

### What's the initial version of a brand-new package?

Determined by the first changeset's bump level. `:patch` produces `v0.0.1`, `:minor` produces `v0.1.0`, `:major` produces `v1.0.0`. There's no separate "initial release" mode; the bump level controls.

### Can I downgrade a published version?

No, git tags are write-once. Releasing `v1.6.0` and wanting to "undo it" means shipping `v1.6.1` (or `v1.7.0`) with a fix. Yanking is GitHub-side: `gh release delete v1.6.0` removes the GitHub Release, but the tag itself stays in git unless force-deleted (which breaks anyone who already pulled it).

### Why does my root module need `tag_prefix = ""` (empty string)?

The empty string is the explicit signal for bare-tag root (`vX.Y.Z` instead of `<prefix>/vX.Y.Z`). The validator requires the field to be set explicitly; omitting it is rejected so a missing field doesn't silently default to bare. Set `tag_prefix = ""` for the root module in your `monorel.toml`.

### Can I rename a package after it's released?

Yes, but the tag prefix stays for historical tags. Edit `monorel.toml`'s package key (the `[packages."<name>"]` block name) and `tag_prefix`. Existing tags under the old prefix continue to point at their commits; future releases use the new prefix. Update changeset frontmatter keys accordingly.

## Pre-release mode

### What happens to changeset files during pre-release?

They're preserved across multiple pre-release cuts. `monorel pre enter rc` followed by ten `monorel release` runs leaves all the changeset files in `.changeset/` the entire time. The files get consumed (deleted, with their bodies merged into CHANGELOGs) only at the next stable release after `monorel pre exit`.

### How do I exit pre-release mode without bumping the version?

`monorel pre exit` only removes the state file; it doesn't trigger a release. The next `monorel release` (or the next merged release PR) is the stable bump. If you want to skip the stable bump too, delete the pending changesets before that release runs.

### Can I switch pre-release channels (e.g. `rc` → `beta`)?

Run `monorel pre exit` first, then `monorel pre enter beta`. monorel rejects entering a new channel while another is active; the version-suffix counter starts fresh per channel.

## Recovery

### `monorel publish` failed partway through. What now?

Re-run it. `CreateRelease` is idempotent on the tag name: each tag the prior run already published returns "release exists" from the provider, which monorel's partial-success path treats as success. The remaining tags publish on the second run. The output line reports `Created N/M releases before failing.` on the first run; the re-run completes the rest.

### A release PR merged without `monorel-Release:` body trailers, and `monorel tag` returned `ErrNoReleaseCommit`. What now?

The merge stripped the trailer block. Most likely cause: a squash-merge setting that drops the commit body (see [Branch protection](/integrations/github#branch-protection)). Recovery: manually create the tags pointing at the merge commit:

```sh
git tag -a <prefix>/v<X.Y.Z> <merge-sha> -m "Release <prefix> v<X.Y.Z>"
# Repeat for each package the release PR was supposed to bump.
git push --follow-tags
GITHUB_TOKEN=... monorel publish
```

Then fix the squash-merge setting before the next release.

### A previous release run partially succeeded (some tags exist, others don't). The next run fails on `ErrTagExists`. How do I recover?

`monorel tag` aborts on the first existing tag in its plan (preflight check). Delete the partial tags so the next run can create the full set:

```sh
git tag -d <prefix>/v<X.Y.Z>
git push origin :refs/tags/<prefix>/v<X.Y.Z>
# Repeat for each tag created by the partial run.
```

Then re-run the release workflow (or `monorel tag` locally followed by push + publish).

## Boundaries

### Can monorel manage a polyglot repo (Go + Python + Rust)?

No, monorel is Go-only by design. The semver math, tag conventions, and changelog format are calibrated to Go's module system. For polyglot, [changesets](https://github.com/changesets/changesets) (with custom adapters) and [Knope](https://knope.tech) are the right shape.

### Can monorel run without git?

No. The planner reads tags from git history to determine the current version per package. There's no separate "manifest file" mode (release-please's pattern); git tags are the source of truth.

### Does monorel require GitHub?

No. As of v0.7, `provider.name` accepts `"github"`, `"gitea"`, and `"gitlab"`. The Gitea implementation also covers Forgejo via API compatibility (set `host` to your Forgejo instance). Bitbucket isn't implemented yet but the provider seam is ready; the path is documented in `internal/provider/factory/factory.go`.

### Can monorel coordinate releases across multiple repos?

No, monorel is single-repo only. Each repo gets its own `monorel.toml` and its own release PR. Cross-repo coordination (one trigger that releases packages across two repos) is out of scope; the release-PR pattern doesn't generalize across remotes cleanly.
