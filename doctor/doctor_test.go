package doctor_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"monorel.disaresta.com/doctor"
)

// stubGitLog returns a [doctor.GitLog] that hands back the given
// paths whenever the substring matches. Tests can model multi-grep
// behavior by switching on messageGrep inside fn directly; this
// helper covers the common single-grep case.
func stubGitLog(t *testing.T, wantGrep string, paths []string) doctor.GitLog {
	t.Helper()
	return func(messageGrep string) ([]string, error) {
		if messageGrep != wantGrep {
			t.Fatalf("GitLog called with grep=%q, want %q", messageGrep, wantGrep)
		}
		return paths, nil
	}
}

// repoWithChangesets creates a temp repo root containing a
// `.changeset/` directory with the given filenames (each created
// empty). Returns the repo root.
func repoWithChangesets(t *testing.T, names ...string) string {
	t.Helper()
	repo := t.TempDir()
	dir := filepath.Join(repo, ".changeset")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(dir, n), nil, 0o644); err != nil {
			t.Fatalf("write %s: %v", n, err)
		}
	}
	return repo
}

func TestRun_NoFindingsWhenNoRevivals(t *testing.T) {
	repo := repoWithChangesets(t, "fresh-feature.md")
	findings, err := doctor.Run(doctor.Options{
		RepoDir: repo,
		GitLog: stubGitLog(t, "chore(release):", []string{
			".changeset/old-shipped.md",
		}),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("got findings %v, want none", findings)
	}
}

func TestRun_FlagsRevivedChangeset(t *testing.T) {
	repo := repoWithChangesets(t, "gitlab-provider.md", "fresh-feature.md")
	findings, err := doctor.Run(doctor.Options{
		RepoDir: repo,
		GitLog: stubGitLog(t, "chore(release):", []string{
			".changeset/gitlab-provider.md",
			".changeset/something-else.md",
		}),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("got %d findings, want 1: %v", len(findings), findings)
	}
	f := findings[0]
	if f.CheckName != doctor.CheckNameRevivedChangeset {
		t.Errorf("CheckName = %q, want %q", f.CheckName, doctor.CheckNameRevivedChangeset)
	}
	if f.Severity != doctor.SeverityError {
		t.Errorf("Severity = %v, want SeverityError", f.Severity)
	}
	if f.Path != ".changeset/gitlab-provider.md" {
		t.Errorf("Path = %q, want .changeset/gitlab-provider.md", f.Path)
	}
}

func TestRun_FlagsAllRevivedSorted(t *testing.T) {
	// When multiple changesets are revived the output is sorted by
	// path so callers can rely on stable ordering.
	repo := repoWithChangesets(t, "z-last.md", "a-first.md", "m-mid.md")
	findings, err := doctor.Run(doctor.Options{
		RepoDir: repo,
		GitLog: stubGitLog(t, "chore(release):", []string{
			".changeset/z-last.md",
			".changeset/a-first.md",
			".changeset/m-mid.md",
		}),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(findings) != 3 {
		t.Fatalf("got %d findings, want 3", len(findings))
	}
	want := []string{
		".changeset/a-first.md",
		".changeset/m-mid.md",
		".changeset/z-last.md",
	}
	for i, f := range findings {
		if f.Path != want[i] {
			t.Errorf("findings[%d].Path = %q, want %q", i, f.Path, want[i])
		}
	}
}

func TestRun_IgnoresNonChangesetDeletions(t *testing.T) {
	// Release commits often delete other files alongside the
	// changeset (e.g. earlier-version artifacts). Doctor must only
	// consider top-level `.changeset/*.md` paths.
	repo := repoWithChangesets(t, "live.md")
	findings, err := doctor.Run(doctor.Options{
		RepoDir: repo,
		GitLog: stubGitLog(t, "chore(release):", []string{
			"CHANGELOG.md",                    // wrong dir
			"docs/whats-new.md",               // wrong dir
			".changeset/sub/nested-but-md.md", // nested, not at root
			".changeset/live.txt",             // wrong extension
			".changeset/.consumed",            // not .md
		}),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("got findings %v, want none", findings)
	}
}

func TestRun_NoChangesetDirIsClean(t *testing.T) {
	// Repos that haven't run `monorel init` yet shouldn't error;
	// there's just nothing to flag. Pass a repo that has no
	// `.changeset/` directory at all.
	repo := t.TempDir()
	findings, err := doctor.Run(doctor.Options{
		RepoDir: repo,
		GitLog:  stubGitLog(t, "chore(release):", []string{".changeset/foo.md"}),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("got findings %v, want none", findings)
	}
}

func TestRun_GitLogErrorIsReturned(t *testing.T) {
	repo := repoWithChangesets(t, "x.md")
	want := errors.New("git boom")
	_, err := doctor.Run(doctor.Options{
		RepoDir: repo,
		GitLog: func(string) ([]string, error) {
			return nil, want
		},
	})
	if err == nil || !errors.Is(err, want) {
		t.Fatalf("err = %v, want wrapped %v", err, want)
	}
}

func TestRun_ValidatesRequiredOptions(t *testing.T) {
	cases := []struct {
		name string
		opts doctor.Options
	}{
		{"missing RepoDir", doctor.Options{
			GitLog: func(string) ([]string, error) { return nil, nil },
		}},
		{"missing GitLog", doctor.Options{
			RepoDir: t.TempDir(),
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := doctor.Run(tc.opts)
			if err == nil {
				t.Fatal("Run returned nil error, want validation error")
			}
		})
	}
}

func TestRun_CustomReleaseCommitGrep(t *testing.T) {
	// Callers integrating with a non-monorel release-commit
	// convention can override the grep; doctor should pass it
	// through to GitLog.
	repo := repoWithChangesets(t, "x.md")
	_, err := doctor.Run(doctor.Options{
		RepoDir:           repo,
		ReleaseCommitGrep: "Release v",
		GitLog:            stubGitLog(t, "Release v", nil),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestSeverity_String(t *testing.T) {
	// Severity is a typed string; the constants ARE the wire form.
	if string(doctor.SeverityError) != "error" {
		t.Errorf("SeverityError = %q, want %q", doctor.SeverityError, "error")
	}
	if string(doctor.SeverityWarning) != "warning" {
		t.Errorf("SeverityWarning = %q, want %q", doctor.SeverityWarning, "warning")
	}
}
