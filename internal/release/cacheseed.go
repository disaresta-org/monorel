package release

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/mod/module"
	"golang.org/x/mod/sumdb/dirhash"
	xzip "golang.org/x/mod/zip"
)

// seededEntry tracks the four files seeded for a single (module,
// version) pair so cleanup can remove only what we wrote.
type seededEntry struct {
	infoPath    string
	modPath     string
	zipPath     string
	zipHashPath string
}

// seedModuleCache writes the four cache files (.info, .mod, .zip,
// .ziphash) for every in-plan released package into the developer's
// Go module cache, in the layout `go mod tidy` expects when running
// offline. Returns a slice of [seededEntry] tracking every file
// written; callers pass it to [clearSeededEntries] to remove the
// entries when done. The slice is populated even on error (whatever
// was written before the failure) so cleanup can still run.
//
// The cleanup is best-effort: per-entry remove failures are not
// returned, since cache entries are content-addressed and a stale
// leftover is inert (the next normal fetch overwrites it).
//
// Skips packages whose Path doesn't contain a go.mod (e.g. a
// pure-changelog package), and packages whose go.mod parse fails
// (those bubble out of the rewriter earlier).
func seedModuleCache(opts Options) (seeded []seededEntry, err error) {
	mc, err := goModCache()
	if err != nil {
		return nil, fmt.Errorf("seed module cache: %w", err)
	}

	// Use the current time for the .info Time field. The actual
	// release commit doesn't exist yet at seed time; tidy reads
	// Time only for cosmetic display, not version comparison, so
	// any reasonable timestamp works.
	now := time.Now().UTC().Format(time.RFC3339)

	for _, r := range opts.Plan.Releases {
		modDir := filepath.Join(opts.RepoDir, r.Config.Path)
		modFilePath := filepath.Join(modDir, "go.mod")

		modBytes, err := os.ReadFile(modFilePath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return seeded, fmt.Errorf("seed module cache: read %s: %w", modFilePath, err)
		}

		mf, err := readModFile(modFilePath)
		if err != nil {
			return seeded, fmt.Errorf("seed module cache: %w", err)
		}
		if mf == nil {
			continue
		}

		mv := module.Version{
			Path:    mf.Module.Mod.Path,
			Version: tagVersion(r.Tag),
		}

		entry, err := writeCacheEntry(mc, mv, modDir, modBytes, now)
		// Append unconditionally: writeCacheEntry returns its
		// (partially-populated) entry on error too, and the caller
		// uses the slice to clean up whatever was written before
		// failure. Empty fields in entry round-trip safely through
		// os.Remove via clearSeededEntries.
		seeded = append(seeded, entry)
		if err != nil {
			return seeded, fmt.Errorf("seed module cache for %s@%s: %w", mv.Path, mv.Version, err)
		}
	}

	return seeded, nil
}

// clearSeededEntries removes every cache file in entries. Called by
// the orchestrator at the end of [tidySubmoduleGoSums] (and between
// iterations of the fixpoint loop). Best-effort: per-file remove
// failures are silently ignored, since cache entries are
// content-addressed and a stale leftover is inert.
func clearSeededEntries(entries []seededEntry) {
	for _, e := range entries {
		_ = os.Remove(e.infoPath)
		_ = os.Remove(e.modPath)
		_ = os.Remove(e.zipPath)
		_ = os.Remove(e.zipHashPath)
	}
}

// writeCacheEntry produces the four cache files for one (module,
// version) under <mc>/cache/download/<module>/@v/. Returns the
// absolute paths so the caller can clean them up later.
func writeCacheEntry(mc string, mv module.Version, modDir string, modBytes []byte, infoTime string) (seededEntry, error) {
	escPath, err := module.EscapePath(mv.Path)
	if err != nil {
		return seededEntry{}, fmt.Errorf("escape module path: %w", err)
	}
	escVer, err := module.EscapeVersion(mv.Version)
	if err != nil {
		return seededEntry{}, fmt.Errorf("escape version: %w", err)
	}

	dir := filepath.Join(mc, "cache", "download", escPath, "@v")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return seededEntry{}, fmt.Errorf("mkdir %s: %w", dir, err)
	}

	entry := seededEntry{
		infoPath:    filepath.Join(dir, escVer+".info"),
		modPath:     filepath.Join(dir, escVer+".mod"),
		zipPath:     filepath.Join(dir, escVer+".zip"),
		zipHashPath: filepath.Join(dir, escVer+".ziphash"),
	}

	infoJSON, err := json.Marshal(struct {
		Version string `json:"Version"`
		Time    string `json:"Time"`
	}{Version: mv.Version, Time: infoTime})
	if err != nil {
		return entry, fmt.Errorf("marshal info: %w", err)
	}
	if err := writeFile(entry.infoPath, infoJSON); err != nil {
		return entry, err
	}
	if err := writeFile(entry.modPath, modBytes); err != nil {
		return entry, err
	}

	// Build the proxy-compatible zip into a buffer, write it to
	// disk, then hash it for the .ziphash file.
	var zipBuf bytes.Buffer
	if err := xzip.CreateFromDir(&zipBuf, mv, modDir); err != nil {
		return entry, fmt.Errorf("zip from %s: %w", modDir, err)
	}
	if err := writeFile(entry.zipPath, zipBuf.Bytes()); err != nil {
		return entry, err
	}

	hash, err := dirhash.HashZip(entry.zipPath, dirhash.Hash1)
	if err != nil {
		return entry, fmt.Errorf("hash zip: %w", err)
	}
	if err := writeFile(entry.zipHashPath, []byte(hash)); err != nil {
		return entry, err
	}
	return entry, nil
}

// goModCache returns the developer's GOMODCACHE. One subprocess
// call per invocation; the orchestrator caches the result for the
// duration of a single tidy pass and threads it down to helpers.
func goModCache() (string, error) {
	out, err := exec.Command("go", "env", "GOMODCACHE").Output()
	if err != nil {
		return "", fmt.Errorf("`go env GOMODCACHE` failed: %w", err)
	}
	v := strings.TrimSpace(string(out))
	if v == "" {
		return "", fmt.Errorf("`go env GOMODCACHE` returned empty")
	}
	return v, nil
}

// tagVersion returns the version part of a release tag.
// "transports/foo/v2.0.1" -> "v2.0.1"; "v1.6.0" -> "v1.6.0".
func tagVersion(tag string) string {
	if i := strings.LastIndex(tag, "/v"); i >= 0 {
		return tag[i+1:]
	}
	return tag
}

// writeFile is os.WriteFile with the project's standard mode.
// Pulled out so the test suite can stub it for cleanup-failure paths.
func writeFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0o644)
}
