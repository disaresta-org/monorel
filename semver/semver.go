// Package semver wraps github.com/Masterminds/semver/v3 with the
// bump-level abstraction monorel uses everywhere it touches versions.
//
// Version strings always carry the leading "v" required by Go module
// tags (e.g. "v1.6.2", "v1.0.0-rc.3"). Functions that produce versions
// emit the "v" form; functions that accept versions require it.
// Inputs without a leading "v" are rejected as invalid; this matches
// what `git tag --list` returns and what `go get` accepts, and catches
// caller bugs at the boundary instead of letting them propagate.
package semver

import (
	"errors"
	"fmt"
	"strings"

	mm "github.com/Masterminds/semver/v3"
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

// parseStrict requires the leading "v" Go module tags use, then
// delegates to Masterminds for the rest. Masterminds on its own
// accepts "1.0.0" / "v1" / "v1.2" — too lenient for tag matching.
func parseStrict(v string) (*mm.Version, error) {
	if !strings.HasPrefix(v, "v") {
		return nil, fmt.Errorf("%w: missing leading v in %q", ErrInvalidVersion, v)
	}
	return mm.NewVersion(v)
}

// Apply bumps current by level. current must be a valid Go module
// version ("vX.Y.Z" or "vX.Y.Z-pre"). For pre-release inputs (e.g.
// "v1.0.0-rc.3"), Apply strips the pre-release suffix first and
// bumps from the canonical version: this matches the changesets
// convention where exiting pre-release mode yields a clean stable
// bump from the last stable version, not the last rc.
//
// Returns ErrInvalidVersion if current is not parseable.
func Apply(current string, level BumpLevel) (string, error) {
	v, err := parseStrict(current)
	if err != nil {
		return "", err
	}
	switch level {
	case None:
		// Strip any pre-release suffix; emit canonical X.Y.Z.
		stripped, err := v.SetPrerelease("")
		if err != nil {
			return "", err
		}
		return "v" + stripped.String(), nil
	case Patch:
		next := v.IncPatch()
		return "v" + next.String(), nil
	case Minor:
		next := v.IncMinor()
		return "v" + next.String(), nil
	case Major:
		next := v.IncMajor()
		return "v" + next.String(), nil
	default:
		return "", fmt.Errorf("unknown bump level %d", int(level))
	}
}

// WithMajor returns v with its major component set to major, keeping
// the minor and patch components (e.g. "v2.3.4" with major 3 becomes
// "v3.3.4"). Used to align a planned version with the major required
// by a Go module's path suffix ("/v3" demands a v3.x.y tag).
//
// Returns ErrInvalidVersion if v does not parse, and an error if
// major is lower than v's current major (lowering a version never has
// a legitimate caller; the path major is a floor, not a rewrite).
func WithMajor(v string, major uint64) (string, error) {
	parsed, err := parseStrict(v)
	if err != nil {
		return "", err
	}
	if major < parsed.Major() {
		return "", fmt.Errorf("would drop version %s below major %d", v, major)
	}
	out := mm.New(major, parsed.Minor(), parsed.Patch(), parsed.Prerelease(), parsed.Metadata())
	return "v" + out.String(), nil
}

// InitialFromBump returns the version a never-released package gets on
// its first release with the given bump level. Major maps to v1.0.0
// (the natural "first stable release" version), minor maps to v0.1.0,
// patch to v0.0.1.
//
// Returns an error if level is None or unrecognized.
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
	v, err := parseStrict(base)
	if err != nil {
		return "", err
	}
	if channel == "" {
		return "", errors.New("pre-release channel must be non-empty")
	}
	if counter < 0 {
		return "", fmt.Errorf("pre-release counter must be >= 0, got %d", counter)
	}
	withPre, err := v.SetPrerelease(fmt.Sprintf("%s.%d", channel, counter))
	if err != nil {
		return "", err
	}
	return "v" + withPre.String(), nil
}

// IsPrerelease reports whether v carries a pre-release suffix
// ("v1.0.0-rc.1" -> true, "v1.0.0" -> false).
func IsPrerelease(v string) bool {
	parsed, err := parseStrict(v)
	if err != nil {
		return false
	}
	return parsed.Prerelease() != ""
}

// IsValid reports whether v parses as a Go module semver version
// (leading "v", SemVer 2.0).
func IsValid(v string) bool {
	_, err := parseStrict(v)
	return err == nil
}

// Compare returns -1 if a < b, 0 if a == b, +1 if a > b. Pre-releases
// sort before their canonical version (v1.0.0-rc.1 < v1.0.0).
//
// Returns a non-nil error if either input fails to parse as a valid
// version. Callers that want to silently skip unparseable values
// should [IsValid]-guard the inputs first; the previous "return 0
// on parse failure" silently-equal behavior was a footgun that hid
// bad inputs from a caller computing a max.
func Compare(a, b string) (int, error) {
	pa, err := parseStrict(a)
	if err != nil {
		return 0, fmt.Errorf("compare: a: %w", err)
	}
	pb, err := parseStrict(b)
	if err != nil {
		return 0, fmt.Errorf("compare: b: %w", err)
	}
	return pa.Compare(pb), nil
}
