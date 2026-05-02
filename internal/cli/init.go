package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	loglayer "go.loglayer.dev/v2"
	"golang.org/x/mod/modfile"

	"monorel.disaresta.com/config"
)

func newInitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Scaffold a monorel.toml + .changeset/ directory in the current repo.",
		Long: `Walks go.mod files in the current repo to detect packages, infers
owner/repo from the git origin remote, and writes:

    monorel.toml  - one [packages] block per detected Go module
    .changeset/   - directory with a README explaining the format

Refuses to overwrite an existing monorel.toml unless --force is given.
With --force, the existing provider/owner/repo are preserved as
defaults (CLI flags still override).`,
		RunE: runInit,
	}
	cmd.Flags().String("provider", "", "Version-control host. Defaults to `github`, or to the existing monorel.toml's provider on --force.")
	cmd.Flags().String("owner", "", "Repo owner. Auto-detected from git origin (or preserved from existing monorel.toml on --force).")
	cmd.Flags().String("repo", "", "Repo name. Auto-detected from git origin (or preserved from existing monorel.toml on --force).")
	cmd.Flags().Bool("force", false, "Overwrite an existing monorel.toml.")
	return cmd
}

// initOptions are the resolved inputs to doInit. All fields are
// optional: empty strings get filled in from the existing
// monorel.toml (when --force) or from `git config remote.origin.url`.
// Type and fields are unexported; the struct exists only as a test
// seam between cobra-driven runInit and the doInit body.
type initOptions struct {
	dir      string // working directory; defaults to os.Getwd().
	provider string // empty -> existing/github fallback.
	owner    string // empty -> existing/git-origin fallback.
	repo     string // empty -> existing/git-origin fallback.
	force    bool
}

func runInit(cmd *cobra.Command, _ []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	opts := initOptions{dir: cwd}
	opts.provider, _ = cmd.Flags().GetString("provider")
	opts.owner, _ = cmd.Flags().GetString("owner")
	opts.repo, _ = cmd.Flags().GetString("repo")
	opts.force, _ = cmd.Flags().GetBool("force")
	log, err := newLogger(cmd)
	if err != nil {
		return err
	}
	return doInit(log, opts)
}

