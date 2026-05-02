package git

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Fake is an in-memory [Repo] for unit tests of higher layers.
//
// Operations record into the corresponding fields so tests can assert
// on them; it doesn't simulate every git invariant. Specifically:
//   - Add/Remove just track which paths have been touched; there's no
//     actual working tree.
//   - Commit appends to Commits with the staged paths cleared.
//   - CreateTag fails on duplicate names (matches git behavior).
//
// Construct with NewFake; the zero value is usable but tests usually
// want to seed initial tags via NewFake's argument.
type Fake struct {
	mu sync.Mutex

	// Tags is the current set of tags. Order is irrelevant; ListTags
	// sorts on output for deterministic comparisons.
	Tags []string

	// Branch tracks the currently-checked-out branch name.
	Branch string

	// HeadSHA is what CurrentSHA returns. Tests may set this to
	// anything; the default (empty) yields a deterministic placeholder.
	HeadSHA string

	// Staged is the set of paths Add has been called with since the
	// last Commit. Each Commit appends a snapshot to Commits and
	// clears Staged.
	Staged []string

	// Removed records every Remove(paths...) call's paths.
	Removed []string

	// Commits is one entry per Commit() call: {Message, Files}.
	Commits []FakeCommit

	// Pushes records every Push call as "<remote>:<ref>[:force]".
	Pushes []string

	// PushedTags records every PushTags(remote) call.
	PushedTags []string

	// Dirty toggles IsClean's return value. By default the fake
	// reports clean; set Dirty=true to make IsClean return false.
	Dirty bool

	// FailNext, when non-nil, is returned by the next operation
	// regardless of which one. Single-shot: cleared after firing.
	// Used to test error-handling paths in higher layers.
	FailNext error
}

// FakeCommit is one recorded commit on a [Fake].
type FakeCommit struct {
	Message string
	Files   []string

	// Deletions are the paths Remove(...) recorded for this commit.
	// Subset of Files. Tracked separately so
	// DeletedFilesInCommitsMatching can answer accurately without
	// the fake needing to know each path's add-vs-delete state.
	Deletions []string
}

// NewFake returns an empty in-memory repo with the given initial
// tags. nil is fine for "no tags yet."
func NewFake(initialTags ...string) *Fake {
	return &Fake{
		Tags:    append([]string{}, initialTags...),
		Branch:  "main",
		HeadSHA: "fake-head-0000000000000000000000000000",
	}
}

// take consumes FailNext if set and returns it. Internal to Fake.
func (f *Fake) take() error {
	if f.FailNext != nil {
		err := f.FailNext
		f.FailNext = nil
		return err
	}
	return nil
}

// ListTags implements [Repo.ListTags].
func (f *Fake) ListTags(prefix string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.take(); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(f.Tags))
	for _, t := range f.Tags {
		if prefix == "" || strings.HasPrefix(t, prefix) {
			out = append(out, t)
		}
	}
	sort.Strings(out)
	return out, nil
}

// TagsAtHead implements [Repo.TagsAtHead]. The fake doesn't track
// per-tag SHAs; it returns every tag created via CreateTag since the
// last call to clear (which the fake doesn't have). Tests that need
// fine-grained "tags pointing at this specific commit" should
// override this with a custom impl.
func (f *Fake) TagsAtHead() ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.take(); err != nil {
		return nil, err
	}
	out := make([]string, len(f.Tags))
	copy(out, f.Tags)
	return out, nil
}

// CurrentSHA implements [Repo.CurrentSHA].
func (f *Fake) CurrentSHA() (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.take(); err != nil {
		return "", err
	}
	return f.HeadSHA, nil
}

// HeadCommitMessage implements [Repo.HeadCommitMessage]. Returns the
// Message of the last [FakeCommit] in Commits, or "" when no commits
// have been recorded.
func (f *Fake) HeadCommitMessage() (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.take(); err != nil {
		return "", err
	}
	if len(f.Commits) == 0 {
		return "", nil
	}
	return f.Commits[len(f.Commits)-1].Message, nil
}

// Add implements [Repo.Add]. Records the paths in Staged.
func (f *Fake) Add(paths ...string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.take(); err != nil {
		return err
	}
	f.Staged = append(f.Staged, paths...)
	return nil
}

// Remove implements [Repo.Remove]. Records the paths in Removed.
func (f *Fake) Remove(paths ...string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.take(); err != nil {
		return err
	}
	f.Removed = append(f.Removed, paths...)
	return nil
}

// Commit implements [Repo.Commit]. Appends a [FakeCommit] with the
// staged paths and clears Staged.
func (f *Fake) Commit(message string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.take(); err != nil {
		return err
	}
	if message == "" {
		return errors.New("commit message is empty")
	}
	if len(f.Staged) == 0 && len(f.Removed) == 0 {
		return errors.New("nothing to commit")
	}
	files := append([]string{}, f.Staged...)
	files = append(files, f.Removed...)
	deletions := append([]string{}, f.Removed...)
	f.Commits = append(f.Commits, FakeCommit{
		Message:   message,
		Files:     files,
		Deletions: deletions,
	})
	f.Staged = nil
	f.Removed = nil
	return nil
}

// CreateTag implements [Repo.CreateTag]. Errors on duplicate.
func (f *Fake) CreateTag(name, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.take(); err != nil {
		return err
	}
	if name == "" {
		return errors.New("tag name is empty")
	}
	for _, t := range f.Tags {
		if t == name {
			return fmt.Errorf("tag %q already exists", name)
		}
	}
	f.Tags = append(f.Tags, name)
	return nil
}

// Push implements [Repo.Push]. Records a "remote:ref[:force]" entry.
func (f *Fake) Push(remote, ref string, force bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.take(); err != nil {
		return err
	}
	entry := remote + ":" + ref
	if force {
		entry += ":force"
	}
	f.Pushes = append(f.Pushes, entry)
	return nil
}

// PushTags implements [Repo.PushTags].
func (f *Fake) PushTags(remote string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.take(); err != nil {
		return err
	}
	f.PushedTags = append(f.PushedTags, remote)
	return nil
}

// CheckoutBranch implements [Repo.CheckoutBranch].
func (f *Fake) CheckoutBranch(branch string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.take(); err != nil {
		return err
	}
	if branch == "" {
		return errors.New("branch name is empty")
	}
	f.Branch = branch
	return nil
}

// IsClean implements [Repo.IsClean].
func (f *Fake) IsClean() (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.take(); err != nil {
		return false, err
	}
	return !f.Dirty, nil
}

// DeletedFilesInCommitsMatching implements
// [Repo.DeletedFilesInCommitsMatching]. Walks the recorded Commits,
// collecting Deletions from any commit whose Message contains
// messageGrep as a literal substring.
func (f *Fake) DeletedFilesInCommitsMatching(messageGrep string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.take(); err != nil {
		return nil, err
	}
	if messageGrep == "" {
		return nil, errors.New("messageGrep is empty")
	}
	var paths []string
	for _, c := range f.Commits {
		if !strings.Contains(c.Message, messageGrep) {
			continue
		}
		paths = append(paths, c.Deletions...)
	}
	return dedupSorted(paths), nil
}
