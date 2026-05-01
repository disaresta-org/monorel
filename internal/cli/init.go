package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

func newInitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Scaffold a monorel.toml + .changeset/ directory in the current repo.",
		Long: `Walks go.mod files in the current repo to detect packages, infers
owner/repo from the git origin remote, and writes:

    monorel.toml  - one [packages] block per detected Go module
    .changeset/   - directory with a README explaining the format

Refuses to overwrite an existing monorel.toml unless --force is given.`,
		RunE: runInit,
	}
	cmd.Flags().String("provider", "github", "Version-control host (github, gitlab, gitea, etc.).")
	cmd.Flags().String("owner", "", "Repo owner. Auto-detected from git origin if empty.")
	cmd.Flags().String("repo", "", "Repo name. Auto-detected from git origin if empty.")
	cmd.Flags().Bool("force", false, "Overwrite an existing monorel.toml.")
	return cmd
}

func runInit(cmd *cobra.Command, _ []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	configPath := filepath.Join(cwd, "monorel.toml")
	force, _ := cmd.Flags().GetBool("force")
	if !force {
		if _, err := os.Stat(configPath); err == nil {
			return errors.New("monorel.toml already exists; rerun with --force to overwrite")
		}
	}

	provider, _ := cmd.Flags().GetString("provider")
	owner, _ := cmd.Flags().GetString("owner")
	repo, _ := cmd.Flags().GetString("repo")
	if owner == "" || repo == "" {
		detectedOwner, detectedRepo, err := detectGitRemote(cwd)
		if err != nil {
			return fmt.Errorf("could not auto-detect owner/repo from git origin: %w (pass --owner/--repo)", err)
		}
		if owner == "" {
			owner = detectedOwner
		}
		if repo == "" {
			repo = detectedRepo
		}
	}

	pkgs, err := detectPackages(cwd)
	if err != nil {
		return err
	}
	if len(pkgs) == 0 {
		return errors.New("no go.mod files found; monorel needs at least one Go module")
	}

	if err := os.WriteFile(configPath, []byte(renderInitTOML(provider, owner, repo, pkgs)), 0644); err != nil {
		return fmt.Errorf("write monorel.toml: %w", err)
	}

	changesetDir := filepath.Join(cwd, ".changeset")
	if err := os.MkdirAll(changesetDir, 0755); err != nil {
		return fmt.Errorf("create .changeset/: %w", err)
	}
	readmePath := filepath.Join(changesetDir, "README.md")
	if _, err := os.Stat(readmePath); errors.Is(err, fs.ErrNotExist) {
		if err := os.WriteFile(readmePath, []byte(changesetReadme), 0644); err != nil {
			return fmt.Errorf("write .changeset/README.md: %w", err)
		}
	}

	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Wrote monorel.toml with %d package(s):\n", len(pkgs))
	for _, p := range pkgs {
		fmt.Fprintf(out, "  %s (path: %s, tag prefix: %q)\n", p.Name, p.Path, p.TagPrefix)
	}
	fmt.Fprintln(out, "Created .changeset/ with a README.")
	fmt.Fprintln(out, "Next steps:")
	fmt.Fprintln(out, "  monorel validate     # confirm the config")
	fmt.Fprintln(out, "  monorel add          # write your first changeset")
	return nil
}

// initPkg is the per-package shape rendered into monorel.toml. The
// fields mirror config.PackageConfig; init.go owns the file format
// rather than re-using the encoder so the output diff stays
// hand-readable (aligned `=`, comments, blank lines between blocks).
type initPkg struct {
	Name      string
	TagPrefix string
	Path      string
	Changelog string
}

// detectGitRemote runs `git config --get remote.origin.url` in dir
// and parses the result. Supports the two URL shapes git emits:
//
//	https://github.com/<owner>/<repo>(.git)
//	git@github.com:<owner>/<repo>(.git)
//
// Other hosts (gitlab.com, gitea.example.com) work the same way; we
// don't validate the host part because monorel itself doesn't care
// which host is named, only what owner/repo to put in monorel.toml.
func detectGitRemote(dir string) (owner, repo string, err error) {
	c := exec.Command("git", "config", "--get", "remote.origin.url")
	c.Dir = dir
	raw, err := c.Output()
	if err != nil {
		return "", "", fmt.Errorf("read git origin: %w", err)
	}
	url := strings.TrimSpace(string(raw))
	url = strings.TrimSuffix(url, ".git")

	// SSH form: git@<host>:<owner>/<repo>
	if at := strings.Index(url, "@"); at >= 0 && strings.Contains(url, ":") && !strings.HasPrefix(url, "http") {
		colon := strings.Index(url, ":")
		path := url[colon+1:]
		return splitOwnerRepo(path)
	}
	// HTTPS form: https://<host>/<owner>/<repo>
	if i := strings.Index(url, "://"); i >= 0 {
		path := url[i+3:]
		// drop host
		if slash := strings.Index(path, "/"); slash >= 0 {
			path = path[slash+1:]
		}
		return splitOwnerRepo(path)
	}
	return "", "", fmt.Errorf("unrecognized remote URL shape: %q", url)
}

