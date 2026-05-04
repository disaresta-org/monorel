package release

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// runOfflineTidy execs `go mod tidy` in modDir against the seeded
// local cache. The Go env is built from scratch (not inherited) so a
// developer's GOFLAGS / GOPROXY / GOWORK overrides can't leak in and
// produce non-deterministic behaviour:
//
//   - GOPROXY=off: never reach out to the public proxy. The seeded
//     cache supplies in-plan siblings; out-of-plan managed siblings
//     are expected to be in the developer's existing cache from
//     prior dev work (the pre-flight check confirms this).
//   - GOSUMDB=off: skip the public checksum DB. Required because
//     sumdb verification would 404 on freshly-released versions.
//   - GOWORK=off: ignore any go.work file in the worktree. With a
//     workspace active, tidy can resolve sibling paths through the
//     workspace's replace directives, bypassing our cache; turning
//     it off ensures every fetch goes through the cache path.
//   - GOFLAGS=: clear caller-set flags (GOFLAGS=-mod=vendor in the
//     environment would otherwise break tidy).
//   - GOTOOLCHAIN=local: don't auto-download a different toolchain.
//   - GOMODCACHE=<resolved>: pinned to the same path seedModuleCache
//     wrote into. On distributions where GOMODCACHE is derived from
//     GOPATH (e.g. golang:alpine has GOPATH=/go but no GOMODCACHE
//     env var), the restricted env strips GOPATH and the subprocess
//     would otherwise default to ~/go/pkg/mod (an empty cache while
//     seeds live at /go/pkg/mod). Pinning makes the seed and the
//     read use the same path.
//
// PATH, HOME, USER, TMPDIR, LANG, LC_* pass through so the toolchain
// itself can run.
func runOfflineTidy(modDir string) error {
	env, err := offlineTidyEnv()
	if err != nil {
		return err
	}
	cmd := exec.Command("go", "mod", "tidy")
	cmd.Dir = modDir
	cmd.Env = env

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("tidy in %s: %w\n%s\n\n"+
			"Hint: this typically means the local Go module cache is missing a "+
			"transitive dependency. Re-run after `go test ./...` (which populates "+
			"the cache), or run `go mod download all` from the repo root",
			modDir, err, out)
	}
	return nil
}

// offlineTidyEnv builds the explicit env slice for the tidy
// subprocess. Variables NOT in the inherited list are dropped; see
// runOfflineTidy's GoDoc for the rationale.
func offlineTidyEnv() ([]string, error) {
	inherit := []string{
		"PATH",
		"HOME",
		"USER",
		"TMPDIR",
		"LANG",
		"GOCACHE", // tidy may compile to discover deps; let it reuse the build cache.
	}
	env := make([]string, 0, len(inherit)+8)
	for _, k := range inherit {
		if v, ok := os.LookupEnv(k); ok {
			env = append(env, k+"="+v)
		}
	}
	// LC_* locale variables: pass through any that are set.
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "LC_") {
			env = append(env, e)
		}
	}
	// Resolve GOMODCACHE under the host's full env and pin it
	// explicitly. Without this, on systems that derive GOMODCACHE from
	// GOPATH (e.g. golang:alpine), the restricted-env subprocess
	// defaults to a different path than the seed wrote into.
	mc, err := goModCache()
	if err != nil {
		return nil, fmt.Errorf("offline tidy env: %w", err)
	}
	env = append(env, "GOMODCACHE="+mc)
	// Fixed values.
	env = append(env,
		"GOPROXY=off",
		"GOSUMDB=off",
		"GOWORK=off",
		"GOFLAGS=",
		"GOTOOLCHAIN=local",
	)
	return env, nil
}

