package release

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/mod/modfile"
)

// rewriteSubmoduleGoMods walks the released packages and cleans each
// one's go.mod for the published-tag state:
//
//  1. Drop replace directives whose target is a sibling package (one
//     of the others in the same release plan) AND whose source is a
//     relative filesystem path. These are dev-only "use the local
//     checkout instead of the proxy" directives that should never
//     ship to the module proxy. External (non-sibling) replaces are
//     left intact.
//  2. Rewrite require lines for sibling packages to the planned
//     release version. Sub-modules hold each other's require version
//     at a placeholder pseudo-version (e.g.
//     v0.0.0-00010101000000-000000000000) during development;
//     without the rewrite, the
//     placeholder ships to the proxy and downstream consumers'
//     `go mod tidy` returns 404 on the placeholder.
//
// Stages each modified file via opts.Repo.Add. Idempotent: a go.mod
// that doesn't need changes isn't touched.
//
// Packages whose Path doesn't contain a go.mod (e.g. the main module
// rooted at the repo root, or a pure-changelog package) are skipped.
//
// Called from applyStable AFTER CHANGELOG writes and BEFORE the
// consumed-changesets deletion so all the file changes land in the
// same release commit.
func rewriteSubmoduleGoMods(opts Options) error {
	// First pass: read every released package's go.mod and build a
	// map from import path (module directive) to planned release
	// version. The planned version is the version part of the
	// release tag (e.g. tag "transports/foo/v2.0.0" -> "v2.0.0").
	// Packages without a go.mod (no sibling-deps to rewrite) get
	// an empty entry, which is skipped on the rewrite pass.
	type releasedPkg struct {
		importPath string
		version    string
		modPath    string // absolute filesystem path
		modFile    *modfile.File
	}
	pkgs := make([]releasedPkg, len(opts.Plan.Releases))
	siblings := make(map[string]string, len(opts.Plan.Releases))
	for i, r := range opts.Plan.Releases {
		modPath := filepath.Join(opts.RepoDir, r.Config.Path, "go.mod")
		data, err := os.ReadFile(modPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("release: read %s: %w", modPath, err)
		}
		mf, err := modfile.Parse(modPath, data, nil)
		if err != nil {
			return fmt.Errorf("release: parse %s: %w", modPath, err)
		}
		if mf.Module == nil {
			return fmt.Errorf("release: %s has no module directive", modPath)
		}
		// Extract the version part of the tag (strip the prefix).
		// Tag shape: "<tag_prefix>/v<X.Y.Z>" or "v<X.Y.Z>" for the
		// root module.
		ver := r.Tag
		if idx := strings.LastIndex(ver, "/v"); idx >= 0 {
			ver = ver[idx+1:]
		}
		pkgs[i] = releasedPkg{
			importPath: mf.Module.Mod.Path,
			version:    ver,
			modPath:    modPath,
			modFile:    mf,
		}
		siblings[mf.Module.Mod.Path] = ver
	}

	// Second pass: rewrite each released package's go.mod using the
	// sibling map. Self-references are skipped explicitly so a
	// package's own import path can't shadow a sibling rewrite.
	for i, p := range pkgs {
		if p.modFile == nil {
			continue
		}
		mf := p.modFile
		changed := false

		// Strip dev-only replace directives.
		for _, rep := range mf.Replace {
			if rep.Old.Path == p.importPath {
				continue // self-replace; weird but leave alone
			}
			if _, ok := siblings[rep.Old.Path]; !ok {
				continue
			}
			if !isRelativePath(rep.New.Path) {
				continue
			}
			if err := mf.DropReplace(rep.Old.Path, rep.Old.Version); err != nil {
				return fmt.Errorf("release: drop replace %s in %s: %w", rep.Old.Path, p.modPath, err)
			}
			changed = true
		}

		// Pin sibling require versions.
		for _, req := range mf.Require {
			newVer, isSibling := siblings[req.Mod.Path]
			if !isSibling {
				continue
			}
			if req.Mod.Path == p.importPath {
				continue
			}
			if req.Mod.Version == newVer {
				continue
			}
			if err := mf.AddRequire(req.Mod.Path, newVer); err != nil {
				return fmt.Errorf("release: pin require %s in %s: %w", req.Mod.Path, p.modPath, err)
			}
			changed = true
		}

		if !changed {
			continue
		}

		mf.Cleanup()
		out, err := mf.Format()
		if err != nil {
			return fmt.Errorf("release: format %s: %w", p.modPath, err)
		}
		if err := os.WriteFile(p.modPath, out, 0o644); err != nil {
			return fmt.Errorf("release: write %s: %w", p.modPath, err)
		}
		rel := filepath.Join(opts.Plan.Releases[i].Config.Path, "go.mod")
		if err := opts.Repo.Add(rel); err != nil {
			return fmt.Errorf("release: stage %s: %w", rel, err)
		}
	}
	return nil
}

// isRelativePath reports whether s names a filesystem path relative
// to the package's go.mod directory. modfile's grammar says any
// replace target starting with "." (so ".", "./x", "..", "../x") is
// a filesystem path; absolute paths, remote module paths, etc. are
// left alone.
func isRelativePath(s string) bool {
	return strings.HasPrefix(s, ".")
}
