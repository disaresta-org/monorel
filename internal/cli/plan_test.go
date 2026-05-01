package cli

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/disaresta-org/monorel/internal/git/testutil"
)

// fixture builds a real on-disk repo with the given monorel.toml and
// changeset files, returning the path to monorel.toml.
type fixture struct {
	r          *testutil.TestRepo
	configPath string
}

func newFixture(t *testing.T, configTOML string, changesets map[string]string, tags []string) *fixture {
	t.Helper()
	r := testutil.NewRepo(t)
	r.WriteFile("monorel.toml", configTOML)
	for name, body := range changesets {
		r.WriteFile(filepath.Join(".changeset", name+".md"), body)
	}
	r.AddCommit("seed",
		append([]string{"monorel.toml"}, changesetPaths(changesets)...)...)
	for _, tag := range tags {
		r.Tag(tag, "")
	}
	return &fixture{r: r, configPath: filepath.Join(r.Dir, "monorel.toml")}
}

func changesetPaths(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for name := range m {
		out = append(out, filepath.Join(".changeset", name+".md"))
	}
	return out
}

// runCmd invokes the root command with the given args, returning
// stdout and stderr as strings plus the error. argv must include the
// subcommand name as the first element.
func runCmd(t *testing.T, argv ...string) (string, string, error) {
	t.Helper()
	root := newRootCmd()
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs(argv)
	err := root.Execute()
	return stdout.String(), stderr.String(), err
}

const singlePackageTOML = `
[forge]
owner = "x"
repo = "y"

[packages.foo]
tag_prefix = "transports/foo"
path = "transports/foo"
changelog = "transports/foo/CHANGELOG.md"
`

func TestPlanCmd_Empty(t *testing.T) {
	f := newFixture(t, singlePackageTOML, nil, []string{"transports/foo/v1.0.0"})

	stdout, _, err := runCmd(t, "plan", "--config", f.configPath)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if !strings.Contains(stdout, "No pending changesets") {
		t.Errorf("expected empty-plan message; got: %s", stdout)
	}
}

func TestPlanCmd_Table(t *testing.T) {
	f := newFixture(t,
		singlePackageTOML,
		map[string]string{
			"first": "---\n\"foo\": minor\n---\n\nA new feature.\n",
		},
		[]string{"transports/foo/v1.5.0"},
	)
	stdout, _, err := runCmd(t, "plan", "--config", f.configPath)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	for _, want := range []string{"foo", "v1.5.0", "minor", "v1.6.0", "transports/foo/v1.6.0", "first"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("plan output missing %q\nfull output:\n%s", want, stdout)
		}
	}
}

func TestPlanCmd_JSON(t *testing.T) {
	f := newFixture(t,
		singlePackageTOML,
		map[string]string{
			"first": "---\n\"foo\": major\n---\n\nBreaking change.\n",
		},
		[]string{"transports/foo/v1.5.0"},
	)
	stdout, _, err := runCmd(t, "plan", "--config", f.configPath, "--json")
	if err != nil {
		t.Fatalf("plan: %v", err)
	}

	var got jsonPlan
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("unmarshal JSON: %v\noutput:\n%s", err, stdout)
	}
	if len(got.Releases) != 1 {
		t.Fatalf("Releases len = %d, want 1", len(got.Releases))
	}
	r := got.Releases[0]
	if r.Name != "foo" || r.From != "v1.5.0" || r.To != "v2.0.0" || r.Bump != "major" {
		t.Errorf("Release = %+v, want {foo v1.5.0 v2.0.0 major}", r)
	}
	if len(got.Consumed) != 1 || got.Consumed[0] != "first" {
		t.Errorf("Consumed = %v, want [first]", got.Consumed)
	}
}

func TestPlanCmd_InitialRelease(t *testing.T) {
	f := newFixture(t,
		singlePackageTOML,
		map[string]string{
			"first": "---\n\"foo\": minor\n---\n\nFirst feature.\n",
		},
		nil, // no prior tags
	)
	stdout, _, err := runCmd(t, "plan", "--config", f.configPath)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	for _, want := range []string{"foo", "(initial)", "v0.1.0"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("plan output missing %q\nfull:\n%s", want, stdout)
		}
	}
}

func TestStatusCmd_Empty(t *testing.T) {
	f := newFixture(t, singlePackageTOML, nil, nil)
	stdout, _, err := runCmd(t, "status", "--config", f.configPath)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if !strings.Contains(stdout, "No pending changesets") {
		t.Errorf("expected empty-status message; got: %s", stdout)
	}
}

func TestStatusCmd_OneChangeset(t *testing.T) {
	f := newFixture(t,
		singlePackageTOML,
		map[string]string{
			"hot-puma": "---\n\"foo\": patch\n---\n\nFix the thing.\n",
		},
		nil,
	)
	stdout, _, err := runCmd(t, "status", "--config", f.configPath)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	for _, want := range []string{"hot-puma", "foo", "patch", "Fix the thing.", "1 changeset(s) pending"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("status missing %q\nfull:\n%s", want, stdout)
		}
	}
}

func TestPlanCmd_UnknownConfig(t *testing.T) {
	_, _, err := runCmd(t, "plan", "--config", "/nonexistent/monorel.toml")
	if err == nil {
		t.Fatal("expected error for missing config")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error %q should mention not found", err)
	}
}
