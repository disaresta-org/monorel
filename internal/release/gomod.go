package release

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/mod/modfile"

	"monorel.disaresta.com/config"
	"monorel.disaresta.com/plan"
)

// rewriteSubmoduleGoMods walks the released packages and cleans each
// one's go.mod for the published-tag state:
//
//  1. Drop replace directives whose target is a managed sibling
//     package AND whose source is a relative filesystem path. These
//     are dev-only "use the local checkout instead of the proxy"
//     directives that should never ship to the module proxy. External
//     (non-sibling) replaces are left intact.
//  2. Rewrite require lines for managed sibling packages to a real
//     version. Sub-modules hold each other's require version at a
//     placeholder pseudo-version (e.g.
//     v0.0.0-00010101000000-000000000000) during development; without
//     the rewrite, the placeholder ships to the proxy and downstream
//     consumers' `go mod tidy` returns 404 on the placeholder.
//
// "Managed sibling" means any package declared in monorel.toml. The
// version each sibling resolves to depends on whether it is part of
// the current release plan:
//
//   - In-plan siblings pin to the planned release version (the
//     version part of opts.Plan.Releases[i].Tag).
//   - Out-of-plan siblings (declared in opts.Config but not being
//     released right now) pin to their latest existing stable tag.
//     This lets a single-package release fix its own go.mod without
//     forcing the rest of the repo into the same release.
//
// Out-of-plan siblings with no existing tag (a freshly-registered
// package about to ship its first release elsewhere) are silently
// skipped: the rewriter has nothing to pin to, so the existing
// require line stays put.
//
// Stages each modified file via opts.Repo.Add. Idempotent: a go.mod
// that doesn't need changes isn't touched.
//
// Packages whose Path doesn't contain a go.mod (e.g. a pure-changelog
// package) are skipped.
//
// Called from applyStable AFTER CHANGELOG writes and BEFORE the
// consumed-changesets deletion so all the file changes land in the
// same release commit.
func rewriteSubmoduleGoMods(opts Options) error {
	siblings, err := buildSiblingMap(opts)
	if err != nil {
		return err
	}

	// Walk released packages' go.mod files and rewrite each one
	// using the combined sibling map. Self-references are skipped
	// explicitly so a package's own import path can't shadow a
	// sibling rewrite.
	for _, r := range opts.Plan.Releases {
		modPath := filepath.Join(opts.RepoDir, r.Config.Path, "go.mod")
		mf, err := readModFile(modPath)
		if err != nil {
			return err
		}
		if mf == nil {
			continue // no go.mod (pure-changelog package)
		}
		ownPath := mf.Module.Mod.Path
		changed := false

		for _, rep := range mf.Replace {
			if rep.Old.Path == ownPath {
				continue
			}
			if _, ok := siblings[rep.Old.Path]; !ok {
				continue
			}
			if !isRelativePath(rep.New.Path) {
				continue
			}
			if err := mf.DropReplace(rep.Old.Path, rep.Old.Version); err != nil {
				return fmt.Errorf("release: drop replace %s in %s: %w", rep.Old.Path, modPath, err)
			}
			changed = true
		}

		for _, req := range mf.Require {
			newVer, isSibling := siblings[req.Mod.Path]
			if !isSibling || newVer == "" {
				continue
			}
			if req.Mod.Path == ownPath {
				continue
			}
			if req.Mod.Version == newVer {
				continue
			}
			if err := mf.AddRequire(req.Mod.Path, newVer); err != nil {
				return fmt.Errorf("release: pin require %s in %s: %w", req.Mod.Path, modPath, err)
			}
			changed = true
		}

		if !changed {
			continue
		}

		mf.Cleanup()
		out, err := mf.Format()
		if err != nil {
			return fmt.Errorf("release: format %s: %w", modPath, err)
		}
		if err := os.WriteFile(modPath, out, 0o644); err != nil {
			return fmt.Errorf("release: write %s: %w", modPath, err)
		}
		rel := filepath.Join(r.Config.Path, "go.mod")
		if err := opts.Repo.Add(rel); err != nil {
			return fmt.Errorf("release: stage %s: %w", rel, err)
		}
	}
	return nil
}

