package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"monorel.disaresta.com/changeset"
)

const twoPackageTOML = `
[provider]
owner = "x"
repo = "y"

[packages.foo]
tag_prefix = "transports/foo"
path = "transports/foo"
changelog = "transports/foo/CHANGELOG.md"

[packages.bar]
tag_prefix = "transports/bar"
path = "transports/bar"
changelog = "transports/bar/CHANGELOG.md"
`

func TestAddCmd_NonInteractive(t *testing.T) {
	f := newFixture(t, twoPackageTOML, nil, nil)

	stdout, _, err := runCmd(t,
		"add",
		"--config", f.configPath,
		"--package", "foo:minor",
		"--package", "bar:patch",
		"--message", "Added a thing.\n\nDetails follow.",
		"--name", "hot-walrus",
	)
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if !strings.Contains(stdout, ".changeset/hot-walrus.md") {
		t.Errorf("output should mention written path: %s", stdout)
	}

	path := filepath.Join(f.r.Dir, ".changeset", "hot-walrus.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}
	cs, err := changeset.Parse(bytes.NewReader(raw), "hot-walrus")
	if err != nil {
		t.Fatalf("parse written file: %v", err)
	}
	if len(cs.Bumps) != 2 {
		t.Errorf("Bumps len = %d, want 2", len(cs.Bumps))
	}
	if cs.Bumps["foo"].String() != "minor" {
		t.Errorf("foo bump = %s, want minor", cs.Bumps["foo"])
	}
	if cs.Bumps["bar"].String() != "patch" {
		t.Errorf("bar bump = %s, want patch", cs.Bumps["bar"])
	}
	if !strings.Contains(cs.Body, "Added a thing.") {
		t.Errorf("body missing: %q", cs.Body)
	}
}

func TestAddCmd_AutoName(t *testing.T) {
	f := newFixture(t, twoPackageTOML, nil, nil)
	stdout, _, err := runCmd(t,
		"add",
		"--config", f.configPath,
		"--package", "foo:patch",
		"--message", "fix",
	)
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	// Auto-name pattern: "<adj>-<noun>" matching IsValidName.
	prefix := "wrote .changeset/"
	if !strings.HasPrefix(stdout, prefix) {
		t.Fatalf("stdout should start with %q: %s", prefix, stdout)
	}
	rel := strings.TrimSpace(strings.TrimPrefix(stdout, prefix))
	rel = strings.TrimSuffix(rel, ".md")
	if !changeset.IsValidName(rel) {
		t.Errorf("auto-generated name %q is not valid", rel)
	}
	if !strings.Contains(rel, "-") {
		t.Errorf("auto-generated name %q should be hyphenated", rel)
	}
}

func TestAddCmd_UnknownPackage(t *testing.T) {
	f := newFixture(t, twoPackageTOML, nil, nil)
	_, _, err := runCmd(t,
		"add",
		"--config", f.configPath,
		"--package", "ghost:patch",
		"--message", "x",
	)
	if err == nil {
		t.Fatal("expected error for unknown package")
	}
	if !strings.Contains(err.Error(), "unknown package") {
		t.Errorf("error = %q, want one mentioning unknown package", err)
	}
}

func TestAddCmd_DuplicatePackage(t *testing.T) {
	f := newFixture(t, twoPackageTOML, nil, nil)
	_, _, err := runCmd(t,
		"add",
		"--config", f.configPath,
		"--package", "foo:minor",
		"--package", "foo:patch",
		"--message", "x",
	)
	if err == nil {
		t.Fatal("expected error for duplicate package")
	}
	if !strings.Contains(err.Error(), "already specified") {
		t.Errorf("error = %q, want one about already-specified", err)
	}
}

func TestAddCmd_BadShape(t *testing.T) {
	f := newFixture(t, twoPackageTOML, nil, nil)
	_, _, err := runCmd(t,
		"add",
		"--config", f.configPath,
		"--package", "foo-without-colon",
		"--message", "x",
	)
	if err == nil {
		t.Fatal("expected error for missing colon")
	}
	if !strings.Contains(err.Error(), "name:level") {
		t.Errorf("error = %q, want one mentioning name:level", err)
	}
}

func TestAddCmd_BadLevel(t *testing.T) {
	f := newFixture(t, twoPackageTOML, nil, nil)
	_, _, err := runCmd(t,
		"add",
		"--config", f.configPath,
		"--package", "foo:huge",
		"--message", "x",
	)
	if err == nil {
		t.Fatal("expected error for unknown bump level")
	}
}

func TestAddCmd_EditorFlag_WritesBodyFromEditor(t *testing.T) {
	f := newFixture(t, twoPackageTOML, nil, nil)

	// Stage a fake editor that produces a known body. Setting EDITOR
	// here drives resolveEditor() → readAndStripComments path.
	editor := fakeEditor(t, "Edited via $EDITOR.\n\nSecond paragraph.")
	t.Setenv("EDITOR", editor)
	t.Setenv("VISUAL", "") // make sure VISUAL doesn't shadow the test EDITOR

	stdout, _, err := runCmd(t,
		"add",
		"--config", f.configPath,
		"--package", "foo:patch",
		"--name", "editor-cat",
		"--editor",
	)
	if err != nil {
		t.Fatalf("add --editor: %v", err)
	}
	if !strings.Contains(stdout, ".changeset/editor-cat.md") {
		t.Errorf("output should mention written path: %s", stdout)
	}

	path := filepath.Join(f.r.Dir, ".changeset", "editor-cat.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	cs, err := changeset.Parse(bytes.NewReader(raw), "editor-cat")
	if err != nil {
		t.Fatal(err)
	}
	if cs.Body != "Edited via $EDITOR.\n\nSecond paragraph." {
		t.Errorf("body = %q, want editor output", cs.Body)
	}
}

func TestAddCmd_EditorAndMessageAreMutuallyExclusive(t *testing.T) {
	f := newFixture(t, twoPackageTOML, nil, nil)
	_, _, err := runCmd(t,
		"add",
		"--config", f.configPath,
		"--package", "foo:patch",
		"--editor",
		"--message", "via flag",
	)
	if err == nil {
		t.Fatal("expected error when --editor and --message are both passed")
	}
	if !strings.Contains(err.Error(), "--editor and --message") {
		t.Errorf("error should name both flags; got: %v", err)
	}
}

func TestAddCmd_InvalidName(t *testing.T) {
	f := newFixture(t, twoPackageTOML, nil, nil)
	_, _, err := runCmd(t,
		"add",
		"--config", f.configPath,
		"--package", "foo:patch",
		"--message", "x",
		"--name", "Bad Name",
	)
	if err == nil {
		t.Fatal("expected error for invalid name")
	}
}

func TestAddCmd_Interactive(t *testing.T) {
	f := newFixture(t, twoPackageTOML, nil, nil)

	// Drive the interactive flow: pick package 1 (bar — sorted),
	// minor bump, two-line body, blank line to terminate.
	input := "1\nminor\nFirst feature.\nMore details.\n\n"
	root := newRootCmd()
	root.SetIn(strings.NewReader(input))
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"add", "--config", f.configPath})
	if err := root.Execute(); err != nil {
		t.Fatalf("add: %v\nstderr: %s", err, stderr.String())
	}

	// Find the changeset file: there's exactly one in .changeset/.
	entries, err := os.ReadDir(filepath.Join(f.r.Dir, ".changeset"))
	if err != nil {
		t.Fatal(err)
	}
	var changesetFile string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".md") {
			changesetFile = e.Name()
			break
		}
	}
	if changesetFile == "" {
		t.Fatal("no changeset file written")
	}
	raw, err := os.ReadFile(filepath.Join(f.r.Dir, ".changeset", changesetFile))
	if err != nil {
		t.Fatal(err)
	}
	cs, err := changeset.Parse(bytes.NewReader(raw), strings.TrimSuffix(changesetFile, ".md"))
	if err != nil {
		t.Fatal(err)
	}
	// "1" = first sorted package = "bar".
	if _, hasBar := cs.Bumps["bar"]; !hasBar {
		t.Errorf("expected bar in Bumps: %+v", cs.Bumps)
	}
	if cs.Bumps["bar"].String() != "minor" {
		t.Errorf("bar level = %s, want minor", cs.Bumps["bar"])
	}
	if !strings.Contains(cs.Body, "First feature.") {
		t.Errorf("body missing 'First feature.': %q", cs.Body)
	}
}

