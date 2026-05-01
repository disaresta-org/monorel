package validate_test

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"monorel.disaresta.com/validate"
)

// scenario writes a temporary monorel.toml + filesystem layout +
// .changeset/ files so each test exercises validate.Run end-to-end
// against a realistic on-disk snapshot.
type scenario struct {
	configBody string
	dirs       []string
	changesets map[string]string // filename → body
}

func setup(t *testing.T, sc scenario) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "monorel.toml"), []byte(sc.configBody), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, d := range sc.dirs {
		if err := os.MkdirAll(filepath.Join(dir, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if sc.changesets != nil {
		csDir := filepath.Join(dir, ".changeset")
		if err := os.MkdirAll(csDir, 0o755); err != nil {
			t.Fatal(err)
		}
		for name, body := range sc.changesets {
			if err := os.WriteFile(filepath.Join(csDir, name), []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	return filepath.Join(dir, "monorel.toml")
}

// codes returns the Code field of each finding sorted, so tests can
// assert on a stable set without caring about emit order.
func codes(findings []validate.Finding) []string {
	out := make([]string, len(findings))
	for i, f := range findings {
		out[i] = f.Code
	}
	sort.Strings(out)
	return out
}

func TestRun_HappyPath(t *testing.T) {
	cfgPath := setup(t, scenario{
		configBody: `
[provider]
name = "github"
owner = "acme"
repo = "widget"

[packages."github.com/acme/widget"]
tag_prefix = ""
path = "."
changelog = "CHANGELOG.md"

[packages."transports/zerolog"]
tag_prefix = "transports/zerolog"
path = "transports/zerolog"
changelog = "transports/zerolog/CHANGELOG.md"
`,
		dirs: []string{".", "transports/zerolog"},
	})

	got := validate.Run(validate.Inputs{ConfigPath: cfgPath})
	if len(got) != 0 {
		t.Fatalf("want clean run, got findings: %+v", got)
	}
	if validate.HasErrors(got) || validate.HasWarnings(got) {
		t.Fatal("HasErrors / HasWarnings returned true on empty findings")
	}
}

func TestRun_ConfigInvalid_NoForgeOwner(t *testing.T) {
	cfgPath := setup(t, scenario{
		configBody: `
[provider]
repo = "widget"

[packages."widget"]
tag_prefix = ""
path = "."
changelog = "CHANGELOG.md"
`,
		dirs: []string{"."},
	})

	got := validate.Run(validate.Inputs{ConfigPath: cfgPath})
	if !validate.HasErrors(got) {
		t.Fatalf("want HasErrors=true for missing forge.owner; got: %+v", got)
	}
	// config.Load wraps schema violations; we don't pin the exact
	// error string but it must surface as config_invalid.
	want := []string{"config_invalid"}
	if g := codes(got); !equalSlices(g, want) {
		t.Errorf("codes = %v, want %v", g, want)
	}
}

func TestRun_PathMissing(t *testing.T) {
	cfgPath := setup(t, scenario{
		configBody: `
[provider]
owner = "acme"
repo = "widget"

[packages."widget"]
tag_prefix = ""
path = "."
changelog = "CHANGELOG.md"

[packages."transports/missing"]
tag_prefix = "transports/missing"
path = "transports/missing"
changelog = "transports/missing/CHANGELOG.md"
`,
		dirs: []string{"."}, // transports/missing intentionally absent
	})

	got := validate.Run(validate.Inputs{ConfigPath: cfgPath})
	want := []string{"changelog_dir_missing", "path_missing"}
	if g := codes(got); !equalSlices(g, want) {
		t.Errorf("codes = %v, want %v\nfindings = %+v", g, want, got)
	}
}

func TestRun_PathDuplicate(t *testing.T) {
	cfgPath := setup(t, scenario{
		configBody: `
[provider]
owner = "acme"
repo = "widget"

[packages."widget"]
tag_prefix = ""
path = "."
changelog = "CHANGELOG.md"

[packages."widget-alias"]
tag_prefix = "alias"
path = "."
changelog = "CHANGELOG-ALIAS.md"
`,
		dirs: []string{"."},
	})

	got := validate.Run(validate.Inputs{ConfigPath: cfgPath})
	if !containsCode(got, "path_duplicate") {
		t.Errorf("want path_duplicate finding; got: %+v", got)
	}
}

func TestRun_PathNotDir(t *testing.T) {
	cfgPath := setup(t, scenario{
		configBody: `
[provider]
owner = "acme"
repo = "widget"

[packages."widget"]
tag_prefix = ""
path = "."
changelog = "CHANGELOG.md"

[packages."ghost"]
tag_prefix = "ghost"
path = "ghost-file"
changelog = "ghost-file/CHANGELOG.md"
`,
		dirs: []string{"."},
	})
	// Create ghost-file as a regular file instead of a directory.
	if err := os.WriteFile(filepath.Join(filepath.Dir(cfgPath), "ghost-file"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := validate.Run(validate.Inputs{ConfigPath: cfgPath})
	if !containsCode(got, "path_not_dir") {
		t.Errorf("want path_not_dir finding; got: %+v", got)
	}
}

func TestRun_ChangesetUnknownPackage(t *testing.T) {
	cfgPath := setup(t, scenario{
		configBody: `
[provider]
owner = "acme"
repo = "widget"

[packages."widget"]
tag_prefix = ""
path = "."
changelog = "CHANGELOG.md"
`,
		dirs: []string{"."},
		changesets: map[string]string{
			"typo.md": `---
"widgett": patch
---

Typo in package name.
`,
		},
	})

	got := validate.Run(validate.Inputs{ConfigPath: cfgPath})
	want := []string{"changeset_unknown_package"}
	if g := codes(got); !equalSlices(g, want) {
		t.Errorf("codes = %v, want %v\nfindings = %+v", g, want, got)
	}
	// Verify the finding pinpoints the offending key, not just the file.
	for _, f := range got {
		if f.Code == "changeset_unknown_package" && f.Package != "widgett" {
			t.Errorf("finding.Package = %q, want %q", f.Package, "widgett")
		}
	}
}

func TestRun_ChangesetParseError(t *testing.T) {
	cfgPath := setup(t, scenario{
		configBody: `
[provider]
owner = "acme"
repo = "widget"

[packages."widget"]
tag_prefix = ""
path = "."
changelog = "CHANGELOG.md"
`,
		dirs: []string{"."},
		changesets: map[string]string{
			"broken.md": "no frontmatter at all\njust prose\n",
		},
	})

	got := validate.Run(validate.Inputs{ConfigPath: cfgPath})
	if !containsCode(got, "changeset_parse_failed") {
		t.Errorf("want changeset_parse_failed finding; got: %+v", got)
	}
}

func TestRun_ChangesetSkipsReservedNames(t *testing.T) {
	cfgPath := setup(t, scenario{
		configBody: `
[provider]
owner = "acme"
repo = "widget"

[packages."widget"]
tag_prefix = ""
path = "."
changelog = "CHANGELOG.md"
`,
		dirs: []string{"."},
		changesets: map[string]string{
			"README.md": "# Changesets\n\nThis is the README, not a changeset.",
			// pre.json is not .md so it'd be skipped anyway, but
			// covering it here documents the contract.
			"pre.json": `{"mode":"none"}`,
		},
	})

	got := validate.Run(validate.Inputs{ConfigPath: cfgPath})
	if len(got) != 0 {
		t.Errorf("want clean run (reserved names skipped); got: %+v", got)
	}
}

func TestRun_ChangesetDirAbsent(t *testing.T) {
	cfgPath := setup(t, scenario{
		configBody: `
[provider]
owner = "acme"
repo = "widget"

[packages."widget"]
tag_prefix = ""
path = "."
changelog = "CHANGELOG.md"
`,
		dirs: []string{"."},
		// no .changeset/ directory written.
	})

	got := validate.Run(validate.Inputs{ConfigPath: cfgPath})
	if len(got) != 0 {
		t.Errorf("absent .changeset/ should be clean; got: %+v", got)
	}
}

func TestRun_MultipleFindingsSurfaceTogether(t *testing.T) {
	// Ensures validate doesn't bail on the first error: schema
	// violation, missing path, and a bad changeset all surface in
	// one run. The whole point of the command.
	cfgPath := setup(t, scenario{
		configBody: `
[provider]
owner = "acme"
repo = "widget"

[packages."widget"]
tag_prefix = ""
path = "."
changelog = "CHANGELOG.md"

[packages."ghost"]
tag_prefix = "ghost"
path = "ghost"
changelog = "ghost/CHANGELOG.md"
`,
		dirs: []string{"."},
		changesets: map[string]string{
			"typo.md": `---
"unknown-pkg": patch
---

bad reference
`,
			"broken.md": "no frontmatter\n",
		},
	})

	got := validate.Run(validate.Inputs{ConfigPath: cfgPath})
	wantCodes := map[string]bool{
		"path_missing":              true, // ghost dir
		"changelog_dir_missing":     true, // ghost/ doesn't exist
		"changeset_unknown_package": true, // typo.md
		"changeset_parse_failed":    true, // broken.md
	}
	for _, f := range got {
		delete(wantCodes, f.Code)
	}
	if len(wantCodes) != 0 {
		t.Errorf("missing expected codes %v\nactual findings: %+v", wantCodes, got)
	}
}

func TestRun_CheckTags_NonSemverTagWarns(t *testing.T) {
	cfgPath := setup(t, scenario{
		configBody: `
[provider]
owner = "acme"
repo = "widget"

[packages."widget"]
tag_prefix = ""
path = "."
changelog = "CHANGELOG.md"
`,
		dirs: []string{"."},
	})

	got := validate.Run(validate.Inputs{
		ConfigPath: cfgPath,
		CheckTags:  true,
		ListTags: func(prefix string) ([]string, error) {
			// Single root package (prefix == ""); return all bare tags.
			return []string{"v1.0.0", "v1.0.1", "vGARBAGE"}, nil
		},
	})

	if !containsCode(got, "tag_not_semver") {
		t.Errorf("want tag_not_semver warning; got: %+v", got)
	}
	for _, f := range got {
		if f.Code == "tag_not_semver" && f.Severity != validate.SeverityWarning {
			t.Errorf("tag_not_semver finding has severity %s, want warning", f.Severity)
		}
	}
}

func TestRun_CheckTags_PrefixIsThreaded(t *testing.T) {
	// Verifies the per-package prefix is passed through to the
	// callback. A well-behaved git-backed implementation uses it
	// to filter (e.g. `git tag --list <prefix>v*`); validate
	// trusts the callback's output without re-filtering.
	cfgPath := setup(t, scenario{
		configBody: `
[provider]
owner = "acme"
repo = "widget"

[packages."widget"]
tag_prefix = ""
path = "."
changelog = "CHANGELOG.md"

[packages."transports/foo"]
tag_prefix = "transports/foo"
path = "transports/foo"
changelog = "transports/foo/CHANGELOG.md"
`,
		dirs: []string{".", "transports/foo"},
	})

	seenPrefixes := []string{}
	got := validate.Run(validate.Inputs{
		ConfigPath: cfgPath,
		CheckTags:  true,
		ListTags: func(prefix string) ([]string, error) {
			seenPrefixes = append(seenPrefixes, prefix)
			switch prefix {
			case "":
				return []string{"v1.0.0"}, nil
			case "transports/foo/":
				return []string{"transports/foo/v1.0.0"}, nil
			}
			return nil, nil
		},
	})
	if len(got) != 0 {
		t.Errorf("want clean run; got: %+v", got)
	}
	wantPrefixes := []string{"", "transports/foo/"}
	sort.Strings(seenPrefixes)
	if !equalSlices(seenPrefixes, wantPrefixes) {
		t.Errorf("seenPrefixes = %v, want %v", seenPrefixes, wantPrefixes)
	}
}

func TestRun_CheckTags_NilCallback(t *testing.T) {
	cfgPath := setup(t, scenario{
		configBody: `
[provider]
owner = "acme"
repo = "widget"

[packages."widget"]
tag_prefix = ""
path = "."
changelog = "CHANGELOG.md"
`,
		dirs: []string{"."},
	})

	got := validate.Run(validate.Inputs{
		ConfigPath: cfgPath,
		CheckTags:  true,
		// ListTags intentionally nil.
	})
	if !containsCode(got, "tag_check_misconfigured") {
		t.Errorf("want tag_check_misconfigured finding; got: %+v", got)
	}
}

func TestRun_ConfigPathInvalid(t *testing.T) {
	got := validate.Run(validate.Inputs{ConfigPath: "/path/that/does/not/exist/monorel.toml"})
	if !validate.HasErrors(got) {
		t.Errorf("want HasErrors for missing config; got: %+v", got)
	}
}

func TestRun_FindingHasContext(t *testing.T) {
	// Exercises that filesystem findings include both Package and
	// Path (operators reading the JSON output use these to navigate).
	cfgPath := setup(t, scenario{
		configBody: `
[provider]
owner = "acme"
repo = "widget"

[packages."widget"]
tag_prefix = ""
path = "."
changelog = "CHANGELOG.md"

[packages."ghost"]
tag_prefix = "ghost"
path = "ghost"
changelog = "ghost/CHANGELOG.md"
`,
		dirs: []string{"."},
	})

	got := validate.Run(validate.Inputs{ConfigPath: cfgPath})
	for _, f := range got {
		if f.Code == "path_missing" {
			if f.Package != "ghost" {
				t.Errorf("path_missing.Package = %q, want %q", f.Package, "ghost")
			}
			if !strings.Contains(f.Path, "ghost") {
				t.Errorf("path_missing.Path = %q, want it to mention 'ghost'", f.Path)
			}
		}
	}
}

func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func containsCode(findings []validate.Finding, code string) bool {
	for _, f := range findings {
		if f.Code == code {
			return true
		}
	}
	return false
}