func splitOwnerRepo(path string) (owner, repo string, err error) {
	parts := strings.SplitN(strings.TrimSuffix(path, "/"), "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("could not split owner/repo from %q", path)
	}
	return parts[0], parts[1], nil
}

// detectPackages walks repoRoot looking for go.mod files. The
// top-level module (if present) gets tag_prefix "" and path "."; sub-
// module go.mod files get tag_prefix and path equal to their relative
// directory. Skips vendor/, node_modules/, and any hidden directory.
//
// Detection is by go.mod presence, not by go.work parsing — go.work
// is optional and missing in many real repos. When it exists, every
// `use ./<path>` entry has its own go.mod, so walking go.mod files
// covers the same packages.
func detectPackages(repoRoot string) ([]initPkg, error) {
	var pkgs []initPkg
	walkErr := filepath.WalkDir(repoRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if path != repoRoot && (strings.HasPrefix(name, ".") || name == "vendor" || name == "node_modules") {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() != "go.mod" {
			return nil
		}
		modPath, err := readModulePath(path)
		if err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		rel, err := filepath.Rel(repoRoot, filepath.Dir(path))
		if err != nil {
			return err
		}
		// Normalize Windows separators in the recorded path.
		rel = filepath.ToSlash(rel)

		if rel == "." {
			pkgs = append(pkgs, initPkg{
				Name:      modPath,
				TagPrefix: "",
				Path:      ".",
				Changelog: "CHANGELOG.md",
			})
			return nil
		}
		pkgs = append(pkgs, initPkg{
			Name:      rel,
			TagPrefix: rel,
			Path:      rel,
			Changelog: rel + "/CHANGELOG.md",
		})
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}
	// Stable order: root first, then sub-modules alphabetically. Keeps
	// the rendered monorel.toml diff-friendly across runs.
	sort.SliceStable(pkgs, func(i, j int) bool {
		if pkgs[i].Path == "." {
			return true
		}
		if pkgs[j].Path == "." {
			return false
		}
		return pkgs[i].Path < pkgs[j].Path
	})
	return pkgs, nil
}

// readModulePath returns the value of go.mod's `module` directive.
// Tolerates blank lines, comments, and the // <comment> trailing form.
func readModulePath(goModPath string) (string, error) {
	f, err := os.Open(goModPath)
	if err != nil {
		return "", err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}
		if !strings.HasPrefix(line, "module ") {
			continue
		}
		rest := strings.TrimSpace(strings.TrimPrefix(line, "module "))
		// Drop trailing comment if present.
		if i := strings.Index(rest, "//"); i >= 0 {
			rest = strings.TrimSpace(rest[:i])
		}
		// Strip surrounding quotes if any (rare but allowed by spec).
		rest = strings.Trim(rest, `"`)
		return rest, nil
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", errors.New("no module directive found")
}

func renderInitTOML(provider, owner, repo string, pkgs []initPkg) string {
	var b strings.Builder
	fmt.Fprintln(&b, "# monorel configuration. See https://monorel.disaresta.com/configuration")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "[provider]")
	fmt.Fprintf(&b, "name  = %q\n", provider)
	fmt.Fprintf(&b, "owner = %q\n", owner)
	fmt.Fprintf(&b, "repo  = %q\n", repo)
	fmt.Fprintln(&b)
	for _, p := range pkgs {
		fmt.Fprintf(&b, "[packages.%q]\n", p.Name)
		fmt.Fprintf(&b, "tag_prefix = %q\n", p.TagPrefix)
		fmt.Fprintf(&b, "path       = %q\n", p.Path)
		fmt.Fprintf(&b, "changelog  = %q\n", p.Changelog)
		fmt.Fprintln(&b)
	}
	return b.String()
}

const changesetReadme = `# Changesets

This directory holds pending release intents for [monorel](https://monorel.disaresta.com).

## What's a changeset?

A ` + "`.changeset/<name>.md`" + ` file declares which packages should release at what bump level when the next release lands, plus the changelog body to use for each release.

Example:

` + "```markdown" + `
---
"<package-name>": minor
---

What changed and why.
` + "```" + `

The frontmatter keys are package names from ` + "`monorel.toml`" + `. Bump levels are ` + "`major`" + `, ` + "`minor`" + `, or ` + "`patch`" + `. The body becomes the rendered changelog entry for every package the changeset names.

## Authoring

Run ` + "`monorel add`" + ` for the interactive flow, or write the file directly. See [the docs](https://monorel.disaresta.com/changesets) for details.
`