// buildSiblingMap constructs the import-path → version map the
// rewriter consults. Every monorel-managed package contributes one
// entry; in-plan packages map to their planned version, out-of-plan
// packages map to their latest existing stable tag (or "" if none).
//
// Falls back to a plan-only map when opts.Config is nil. Preserves
// behavior for callers that haven't been updated to thread the
// config through.
func buildSiblingMap(opts Options) (map[string]string, error) {
	planned := make(map[string]string, len(opts.Plan.Releases))
	for _, r := range opts.Plan.Releases {
		ver := r.Tag
		if idx := strings.LastIndex(ver, "/v"); idx >= 0 {
			ver = ver[idx+1:]
		}
		planned[r.Name] = ver
	}

	siblings := make(map[string]string)

	// In-plan packages first.
	for _, r := range opts.Plan.Releases {
		modPath := filepath.Join(opts.RepoDir, r.Config.Path, "go.mod")
		mf, err := readModFile(modPath)
		if err != nil {
			return nil, err
		}
		if mf == nil {
			continue
		}
		siblings[mf.Module.Mod.Path] = planned[r.Name]
	}

	if opts.Config == nil {
		return siblings, nil
	}

	// Out-of-plan managed packages: load tags once, then walk every
	// managed package not in the plan, read its go.mod for the
	// import path, and look up its latest existing stable tag.
	if !hasOutOfPlanPackages(opts.Config.Packages, planned) {
		return siblings, nil
	}
	allTags, err := opts.Repo.ListTags("")
	if err != nil {
		return nil, fmt.Errorf("release: list tags for sibling lookup: %w", err)
	}
	for name, pkg := range opts.Config.Packages {
		if _, inPlan := planned[name]; inPlan {
			continue
		}
		modPath := filepath.Join(opts.RepoDir, pkg.Path, "go.mod")
		mf, err := readModFile(modPath)
		if err != nil {
			return nil, err
		}
		if mf == nil {
			continue
		}
		ver, ok := plan.LatestStableTagVersion(allTags, pkg)
		if !ok {
			// No existing tag for this managed package; record
			// the import path with an empty version so the
			// rewriter recognizes it as a managed sibling
			// (replace dropping still applies) but skips the
			// require pinning.
			siblings[mf.Module.Mod.Path] = ""
			continue
		}
		siblings[mf.Module.Mod.Path] = ver
	}
	return siblings, nil
}

// readModFile reads and parses the go.mod at path. Returns (nil, nil)
// if the file doesn't exist (the package has no go.mod, e.g. a
// pure-changelog package). On success the returned File is guaranteed
// to have a non-nil Module directive (parse errors out otherwise).
func readModFile(path string) (*modfile.File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("release: read %s: %w", path, err)
	}
	mf, err := modfile.Parse(path, data, nil)
	if err != nil {
		return nil, fmt.Errorf("release: parse %s: %w", path, err)
	}
	if mf.Module == nil {
		return nil, fmt.Errorf("release: %s has no module directive", path)
	}
	return mf, nil
}

// hasOutOfPlanPackages reports whether any package in pkgs is not
// listed in planned. Used to skip the ListTags call when every
// managed package is part of the current plan.
func hasOutOfPlanPackages(pkgs map[string]config.PackageConfig, planned map[string]string) bool {
	for name := range pkgs {
		if _, inPlan := planned[name]; !inPlan {
			return true
		}
	}
	return false
}

// isRelativePath reports whether s names a filesystem path relative
// to the package's go.mod directory. modfile's grammar says any
// replace target starting with "." (so ".", "./x", "..", "../x") is
// a filesystem path; absolute paths, remote module paths, etc. are
// left alone.
func isRelativePath(s string) bool {
	return strings.HasPrefix(s, ".")
}
