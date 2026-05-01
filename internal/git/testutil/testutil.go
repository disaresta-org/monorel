// Package testutil creates real on-disk git repositories for
// integration tests of monorel's higher layers.
//
// Each helper takes a *testing.T and returns a TestRepo bound to
// t.TempDir() so cleanup is automatic. Operations panic on failure
// via t.Fatal — callers don't need to thread errors.
package testutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"monorel.disaresta.com/internal/git"
)

// TestRepo is a real git repo in a temp directory, populated by the
// builder methods on this type.
type TestRepo struct {
	t   *testing.T
	Dir string

	// Repo is the [git.Exec] bound to Dir, ready to pass into
	// monorel's higher-layer code under test.
	Repo *git.Exec
}

// NewRepo creates a fresh git repo in t.TempDir() with deterministic
// committer metadata (author=test, email=test@test). An initial empty
// commit is made so HEAD is valid.
func NewRepo(t *testing.T) *TestRepo {
	t.Helper()
	dir := t.TempDir()

	exec := git.Open(dir)
	// Disable global git config so contributor environments
	// (signing keys, SSH agents like 1Password, hooks, etc.) can't
	// reach into hermetic test commits. GIT_CONFIG_GLOBAL=/dev/null
	// + GIT_CONFIG_SYSTEM=/dev/null is the documented way to do it.
	exec.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test",
		"GIT_AUTHOR_EMAIL=test@test",
		"GIT_COMMITTER_NAME=test",
		"GIT_COMMITTER_EMAIL=test@test",
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_SYSTEM=/dev/null",
	)

	r := &TestRepo{t: t, Dir: dir, Repo: exec}
	r.runOrFatal("init", "-q", "-b", "main")
	// Persist signing-disabled to the repo's local config so any
	// later git invocation against this repo (including from code
	// under test that opens its own git.Exec) doesn't try to use
	// the contributor's signing key / agent.
	r.runOrFatal("config", "commit.gpgsign", "false")
	r.runOrFatal("config", "tag.gpgsign", "false")
	r.runOrFatal("commit", "-q", "--allow-empty", "-m", "init")
	return r
}

// WriteFile writes content to dir/<rel>, creating parent dirs.
func (r *TestRepo) WriteFile(rel, content string) {
	r.t.Helper()
	path := filepath.Join(r.Dir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		r.t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		r.t.Fatalf("WriteFile: %v", err)
	}
}

// AddCommit stages and commits the given files under the supplied
// message. Files must already exist (use WriteFile first).
func (r *TestRepo) AddCommit(message string, files ...string) {
	r.t.Helper()
	if len(files) == 0 {
		r.runOrFatal("commit", "-q", "--allow-empty", "-m", message)
		return
	}
	args := append([]string{"add", "--"}, files...)
	r.runOrFatal(args...)
	r.runOrFatal("commit", "-q", "-m", message)
}

// Tag creates an annotated tag at HEAD. message is the annotation;
// pass an empty string for a lightweight tag (matches git CLI).
func (r *TestRepo) Tag(name, message string) {
	r.t.Helper()
	if name == "" {
		r.t.Fatal("Tag: name is empty")
	}
	if message == "" {
		r.runOrFatal("tag", name)
	} else {
		r.runOrFatal("tag", "-a", name, "-m", message)
	}
}

// HeadSHA returns the SHA of the current HEAD.
func (r *TestRepo) HeadSHA() string {
	r.t.Helper()
	out := r.runOrFatal("rev-parse", "HEAD")
	return strings.TrimSpace(out)
}

// runOrFatal invokes git with the given args and fails the test on
// non-zero exit. Returns combined stdout+stderr (since some commands
// emit useful info on stderr).
func (r *TestRepo) runOrFatal(args ...string) string {
	r.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = r.Dir
	cmd.Env = r.Repo.Env
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}
