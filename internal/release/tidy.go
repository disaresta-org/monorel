package release

import (
	"fmt"
	"os"
	"os/exec"
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
//
// PATH, HOME, USER, TMPDIR, LANG, LC_*, GOMODCACHE pass through so
// the toolchain itself can run.
func runOfflineTidy(modDir string) error {
	cmd := exec.Command("go", "mod", "tidy")
	cmd.Dir = modDir
	cmd.Env = offlineTidyEnv()

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
// subprocess. Variables NOT in the inherited list are dropped — see
// runOfflineTidy's GoDoc for the rationale.
func offlineTidyEnv() []string {
	inherit := []string{
		"PATH",
		"HOME",
		"USER",
		"TMPDIR",
		"LANG",
		"GOMODCACHE",
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
	// Fixed values.
	env = append(env,
		"GOPROXY=off",
		"GOSUMDB=off",
		"GOWORK=off",
		"GOFLAGS=",
		"GOTOOLCHAIN=local",
	)
	return env
}
