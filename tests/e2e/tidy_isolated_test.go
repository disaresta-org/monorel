//go:build e2e_tidy

// Package e2e_tidy exercises the multi-module + offline `go mod tidy`
// path from `monorel apply` against a clean Go module cache, the
// shape every CI runner sees on first run.
//
// The default `tests/e2e/` suite runs against the developer's local
// GOMODCACHE, which has accumulated state from prior monorel dev
// runs and accidentally masked a real bug: GOPATH-derived GOMODCACHE
// defaults (e.g. inside `golang:alpine` images) caused the tidy
// subprocess to read from a different path than the seed-cache
// step wrote into, surfacing as `module lookup disabled by
// GOPROXY=off`. The Forgejo lifecycle scenarios don't surface this
// because they share the host's cache (so the host's GOPATH-default
// matches at both ends).
//
// This test pins the contract by running `monorel apply` inside
// `golang:1.26-alpine` (the same image `ghcr.io/disaresta-org/monorel`
// is built from), where GOMODCACHE is empty and GOPATH is /go.
//
// Build tag `e2e_tidy` keeps it out of the default `go test ./...`
// run; CI invokes it with `go test -tags=e2e_tidy ./tests/e2e/`.
package e2e_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const tidyImage = "golang:1.26-alpine"

// TestApply_MultiModule_CleanCache pins the contract that
// `monorel apply` succeeds end-to-end on a clean Go module cache for
// a multi-module repo with sibling `require`s. Runs `monorel apply`
// inside `golang:1.26-alpine` so GOMODCACHE is empty (the shape every
// CI runner sees on first run); the developer's local cache happens
// to mask the failure on a workstation.
//
// Regression coverage for the GOMODCACHE-divergence bug fixed in
// internal/release/tidy.go (offlineTidyEnv now pins GOMODCACHE
// explicitly so the tidy subprocess and the cache-seed step agree on
// the cache path even when GOPATH-derived defaults would diverge).
func TestApply_MultiModule_CleanCache(t *testing.T) {
	ctx := context.Background()

	// Build a static linux/amd64 monorel binary from the current
	// source tree. The container will exec it directly (no Go
	// toolchain layering needed; alpine ships its own).
	binDir := t.TempDir()
	binPath := filepath.Join(binDir, "monorel")
	build := exec.Command("go", "build", "-o", binPath, "monorel.disaresta.com/cmd/monorel")
	build.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS=linux", "GOARCH=amd64")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build monorel: %v\n%s", err, out)
	}

	// Build the multi-module scaffold mirroring helpers_test.go's
	// ScaffoldMultiModule: root + transports/foo + plugins/bar,
	// each sub-module with a `replace example.com/widget => ../..`
	// and `import _ "example.com/widget"` in its source.
	//
	// Use MkdirTemp + manual cleanup instead of t.TempDir(): the
	// container runs as root and writes git refs the host user can't
	// remove. Manual cleanup runs `chmod -R` first inside the
	// container before the host removes the directory tree.
	repoDir, err := os.MkdirTemp("", "monorel-tidy-")
	if err != nil {
		t.Fatalf("mkdir temp: %v", err)
	}
	// Register host-side removal immediately so an early failure
	// (scaffold writes, container start) doesn't leak repoDir. The
	// happy-path defer below chmods inside the container first so
	// this RemoveAll has writable trees to walk.
	t.Cleanup(func() { _ = os.RemoveAll(repoDir) })
	scaffoldMultiModuleForTidy(t, repoDir)

	// Spin up an alpine container with the repo + binary mounted.
	// `golang:1.26-alpine` ships its own Go toolchain at /usr/local/go.
	req := testcontainers.ContainerRequest{
		Image: tidyImage,
		Cmd:   []string{"sleep", "infinity"},
		Mounts: testcontainers.ContainerMounts{
			testcontainers.BindMount(repoDir, "/work"),
			testcontainers.BindMount(binPath, "/usr/local/bin/monorel"),
		},
		WorkingDir: "/work",
		WaitingFor: wait.ForExec([]string{"go", "version"}).WithExitCodeMatcher(func(c int) bool { return c == 0 }).WithStartupTimeout(60 * time.Second),
	}
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("start container: %v", err)
	}
	defer func() {
		// Inside the container is root; it created repoDir/.git contents
		// that aren't writable by the host user. Make everything host-
		// removable before terminating the container so the host-side
		// RemoveAll registered above has writable trees to walk.
		_, _, _ = container.Exec(ctx, []string{"sh", "-c", "chmod -R 0777 /work || true"})
		_ = container.Terminate(ctx)
	}()

	// Install git (golang:alpine doesn't ship it) and init the repo.
	// monorel apply needs a clean working tree, so the scaffold has to
	// be committed before invoking apply.
	for _, args := range [][]string{
		{"apk", "add", "--no-cache", "git"},
		{"git", "config", "--global", "user.email", "t@t"},
		{"git", "config", "--global", "user.name", "t"},
		{"git", "config", "--global", "init.defaultBranch", "main"},
		{"git", "config", "--global", "--add", "safe.directory", "/work"},
		{"sh", "-c", "cd /work && git init -q && git add -A && git commit -qm init"},
	} {
		code, reader, err := container.Exec(ctx, args)
		if err != nil || code != 0 {
			out, _ := readAll(reader)
			t.Fatalf("setup `%s`: code=%d err=%v\n%s", strings.Join(args, " "), code, err, out)
		}
	}

	// Run monorel apply.
	code, reader, err := container.Exec(ctx, []string{"sh", "-c", "cd /work && monorel apply"})
	if err != nil {
		t.Fatalf("exec monorel apply: %v", err)
	}
	out, _ := readAll(reader)

	if code != 0 {
		t.Fatalf("monorel apply exited %d on a clean cache (multi-module + offline tidy regression):\n%s",
			code, out)
	}

	// Verify the apply produced the expected post-state.
	for _, sub := range []string{"transports/foo", "plugins/bar"} {
		body := readFile(t, filepath.Join(repoDir, sub, "go.mod"))
		if strings.Contains(body, "replace example.com/widget") {
			t.Errorf("%s/go.mod still has replace directive after apply:\n%s", sub, body)
		}
		if !strings.Contains(body, "require example.com/widget v0.1.0") {
			t.Errorf("%s/go.mod missing pinned require for v0.1.0:\n%s", sub, body)
		}
		if _, err := os.Stat(filepath.Join(repoDir, sub, "go.sum")); err != nil {
			t.Errorf("%s/go.sum not created by tidy: %v", sub, err)
		}
	}
}

