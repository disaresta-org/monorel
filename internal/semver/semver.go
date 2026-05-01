// Package semver wraps golang.org/x/mod/semver with the bump-level
// abstraction monorel uses everywhere it touches versions.
//
// All version strings carry the leading "v" required by Go module tags
// (e.g. "v1.6.2", "v1.0.0-rc.3"). Functions that produce versions
// always emit the "v" form; functions that accept them tolerate either.
package semver

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/mod/semver"
)

// BumpLevel encodes the kind of version bump a changeset requests.
//
// The zero value is None — useful when accumulating "max bump across
// changesets affecting this package" because absence is the natural
// identity. Higher values dominate lower ones in [Max].
type BumpLevel int

const (
	// None means no bump (no changesets affect this package).
	None BumpLevel = iota
	// Patch is for backwards-compatible bug fixes.
	Patch
	// Minor is for backwards-compatible feature additions.
	Minor
	// Major is for breaking changes.
	Major
)

// String returns the lowercase canonical name ("none", "patch",
// "minor", "major"). Matches the form used in changeset frontmatter.
func (b BumpLevel) String() string {
	switch b {
	case None:
		return "none"
	case Patch:
		return "patch"
	case Minor:
		return "minor"
	case Major:
		return "major"
	default:
		return fmt.Sprintf("BumpLevel(%d)", int(b))
	}
}

// ParseBumpLevel converts a changeset-frontmatter string to a
// BumpLevel. Accepts "major"/"minor"/"patch" case-insensitively.
// "none" is rejected: a changeset that says "none" for a package
// shouldn't list that package at all.
func ParseBumpLevel(s string) (BumpLevel, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "major":
		return Major, nil
	case "minor":
		return Minor, nil
	case "patch":
		return Patch, nil
	}
	return None, fmt.Errorf("invalid bump level %q (expected major|minor|patch)", s)
}

// Max returns the higher of two bump levels. Used to combine multiple
// changesets affecting the same package: the strongest bump wins.
func Max(a, b BumpLevel) BumpLevel {
	if a > b {
		return a
	}
	return b
}

// ErrInvalidVersion is returned when a version string does not parse
// as a Go module version (must start with "v" and follow SemVer 2.0).
var ErrInvalidVersion = errors.New("invalid semver version")

// Apply bumps current by level. current must be a valid Go module
// version ("vX.Y.Z" or "vX.Y.Z-pre"). For pre-release inputs (e.g.
// "v1.0.0-rc.3"), Apply strips the pre-release suffix first and bumps
// from the canonical version: this matches the changesets convention
// where exiting pre-release mode yields a clean stable bump from the
// last stable version, not the last rc.
//
// Returns ErrInvalidVersion if current is not parseable.
func Apply(current string, level BumpLevel) (string, error) {
	if !semver.IsValid(current) {
		return "", fmt.Errorf("%w: %q", ErrInvalidVersion, current)
	}
	// Strip any pre-release suffix so we bump from the X.Y.Z core
	// regardless of what suffix current carried.
	core := strings.TrimSuffix(semver.Canonical(current), semver.Prerelease(current))
	major, minor, patch, err := splitCore(core)
	if err != nil {
		return "", err
	}
	switch level {
	case None:
		return core, nil
	case Patch:
		patch++
	case Minor:
		minor++
		patch = 0
	case Major:
		major++
		minor = 0
		patch = 0
	default:
		return "", fmt.Errorf("unknown bump level %d", int(level))
	}
	return fmt.Sprintf("v%d.%d.%d", major, minor, patch), nil
}

// InitialFromBump returns the version a never-released package gets on
// its first release with the given bump level. Major maps to v1.0.0
// (the natural "first stable release" version), minor maps to v0.1.0,
// patch to v0.0.1.
//
// Returns ErrInvalidVersion if level is None or unrecognized.
func InitialFromBump(level BumpLevel) (string, error) {
	switch level {
	case Major:
		return "v1.0.0", nil
	case Minor:
		return "v0.1.0", nil
	case Patch:
		return "v0.0.1", nil
	case None:
		return "", errors.New("cannot derive initial version from None bump")
	default:
		return "", fmt.Errorf("unknown bump level %d", int(level))
	}
}

// ApplyPrerelease appends a pre-release suffix to base, with the given
// channel and counter. Yields e.g. "v1.0.0-rc.3" from base "v1.0.0",
// channel "rc", counter 3.
//
// channel must be a non-empty identifier matching SemVer 2.0's
// pre-release rules (alphanumerics + hyphens). counter must be >= 0.
func ApplyPrerelease(base, channel string, counter int) (string, error) {
	if !semver.IsValid(base) {
		return "", fmt.Errorf("%w: %q", ErrInvalidVersion, base)
	}
	if channel == "" {
		return "", errors.New("pre-release channel must be non-empty")
	}
	if counter < 0 {
		return "", fmt.Errorf("pre-release counter must be >= 0, got %d", counter)
	}
	canonical := semver.Canonical(base)
	return fmt.Sprintf("%s-%s.%d", canonical, channel, counter), nil
}

// IsPrerelease reports whether v carries a pre-release suffix
// ("v1.0.0-rc.1" -> true, "v1.0.0" -> false).
func IsPrerelease(v string) bool {
	return semver.Prerelease(v) != ""
}

// IsValid reports whether v parses as a Go module semver version
// (leading "v", SemVer 2.0). Thin wrapper over semver.IsValid for
// callers that don't want a direct x/mod dep.
func IsValid(v string) bool {
	return semver.IsValid(v)
}

// Compare returns -1 if a < b, 0 if a == b, +1 if a > b. Same
// semantics as semver.Compare; pre-releases sort before their
// canonical version (v1.0.0-rc.1 < v1.0.0).
func Compare(a, b string) int {
	return semver.Compare(a, b)
}

// splitCore parses "vX.Y.Z" into its three integer parts. Assumes the
// input is already canonical (no pre-release or build suffix).
func splitCore(canonical string) (major, minor, patch int, err error) {
	if len(canonical) < 2 || canonical[0] != 'v' {
		return 0, 0, 0, fmt.Errorf("%w: missing leading v in %q", ErrInvalidVersion, canonical)
	}
	parts := strings.SplitN(canonical[1:], ".", 3)
	if len(parts) != 3 {
		return 0, 0, 0, fmt.Errorf("%w: %q has %d parts", ErrInvalidVersion, canonical, len(parts))
	}
	if major, err = strconv.Atoi(parts[0]); err != nil {
		return 0, 0, 0, fmt.Errorf("%w: bad major in %q: %v", ErrInvalidVersion, canonical, err)
	}
	if minor, err = strconv.Atoi(parts[1]); err != nil {
		return 0, 0, 0, fmt.Errorf("%w: bad minor in %q: %v", ErrInvalidVersion, canonical, err)
	}
	if patch, err = strconv.Atoi(parts[2]); err != nil {
		return 0, 0, 0, fmt.Errorf("%w: bad patch in %q: %v", ErrInvalidVersion, canonical, err)
	}
	return major, minor, patch, nil
}
