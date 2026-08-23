// Package config loads and validates monorel.toml.
//
// monorel.toml is the single source of truth for which packages exist
// in the repo, where their go.mod and CHANGELOG live, what scope they
// own in commit messages, and how their tags are prefixed. Everything
// else (changesets, the planner) refers to packages by the names
// declared here.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
)

// Config is the parsed contents of monorel.toml.
//
// Packages is keyed by the human-friendly package name used as both the
// changeset frontmatter key and (combined with TagPrefix) the git tag
// prefix. Names are arbitrary identifiers; convention is to use the
// package's filesystem path for sub-modules ("transports/zerolog") and
// the import path for the root module ("github.com/foo/bar").
type Config struct {
	Provider ProviderConfig           `toml:"provider"`
	Packages map[string]PackageConfig `toml:"packages"`
}

// ProviderConfig identifies the version-control host monorel manages
// releases on (GitHub, GitLab, Gitea, etc.). The Name field selects
// the implementation; Owner/Repo (and Host for self-hosted
// installations) identify the specific repository.
type ProviderConfig struct {
	// Name selects the provider implementation. Recognized values:
	// "github" (default when empty). Future: "gitlab", "gitea",
	// "bitbucket", "forgejo".
	Name string `toml:"name"`

	// Owner is the user or org that owns the repo. On GitLab this
	// maps to the namespace; on Bitbucket to the workspace.
	Owner string `toml:"owner"`

	// Repo is the repository name.
	Repo string `toml:"repo"`

	// Host is the API host for self-hosted installations (e.g.
	// "gitlab.example.com"). Empty means use the provider's default
	// public host.
	Host string `toml:"host"`
}

// PackageConfig is the per-package settings block in monorel.toml.
type PackageConfig struct {
	// TagPrefix is prepended (with a "/" separator) to the version
	// when forming the git tag. Empty means bare tags ("v1.6.2"); a
	// value like "transports/foo" produces "transports/foo/v1.6.2".
	//
	// Bare tags are required by Go for the main module at the repo
	// root; prefixed tags are required for sub-modules so go get can
	// resolve them.
	TagPrefix string `toml:"tag_prefix"`

	// Path is the package directory relative to the repo root. The
	// planner uses this to locate go.mod for v2+ major-version updates
	// (Go's "/v2" import path quirk). For v1 packages it's purely
	// informational.
	Path string `toml:"path"`

	// Changelog is the path (relative to the repo root) of the
	// per-package CHANGELOG.md monorel writes new entries to.
	Changelog string `toml:"changelog"`

	// ModuleMajor is the major implied by the package's Go module
	// directive ("go.loglayer.dev/transports/foo/v3" -> 3, 0 when
	// unversioned). Populated by config.Load from the package's
	// go.mod; the path itself is repo-relative and not reliable for
	// this (a directory name can end in /vN without the module being
	// versioned there, and vice versa). Plan uses it to align planned
	// releases with the major Go requires for the module path.
	ModuleMajor uint64 `toml:"-"`
}

// FullTagPrefix returns the prefix used to match tags for this
// package, including the trailing "/" separator. Empty when
// TagPrefix is empty (bare-tag root module).
func (p PackageConfig) FullTagPrefix() string {
	if p.TagPrefix == "" {
		return ""
	}
	return p.TagPrefix + "/"
}

// TagFor returns the full git tag for the given version, e.g.
// "transports/zerolog/v1.6.2" or "v1.6.2".
func (p PackageConfig) TagFor(version string) string {
	return p.FullTagPrefix() + version
}

// PackageNames returns every package name in lexicographic order.
// Stable iteration is convenient for tests, log output, and PR
// rendering.
func (c *Config) PackageNames() []string {
	names := make([]string, 0, len(c.Packages))
	for name := range c.Packages {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Load reads the file at path, parses it as TOML, validates the
// result, then walks each package's go.mod to populate ModuleMajor
// (the major its module path demands, per Go module convention).
// Returns the parsed config or a wrapped error explaining what went
// wrong. A package whose go.mod is missing or unreadable keeps
// ModuleMajor 0 (unversioned), so a stale or partial checkout never
// blocks a release.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var cfg Config
	meta, err := toml.Decode(string(data), &cfg)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if undecoded := meta.Undecoded(); len(undecoded) > 0 {
		// Fail loudly on unknown keys so config typos don't silently
		// degrade behavior. Future config additions go through proper
		// schema evolution rather than tolerating drift.
		keys := make([]string, len(undecoded))
		for i, k := range undecoded {
			keys[i] = k.String()
		}
		return nil, fmt.Errorf("parse %s: unknown keys: %v", path, keys)
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate %s: %w", path, err)
	}
	cfg.populateModuleMajors(filepath.Dir(path))
	return &cfg, nil
}

// populateModuleMajors fills pkg.ModuleMajor for each package by
// reading the package's go.mod module directive. repoDir is the
// directory containing monorel.toml.
func (c *Config) populateModuleMajors(repoDir string) {
	for name, pkg := range c.Packages {
		modDir := filepath.Join(repoDir, pkg.Path, "go.mod")
		data, err := os.ReadFile(modDir)
		if err != nil {
			continue // no go.mod (e.g. pure-changelog package): unversioned
		}
		mod := moduleMajorFromGoMod(data)
		pkg.ModuleMajor = mod
		c.Packages[name] = pkg
	}
}

// moduleMajorFromGoMod parses data as a go.mod file and returns the
// major implied by its module directive's version suffix, or 0 when
// the directive is missing or unversioned. A malformed suffix (a
// non-numeric "/vX") is treated as unversioned.
func moduleMajorFromGoMod(data []byte) uint64 {
	line := strings.TrimPrefix(strings.SplitN(string(data), "\n", 2)[0], "module ")
	line = strings.TrimLeft(line, " \t")
	modPath := strings.Fields(line)[0]
	if idx := strings.LastIndex(modPath, "/"); idx >= 0 {
		suffix := modPath[idx+1:]
		if strings.HasPrefix(suffix, "v") && len(suffix) > 1 {
			if n, err := strconv.ParseUint(suffix[1:], 10, 64); err == nil {
				return n
			}
		}
	}
	return 0
}

// ErrNoPackages is returned when monorel.toml declares no packages.
// At least one package is required: monorel has nothing to release
// otherwise.
var ErrNoPackages = errors.New("no packages declared")

// Validate checks invariants beyond what TOML parsing enforces.
// Returns the first violation encountered.
func (c *Config) Validate() error {
	if c.Provider.Owner == "" {
		return errors.New("provider.owner is required")
	}
	if c.Provider.Repo == "" {
		return errors.New("provider.repo is required")
	}
	if c.Provider.Name != "" && !IsKnownProvider(c.Provider.Name) {
		return fmt.Errorf("provider.name %q is not recognized (use one of: %v)",
			c.Provider.Name, KnownProviders)
	}
	if len(c.Packages) == 0 {
		return ErrNoPackages
	}
	// Detect duplicate tag prefixes; two packages mapping to the
	// same tag namespace would silently collide on release.
	prefixes := make(map[string]string)
	for _, name := range c.PackageNames() {
		pkg := c.Packages[name]
		if pkg.Path == "" {
			return fmt.Errorf("packages.%q: path is required", name)
		}
		if pkg.Changelog == "" {
			return fmt.Errorf("packages.%q: changelog is required", name)
		}
		if existing, taken := prefixes[pkg.TagPrefix]; taken {
			return fmt.Errorf("packages.%q and packages.%q share tag_prefix %q",
				existing, name, pkg.TagPrefix)
		}
		prefixes[pkg.TagPrefix] = name
	}
	return nil
}
