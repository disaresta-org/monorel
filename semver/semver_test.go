package semver

import (
	"errors"
	"testing"
)

// TestApply_LenientInputs documents (and locks in) that Apply
// accepts looser inputs than strict Go module tags would: short-form
// versions like "v1" and "v1.2", and zero-padded forms like
// "v01.02.03", all normalize to canonical "vX.Y.Z" output.
//
// Behavior comes from Masterminds/semver/v3, which normalizes during
// parse. The previous library (golang.org/x/mod/semver) rejected
// these. The planner only feeds Apply tags it pulled from `git tag`,
// which are well-formed in practice; this test exists so a future
// contributor doesn't tighten the input-validation pre-check
// without realizing they'd be breaking documented behavior.
func TestApply_LenientInputs(t *testing.T) {
	cases := []struct {
		in   string
		bump BumpLevel
		want string
	}{
		{"v1", Patch, "v1.0.1"},        // short-form normalizes to v1.0.0 first
		{"v1.2", Minor, "v1.3.0"},      // ditto
		{"v01.02.03", Patch, "v1.2.4"}, // zero-padded normalizes
	}
	for _, tc := range cases {
		got, err := Apply(tc.in, tc.bump)
		if err != nil {
			t.Errorf("Apply(%q, %s) = err %v, want %s", tc.in, tc.bump, err, tc.want)
			continue
		}
		if got != tc.want {
			t.Errorf("Apply(%q, %s) = %s, want %s", tc.in, tc.bump, got, tc.want)
		}
	}
}

