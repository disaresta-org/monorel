package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReleaseCmd_HappyPath(t *testing.T) {
	f := newFixture(t,
		singlePackageTOML,
		map[string]string{
			"first": "---\n\"foo\": minor\n---\n\nFirst feature.\n",
		},
		[]string{"transports/foo/v1.5.0"},
	)
	// Pre-existing CHANGELOG.md so the writer has somewhere to insert.
	f.r.WriteFile("transports/foo/CHANGELOG.md", "# Changelog\n")
	f.r.AddCommit("seed changelog", "transports/foo/CHANGELOG.md")

	stdout, _, err := runCmd(t, "release", "--config", f.configPath)
	if err != nil {
		t.Fatalf("release: %v", err)
	}

	for _, want := range []string{"Released 1 package(s)", "transports/foo/v1.6.0"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("output missing %q\nfull:\n%s", want, stdout)
		}
	}

	// Tag exists.
	tags, err := f.r.Repo.ListTags("transports/foo/v1.6")
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 1 || tags[0] != "transports/foo/v1.6.0" {
		t.Errorf("tags after release = %v", tags)
	}

	// Changeset gone.
	if _, err := os.Stat(filepath.Join(f.r.Dir, ".changeset", "first.md")); !os.IsNotExist(err) {
		t.Errorf("changeset still exists: %v", err)
	}
}

func TestReleaseCmd_EmptyChangesetsExitsCleanly(t *testing.T) {
	f := newFixture(t, singlePackageTOML, nil, []string{"transports/foo/v1.0.0"})
	stdout, _, err := runCmd(t, "release", "--config", f.configPath)
	if err != nil {
		t.Fatalf("release: %v", err)
	}
	if !strings.Contains(stdout, "No pending changesets") {
		t.Errorf("expected empty-plan message; got: %s", stdout)
	}
}

func TestReleaseCmd_DirtyTreeRejected(t *testing.T) {
	f := newFixture(t,
		singlePackageTOML,
		map[string]string{"first": "---\n\"foo\": patch\n---\n\nFix.\n"},
		nil,
	)
	// Add an untracked file to make the tree dirty.
	if err := os.WriteFile(filepath.Join(f.r.Dir, "uncommitted.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, err := runCmd(t, "release", "--config", f.configPath)
	if err == nil {
		t.Fatal("expected error on dirty tree")
	}
	if !strings.Contains(err.Error(), "uncommitted") {
		t.Errorf("error %q should mention uncommitted", err)
	}
}
