## What this changes

<!-- A short description of the change. Lead with the conclusion. -->

## Changeset

- [ ] I added a `.changeset/*.md` file describing this change for users.
- [ ] OR: this PR doesn't need a release entry (typo, refactor with no observable change, docs-only).

If you're not sure, run `monorel add` and follow the prompts.

## Verification

- [ ] `go test ./...` passes locally.
- [ ] If touching docs: `cd docs && bun run docs:build` is clean.
- [ ] If touching the `forge.Client` contract: I added or updated tests against `forge.NewFake`.

## Notes for reviewers

<!-- Anything tricky, intentional, or worth pushing back on. -->
