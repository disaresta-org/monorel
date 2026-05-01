package git

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// Exec is a [Repo] backed by shell-outs to the `git` CLI in a
// specific working directory.
//
// Construct via [Open]; the zero value is not usable (Dir must be
// set).
type Exec struct {
	// Dir is the working directory git commands run in. Required.
	Dir string

	// Env is the environment passed to git invocations. Empty means
	// inherit os.Environ; tests override this to set GIT_AUTHOR_*
	// for deterministic commit metadata.
	Env []string
}

// Open returns an Exec rooted at dir. The directory is not validated
// here; the first git invocation surfaces the error if dir is not
// inside a git repo.
func Open(dir string) *Exec {
	return &Exec{Dir: dir}
}

// run executes `git args...` in e.Dir, returning stdout. On failure
// the wrapped error includes stderr.
func (e *Exec) run(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = e.Dir
	if e.Env != nil {
		cmd.Env = e.Env
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s",
			strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

// ListTags implements [Repo.ListTags].
func (e *Exec) ListTags(prefix string) ([]string, error) {
	args := []string{"tag", "--list"}
	if prefix != "" {
		// `git tag --list <pattern>` accepts shell glob; we
		// list tags whose name starts with prefix by appending
		// "*". Tag names allow most printable chars except a
		// few git reserves; the glob is exact-match-style for
		// our prefixes (slash-separated paths).
		args = append(args, prefix+"*")
	}
	out, err := e.run(args...)
	if err != nil {
		return nil, err
	}
	tags := splitLines(out)
	return tags, nil
}

// TagsAtHead implements [Repo.TagsAtHead].
func (e *Exec) TagsAtHead() ([]string, error) {
	out, err := e.run("tag", "--points-at", "HEAD")
	if err != nil {
		return nil, err
	}
	return splitLines(out), nil
}

// CurrentSHA implements [Repo.CurrentSHA].
func (e *Exec) CurrentSHA() (string, error) {
	out, err := e.run("rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// Add implements [Repo.Add].
func (e *Exec) Add(paths ...string) error {
	if len(paths) == 0 {
		return nil
	}
	args := append([]string{"add", "--"}, paths...)
	_, err := e.run(args...)
	return err
}

// Remove implements [Repo.Remove].
func (e *Exec) Remove(paths ...string) error {
	if len(paths) == 0 {
		return nil
	}
	args := append([]string{"rm", "-f", "--"}, paths...)
	_, err := e.run(args...)
	return err
}

// Commit implements [Repo.Commit].
func (e *Exec) Commit(message string) error {
	if message == "" {
		return errors.New("commit message is empty")
	}
	_, err := e.run("commit", "-m", message)
	return err
}

// CreateTag implements [Repo.CreateTag].
func (e *Exec) CreateTag(name, message string) error {
	if name == "" {
		return errors.New("tag name is empty")
	}
	args := []string{"tag"}
	if message != "" {
		args = append(args, "-a", name, "-m", message)
	} else {
		args = append(args, name)
	}
	_, err := e.run(args...)
	return err
}

// Push implements [Repo.Push].
func (e *Exec) Push(remote, ref string, force bool) error {
	args := []string{"push"}
	if force {
		args = append(args, "--force-with-lease")
	}
	args = append(args, remote, ref)
	_, err := e.run(args...)
	return err
}

// PushTags implements [Repo.PushTags].
func (e *Exec) PushTags(remote string) error {
	_, err := e.run("push", remote, "--tags")
	return err
}

// CheckoutBranch implements [Repo.CheckoutBranch].
func (e *Exec) CheckoutBranch(branch string) error {
	if branch == "" {
		return errors.New("branch name is empty")
	}
	_, err := e.run("checkout", "-B", branch)
	return err
}

// IsClean implements [Repo.IsClean].
func (e *Exec) IsClean() (bool, error) {
	out, err := e.run("status", "--porcelain")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) == "", nil
}

// splitLines splits raw on '\n', trimming carriage returns, and drops
// empty lines (including the trailing empty after the final newline).
func splitLines(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(strings.TrimRight(raw, "\n"), "\n")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimRight(p, "\r")
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