// scaffoldMultiModuleForTidy writes a 3-module scaffold (root +
// transports/foo + plugins/bar) into dir. Mirrors helpers_test.go's
// ScaffoldMultiModule but writes everything under a single dir
// (caller's t.TempDir()) and uses `example.com/widget` as the
// canonical module path so the test exercises the same path the
// GitLab live test exercised.
func scaffoldMultiModuleForTidy(t *testing.T, dir string) {
	t.Helper()
	mustWrite(t, filepath.Join(dir, "go.mod"), "module example.com/widget\n\ngo 1.25\n")
	mustWrite(t, filepath.Join(dir, "widget.go"), "package widget\n")
	mustWrite(t, filepath.Join(dir, "CHANGELOG.md"), "# Changelog\n\n## [Unreleased]\n\n")

	for _, sub := range []string{"transports/foo", "plugins/bar"} {
		_ = os.MkdirAll(filepath.Join(dir, sub), 0o755)
		pkg := filepath.Base(sub)
		mustWrite(t, filepath.Join(dir, sub, "go.mod"),
			"module example.com/widget/"+sub+"\n\n"+
				"go 1.25\n\n"+
				"require example.com/widget v0.0.0-00010101000000-000000000000\n\n"+
				"replace example.com/widget => ../..\n")
		mustWrite(t, filepath.Join(dir, sub, pkg+".go"),
			"package "+pkg+"\n\nimport _ \"example.com/widget\"\n")
		mustWrite(t, filepath.Join(dir, sub, "CHANGELOG.md"), "# Changelog\n\n## [Unreleased]\n\n")
	}

	mustWrite(t, filepath.Join(dir, ".changeset", "README.md"), "# Changesets\n")
	mustWrite(t, filepath.Join(dir, ".changeset", "feat.md"),
		"---\n"+
			`"example.com/widget": minor`+"\n"+
			`"transports/foo": minor`+"\n"+
			`"plugins/bar": patch`+"\n"+
			"---\n\n"+
			"Initial multi-module release.\n")
	mustWrite(t, filepath.Join(dir, "monorel.toml"),
		`[provider]
name = "github"
owner = "test"
repo = "test"

[packages."example.com/widget"]
tag_prefix = ""
path       = "."
changelog  = "CHANGELOG.md"

[packages."transports/foo"]
tag_prefix = "transports/foo"
path       = "transports/foo"
changelog  = "transports/foo/CHANGELOG.md"

[packages."plugins/bar"]
tag_prefix = "plugins/bar"
path       = "plugins/bar"
changelog  = "plugins/bar/CHANGELOG.md"
`)
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// readFile is local-only because helpers_test.go is gated on
// build tag `e2e` while this file uses `e2e_tidy`; the test is
// self-contained.
func readFile(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(body)
}

// readAll consumes a testcontainers Exec reader into a string. The
// reader's stream includes a small framing header per line; this
// helper drops nothing and just returns the raw bytes for diagnostic
// printing in test failures.
func readAll(r interface{ Read(p []byte) (int, error) }) (string, error) {
	var buf [4096]byte
	var sb strings.Builder
	for {
		n, err := r.Read(buf[:])
		if n > 0 {
			sb.Write(buf[:n])
		}
		if err != nil {
			return sb.String(), err
		}
	}
}
