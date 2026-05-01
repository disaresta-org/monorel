package git_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"monorel.disaresta.com/internal/git"
	"monorel.disaresta.com/internal/git/testutil"
)

// Tests target both the Exec impl (via testutil.NewRepo) and the
// Fake impl, since they must satisfy the same Repo contract.

func TestExec_ListTagsAndHead(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Tag("v1.0.0", "")
	r.Tag("v1.1.0", "")
	r.Tag("transports/foo/v1.0.0", "")
	r.Tag("transports/foo/v1.6.1", "")
	r.Tag("transports/bar/v0.1.0", "")

	bare, err := r.Repo.ListTags("v")
	if err != nil {
		t.Fatalf("ListTags(v): %v", err)
	}
	if !sliceContains(bare, "v1.0.0") || !sliceContains(bare, "v1.1.0") {
		t.Errorf("ListTags(v) missing root tags: %v", bare)
	}
	// "v" matches the start of "v*" tags but not "transports/...".
	for _, tag := range bare {
		if strings.HasPrefix(tag, "transports/") {
			t.Errorf("ListTags(v) leaked sub-module tag: %s", tag)
		}
	}

	foo, err := r.Repo.ListTags("transports/foo/")
	if err != nil {
		t.Fatalf("ListTags(transports/foo/): %v", err)
	}
	wantFoo := []string{"transports/foo/v1.0.0", "transports/foo/v1.6.1"}
	if len(foo) != len(wantFoo) {
		t.Errorf("ListTags(foo): got %v, want %v", foo, wantFoo)
	}
	for _, want := range wantFoo {
		if !sliceContains(foo, want) {
			t.Errorf("ListTags(foo) missing %q: %v", want, foo)
		}
	}

	all, err := r.Repo.ListTags("")
	if err != nil {
		t.Fatalf("ListTags(): %v", err)
	}
	if len(all) != 5 {
		t.Errorf("ListTags(): got %d, want 5: %v", len(all), all)
	}

	sha, err := r.Repo.CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA: %v", err)
	}
	if len(sha) != 40 {
		t.Errorf("CurrentSHA = %q, want 40-char SHA", sha)
	}
	if sha != r.HeadSHA() {
		t.Errorf("CurrentSHA mismatch: %s vs testutil %s", sha, r.HeadSHA())
	}
}

