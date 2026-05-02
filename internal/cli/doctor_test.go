package cli

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"monorel.disaresta.com/internal/git/testutil"
)

// doctorFixture builds an on-disk repo with monorel.toml + an
// arbitrary changeset state and a recorded chore(release) commit
// that deleted some files. Returns the absolute path to monorel.toml
// for runCmd.
func doctorFixture(
	t *testing.T,
	preReleaseChangesets []string, // committed before the release commit
	releasedChangesets []string, // deleted by the release commit
	currentChangesets []string, // present at HEAD
) string {
	t.Helper()
	r := testutil.NewRepo(t)
	r.WriteFile("monorel.toml", singlePackageTOML)

	// Initial commit: add monorel.toml plus all "before-release" files.
	files := []string{"monorel.toml"}
	for _, name := range preReleaseChangesets {
		path := ".changeset/" + name + ".md"
		r.WriteFile(path, "---\n\"foo\": minor\n---\n\nseed.\n")
		files = append(files, path)
	}
	r.AddCommit("seed", files...)

	// The release commit: delete `releasedChangesets`.
	for _, name := range releasedChangesets {
		path := ".changeset/" + name + ".md"
		// Remove via git rm so the commit records a deletion.
		if err := r.Repo.Remove(path); err != nil {
			t.Fatalf("remove %s: %v", path, err)
		}
	}
	if len(releasedChangesets) > 0 {
		if err := r.Repo.Commit("chore(release): pkg v1.0.0\n\nmonorel-Release: pkg v1.0.0\n"); err != nil {
			t.Fatalf("release commit: %v", err)
		}
	}

	// Final state: write the "current" set of changesets. Any name
	// shared with releasedChangesets simulates a revival; any new
	// name is a fresh changeset that should NOT be flagged.
	for _, name := range currentChangesets {
		path := ".changeset/" + name + ".md"
		r.WriteFile(path, "---\n\"foo\": minor\n---\n\nrevived or new.\n")
	}

	return r.Dir + "/monorel.toml"
}

func TestDoctorCmd_Clean(t *testing.T) {
	configPath := doctorFixture(t,
		[]string{"shipped"},   // pre-release: present
		[]string{"shipped"},   // released: deleted
		[]string{"new-thing"}, // current: only fresh content
	)
	stdout, _, err := runCmd(t, "doctor", "--config", configPath)
	if err != nil {
		t.Fatalf("doctor: %v", err)
	}
	if !strings.Contains(stdout, "No findings") {
		t.Errorf("expected clean output; got: %s", stdout)
	}
}

func TestDoctorCmd_FlagsRevival(t *testing.T) {
	configPath := doctorFixture(t,
		[]string{"shipped"},
		[]string{"shipped"},
		[]string{"shipped", "fresh"}, // shipped is back on disk: revival
	)
	stdout, _, err := runCmd(t, "doctor", "--config", configPath)
	if err == nil {
		t.Fatal("doctor should have returned a non-zero exit on revival")
	}
	var ee ErrExit
	if !errors.As(err, &ee) || int(ee) != 1 {
		t.Fatalf("err = %v, want ErrExit(1)", err)
	}
	for _, want := range []string{
		"revived-changeset",
		".changeset/shipped.md",
		"1 error(s)",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("doctor output missing %q\nfull:\n%s", want, stdout)
		}
	}
	// "fresh" shouldn't be flagged.
	if strings.Contains(stdout, "fresh.md") {
		t.Errorf("doctor flagged a fresh changeset; output:\n%s", stdout)
	}
}

func TestDoctorCmd_JSONShape(t *testing.T) {
	configPath := doctorFixture(t,
		[]string{"a", "b"},
		[]string{"a", "b"},
		[]string{"a", "b"},
	)
	stdout, _, err := runCmd(t, "doctor", "--config", configPath, "--json")
	if err == nil {
		t.Fatal("doctor should have returned a non-zero exit on revival")
	}

	var got struct {
		Findings []struct {
			Severity  string `json:"severity"`
			CheckName string `json:"check_name"`
			Path      string `json:"path"`
			Message   string `json:"message"`
		} `json:"findings"`
		Errors   int `json:"errors"`
		Warnings int `json:"warnings"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("unmarshal: %v\noutput:\n%s", err, stdout)
	}
	if got.Errors != 2 || got.Warnings != 0 || len(got.Findings) != 2 {
		t.Fatalf("counts wrong: %+v", got)
	}
	// Findings are sorted by Path.
	if got.Findings[0].Path != ".changeset/a.md" || got.Findings[1].Path != ".changeset/b.md" {
		t.Errorf("paths in wrong order: %+v", got.Findings)
	}
	for _, f := range got.Findings {
		if f.Severity != "error" || f.CheckName != "revived-changeset" {
			t.Errorf("unexpected finding: %+v", f)
		}
	}
}

func TestDoctorCmd_NoChangesetDir(t *testing.T) {
	// A repo without a .changeset directory at all should report
	// no findings rather than crash.
	r := testutil.NewRepo(t)
	r.WriteFile("monorel.toml", singlePackageTOML)
	r.AddCommit("seed", "monorel.toml")

	stdout, _, err := runCmd(t, "doctor", "--config", r.Dir+"/monorel.toml")
	if err != nil {
		t.Fatalf("doctor: %v", err)
	}
	if !strings.Contains(stdout, "No findings") {
		t.Errorf("expected clean output; got: %s", stdout)
	}
}
