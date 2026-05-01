// Package validate runs a fixed set of static checks against a
// monorel.toml + the surrounding repo state and reports findings.
//
// Unlike the other commands' single-error fail-fast loaders (config.Load
// returns the first violation; changeset.LoadAll bails on the first
// malformed file), validate is fault-tolerant by design: it surfaces
// every issue in one pass so authors fix them in one round-trip.
//
// Checks fall into four buckets:
//
//   - Schema: provider fields, package fields, no duplicate tag prefixes.
//     Delegated to config.Config.Validate().
//   - Filesystem: every package's Path exists, no two packages share a
//     Path, every Changelog's parent directory exists.
//   - Changesets: every .changeset/*.md parses cleanly and only names
//     packages that exist in monorel.toml.
//   - Tags (opt-in): the latest tag matching each package's prefix
//     parses as valid semver. Non-semver tags surface as warnings.
//
// validate intentionally does no network I/O and no mutation; it does
// not compute the release plan (that's monorel plan).
package validate

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"monorel.disaresta.com/changeset"
	"monorel.disaresta.com/config"
	"monorel.disaresta.com/semver"
)

// Severity classifies a finding. Errors fail the run; warnings only
// fail when the caller passes Strict=true.
type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
)

// Finding is a single validation issue. The Code is a stable
// identifier (used by tests, CI integrations, and the JSON output);
// the Message is the human-readable rendering.
type Finding struct {
	Severity Severity `json:"severity"`
	Code     string   `json:"code"`
	Message  string   `json:"message"`
	// Path is the file path the finding refers to, when applicable
	// (e.g. the changeset that failed to parse). Relative to the
	// repository root for changeset findings; absolute for config
	// findings.
	Path string `json:"path,omitempty"`
	// Package is the monorel.toml package key the finding refers to,
	// when applicable.
	Package string `json:"package,omitempty"`
}

// Inputs bundle the validate run's parameters. ConfigPath is required;
// everything else is optional with sensible defaults.
type Inputs struct {
	// ConfigPath is the path to monorel.toml. Need not be absolute;
	// validate resolves it.
	ConfigPath string

	// ChangesetDir is the directory containing .changeset/*.md files.
	// Empty defaults to <ConfigPath dir>/.changeset.
	ChangesetDir string

	// CheckTags enables the opt-in semver-validation pass over each
	// package's tag namespace.
	CheckTags bool

	// ListTags supplies the tag list for a given prefix when
	// CheckTags is true. The prefix is the value returned by
	// PackageConfig.FullTagPrefix() (e.g. "transports/zerolog/" or
	// "" for bare-tag root). A well-behaved implementation should
	// return only tags whose names begin with the prefix; validate
	// re-filters defensively so a misbehaving callback that returns
	// extra tags will not cross-pollinate findings.
	//
	// Stays a callback so the validate package doesn't pull in
	// internal/git. When CheckTags is true and ListTags is nil,
	// validate emits a single error finding and skips the tag pass.
	ListTags func(prefix string) ([]string, error)
}

// Run executes every applicable check in order and returns the
// aggregated findings. Findings are returned in a stable order:
// schema first, then filesystem (per package, lex order), then
// changesets (per file, lex order), then tags (per package, lex
// order).
//
// Run never panics on bad inputs; every error path produces a Finding
// instead. An empty return slice means the run is clean.
func Run(in Inputs) []Finding {
	var findings []Finding

	absConfig, err := filepath.Abs(in.ConfigPath)
	if err != nil {
		return []Finding{{
			Severity: SeverityError,
			Code:     "config_path_invalid",
			Message:  fmt.Sprintf("resolve --config: %v", err),
			Path:     in.ConfigPath,
		}}
	}

	cfg, err := config.Load(absConfig)
	if err != nil {
		// config.Load runs Validate() and returns the first
		// violation it sees (schema or parse). Surface as a single
		// finding; we can't proceed without a parsed config.
		return []Finding{{
			Severity: SeverityError,
			Code:     "config_invalid",
			Message:  err.Error(),
			Path:     absConfig,
		}}
	}

	repoDir := filepath.Dir(absConfig)
	findings = append(findings, validateFilesystem(cfg, repoDir)...)

	changesetDir := in.ChangesetDir
	if changesetDir == "" {
		changesetDir = filepath.Join(repoDir, ".changeset")
	}
	findings = append(findings, validateChangesets(cfg, changesetDir)...)

	if in.CheckTags {
		if in.ListTags == nil {
			findings = append(findings, Finding{
				Severity: SeverityError,
				Code:     "tag_check_misconfigured",
				Message:  "CheckTags is set but ListTags is nil",
			})
		} else {
			findings = append(findings, validateTags(cfg, in.ListTags)...)
		}
	}

	return findings
}

// HasErrors reports whether any finding has SeverityError. Useful for
// callers picking an exit code.
func HasErrors(findings []Finding) bool {
	for _, f := range findings {
		if f.Severity == SeverityError {
			return true
		}
	}
	return false
}

// HasWarnings reports whether any finding has SeverityWarning.
func HasWarnings(findings []Finding) bool {
	for _, f := range findings {
		if f.Severity == SeverityWarning {
			return true
		}
	}
	return false
}

