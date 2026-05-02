// Package doctor diagnoses repository state issues that monorel's
// release planner won't catch on its own.
//
// The motivating case: a stale-branch + squash-merge revival of a
// previously-consumed `.changeset/*.md` file. After a release ships,
// the `chore(release):` commit deletes the consumed changesets. If
// a contributor branched from main BEFORE that release commit and
// later squash-merges their PR, GitHub re-introduces the deleted
// files. The next release-pr cycle then re-ships the same content
// under a new version. monorel's planner is doing exactly what its
// spec says (changesets on main = stuff to release); the input is
// the bug. doctor catches the bad input.
//
// monorel's CLI exposes Run via `monorel doctor`. Programmatic
// callers (custom CI checks, lint scripts) can invoke Run directly
// against any git seam by supplying a [GitLog] function.
package doctor

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Severity classifies a [Finding]'s urgency. The string form is the
// JSON wire value; matches the constants the validate package uses
// for the same concept.
type Severity string

const (
	// SeverityError flags an issue that will produce a wrong
	// release if not fixed. Callers should fail closed.
	SeverityError Severity = "error"

	// SeverityWarning flags an issue that may not break a release
	// but usually indicates a mistake. Callers may surface as a
	// warning without failing the build.
	SeverityWarning Severity = "warning"
)

// Finding is one issue [Run] surfaced.
type Finding struct {
	// Severity is the urgency classification.
	Severity Severity

	// CheckName names the diagnostic that produced the finding
	// (e.g. "revived-changeset"). Stable; safe to match in tests.
	CheckName string

	// Path is the repo-relative path the finding refers to.
	// May be empty for findings not tied to a single file.
	Path string

	// Message is a one-sentence human-readable explanation.
	Message string
}

// GitLog is the slice of git history doctor needs. The function
// returns every file path deleted by a commit whose subject or body
// contains messageGrep as a literal substring. Match is
// case-sensitive; not a regex.
//
// Implemented by `internal/git.Repo.DeletedFilesInCommitsMatching`;
// other callers can adapt any git library by supplying a function
// with this signature.
type GitLog func(messageGrep string) ([]string, error)

// Options configures a [Run] invocation.
type Options struct {
	// RepoDir is the repository root (the directory containing
	// `monorel.toml` and `.changeset/`). Required.
	//
	// The doctor package only supports the canonical changeset
	// directory name `.changeset` — both because monorel itself
	// only writes that name and because the historical scan
	// matches against `git log --name-only` output, which always
	// emits repo-relative paths starting with that prefix. If a
	// future monorel version supports a different name, this
	// option would gain a sibling for the override.
	RepoDir string

	// GitLog returns previously-deleted files for a given commit-
	// message substring. Required.
	GitLog GitLog

	// ReleaseCommitGrep is the literal substring used to identify
	// release commits in the git log. Defaults to
	// "chore(release):" (matching monorel's own release commits).
	// Override only when integrating against a release-commit
	// convention monorel did not produce.
	ReleaseCommitGrep string
}

// DefaultReleaseCommitGrep is the substring [Run] uses when
// Options.ReleaseCommitGrep is empty. Matches every commit message
// monorel writes via `monorel apply` / `monorel release`.
const DefaultReleaseCommitGrep = "chore(release):"

// Run executes every built-in check against the repository state
// described by opts. Returns the findings in deterministic order
// (sorted by CheckName, then Path) plus any error encountered while
// gathering inputs (e.g. failure to list previously-deleted files).
//
// An empty findings slice with a nil error means "nothing wrong."
// A non-empty slice with a nil error means the checks ran cleanly
// and surfaced issues in the repository; callers decide whether to
// treat findings as blocking based on each finding's Severity.
func Run(opts Options) ([]Finding, error) {
	if opts.RepoDir == "" {
		return nil, errors.New("doctor: RepoDir is required")
	}
	if opts.GitLog == nil {
		return nil, errors.New("doctor: GitLog is required")
	}
	grep := opts.ReleaseCommitGrep
	if grep == "" {
		grep = DefaultReleaseCommitGrep
	}

	findings, err := checkRevivedChangesets(opts.RepoDir, opts.GitLog, grep)
	if err != nil {
		return nil, err
	}

	sort.SliceStable(findings, func(i, j int) bool {
		if findings[i].CheckName != findings[j].CheckName {
			return findings[i].CheckName < findings[j].CheckName
		}
		return findings[i].Path < findings[j].Path
	})
	return findings, nil
}

// changesetDirName is the only changeset directory name doctor
// recognizes. Both git's repo-relative output and the live-tree scan
// key off this name — see the doc comment on Options.RepoDir.
const changesetDirName = ".changeset"

// changesetPathPrefix is the path prefix every `.changeset/*.md`
// entry has in `git log --name-only` output and in the live-tree
// comparison key.
const changesetPathPrefix = changesetDirName + "/"

// checkRevivedChangesets surfaces every `.changeset/*.md` file
// currently on disk whose path was previously deleted by a
// release-style commit. Each match becomes a SeverityError finding.
func checkRevivedChangesets(repoDir string, gitLog GitLog, grep string) ([]Finding, error) {
	deleted, err := gitLog(grep)
	if err != nil {
		return nil, fmt.Errorf("doctor: list previously-deleted files: %w", err)
	}
	deletedSet := make(map[string]struct{}, len(deleted))
	for _, p := range deleted {
		// The grep is a substring match against the commit
		// message, which also catches unrelated deletions in
		// the same commit; filter to `.changeset/*.md`
		// (top-level only, no nested paths) here.
		if !strings.HasPrefix(p, changesetPathPrefix) {
			continue
		}
		rest := p[len(changesetPathPrefix):]
		if strings.Contains(rest, "/") || !strings.HasSuffix(rest, ".md") {
			continue
		}
		deletedSet[p] = struct{}{}
	}
	if len(deletedSet) == 0 {
		return nil, nil
	}

	entries, err := os.ReadDir(filepath.Join(repoDir, changesetDirName))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// No `.changeset/` at all means no live changesets
			// to compare against; nothing to flag.
			return nil, nil
		}
		return nil, fmt.Errorf("doctor: read %s: %w", filepath.Join(repoDir, changesetDirName), err)
	}

	var findings []Finding
	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}
		name := ent.Name()
		if !strings.HasSuffix(name, ".md") {
			continue
		}
		// The deletion entries are repo-relative because git
		// log emits them that way; build the comparison key
		// the same way.
		relPath := changesetPathPrefix + name
		if _, revived := deletedSet[relPath]; !revived {
			continue
		}
		findings = append(findings, Finding{
			Severity:  SeverityError,
			CheckName: "revived-changeset",
			Path:      relPath,
			Message: fmt.Sprintf(
				"%s was deleted by a previous %s commit but is back on disk; "+
					"likely cause: stale-branch + squash-merge revived it. "+
					"Delete the file and the next release plan will re-evaluate.",
				relPath, strings.TrimSuffix(grep, ":")),
		})
	}
	return findings, nil
}
