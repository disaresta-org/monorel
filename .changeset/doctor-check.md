---
"monorel.disaresta.com": minor
---

Add `monorel doctor` command and public `doctor` package.

The new diagnostic catches a stale-branch + squash-merge revival of a
previously-consumed `.changeset/*.md` file: a contributor branches
from `main` BEFORE a release commit lands, and their PR is later
squash-merged. GitHub's squash-merge re-introduces the file the
release commit deleted; the next release plan re-ships the same
content under a new version. monorel's planner does what its spec
says (changesets on main = stuff to release); the input is the bug.
doctor catches the bad input.

CLI:

```sh
monorel doctor          # text output
monorel doctor --json   # machine-readable
```

Exits non-zero on any error-severity finding so CI can gate on it.

Library:

```go
findings, err := doctor.Run(doctor.Options{
    ChangesetDir: ".changeset",
    GitLog:       repo.DeletedFilesInCommitsMatching,
})
```

`doctor.GitLog` is a function value; any git library works as the
backing store. The check itself walks `git log --diff-filter=D
--grep='chore(release):'` to build the set of previously-deleted
changeset filenames, then intersects with the live `.changeset/`
directory.

Built as a check-runner internally so future checks (orphan
changesets, malformed frontmatter, etc.) drop in without
re-architecting; today only `revived-changeset` ships.

Verified end-to-end against the real revival incident on the monorel
repo (PR #29 changeset that PR #30 deleted but PRs #31 + #32
revived via stale-branch + squash-merge): doctor flags it as a
SeverityError with check name `revived-changeset`, the prior PR
hand-cleanup (#34) is no longer needed.