func TestAddCmd_Interactive_NoPicks(t *testing.T) {
	f := newFixture(t, twoPackageTOML, nil, nil)
	input := "\n" // empty pick line
	root := newRootCmd()
	root.SetIn(strings.NewReader(input))
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"add", "--config", f.configPath})
	if err := root.Execute(); err == nil {
		t.Fatal("expected error for empty selection")
	}
}

// promptHuh has no end-to-end test. huh accessible mode is the only
// way to drive the form deterministically from a test, but each
// accessible field constructs its own bufio.Scanner over the supplied
// io.Reader. Scanners over-read into their internal 64KB buffer, so
// the first form's scanner consumes the second and third forms'
// inputs, leaving them at EOF and hanging or returning garbage.
// Working around that requires synchronized io.Pipe writes, which is
// more harness than the glue layer is worth.
//
// What's tested elsewhere:
//   - The bufio fallback path (TestAddCmd_Interactive*) exercises the
//     same parsePackageFlags/RandomName/IsValidName/WriteFile pipeline.
//   - semver.ParseBumpLevel covers the "patch"/"minor"/"major" parsing.
//   - changeset.{Parse,WriteFile,RandomName} are unit-tested in the
//     changeset package.
//
// What's not tested: the form composition (option order, default
// selection, validation lambdas). Those need a manual TTY check. Add
// a teatest harness here if huh form composition becomes a regression
// hotspot.