// primeModuleCache populates the local module cache with the
// third-party deps modDir's go.mod transitively requires. Subsequent
// offline tidy with GOPROXY=off can resolve those deps from the
// cache without reaching out to the network.
//
// Uses the inherited GOPROXY (typically https://proxy.golang.org,direct)
// and GOSUMDB so go.sum hashes are verified during download. Does
// NOT mutate go.sum: `go mod download` reads go.sum, downloads the
// listed modules, and writes nothing. The download is bounded by the
// existing entries in go.sum: any module not already pinned wouldn't
// be downloaded (tidy adds pins later, after the in-plan siblings
// are seeded).
//
// inPlan is the set of module import paths being released in the
// current plan. Entries in go.sum whose module path matches a key
// in inPlan are skipped: those modules will be seeded by the caller's
// seedModuleCache step, not fetched from the proxy. Both full-zip
// entries and /go.mod-only entries are processed; /go.mod-only entries
// represent modules needed for graph resolution but not imported.
//
// If no go.sum exists in modDir, the function returns without running
// the download command. A missing go.sum means tidy hasn't yet
// recorded any third-party hashes, so there is nothing to pre-fill.
//
// PATH, HOME, USER, TMPDIR, LANG, LC_*, GOMODCACHE, GOCACHE, GOPROXY,
// GOSUMDB pass through.
func primeModuleCache(modDir string, inPlan map[string]string) error {
	goSumPath := filepath.Join(modDir, "go.sum")
	goSumBytes, err := os.ReadFile(goSumPath)
	if os.IsNotExist(err) {
		// Nothing pinned yet; no third-party deps to pre-fill.
		return nil
	} else if err != nil {
		return fmt.Errorf("prime module cache: read %s: %w", goSumPath, err)
	}

	// Collect module@version pairs from go.sum, excluding in-plan
	// modules (which will be seeded by the caller).
	// /go.mod-only entries (modules pruned from the build list but
	// needed for graph resolution) are included: strip the /go.mod
	// suffix to get the bare version, then deduplicate against any
	// full-zip entry for the same module@version.
	var targets []string
	seen := make(map[string]bool)
	for _, line := range strings.Split(string(goSumBytes), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		modPath := parts[0]
		modVer := parts[1]
		// Strip /go.mod suffix to get the bare version used as the
		// download target. Deduplication via `seen` ensures we don't
		// enqueue a module twice when both the /go.mod and full-zip
		// entries are present.
		modVer = strings.TrimSuffix(modVer, "/go.mod")
		// Skip in-plan modules; seedModuleCache handles those.
		if _, ok := inPlan[modPath]; ok {
			continue
		}
		key := modPath + "@" + modVer
		if seen[key] {
			continue
		}
		seen[key] = true
		targets = append(targets, key)
	}
	if len(targets) == 0 {
		return nil
	}

	env, err := primeCacheEnv()
	if err != nil {
		return err
	}
	args := append([]string{"mod", "download"}, targets...)
	cmd := exec.Command("go", args...)
	cmd.Dir = modDir
	cmd.Env = env

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("prime module cache in %s: %w\n%s\n\n"+
			"Hint: this typically means GOPROXY isn't reachable from this environment. "+
			"Confirm `GOPROXY` is a real proxy URL (e.g. https://proxy.golang.org,direct), "+
			"or set `GOPROXY=direct` to fetch straight from the source repo",
			modDir, err, out)
	}
	return nil
}

// primeCacheEnv builds the env slice for the prime-cache subprocess.
// Mirrors offlineTidyEnv's "scratch env, no caller GOFLAGS leak"
// shape, but inherits GOPROXY and GOSUMDB so download can fetch from
// the network. GOMODCACHE is pinned explicitly for the same reason
// runOfflineTidy pins it.
func primeCacheEnv() ([]string, error) {
	inherit := []string{
		"PATH",
		"HOME",
		"USER",
		"TMPDIR",
		"LANG",
		"GOCACHE",
		"GOPROXY",
		"GOSUMDB",
	}
	env := make([]string, 0, len(inherit)+8)
	for _, k := range inherit {
		if v, ok := os.LookupEnv(k); ok {
			env = append(env, k+"="+v)
		}
	}
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "LC_") {
			env = append(env, e)
		}
	}
	mc, err := goModCache()
	if err != nil {
		return nil, fmt.Errorf("prime cache env: %w", err)
	}
	env = append(env, "GOMODCACHE="+mc)
	env = append(env,
		"GOWORK=off",
		"GOFLAGS=",
		"GOTOOLCHAIN=local",
	)
	return env, nil
}
