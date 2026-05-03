package release

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/mod/module"
)

// tidySubmoduleGoSums runs `go mod tidy` (offline) in every released
// sub-module that requires an in-plan sibling, so the release commit
// includes correct go.sum hashes (and any new // indirect lines) for
// the freshly-pinned versions. Without this pass, consumers pulling
// the release see a dirty `go mod tidy` diff in main.
//
// Two-pass orchestration: every tidy runs first (working tree mutated
// as tidy writes its output); only after every tidy succeeds does the
// orchestrator stage the changes via opts.Repo.Add. On failure, the
// working tree shows dirty go.mod / go.sum files but the git index
// stays clean — the maintainer reverts with `git checkout` and
// retries.
//
// Skipped when:
//
//   - The plan has no releases (caller's invariant; defensive).
//   - opts.PreState != nil (pre-release mode; applyPrerelease
//     doesn't rewrite go.mod, so go.sum can't drift).
//
// Called from applyStable AFTER rewriteSubmoduleGoMods and BEFORE
// the consumed-changesets deletion so all the file changes land in
// the same release commit.
func tidySubmoduleGoSums(opts Options) error {
	if opts.PreState != nil {
		return nil
	}
	if opts.Plan == nil || len(opts.Plan.Releases) == 0 {
		return nil
	}

	// 1. Build the in-plan module set so we can detect sibling
	//    requires in each sub-module's go.mod.
	inPlan, err := inPlanSiblings(opts)
	if err != nil {
		return err
	}

	// 2. Determine which sub-modules to tidy (skip ones with no
	//    in-plan sibling requires; nothing for tidy to add).
	affected, err := affectedSubmodules(opts, inPlan)
	if err != nil {
		return err
	}
	if len(affected) == 0 {
		return nil
	}

	// 3. Pre-flight: confirm out-of-plan managed siblings used by
	//    the affected sub-modules are in the developer's cache.
	if err := preflightOutOfPlanCache(opts, affected, inPlan); err != nil {
		return err
	}

	// 4. Seed the cache with in-plan releases.
	cleanup, err := seedModuleCache(opts)
	defer cleanup()
	if err != nil {
		return err
	}

	// 5. Run tidy in each affected sub-module. Hard-fail on the
	//    first failure; staging happens only after all succeed.
	for _, sub := range affected {
		modDir := filepath.Join(opts.RepoDir, sub)
		if err := runOfflineTidy(modDir); err != nil {
			return err
		}
	}

	// 6. Stage every (potentially) modified go.mod / go.sum.
	for _, sub := range affected {
		for _, name := range []string{"go.mod", "go.sum"} {
			rel := filepath.Join(sub, name)
			abs := filepath.Join(opts.RepoDir, rel)
			if _, err := os.Stat(abs); os.IsNotExist(err) {
				continue
			} else if err != nil {
				return fmt.Errorf("tidy: stat %s: %w", abs, err)
			}
			if err := opts.Repo.Add(rel); err != nil {
				return fmt.Errorf("tidy: stage %s: %w", rel, err)
			}
		}
	}
	return nil
}

// inPlanSiblings returns the set of import paths being released in
// this run, mapped to their planned versions. The map's presence
// (not value) is what gosum-orchestration cares about.
func inPlanSiblings(opts Options) (map[string]string, error) {
	out := make(map[string]string, len(opts.Plan.Releases))
	for _, r := range opts.Plan.Releases {
		modPath := filepath.Join(opts.RepoDir, r.Config.Path, "go.mod")
		mf, err := readModFile(modPath)
		if err != nil {
			return nil, fmt.Errorf("tidy: %w", err)
		}
		if mf == nil {
			continue
		}
		out[mf.Module.Mod.Path] = tagVersion(r.Tag)
	}
	return out, nil
}

