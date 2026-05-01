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

func TestDetectGitRemote(t *testing.T) {
	cases := []struct {
		name      string
		remoteURL string
		owner     string
		repo      string
	}{
		{"https with .git", "https://github.com/acme/widget.git", "acme", "widget"},
		{"https without .git", "https://github.com/acme/widget", "acme", "widget"},
		{"ssh", "git@github.com:acme/widget.git", "acme", "widget"},
		{"ssh no .git", "git@gitlab.com:acme/widget", "acme", "widget"},
		{"https with subgroup (gitlab)", "https://gitlab.com/team/sub/widget.git", "team", "sub/widget"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			runGit(t, dir, "init", "-q")
			runGit(t, dir, "remote", "add", "origin", tc.remoteURL)
			owner, repo, err := detectGitRemote(dir)
			if err != nil {
				t.Fatalf("detectGitRemote: %v", err)
			}
			if owner != tc.owner || repo != tc.repo {
				t.Errorf("got owner=%q repo=%q, want %q, %q", owner, repo, tc.owner, tc.repo)
			}
		})
	}
}

func TestDetectGitRemote_NoRemote(t *testing.T) {
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
	// vendor and hidden dirs ignored.
	writeMod(t, dir, "vendor/github.com/x/y/go.mod", "module github.com/x/y\n")
	writeMod(t, dir, ".tooling/skip/go.mod", "module example.com/skip\n")

	pkgs, err := detectPackages(dir)
	if err != nil {
		t.Fatalf("detectPackages: %v", err)
	}
	if len(pkgs) != 3 {
		t.Fatalf("want 3 packages, got %d: %+v", len(pkgs), pkgs)
	}
	// Root first, then sub-modules alphabetical by path.
	if pkgs[0].Path != "." || pkgs[0].Name != "example.com/root" {
		t.Errorf("[0] = %+v, want root", pkgs[0])
	}
	if pkgs[1].Path != "transports/zap" {
		t.Errorf("[1] = %+v, want transports/zap", pkgs[1])
	}
	if pkgs[2].Path != "transports/zerolog" {
		t.Errorf("[2] = %+v, want transports/zerolog", pkgs[2])
	}
	if pkgs[1].Changelog != "transports/zap/CHANGELOG.md" {
		t.Errorf("[1].Changelog = %q", pkgs[1].Changelog)
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

func TestInitCmd_EndToEnd(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init", "-q")
	runGit(t, dir, "remote", "add", "origin", "https://github.com/acme/widget.git")
	writeMod(t, dir, "go.mod", "module github.com/acme/widget\n")

	prevWd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(prevWd) })

	root := newRootCmd()
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"init"})
	if err := root.Execute(); err != nil {
		t.Fatalf("init: %v\nstderr: %s", err, stderr.String())
	}

	if !fileExists(filepath.Join(dir, "monorel.toml")) {
		t.Fatal("monorel.toml not created")
	}
	if !fileExists(filepath.Join(dir, ".changeset", "README.md")) {
		t.Fatal(".changeset/README.md not created")
	}
	body, _ := os.ReadFile(filepath.Join(dir, "monorel.toml"))
	for _, want := range []string{`name  = "github"`, `owner = "acme"`, `repo  = "widget"`, `[packages."github.com/acme/widget"]`} {
		if !strings.Contains(string(body), want) {
			t.Errorf("monorel.toml missing %q", want)
		}
	}

	// Re-run without --force should refuse.
	root2 := newRootCmd()
	var stderr2 bytes.Buffer
	root2.SetOut(&bytes.Buffer{})
	root2.SetErr(&stderr2)
	root2.SetArgs([]string{"init"})
	if err := root2.Execute(); err == nil {
		t.Fatal("expected error on rerun without --force")
	}

	// With --force, allowed.
	root3 := newRootCmd()
	root3.SetOut(&bytes.Buffer{})
	root3.SetErr(&bytes.Buffer{})
	root3.SetArgs([]string{"init", "--force"})
	if err := root3.Execute(); err != nil {
		t.Errorf("init --force should succeed: %v", err)
	}
}

func TestInitCmd_OwnerRepoOverride(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init", "-q")
	// No origin remote configured. --owner/--repo flags must be honored.
	writeMod(t, dir, "go.mod", "module example.com/foo\n")

	prevWd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(prevWd) })

	root := newRootCmd()
	var stderr bytes.Buffer
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&stderr)
	root.SetArgs([]string{"init", "--owner", "acme", "--repo", "widget", "--provider", "gitlab"})
	if err := root.Execute(); err != nil {
		t.Fatalf("init: %v\nstderr: %s", err, stderr.String())
	}
	body, _ := os.ReadFile(filepath.Join(dir, "monorel.toml"))
	for _, want := range []string{`name  = "gitlab"`, `owner = "acme"`, `repo  = "widget"`} {
		if !strings.Contains(string(body), want) {
			t.Errorf("monorel.toml missing %q", want)
		}
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