// doInit is the body of `monorel init`, factored out of runInit so
// tests can drive it without process-global os.Chdir.
func doInit(log *loglayer.LogLayer, opts initOptions) error {
	configPath := filepath.Join(opts.dir, "monorel.toml")

	if !opts.force {
		if _, err := os.Stat(configPath); err == nil {
			return errors.New("monorel.toml already exists; rerun with --force to overwrite")
		}
	}

	// On --force, reload the existing config so we can preserve
	// provider/owner/repo edits the user made by hand. If the file
	// doesn't exist (--force on a fresh repo), silently fall through
	// to auto-detect; any other load failure (malformed TOML, schema
	// drift, validation error) gets a warning so the user knows their
	// preserved-fields expectation just didn't fire.
	var existing *config.Config
	if opts.force {
		cfg, err := config.Load(configPath)
		switch {
		case err == nil:
			existing = cfg
		case errors.Is(err, fs.ErrNotExist):
			// Fresh repo; nothing to preserve.
		default:
			log.Warn("existing monorel.toml could not be loaded (%v); falling back to auto-detect", err)
		}
	}

	provider := opts.provider
	if provider == "" {
		if existing != nil && existing.Provider.Name != "" {
			provider = existing.Provider.Name
		} else {
			provider = "github"
		}
	}
	// Fail upfront if the provider isn't one this build recognizes.
	// Otherwise init would happily write a monorel.toml that
	// `monorel validate` (and every command that calls config.Load)
	// would refuse — confusing for a user who just ran scaffold.
	if !config.IsKnownProvider(provider) {
		return fmt.Errorf("provider %q is not recognized; supported: %v", provider, config.KnownProviders)
	}

	owner := opts.owner
	repo := opts.repo
	if existing != nil {
		if owner == "" {
			owner = existing.Provider.Owner
		}
		if repo == "" {
			repo = existing.Provider.Repo
		}
	}
	if owner == "" || repo == "" {
		detectedOwner, detectedRepo, err := detectGitRemote(opts.dir)
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

	pkgs, err := detectPackages(opts.dir)
	if err != nil {
		return err
	}
	if len(pkgs) == 0 {
		return errors.New("no go.mod files found; monorel needs at least one Go module")
	}

	if err := os.WriteFile(configPath, []byte(renderInitTOML(provider, owner, repo, pkgs)), 0644); err != nil {
		return fmt.Errorf("write monorel.toml: %w", err)
	}

	changesetDir := filepath.Join(opts.dir, ".changeset")
	if err := os.MkdirAll(changesetDir, 0755); err != nil {
		return fmt.Errorf("create .changeset/: %w", err)
	}
	readmePath := filepath.Join(changesetDir, "README.md")
	switch _, err := os.Stat(readmePath); {
	case errors.Is(err, fs.ErrNotExist):
		if err := os.WriteFile(readmePath, []byte(changesetReadme), 0644); err != nil {
			return fmt.Errorf("write %s: %w", readmePath, err)
		}
	case err != nil:
		return fmt.Errorf("stat %s: %w", readmePath, err)
		// else: file present, leave it alone.
	}

	log.Info("Wrote monorel.toml with %d package(s):", len(pkgs))
	for _, p := range pkgs {
		log.Info("  %s (path: %s, tag prefix: %q)", p.Name, p.Path, p.TagPrefix)
	}
	log.Info("Created .changeset/ with a README.")
	log.Info("Next steps:")
	log.Info("  monorel validate     # confirm the config")
	log.Info("  monorel add          # write your first changeset")
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
// and parses the result. See parseGitRemote for supported URL shapes.
func detectGitRemote(dir string) (owner, repo string, err error) {
	c := exec.Command("git", "config", "--get", "remote.origin.url")
	c.Dir = dir
	raw, err := c.Output()
	if err != nil {
		return "", "", fmt.Errorf("read git origin: %w", err)
	}
	return parseGitRemote(strings.TrimSpace(string(raw)))
}

// parseGitRemote covers the URL shapes git emits in the wild:
//
//   - HTTPS / HTTP: https://<host>/<owner>/<repo>(.git)
//   - SSH (with scheme): ssh://git@<host>/<owner>/<repo>(.git)
//   - SSH (scp-like, no scheme): git@<host>:<owner>/<repo>(.git)
//   - git: git://<host>/<owner>/<repo>(.git)
//
// Schemed URLs are parsed via net/url, which handles credentials,
// custom ports, IPv6 hosts, and percent-encoding for free. file://
// URLs are explicitly rejected since they have no owner/repo concept.
//
// monorel doesn't validate the host part because it doesn't care
// which host is named, only what owner/repo to put in monorel.toml.
func parseGitRemote(raw string) (owner, repo string, err error) {
	raw = strings.TrimSuffix(raw, ".git")

	// Schemed URLs (https://, ssh://, git://, file://, etc.).
	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil {
			return "", "", fmt.Errorf("parse remote URL %q: %w", raw, err)
		}
		if u.Scheme == "file" {
			return "", "", fmt.Errorf("remote URL is file://; pass --owner/--repo explicitly")
		}
		path := strings.TrimPrefix(u.Path, "/")
		return splitOwnerRepo(path)
	}

	// SSH scp-like form: git@<host>:<owner>/<repo>
	if at := strings.Index(raw, "@"); at >= 0 {
		// Find the colon AFTER the @ to skip user@ separators.
		colon := strings.Index(raw[at:], ":")
		if colon >= 0 {
			path := raw[at+colon+1:]
			return splitOwnerRepo(path)
		}
	}
	return "", "", fmt.Errorf("unrecognized remote URL shape: %q (pass --owner/--repo)", raw)
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

// readModulePath returns the value of go.mod's `module` directive,
// using the official x/mod/modfile parser so every spec-allowed shape
// (block-form modules, comments, quoted paths, retract/exclude/replace
// directives) is handled the same way `go` itself handles it.
func readModulePath(goModPath string) (string, error) {
	data, err := os.ReadFile(goModPath)
	if err != nil {
		return "", err
	}
	f, err := modfile.Parse(goModPath, data, nil)
	if err != nil {
		return "", err
	}
	if f.Module == nil {
		return "", errors.New("no module directive found")
	}
	return f.Module.Mod.Path, nil
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