func TestBumpLevel_String(t *testing.T) {
	cases := []struct {
		in   BumpLevel
		want string
	}{
		{None, "none"},
		{Patch, "patch"},
		{Minor, "minor"},
		{Major, "major"},
	}
	for _, c := range cases {
		if got := c.in.String(); got != c.want {
			t.Errorf("BumpLevel(%d).String() = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestParseBumpLevel(t *testing.T) {
	cases := []struct {
		in      string
		want    BumpLevel
		wantErr bool
	}{
		{"major", Major, false},
		{"Minor", Minor, false},
		{" PATCH ", Patch, false},
		{"none", None, true},
		{"breaking", None, true},
		{"", None, true},
	}
	for _, c := range cases {
		got, err := ParseBumpLevel(c.in)
		if (err != nil) != c.wantErr {
			t.Errorf("ParseBumpLevel(%q): wantErr=%v, got %v (%v)", c.in, c.wantErr, got, err)
			continue
		}
		if !c.wantErr && got != c.want {
			t.Errorf("ParseBumpLevel(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestMax(t *testing.T) {
	if got := Max(Patch, Minor); got != Minor {
		t.Errorf("Max(Patch,Minor) = %v, want Minor", got)
	}
	if got := Max(Major, Patch); got != Major {
		t.Errorf("Max(Major,Patch) = %v, want Major", got)
	}
	if got := Max(None, Patch); got != Patch {
		t.Errorf("Max(None,Patch) = %v, want Patch", got)
	}
	if got := Max(None, None); got != None {
		t.Errorf("Max(None,None) = %v, want None", got)
	}
}

func TestApply(t *testing.T) {
	cases := []struct {
		base    string
		level   BumpLevel
		want    string
		wantErr bool
	}{
		{"v1.0.0", Patch, "v1.0.1", false},
		{"v1.0.0", Minor, "v1.1.0", false},
		{"v1.0.0", Major, "v2.0.0", false},
		{"v0.0.0", Patch, "v0.0.1", false},
		{"v0.5.7", Patch, "v0.5.8", false},
		{"v0.5.7", Minor, "v0.6.0", false},
		{"v0.5.7", Major, "v1.0.0", false},
		{"v1.6.2", None, "v1.6.2", false},
		// Pre-release input bumps from canonical, dropping suffix.
		{"v1.0.0-rc.3", Minor, "v1.1.0", false},
		{"v2.0.0-beta.1", Major, "v3.0.0", false},
		// Errors.
		{"1.0.0", Minor, "", true},  // missing v prefix
		{"vX.Y.Z", Minor, "", true}, // not numeric
		{"v1.0.0", BumpLevel(99), "", true},
	}
	for _, c := range cases {
		got, err := Apply(c.base, c.level)
		if (err != nil) != c.wantErr {
			t.Errorf("Apply(%q,%v): wantErr=%v, got %q (%v)", c.base, c.level, c.wantErr, got, err)
			continue
		}
		if !c.wantErr && got != c.want {
			t.Errorf("Apply(%q,%v) = %q, want %q", c.base, c.level, got, c.want)
		}
	}
}

func TestInitialFromBump(t *testing.T) {
	cases := []struct {
		in      BumpLevel
		want    string
		wantErr bool
	}{
		{Major, "v1.0.0", false},
		{Minor, "v0.1.0", false},
		{Patch, "v0.0.1", false},
		{None, "", true},
		{BumpLevel(99), "", true},
	}
	for _, c := range cases {
		got, err := InitialFromBump(c.in)
		if (err != nil) != c.wantErr {
			t.Errorf("InitialFromBump(%v): wantErr=%v, got %q (%v)", c.in, c.wantErr, got, err)
			continue
		}
		if !c.wantErr && got != c.want {
			t.Errorf("InitialFromBump(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestApplyPrerelease(t *testing.T) {
	cases := []struct {
		base    string
		channel string
		counter int
		want    string
		wantErr bool
	}{
		{"v1.0.0", "rc", 0, "v1.0.0-rc.0", false},
		{"v1.0.0", "rc", 3, "v1.0.0-rc.3", false},
		{"v0.5.0", "beta", 1, "v0.5.0-beta.1", false},
		{"v2.0.0", "alpha", 12, "v2.0.0-alpha.12", false},
		// Errors.
		{"1.0.0", "rc", 0, "", true},   // missing v
		{"v1.0.0", "", 0, "", true},    // empty channel
		{"v1.0.0", "rc", -1, "", true}, // negative counter
	}
	for _, c := range cases {
		got, err := ApplyPrerelease(c.base, c.channel, c.counter)
		if (err != nil) != c.wantErr {
			t.Errorf("ApplyPrerelease(%q,%q,%d): wantErr=%v, got %q (%v)",
				c.base, c.channel, c.counter, c.wantErr, got, err)
			continue
		}
		if !c.wantErr && got != c.want {
			t.Errorf("ApplyPrerelease(%q,%q,%d) = %q, want %q",
				c.base, c.channel, c.counter, got, c.want)
		}
	}
}

func TestIsPrerelease(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"v1.0.0", false},
		{"v1.0.0-rc.1", true},
		{"v0.5.0-beta.2", true},
		{"v1.0.0-alpha", true},
	}
	for _, c := range cases {
		if got := IsPrerelease(c.in); got != c.want {
			t.Errorf("IsPrerelease(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestCompare(t *testing.T) {
	cmp := func(a, b string) int {
		t.Helper()
		got, err := Compare(a, b)
		if err != nil {
			t.Fatalf("Compare(%q, %q) error: %v", a, b, err)
		}
		return got
	}
	if cmp("v1.0.0", "v1.0.1") >= 0 {
		t.Error("v1.0.0 should be < v1.0.1")
	}
	if cmp("v1.0.0-rc.1", "v1.0.0") >= 0 {
		t.Error("v1.0.0-rc.1 should be < v1.0.0")
	}
	if cmp("v1.0.0", "v1.0.0") != 0 {
		t.Error("v1.0.0 == v1.0.0 should be 0")
	}
}

func TestCompare_InvalidInput(t *testing.T) {
	if _, err := Compare("not-a-version", "v1.0.0"); err == nil {
		t.Error("want error for invalid `a`, got nil")
	}
	if _, err := Compare("v1.0.0", "not-a-version"); err == nil {
		t.Error("want error for invalid `b`, got nil")
	}
}

// Sanity check: Apply errors satisfy errors.Is on ErrInvalidVersion so
// callers can branch on the specific failure.
func TestApply_ErrorsIs(t *testing.T) {
	_, err := Apply("not-a-version", Minor)
	if !errors.Is(err, ErrInvalidVersion) {
		t.Errorf("Apply with bad version: errors.Is(err, ErrInvalidVersion) = false, err = %v", err)
	}
}