// affectedSubmodules returns the relative paths of released
// sub-modules whose go.mod requires at least one in-plan sibling.
// Sub-modules without a go.mod and sub-modules that don't require
// any in-plan sibling are skipped (they have nothing for tidy to
// touch).
func affectedSubmodules(opts Options, inPlan map[string]string) ([]string, error) {
	var out []string
	for _, r := range opts.Plan.Releases {
		modPath := filepath.Join(opts.RepoDir, r.Config.Path, "go.mod")
		mf, err := readModFile(modPath)
		if err != nil {
			return nil, fmt.Errorf("tidy: %w", err)
		}
		if mf == nil {
			continue
		}
		ownPath := mf.Module.Mod.Path
		for _, req := range mf.Require {
			if req.Mod.Path == ownPath {
				continue
			}
			if _, ok := inPlan[req.Mod.Path]; ok {
				out = append(out, r.Config.Path)
				break
			}
		}
	}
	return out, nil
}

// preflightOutOfPlanCache verifies that any managed-but-not-in-plan
// sibling required by an affected sub-module already has a cache
// entry. The smarter rewriter (#44) pins these out-of-plan siblings
// to their existing tag; tidy with GOPROXY=off needs them present in
// the local cache. Surfacing a precise error here saves the
// maintainer from debugging tidy's generic "missing module" output.
func preflightOutOfPlanCache(opts Options, affected []string, inPlan map[string]string) error {
	if opts.Config == nil {
		return nil
	}
	managed := managedImportPaths(opts)
	mc, err := goModCache()
	if err != nil {
		return err
	}

	for _, sub := range affected {
		modPath := filepath.Join(opts.RepoDir, sub, "go.mod")
		mf, err := readModFile(modPath)
		if err != nil {
			return fmt.Errorf("tidy: pre-flight: %w", err)
		}
		if mf == nil {
			continue
		}
		ownPath := mf.Module.Mod.Path

		for _, req := range mf.Require {
			if req.Mod.Path == ownPath {
				continue
			}
			if !managed[req.Mod.Path] {
				continue
			}
			if _, ok := inPlan[req.Mod.Path]; ok {
				continue // we'll seed it
			}
			// Out-of-plan managed sibling. Cache entry must exist.
			escPath, err := module.EscapePath(req.Mod.Path)
			if err != nil {
				return fmt.Errorf("tidy: pre-flight: escape %s: %w", req.Mod.Path, err)
			}
			escVer, err := module.EscapeVersion(req.Mod.Version)
			if err != nil {
				return fmt.Errorf("tidy: pre-flight: escape %s: %w", req.Mod.Version, err)
			}
			info := filepath.Join(mc, "cache", "download", escPath, "@v", escVer+".info")
			if _, err := os.Stat(info); os.IsNotExist(err) {
				return fmt.Errorf(
					"tidy pre-flight failed for %s\n\n"+
						"The release would resolve %s %s (a monorel-managed package not in the "+
						"current release plan), but its cache entry is missing. Run "+
						"`go mod download %s@%s` to populate the cache, or "+
						"`go mod download all` from the repo root, then retry",
					sub, req.Mod.Path, req.Mod.Version, req.Mod.Path, req.Mod.Version)
			} else if err != nil {
				return fmt.Errorf("tidy: pre-flight: stat %s: %w", info, err)
			}
		}
	}
	return nil
}

// managedImportPaths returns the set of import paths declared in
// monorel.toml's [packages] table. Built by reading each package's
// go.mod for its module directive, since the toml key is a
// repo-relative name and the import path may differ.
func managedImportPaths(opts Options) map[string]bool {
	out := make(map[string]bool, len(opts.Config.Packages))
	for _, pkg := range opts.Config.Packages {
		modPath := filepath.Join(opts.RepoDir, pkg.Path, "go.mod")
		mf, err := readModFile(modPath)
		if err != nil || mf == nil {
			continue
		}
		out[mf.Module.Mod.Path] = true
	}
	return out
}
