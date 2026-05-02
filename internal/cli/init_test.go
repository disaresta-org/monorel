package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSplitOwnerRepo(t *testing.T) {
	cases := []struct {
		in        string
		owner     string
		repo      string
		shouldErr bool
	}{
		{"acme/widget", "acme", "widget", false},
		{"acme/widget/", "acme", "widget", false},
		{"a/b/c", "a", "b/c", false},
		{"acme", "", "", true},
		{"/widget", "", "", true},
		{"acme/", "", "", true},
		{"", "", "", true},
	}
	for _, tc := range cases {
		owner, repo, err := splitOwnerRepo(tc.in)
		gotErr := err != nil
		if gotErr != tc.shouldErr {
			t.Errorf("splitOwnerRepo(%q): err=%v want shouldErr=%v", tc.in, err, tc.shouldErr)
		}
		if !tc.shouldErr && (owner != tc.owner || repo != tc.repo) {
			t.Errorf("splitOwnerRepo(%q) = %q, %q, want %q, %q", tc.in, owner, repo, tc.owner, tc.repo)
		}
	}
}

func TestParseGitRemote(t *testing.T) {
	cases := []struct {
		name      string
		remoteURL string
		owner     string
		repo      string
		shouldErr bool
	}{
		{"https with .git", "https://github.com/acme/widget.git", "acme", "widget", false},
		{"https without .git", "https://github.com/acme/widget", "acme", "widget", false},
		{"http", "http://gitea.example.com/acme/widget.git", "acme", "widget", false},
		{"ssh-with-scheme", "ssh://git@github.com/acme/widget.git", "acme", "widget", false},
		{"ssh-with-scheme custom port", "ssh://git@github.com:2222/acme/widget.git", "acme", "widget", false},
		{"ssh scp-like", "git@github.com:acme/widget.git", "acme", "widget", false},
		{"ssh scp-like no .git", "git@gitlab.com:acme/widget", "acme", "widget", false},
		{"git scheme", "git://github.com/acme/widget.git", "acme", "widget", false},
		{"https with credentials", "https://user:pass@github.com/acme/widget.git", "acme", "widget", false},
		{"https with subgroup (gitlab)", "https://gitlab.com/team/sub/widget.git", "team", "sub/widget", false},
		{"https IPv6 host", "https://[::1]/acme/widget", "acme", "widget", false},
		// Rejected shapes.
		{"file URL", "file:///home/me/repo", "", "", true},
		{"unrecognized", "random-string-no-colon", "", "", true},
		{"git@ no colon", "git@github.com", "", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			owner, repo, err := parseGitRemote(tc.remoteURL)
			gotErr := err != nil
			if gotErr != tc.shouldErr {
				t.Errorf("parseGitRemote(%q): err=%v want shouldErr=%v", tc.remoteURL, err, tc.shouldErr)
			}
			if !tc.shouldErr && (owner != tc.owner || repo != tc.repo) {
				t.Errorf("parseGitRemote(%q) = %q, %q, want %q, %q", tc.remoteURL, owner, repo, tc.owner, tc.repo)
			}
		})
	}
}

func TestDetectGitRemote(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()
	runGit(t, dir, "init", "-q")
	runGit(t, dir, "remote", "add", "origin", "https://github.com/acme/widget.git")
	owner, repo, err := detectGitRemote(dir)
	if err != nil {
		t.Fatalf("detectGitRemote: %v", err)
	}
	if owner != "acme" || repo != "widget" {
		t.Errorf("got owner=%q repo=%q, want acme/widget", owner, repo)
	}
}

func TestDetectGitRemote_NoRemote(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()
	runGit(t, dir, "init", "-q")
	if _, _, err := detectGitRemote(dir); err == nil {
		t.Fatal("expected error when origin remote is missing")
	}
}

