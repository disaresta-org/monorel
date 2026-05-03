package changeset

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// PreState is the on-disk representation of pre-release mode. Lives
// at .changeset/pre.json when monorel is in a pre-release window.
//
// While in pre-release mode, monorel release appends "-<channel>.N"
// to each released package's version and increments that package's
// counter. Exiting pre-release mode (monorel pre exit) deletes the
// file; the next stable release bumps from the last *stable* tag,
// applying every changeset accumulated since.
type PreState struct {
	// SchemaVersion identifies the on-disk format. Currently 1.
	// Older files written before this field existed deserialize as
	// 0 and are treated as version 1 (backward-compatible). Newer
	// versions (greater than [PreStateCurrentSchemaVersion]) are
	// rejected with a clear error so a stale monorel binary can't
	// silently misread a future format.
	SchemaVersion int `json:"schemaVersion,omitempty"`

	// Mode is always "pre" while the state file exists. Reserved
	// for forward-compatibility with future modes (e.g. "exit"
	// rendering the file inert without deleting it).
	Mode string `json:"mode"`

	// Channel is the pre-release identifier ("rc", "beta", "alpha",
	// or any non-empty SemVer-2.0-compatible label). Used as the
	// suffix prefix: -<channel>.<counter>.
	Channel string `json:"channel"`

	// Counters tracks the next pre-release counter per package. A
	// missing entry means counter 0 for the next release.
	Counters map[string]int `json:"counters,omitempty"`
}

// PreStateFilename is the basename of the on-disk state file.
const PreStateFilename = "pre.json"

// PreStateCurrentSchemaVersion is the version this build of monorel
// writes to .changeset/pre.json. Bump (and add a migration in
// LoadPreState) when the on-disk shape changes incompatibly.
const PreStateCurrentSchemaVersion = 1

// LoadPreState reads .changeset/pre.json. Returns (nil, nil) if the
// file does not exist (the common, "not in pre-release mode" case).
func LoadPreState(dir string) (*PreState, error) {
	path := filepath.Join(dir, PreStateFilename)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var s PreState
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	// Schema-version gate. 0 means "written by a monorel before the
	// field existed"; treat as version 1 (the original shape). A
	// version higher than this build understands is a hard error
	// because we can't safely interpret unknown fields or migrated
	// semantics.
	if s.SchemaVersion == 0 {
		s.SchemaVersion = 1
	}
	if s.SchemaVersion > PreStateCurrentSchemaVersion {
		return nil, fmt.Errorf("parse %s: schemaVersion %d is newer than this build supports (max %d); upgrade monorel",
			path, s.SchemaVersion, PreStateCurrentSchemaVersion)
	}
	if s.Mode != "pre" {
		return nil, fmt.Errorf("parse %s: unknown mode %q (expected \"pre\")", path, s.Mode)
	}
	if s.Channel == "" {
		return nil, fmt.Errorf("parse %s: channel is empty", path)
	}
	return &s, nil
}

// Write serializes the state to .changeset/pre.json with stable
// formatting (2-space indent, sorted keys). Stable formatting keeps
// CI diffs minimal across release runs.
func (s *PreState) Write(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	if s.Counters == nil {
		s.Counters = map[string]int{}
	}
	if s.SchemaVersion == 0 {
		s.SchemaVersion = PreStateCurrentSchemaVersion
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal pre.json: %w", err)
	}
	data = append(data, '\n')
	path := filepath.Join(dir, PreStateFilename)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// Remove deletes .changeset/pre.json. Returns nil if the file does
// not exist (idempotent).
func RemovePreState(dir string) error {
	path := filepath.Join(dir, PreStateFilename)
	err := os.Remove(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove %s: %w", path, err)
	}
	return nil
}

// CounterFor returns the next pre-release counter for the given
// package, defaulting to 0 if no entry exists.
func (s *PreState) CounterFor(pkg string) int {
	if s == nil || s.Counters == nil {
		return 0
	}
	return s.Counters[pkg]
}

// IncrementCounter raises the counter for pkg by one and returns
// the value before the increment (i.e. the value to USE for the
// release that triggered the increment).
//
// Initializes the Counters map if nil so that callers don't have to.
func (s *PreState) IncrementCounter(pkg string) int {
	if s.Counters == nil {
		s.Counters = map[string]int{}
	}
	current := s.Counters[pkg]
	s.Counters[pkg] = current + 1
	return current
}