func TestExec_AddCommitTag(t *testing.T) {
	r := testutil.NewRepo(t)
	r.WriteFile("hello.txt", "hi\n")

	if err := r.Repo.Add("hello.txt"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := r.Repo.Commit("add hello"); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if err := r.Repo.CreateTag("v0.1.0", "release"); err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	tags, err := r.Repo.ListTags("v0")
	if err != nil {
		t.Fatalf("ListTags: %v", err)
	}
	if !sliceContains(tags, "v0.1.0") {
		t.Errorf("ListTags missing v0.1.0: %v", tags)
	}
	clean, err := r.Repo.IsClean()
	if err != nil {
		t.Fatalf("IsClean: %v", err)
	}
	if !clean {
		t.Error("IsClean = false after commit; expected clean")
	}
}

func TestExec_DirtyTree(t *testing.T) {
	r := testutil.NewRepo(t)
	r.WriteFile("staged.txt", "x\n")

	clean, err := r.Repo.IsClean()
	if err != nil {
		t.Fatalf("IsClean: %v", err)
	}
	if clean {
		t.Error("IsClean = true with untracked file; expected dirty")
	}
}

func TestExec_Remove(t *testing.T) {
	r := testutil.NewRepo(t)
	r.WriteFile("doomed.txt", "bye\n")
	r.AddCommit("add doomed.txt", "doomed.txt")

	if err := r.Repo.Remove("doomed.txt"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if err := r.Repo.Commit("remove doomed.txt"); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if _, err := os.Stat(filepath.Join(r.Dir, "doomed.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Remove did not delete file: stat err=%v", err)
	}
}

func TestExec_CheckoutBranch(t *testing.T) {
	r := testutil.NewRepo(t)
	if err := r.Repo.CheckoutBranch("monorel/release"); err != nil {
		t.Fatalf("CheckoutBranch: %v", err)
	}
	// Verify by asking git directly via testutil.
	out := r.HeadSHA()
	if out == "" {
		t.Error("HeadSHA empty after CheckoutBranch")
	}
}

func TestExec_NoOpsNoError(t *testing.T) {
	r := testutil.NewRepo(t)
	if err := r.Repo.Add(); err != nil {
		t.Errorf("Add() with no paths: %v", err)
	}
	if err := r.Repo.Remove(); err != nil {
		t.Errorf("Remove() with no paths: %v", err)
	}
}

// Fake-side coverage: same contract.

func TestFake_ListTags(t *testing.T) {
	f := git.NewFake("v1.0.0", "v1.6.1", "transports/foo/v1.0.0", "transports/foo/v1.6.1")

	got, err := f.ListTags("transports/foo/")
	if err != nil {
		t.Fatalf("ListTags: %v", err)
	}
	want := []string{"transports/foo/v1.0.0", "transports/foo/v1.6.1"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	// Empty prefix returns all tags.
	all, _ := f.ListTags("")
	if len(all) != 4 {
		t.Errorf("ListTags() returned %d tags, want 4", len(all))
	}
}

func TestFake_AddCommit(t *testing.T) {
	f := git.NewFake()
	if err := f.Add("foo.txt", "bar.txt"); err != nil {
		t.Fatal(err)
	}
	if len(f.Staged) != 2 {
		t.Errorf("Staged = %v, want 2 entries", f.Staged)
	}
	if err := f.Commit("test"); err != nil {
		t.Fatal(err)
	}
	if len(f.Commits) != 1 {
		t.Fatalf("Commits = %d, want 1", len(f.Commits))
	}
	if len(f.Staged) != 0 {
		t.Errorf("Staged should be cleared after Commit, got %v", f.Staged)
	}
	if f.Commits[0].Message != "test" {
		t.Errorf("Commits[0].Message = %q, want test", f.Commits[0].Message)
	}
}

func TestFake_Commit_Empty(t *testing.T) {
	f := git.NewFake()
	if err := f.Commit("nothing staged"); err == nil {
		t.Error("expected error from Commit with no staged files")
	}
}

func TestFake_CreateTag_Duplicate(t *testing.T) {
	f := git.NewFake("v1.0.0")
	if err := f.CreateTag("v1.0.0", ""); err == nil {
		t.Error("expected duplicate error")
	}
	if err := f.CreateTag("v1.0.1", ""); err != nil {
		t.Errorf("unique tag should succeed: %v", err)
	}
}

func TestFake_PushAndTags(t *testing.T) {
	f := git.NewFake()
	if err := f.Push("origin", "main", false); err != nil {
		t.Fatal(err)
	}
	if err := f.Push("origin", "monorel/release", true); err != nil {
		t.Fatal(err)
	}
	if err := f.PushTags("origin"); err != nil {
		t.Fatal(err)
	}
	want := []string{"origin:main", "origin:monorel/release:force"}
	if len(f.Pushes) != len(want) {
		t.Fatalf("Pushes = %v, want %v", f.Pushes, want)
	}
	for i := range f.Pushes {
		if f.Pushes[i] != want[i] {
			t.Errorf("Pushes[%d] = %q, want %q", i, f.Pushes[i], want[i])
		}
	}
	if len(f.PushedTags) != 1 || f.PushedTags[0] != "origin" {
		t.Errorf("PushedTags = %v, want [origin]", f.PushedTags)
	}
}

func TestFake_FailNext(t *testing.T) {
	wantErr := errors.New("synthetic failure")
	f := git.NewFake()
	f.FailNext = wantErr

	if _, err := f.ListTags(""); !errors.Is(err, wantErr) {
		t.Errorf("ListTags after FailNext: err = %v, want %v", err, wantErr)
	}
	// FailNext is single-shot.
	if _, err := f.ListTags(""); err != nil {
		t.Errorf("second ListTags: err = %v, want nil (FailNext is one-shot)", err)
	}
}

func TestFake_IsCleanDirty(t *testing.T) {
	f := git.NewFake()
	clean, err := f.IsClean()
	if err != nil || !clean {
		t.Errorf("default IsClean = (%v, %v), want (true, nil)", clean, err)
	}
	f.Dirty = true
	clean, err = f.IsClean()
	if err != nil || clean {
		t.Errorf("Dirty=true IsClean = (%v, %v), want (false, nil)", clean, err)
	}
}

func TestFake_CheckoutBranch(t *testing.T) {
	f := git.NewFake()
	if err := f.CheckoutBranch("foo"); err != nil {
		t.Fatal(err)
	}
	if f.Branch != "foo" {
		t.Errorf("Branch = %q, want foo", f.Branch)
	}
}

func sliceContains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