func TestReadModulePath(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"basic", "module github.com/acme/widget\n\ngo 1.23\n", "github.com/acme/widget"},
		{"leading blank", "\n\nmodule example.com/foo\n", "example.com/foo"},
		{"with comment", "// header\nmodule example.com/foo\n", "example.com/foo"},
		{"trailing comment", "module example.com/foo // canonical\n", "example.com/foo"},
		{"quoted", `module "example.com/foo"` + "\n", "example.com/foo"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "go.mod")
			if err := os.WriteFile(path, []byte(tc.body), 0644); err != nil {
				t.Fatal(err)
			}
			got, err := readModulePath(path)
			if err != nil {
				t.Fatalf("readModulePath: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestReadModulePath_Missing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "go.mod")
	if err := os.WriteFile(path, []byte("// no module here\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := readModulePath(path); err == nil {
		t.Fatal("expected error when module directive is missing")
	}
}

func TestDetectPackages(t *testing.T) {
	dir := t.TempDir()
	writeMod(t, dir, "go.mod", "module example.com/root\n")
	writeMod(t, dir, "transports/zerolog/go.mod", "module example.com/root/transports/zerolog\n")
	writeMod(t, dir, "transports/zap/go.mod", "module example.com/root/transports/zap\n")
	// Three-level nesting. Tests that filepath.ToSlash normalizes
	// separators and that the rendered path contains forward slashes.
	writeMod(t, dir, "plugins/foo/livetest/go.mod", "module example.com/root/plugins/foo/livetest\n")
	// vendor and hidden dirs ignored.
	writeMod(t, dir, "vendor/github.com/x/y/go.mod", "module github.com/x/y\n")
	writeMod(t, dir, ".tooling/skip/go.mod", "module example.com/skip\n")

	pkgs, err := detectPackages(dir)
	if err != nil {
		t.Fatalf("detectPackages: %v", err)
	}
	if len(pkgs) != 4 {
		t.Fatalf("want 4 packages, got %d: %+v", len(pkgs), pkgs)
	}
	// Root first, then sub-modules alphabetical by path.
	if pkgs[0].Path != "." || pkgs[0].Name != "example.com/root" {
		t.Errorf("[0] = %+v, want root", pkgs[0])
	}
	if pkgs[1].Path != "plugins/foo/livetest" {
		t.Errorf("[1] = %+v, want plugins/foo/livetest", pkgs[1])
	}
	if pkgs[1].Changelog != "plugins/foo/livetest/CHANGELOG.md" {
		t.Errorf("[1].Changelog = %q, want plugins/foo/livetest/CHANGELOG.md", pkgs[1].Changelog)
	}
	if pkgs[2].Path != "transports/zap" {
		t.Errorf("[2] = %+v, want transports/zap", pkgs[2])
	}
	if pkgs[3].Path != "transports/zerolog" {
		t.Errorf("[3] = %+v, want transports/zerolog", pkgs[3])
	}
}

func TestRenderInitTOML(t *testing.T) {
	got := renderInitTOML("github", "acme", "widget", []initPkg{
		{Name: "example.com/root", TagPrefix: "", Path: ".", Changelog: "CHANGELOG.md"},
		{Name: "transports/zap", TagPrefix: "transports/zap", Path: "transports/zap", Changelog: "transports/zap/CHANGELOG.md"},
	})
	for _, want := range []string{
		`[provider]`,
		`name  = "github"`,
		`owner = "acme"`,
		`repo  = "widget"`,
		`[packages."example.com/root"]`,
		`tag_prefix = ""`,
		`path       = "."`,
		`[packages."transports/zap"]`,
		`tag_prefix = "transports/zap"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered TOML missing %q\nfull:\n%s", want, got)
		}
	}
}

func TestDoInit_FreshRepo(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()
	runGit(t, dir, "init", "-q")
	runGit(t, dir, "remote", "add", "origin", "https://github.com/acme/widget.git")
	writeMod(t, dir, "go.mod", "module github.com/acme/widget\n")

	var out bytes.Buffer
	if err := doInit(&out, initOptions{dir: dir}); err != nil {
		t.Fatalf("doInit: %v", err)
	}

	if !fileExists(filepath.Join(dir, "monorel.toml")) {
		t.Fatal("monorel.toml not created")
	}
	if !fileExists(filepath.Join(dir, ".changeset", "README.md")) {
		t.Fatal(".changeset/README.md not created")
	}
	body, _ := os.ReadFile(filepath.Join(dir, "monorel.toml"))
	for _, want := range []string{
		`name  = "github"`,
		`owner = "acme"`,
		`repo  = "widget"`,
		`[packages."github.com/acme/widget"]`,
	} {
		if !strings.Contains(string(body), want) {
			t.Errorf("monorel.toml missing %q", want)
		}
	}
}

func TestDoInit_RefusesWithoutForce(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "monorel.toml"), []byte("# stub\n"), 0644); err != nil {
		t.Fatal(err)
	}
	writeMod(t, dir, "go.mod", "module example.com/foo\n")

	err := doInit(&bytes.Buffer{}, initOptions{dir: dir, owner: "a", repo: "b"})
	if err == nil {
		t.Fatal("expected error when monorel.toml exists without --force")
	}
}

func TestDoInit_ForcePreservesExistingProvider(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()
	// Existing monorel.toml with hand-edited provider/owner/repo. Uses
	// "github" because Validate() rejects unknown provider names; the
	// preserve path requires a loadable existing config.
	original := `[provider]
name  = "github"
owner = "preserved-team"
repo  = "preserved-name"

[packages."example.com/foo"]
tag_prefix = ""
path       = "."
changelog  = "CHANGELOG.md"
`
	if err := os.WriteFile(filepath.Join(dir, "monorel.toml"), []byte(original), 0644); err != nil {
		t.Fatal(err)
	}
	writeMod(t, dir, "go.mod", "module example.com/foo\n")
	// Origin remote points somewhere else; init must NOT overwrite the
	// owner/repo with the auto-detected values when --force reloads the
	// existing config.
	runGit(t, dir, "init", "-q")
	runGit(t, dir, "remote", "add", "origin", "https://github.com/wrong/wrong.git")

	if err := doInit(&bytes.Buffer{}, initOptions{dir: dir, force: true}); err != nil {
		t.Fatalf("doInit --force: %v", err)
	}

	body, _ := os.ReadFile(filepath.Join(dir, "monorel.toml"))
	for _, want := range []string{
		`name  = "github"`,
		`owner = "preserved-team"`,
		`repo  = "preserved-name"`,
	} {
		if !strings.Contains(string(body), want) {
			t.Errorf("preserved field missing: want %q\nfull:\n%s", want, body)
		}
	}
	// Sanity: the wrong values from origin must NOT appear.
	if strings.Contains(string(body), `owner = "wrong"`) {
		t.Error("origin auto-detect leaked into preserved provider block")
	}
}

func TestDoInit_ForceFlagsOverrideExisting(t *testing.T) {
	dir := t.TempDir()
	original := `[provider]
name  = "github"
owner = "old-team"
repo  = "old-name"

[packages."example.com/foo"]
tag_prefix = ""
path       = "."
changelog  = "CHANGELOG.md"
`
	if err := os.WriteFile(filepath.Join(dir, "monorel.toml"), []byte(original), 0644); err != nil {
		t.Fatal(err)
	}
	writeMod(t, dir, "go.mod", "module example.com/foo\n")

	if err := doInit(&bytes.Buffer{}, initOptions{
		dir:   dir,
		force: true,
		owner: "new-team",
		repo:  "new-name",
	}); err != nil {
		t.Fatalf("doInit: %v", err)
	}

	body, _ := os.ReadFile(filepath.Join(dir, "monorel.toml"))
	for _, want := range []string{
		`owner = "new-team"`,
		`repo  = "new-name"`,
	} {
		if !strings.Contains(string(body), want) {
			t.Errorf("flag override missing: %q\nfull:\n%s", want, body)
		}
	}
	if strings.Contains(string(body), `"old-team"`) || strings.Contains(string(body), `"old-name"`) {
		t.Errorf("override should drop old values\nfull:\n%s", body)
	}
}

func TestDoInit_ForcePreservesExistingReadme(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".changeset"), 0755); err != nil {
		t.Fatal(err)
	}
	customReadme := "# My custom changeset readme\n"
	readmePath := filepath.Join(dir, ".changeset", "README.md")
	if err := os.WriteFile(readmePath, []byte(customReadme), 0644); err != nil {
		t.Fatal(err)
	}
	writeMod(t, dir, "go.mod", "module example.com/foo\n")

	if err := doInit(&bytes.Buffer{}, initOptions{
		dir:   dir,
		owner: "acme",
		repo:  "widget",
	}); err != nil {
		t.Fatalf("doInit: %v", err)
	}
	got, _ := os.ReadFile(readmePath)
	if string(got) != customReadme {
		t.Errorf("README.md was overwritten: got %q, want %q", got, customReadme)
	}
}

func TestDoInit_NoGoMod(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()
	runGit(t, dir, "init", "-q")
	runGit(t, dir, "remote", "add", "origin", "https://github.com/acme/widget.git")
	if err := doInit(&bytes.Buffer{}, initOptions{dir: dir}); err == nil {
		t.Fatal("expected error when no go.mod files are present")
	}
}

func TestDoInit_OwnerRepoOverrideNoRemote(t *testing.T) {
	dir := t.TempDir()
	// No git init at all. Must rely on flag-supplied values. Provider
	// stays at the default (github) because init validates against
	// KnownProviders before writing.
	writeMod(t, dir, "go.mod", "module example.com/foo\n")
	if err := doInit(&bytes.Buffer{}, initOptions{
		dir:   dir,
		owner: "acme",
		repo:  "widget",
	}); err != nil {
		t.Fatalf("doInit: %v", err)
	}
	body, _ := os.ReadFile(filepath.Join(dir, "monorel.toml"))
	for _, want := range []string{`name  = "github"`, `owner = "acme"`, `repo  = "widget"`} {
		if !strings.Contains(string(body), want) {
			t.Errorf("monorel.toml missing %q", want)
		}
	}
}

func TestDoInit_RejectsUnknownProvider(t *testing.T) {
	dir := t.TempDir()
	writeMod(t, dir, "go.mod", "module example.com/foo\n")
	err := doInit(&bytes.Buffer{}, initOptions{
		dir:      dir,
		provider: "bitbucket", // not in KnownProviders today.
		owner:    "acme",
		repo:     "widget",
	})
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
	if !strings.Contains(err.Error(), "not recognized") {
		t.Errorf("err = %v, want one mentioning unrecognized provider", err)
	}
	if fileExists(filepath.Join(dir, "monorel.toml")) {
		t.Error("monorel.toml should not be written when provider is rejected")
	}
}

func TestDoInit_ForceMalformedConfigFallsThrough(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()
	// Existing config that fails to parse (unknown TOML key from
	// the pre-v0.3.0 layout). config.Load returns an error; init
	// should warn AND fall through to git auto-detect rather than
	// silently using empty defaults.
	stale := `[forge]
name  = "github"
owner = "old-team"
repo  = "old-name"

[packages."example.com/foo"]
tag_prefix = ""
path       = "."
changelog  = "CHANGELOG.md"
`
	if err := os.WriteFile(filepath.Join(dir, "monorel.toml"), []byte(stale), 0644); err != nil {
		t.Fatal(err)
	}
	writeMod(t, dir, "go.mod", "module example.com/foo\n")
	runGit(t, dir, "init", "-q")
	runGit(t, dir, "remote", "add", "origin", "https://github.com/auto-detected/from-origin.git")

	var out bytes.Buffer
	if err := doInit(&out, initOptions{dir: dir, force: true}); err != nil {
		t.Fatalf("doInit: %v", err)
	}
	if !strings.Contains(out.String(), "warning") {
		t.Errorf("expected a warning when malformed config falls through, got:\n%s", out.String())
	}
	body, _ := os.ReadFile(filepath.Join(dir, "monorel.toml"))
	if !strings.Contains(string(body), `owner = "auto-detected"`) || !strings.Contains(string(body), `repo  = "from-origin"`) {
		t.Errorf("expected git-origin auto-detect to fill owner/repo:\n%s", body)
	}
	// And the stale "old-team"/"old-name" must NOT appear (preserved
	// values from a malformed file would be a footgun).
	if strings.Contains(string(body), `"old-team"`) || strings.Contains(string(body), `"old-name"`) {
		t.Errorf("stale values leaked from malformed config:\n%s", body)
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	c := exec.Command("git", args...)
	c.Dir = dir
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

// requireGit skips when git isn't on PATH. CI is expected to have
// git; this guard exists for contributor laptops or sparse runner
// images. If a CI run reports lots of skipped tests, check that git
// is installed in the runner image — silent skips would otherwise
// erode integration coverage unnoticed.
func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git not on PATH: %v", err)
	}
}

func writeMod(t *testing.T, root, rel, body string) {
	t.Helper()
	full := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