// validateFilesystem checks each package's Path and Changelog against
// the actual on-disk layout. Repo-relative paths are resolved against
// repoDir.
func validateFilesystem(cfg *config.Config, repoDir string) []Finding {
	var findings []Finding
	pathOwner := make(map[string]string) // package.Path → first package using it

	for _, name := range cfg.PackageNames() {
		pkg := cfg.Packages[name]

		absPath := filepath.Join(repoDir, pkg.Path)
		info, err := os.Stat(absPath)
		switch {
		case err == nil && !info.IsDir():
			findings = append(findings, Finding{
				Severity: SeverityError,
				Code:     "path_not_dir",
				Message:  fmt.Sprintf("packages.%q.path %q is not a directory", name, pkg.Path),
				Package:  name,
				Path:     pkg.Path,
			})
		case err != nil:
			findings = append(findings, Finding{
				Severity: SeverityError,
				Code:     "path_missing",
				Message:  fmt.Sprintf("packages.%q.path %q does not exist", name, pkg.Path),
				Package:  name,
				Path:     pkg.Path,
			})
		}

		if first, taken := pathOwner[pkg.Path]; taken {
			findings = append(findings, Finding{
				Severity: SeverityError,
				Code:     "path_duplicate",
				Message:  fmt.Sprintf("packages.%q and packages.%q both have path %q", first, name, pkg.Path),
				Package:  name,
				Path:     pkg.Path,
			})
		} else {
			pathOwner[pkg.Path] = name
		}

		// Changelog's directory must exist; the file itself can be
		// absent (first release creates it).
		clDir := filepath.Dir(filepath.Join(repoDir, pkg.Changelog))
		if _, err := os.Stat(clDir); err != nil {
			findings = append(findings, Finding{
				Severity: SeverityError,
				Code:     "changelog_dir_missing",
				Message:  fmt.Sprintf("packages.%q.changelog %q: parent directory does not exist", name, pkg.Changelog),
				Package:  name,
				Path:     pkg.Changelog,
			})
		}
	}
	return findings
}

// validateChangesets walks dir and runs changeset.Parse on every
// .md file that isn't a reserved name. Parse errors are reported
// per file and the validator continues. Package keys in each
// changeset's frontmatter are checked against the config.
//
// A missing changeset directory is fine: it just means no pending
// releases. An unreadable directory (permission errors etc.) is an
// error since the user clearly intended one to exist.
func validateChangesets(cfg *config.Config, dir string) []Finding {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return []Finding{{
			Severity: SeverityError,
			Code:     "changeset_dir_unreadable",
			Message:  fmt.Sprintf("read changeset directory: %v", err),
			Path:     dir,
		}}
	}

	known := make(map[string]struct{}, len(cfg.Packages))
	for k := range cfg.Packages {
		known[k] = struct{}{}
	}

	// os.ReadDir returns entries sorted by name (Go 1.16+), so the
	// emit order below is already stable without an explicit sort.
	var findings []Finding
	for _, entry := range entries {
		if entry.IsDir() || changeset.IsReserved(entry.Name()) {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		f, err := os.Open(path)
		if err != nil {
			findings = append(findings, Finding{
				Severity: SeverityError,
				Code:     "changeset_unreadable",
				Message:  fmt.Sprintf("open %s: %v", entry.Name(), err),
				Path:     path,
			})
			continue
		}
		cs, err := changeset.Parse(f, strings.TrimSuffix(entry.Name(), ".md"))
		f.Close()
		if err != nil {
			findings = append(findings, Finding{
				Severity: SeverityError,
				Code:     "changeset_parse_failed",
				Message:  fmt.Sprintf("parse %s: %v", entry.Name(), err),
				Path:     path,
			})
			continue
		}
		for _, pkg := range cs.PackageNames() {
			if _, ok := known[pkg]; !ok {
				findings = append(findings, Finding{
					Severity: SeverityError,
					Code:     "changeset_unknown_package",
					Message:  fmt.Sprintf("changeset references package %q which is not declared in monorel.toml", pkg),
					Path:     path,
					Package:  pkg,
				})
			}
		}
	}
	return findings
}

// validateTags asks the caller for each package's tag list (filtered
// by the package's prefix) and checks that every returned tag has a
// parseable semver version. Non-semver tags surface as warnings.
//
// Per-package querying lets a git-backed implementation translate the
// prefix into a `git tag --list <prefix>v*`-style filter and avoid
// the O(packages * total_tags) cross-product of the previous design.
// It also removes the need for an empty-prefix root special-case: a
// well-behaved ListTags("") returns only bare-vX.Y.Z tags rather
// than every sub-module tag in the repo.
func validateTags(cfg *config.Config, listTags func(prefix string) ([]string, error)) []Finding {
	var findings []Finding
	for _, name := range cfg.PackageNames() {
		pkg := cfg.Packages[name]
		prefix := pkg.FullTagPrefix()

		tags, err := listTags(prefix)
		if err != nil {
			findings = append(findings, Finding{
				Severity: SeverityError,
				Code:     "tag_list_failed",
				Message:  fmt.Sprintf("list tags for package %q (prefix %q): %v", name, prefix, err),
				Package:  name,
			})
			continue
		}

		for _, tag := range tags {
			// Defensive: a misbehaving caller that returns tags
			// outside the prefix would otherwise produce nonsense
			// findings against this package's namespace.
			if !strings.HasPrefix(tag, prefix) {
				continue
			}
			version := strings.TrimPrefix(tag, prefix)
			if !semver.IsValid(version) {
				findings = append(findings, Finding{
					Severity: SeverityWarning,
					Code:     "tag_not_semver",
					Message:  fmt.Sprintf("tag %q (package %q): version part %q is not valid semver", tag, name, version),
					Package:  name,
				})
			}
		}
	}
	return findings
}
