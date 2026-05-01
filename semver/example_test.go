package semver_test

import (
	"fmt"

	"monorel.disaresta.com/semver"
)

// ExampleApply demonstrates the basic version-bump operation: take
// a current version string (with the leading "v") and a bump level,
// return the next version (also with the "v"). Inputs without the
// "v" are rejected — Go module tags always carry it.
func ExampleApply() {
	for _, level := range []semver.BumpLevel{semver.Patch, semver.Minor, semver.Major} {
		next, _ := semver.Apply("v1.6.2", level)
		fmt.Printf("%s + %s = %s\n", "v1.6.2", level, next)
	}
	// Output:
	// v1.6.2 + patch = v1.6.3
	// v1.6.2 + minor = v1.7.0
	// v1.6.2 + major = v2.0.0
}

// ExampleMax shows how monorel collapses multiple changesets that
// name the same package into a single bump: take the maximum bump
// level across all of them.
func ExampleMax() {
	bumps := []semver.BumpLevel{semver.Patch, semver.Minor, semver.Patch}
	winner := semver.None
	for _, b := range bumps {
		winner = semver.Max(winner, b)
	}
	fmt.Println("winning bump:", winner)
	// Output: winning bump: minor
}

// ExampleInitialFromBump shows the rule for a never-released package's
// first release: major→1.0.0, minor→0.1.0, patch→0.0.1. The leading
// "v" is included.
func ExampleInitialFromBump() {
	for _, level := range []semver.BumpLevel{semver.Patch, semver.Minor, semver.Major} {
		v, _ := semver.InitialFromBump(level)
		fmt.Printf("%s → %s\n", level, v)
	}
	// Output:
	// patch → v0.0.1
	// minor → v0.1.0
	// major → v1.0.0
}
