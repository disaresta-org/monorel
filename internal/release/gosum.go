package release

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
// stays clean. The maintainer reverts with `git checkout` and
// retries.
//
// Skipped when:
//
//   - The plan has no releases (caller's invariant; defensive).
//   - opts.PreState != nil (pre-release mode; applyPrerelease
//     doesn't rewrite go.mod, so go.sum can't drift).
//
// Called from applyStable AFTER rewriteSubmoduleGoMods AND AFTER
// the consumed-changesets deletion. The deletion is upstream of the
// seed step so the working tree already has its final commit shape;
// otherwise the seed's hash wouldn't match what `git archive` of
// the published tag produces. See
// docs/superpowers/specs/2026-05-03-cacheseed-fixpoint-and-release-pipeline-fixes-design.md.
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

	// 4. Prime the module cache with third-party deps each affected
	//    sub-module transitively requires. Tidy under GOPROXY=off
	//    can only resolve from the cache; managed siblings are
	//    seeded by the loop below, but third-party deps depend on
	//    the cache being warm. Dev cache is usually warm from prior
	//    builds; fresh CI runner cache isn't. Populate the cache
	//    explicitly so the offline tidy that follows resolves
	//    every transitive dep from the cache.
	for _, sub := range affected {
		modDir := filepath.Join(opts.RepoDir, sub)
		if err := primeModuleCache(modDir, inPlan); err != nil {
			return err
		}
	}

	// 5. Iterate seed-and-tidy to fixpoint. Each iteration zips
	//    every in-plan module's working tree, seeds the cache with
	//    the resulting hashes, and runs offline tidy in each
	//    affected sub-module. If any sub-module's go.sum was
	//    mutated by the iteration's tidy, the working tree changed
	//    too, so the next iteration re-seeds and re-tidies. The
	//    loop converges when no go.sum is mutated; at that point
	//    every recorded h1: matches the seeded hash, which matches
	//    the working-tree hash, which is what `git archive` of the
	//    published tag will produce.
	//
	//    Iteration cap defends against cycles/non-determinism. The
	//    practical bound is the depth of the in-plan sibling dep
	//    graph; a typical monorepo converges in 1-3 iterations.
	const maxTidyIterations = 10
	var seeded []seededEntry
	defer func() { clearSeededEntries(seeded) }()

	var lastDiffs map[string][2][]byte
	for i := 0; i < maxTidyIterations; i++ {
		clearSeededEntries(seeded)
		seeded, err = seedModuleCache(opts)
		if err != nil {
			return err
		}

		before, err := readGoSums(opts.RepoDir, affected)
		if err != nil {
			return err
		}

		// Remove existing go.sum files before each tidy pass so the
		// re-seed's new hashes take effect without triggering a
		// SECURITY ERROR from Go's local go.sum verification. Tidy
		// recreates go.sum from the seeded cache entries; the fixpoint
		// check then compares the freshly-written go.sum against the
		// pre-deletion snapshot.
		for _, sub := range affected {
			goSumPath := filepath.Join(opts.RepoDir, sub, "go.sum")
			if err := os.Remove(goSumPath); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("tidy: remove %s before re-tidy: %w", goSumPath, err)
			}
		}

		for _, sub := range affected {
			modDir := filepath.Join(opts.RepoDir, sub)
			if err := runOfflineTidy(modDir); err != nil {
				return err
			}
		}

		if tidyHook != nil {
			if err := tidyHook(i); err != nil {
				return err
			}
		}

		after, err := readGoSums(opts.RepoDir, affected)
		if err != nil {
			return err
		}

		if !goSumsChanged(before, after) {
			// Fixpoint reached.
			return stageAffected(opts, affected)
		}

		// Capture the diff for diagnostic in case fixpoint never
		// converges. Overwritten each iteration; we only emit the
		// final iteration's diff if we exit via the cap.
		lastDiffs = make(map[string][2][]byte, len(before))
		for k := range before {
			lastDiffs[k] = [2][]byte{before[k], after[k]}
		}
	}

	return &errFixpointNotReached{
		iterations: maxTidyIterations,
		finalDiffs: lastDiffs,
	}
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

