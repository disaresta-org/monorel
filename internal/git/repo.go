// Package git is the I/O seam between monorel's planning logic and an
// underlying git repository.
//
// Higher layers (planner, release applier, action orchestrator) take a
// Repo and never shell out themselves. The default implementation in
// this package shells out to the `git` CLI; tests use Fake.
package git

// Repo is the slice of git operations monorel needs. Methods are
// intentionally narrow: monorel does not implement git itself, just
// the handful of commands a release flow performs.
//
// Implementations:
//   - [Exec] (default) — shells out to the `git` CLI in a working
//     directory. The CLI is already on every CI runner and dev box;
//     we don't pull in go-git's 50k-LOC partial reimplementation just
//     for these few commands.
//   - [Fake] (in-memory) — for unit tests of higher layers.
//
// All methods return wrapped errors that include the underlying git
// stderr output when shelling out fails.
type Repo interface {
	// ListTags returns every tag whose name starts with prefix. An
	// empty prefix returns all tags. Order is unspecified; callers
	// that need semver order sort with semver.Compare.
	ListTags(prefix string) ([]string, error)

	// TagsAtHead returns every tag pointing at the current HEAD
	// commit (i.e. `git tag --points-at HEAD`). Used by `monorel
	// publish` to find which tags to create provider Releases for.
	// Order is unspecified.
	TagsAtHead() ([]string, error)

	// CurrentSHA returns the SHA at HEAD.
	CurrentSHA() (string, error)

	// HeadCommitMessage returns the full commit message of HEAD
	// (subject + body, including any trailers). Used by `monorel tag`
	// to read the trailers `monorel apply` writes for downstream
	// tag creation.
	HeadCommitMessage() (string, error)

	// Add stages the named paths. No-op when paths is empty.
	Add(paths ...string) error

	// Remove unstages and deletes the named paths from the working
	// tree (i.e. `git rm`). Used to consume changeset files at
	// release time.
	Remove(paths ...string) error

	// Commit creates a commit with the given message at HEAD.
	// Returns an error if there is nothing staged.
	Commit(message string) error

	// CreateTag creates an annotated tag at HEAD. message is the
	// tag annotation; pass an empty string for a lightweight tag.
	CreateTag(name, message string) error

	// Push pushes ref to remote (e.g. Push("origin", "main") or
	// Push("origin", "monorel/release"). force=true uses --force-
	// with-lease so a stale local view doesn't clobber an upstream
	// update we don't know about.
	Push(remote, ref string, force bool) error

	// PushTags pushes every local tag to remote. monorel pushes
	// tags after the release commit lands so the order in the
	// remote's reflog is "commit, then its tags."
	PushTags(remote string) error

	// CheckoutBranch checks out branch, creating it from the
	// current HEAD if it doesn't exist (`git checkout -B`). Used by
	// the action orchestrator to manage the always-open release
	// branch.
	CheckoutBranch(branch string) error

	// IsClean reports whether the working tree has no uncommitted
	// or untracked changes (i.e. `git status --porcelain` is empty).
	// Pre-release safety check: monorel refuses to run release on
	// a dirty tree.
	IsClean() (bool, error)

	// DeletedFilesInCommitsMatching returns the union of every file
	// path deleted (`--diff-filter=D`) by a commit whose subject or
	// body literally contains messageGrep. The substring match is
	// case-sensitive and does NOT use regex.
	//
	// Used by `monorel doctor` to scan past `chore(release):`
	// commits for the changeset filenames they consumed, so a
	// stale-branch + squash-merge revival can be detected.
	//
	// Order: deduplicated, sorted lexicographically. Includes paths
	// that were re-added by later commits — callers intersect
	// against the live tree to flag that case.
	DeletedFilesInCommitsMatching(messageGrep string) ([]string, error)
}