// preflightOutOfPlanCache verifies that no affected sub-module's
// require of an out-of-plan managed sibling is left at its dev
// placeholder. The check runs after rewriteSubmoduleGoMods, which
// pins managed siblings to a real version: in-plan siblings to the
// planned version, tagged out-of-plan siblings to their latest
// existing tag. A placeholder surviving here means the sibling has no
// existing tag, so it cannot be pinned and the offline tidy
// (GOPROXY=off) would fail on it. Surfacing a precise error saves the
// maintainer from debugging tidy's generic "missing module" output.
func preflightOutOfPlanCache(opts Options, affected []string, inPlan map[string]string) error {
	if opts.Config == nil {
		return nil
	}
	managed := managedImportPaths(opts)

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
			if !isPlaceholderVersion(req.Mod.Version) {
				continue // real pinned version; offline tidy can use it
			}
			// A placeholder surviving to this point means the
			// sibling has no tag for the rewriter to pin.
			return fmt.Errorf(
				"tidy pre-flight failed for %s\n\n"+
					"The release would resolve %s (a monorel-managed sibling not in the "+
					"current release plan), but that sibling has no existing tag, so its "+
					"require cannot be pinned to a real version. Include %s in this "+
					"release plan, or release it first in a separate cut",
				sub, req.Mod.Path, req.Mod.Path)
		}
	}
	return nil
}

// isPlaceholderVersion reports whether v is the dev-only pseudo-version
// placeholder that sub-modules use in go.mod requires until a release
// rewrites them to a real version (the canonical form is
// v0.0.0-00010101000000-000000000000, or the module's major+".0"
// with the same zero timestamp for higher majors, e.g. v3.0.0-...).
// A placeholder's timestamp predates every real commit, so the
// version exists nowhere on the proxy and in no module cache; it can
// only resolve through a local replace.
func isPlaceholderVersion(v string) bool {
	return strings.HasSuffix(v, "-00010101000000-000000000000") &&
		strings.HasPrefix(v, "v")
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

// readGoSums reads the go.sum bytes for every sub-module path in
// affected. Missing files are recorded as nil byte slices (a
// sub-module that hasn't yet been tidied may not have a go.sum
// file). Used by [tidySubmoduleGoSums]'s fixpoint loop to detect
// whether the most recent tidy iteration mutated any go.sum.
func readGoSums(repoDir string, affected []string) (map[string][]byte, error) {
	out := make(map[string][]byte, len(affected))
	for _, sub := range affected {
		path := filepath.Join(repoDir, sub, "go.sum")
		b, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				out[sub] = nil
				continue
			}
			return nil, fmt.Errorf("readGoSums: %s: %w", path, err)
		}
		out[sub] = b
	}
	return out, nil
}

// goSumsChanged reports whether before and after differ on any key.
// Caller passes snapshots from [readGoSums]; both maps must have the
// same key set.
func goSumsChanged(before, after map[string][]byte) bool {
	for k, b := range before {
		if !bytes.Equal(b, after[k]) {
			return true
		}
	}
	return false
}

// stageAffected stages each affected sub-module's go.mod and go.sum
// in the repo. Idempotent: missing files are skipped; non-Stat
// failures bubble up. Used by [tidySubmoduleGoSums] after the
// fixpoint loop converges.
func stageAffected(opts Options, affected []string) error {
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

// errFixpointNotReached is returned by [tidySubmoduleGoSums] when
// the seed-and-tidy loop fails to converge within maxTidyIterations.
// The error carries the per-iteration go.sum diffs as a diagnostic
// payload so the maintainer can see exactly which sub-module's
// go.sum kept changing across iterations. This typically indicates
// a monorel bug (cycle, non-determinism); the message hints the
// maintainer to file an issue.
type errFixpointNotReached struct {
	iterations int
	finalDiffs map[string][2][]byte // per sub-module: [0]=before, [1]=after for the last iteration
}

func (e *errFixpointNotReached) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "tidy did not converge after %d iterations; this indicates a monorel bug.\n",
		e.iterations)
	fmt.Fprintf(&b, "Please file an issue at https://github.com/disaresta-org/monorel/issues with the following diffs:\n\n")
	for sub, ba := range e.finalDiffs {
		if bytes.Equal(ba[0], ba[1]) {
			continue
		}
		fmt.Fprintf(&b, "==> %s/go.sum (last iteration's diff):\n", sub)
		fmt.Fprintf(&b, "BEFORE:\n%s\n", ba[0])
		fmt.Fprintf(&b, "AFTER:\n%s\n", ba[1])
	}
	return b.String()
}

// tidyHook is a per-iteration test hook used by the fixpoint loop
// to inject non-determinism (or any other behavior) for testing
// errFixpointNotReached. Production code never sets it; the only
// caller is the gosum_test.go suite. nil means "no-op."
var tidyHook func(iteration int) error
