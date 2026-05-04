//go:build e2e

package e2e_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ScenarioRepo is the per-test handle: a fresh Gitea repo + a local
// clone in t.TempDir() + the helpers needed to drive monorel against
// it.
type ScenarioRepo struct {
	Name    string // unique repo name on Gitea
	Owner   string // always adminUsername in this test suite
	Local   string // local working tree (t.TempDir())
	Author  string // git user.name for commits
	Email   string // git user.email for commits
	cleanup func()
}

// newScenarioRepo creates a fresh Gitea repo with auto-init, clones
// it to a tmpdir, and returns a handle. Cleanup happens automatically
// at test end via t.Cleanup.
func newScenarioRepo(t *testing.T, suffix string) *ScenarioRepo {
	t.Helper()

	name := fmt.Sprintf("e2e-%s-%d", suffix, time.Now().UnixNano())
	if err := giteaCreateRepo(name); err != nil {
		t.Fatalf("create repo: %v", err)
	}

	dir := t.TempDir()
	cloneURL := fmt.Sprintf("%s/%s/%s.git", strings.Replace(giteaHost, "://", "://"+giteaUsername+":"+giteaToken+"@", 1), giteaUsername, name)
	cmd := exec.Command("git", "clone", "-q", cloneURL, dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git clone: %v\n%s", err, out)
	}

	r := &ScenarioRepo{
		Name:   name,
		Owner:  giteaUsername,
		Local:  dir,
		Author: "Monorel E2E",
		Email:  "e2e@example.com",
	}
	r.run(t, "git", "config", "user.name", r.Author)
	r.run(t, "git", "config", "user.email", r.Email)

	t.Cleanup(func() {
		// Best-effort delete the Gitea repo. If the test failed and
		// the user wants to inspect, set MONOREL_E2E_KEEP=1 to skip.
		if os.Getenv("MONOREL_E2E_KEEP") != "" {
			t.Logf("MONOREL_E2E_KEEP set; leaving Gitea repo %s in place", name)
			return
		}
		if err := giteaDeleteRepo(name); err != nil {
			t.Logf("delete repo cleanup: %v", err)
		}
	})

	return r
}

// run executes a command in the repo's local working tree and fails
// the test on non-zero exit. Use for plumbing commands (git config,
// git add, git commit) where we expect success.
func (r *ScenarioRepo) run(t *testing.T, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = r.Local
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, out)
	}
}

// runOutput is like run but returns combined output (no failure on
// non-zero). Use for commands whose output the caller needs to
// inspect; failure handling is the caller's job.
func (r *ScenarioRepo) runOutput(t *testing.T, name string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = r.Local
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// WriteFile writes content to a path relative to the local working
// tree.
func (r *ScenarioRepo) WriteFile(t *testing.T, relpath, content string) {
	t.Helper()
	full := filepath.Join(r.Local, relpath)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(full), err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", relpath, err)
	}
}

// ScaffoldSinglePackage writes a minimal monorel.toml + one-package
// scaffold into the local working tree. tagPrefix is the package's
// tag prefix; pass "" for a bare-tag root.
func (r *ScenarioRepo) ScaffoldSinglePackage(t *testing.T, pkgName, pkgPath, tagPrefix string) {
	t.Helper()
	r.WriteFile(t, "monorel.toml", fmt.Sprintf(`[provider]
name = "gitea"
host = %q
owner = %q
repo = %q

[packages.%q]
tag_prefix = %q
path = %q
changelog = %q
`, giteaHost, r.Owner, r.Name, pkgName, tagPrefix, pkgPath, filepath.Join(pkgPath, "CHANGELOG.md")))

	r.WriteFile(t, filepath.Join(pkgPath, "README.md"), "# "+pkgName+"\n")
	r.WriteFile(t, filepath.Join(pkgPath, "CHANGELOG.md"), "# Changelog\n\n## [Unreleased]\n\n")
	r.WriteFile(t, ".changeset/README.md", "# Changesets\n")
}

// ScaffoldMultiPackage writes a monorel.toml plus README + CHANGELOG
// stubs for N flat packages where the package name, tag prefix, and
// path are all the same string (the common multi-package shape used
// in versioning scenarios). Single-module repo: no go.mod, no
// `replace` directives. For the multi-module loglayer-shaped layout
// use ScaffoldMultiModule.
//
// The TOML is always rewritten; per-package README + CHANGELOG files
// are written only if they don't already exist, so calling this
// helper mid-life (to register a NEW package alongside an existing
// one) preserves the existing package's release history on disk.
func (r *ScenarioRepo) ScaffoldMultiPackage(t *testing.T, pkgs ...string) {
	t.Helper()
	var sb strings.Builder
	fmt.Fprintf(&sb, "[provider]\nname = \"gitea\"\nhost = %q\nowner = %q\nrepo = %q\n",
		giteaHost, r.Owner, r.Name)
	for _, p := range pkgs {
		fmt.Fprintf(&sb, "\n[packages.%q]\ntag_prefix = %q\npath = %q\nchangelog = %q\n",
			p, p, p, filepath.Join(p, "CHANGELOG.md"))
	}
	r.WriteFile(t, "monorel.toml", sb.String())
	for _, p := range pkgs {
		readme := filepath.Join(r.Local, p, "README.md")
		if _, err := os.Stat(readme); os.IsNotExist(err) {
			r.WriteFile(t, filepath.Join(p, "README.md"), "# "+p+"\n")
		}
		changelog := filepath.Join(r.Local, p, "CHANGELOG.md")
		if _, err := os.Stat(changelog); os.IsNotExist(err) {
			r.WriteFile(t, filepath.Join(p, "CHANGELOG.md"), "# Changelog\n\n## [Unreleased]\n\n")
		}
	}
	if _, err := os.Stat(filepath.Join(r.Local, ".changeset/README.md")); os.IsNotExist(err) {
		r.WriteFile(t, ".changeset/README.md", "# Changesets\n")
	}
}

// ScaffoldMultiModule writes a 3-module scaffold mirroring the
// loglayer-go layout: a bare-tag root + two path-prefixed sub-modules
// with `replace` directives during dev. Returns the package keys.
func (r *ScenarioRepo) ScaffoldMultiModule(t *testing.T) (rootKey, fooKey, barKey string) {
	t.Helper()
	rootKey = "go.example.com"
	fooKey = "go.example.com/transports/foo"
	barKey = "go.example.com/plugins/bar"

	r.WriteFile(t, "monorel.toml", fmt.Sprintf(`[provider]
name = "gitea"
host = %q
owner = %q
repo = %q

[packages.%q]
tag_prefix = ""
path = "."
changelog = "CHANGELOG.md"

[packages.%q]
tag_prefix = "transports/foo"
path = "transports/foo"
changelog = "transports/foo/CHANGELOG.md"

[packages.%q]
tag_prefix = "plugins/bar"
path = "plugins/bar"
changelog = "plugins/bar/CHANGELOG.md"
`, giteaHost, r.Owner, r.Name, rootKey, fooKey, barKey))

	r.WriteFile(t, "go.mod", "module go.example.com\n\ngo 1.23\n")
	r.WriteFile(t, "root.go", "package root\n")
	r.WriteFile(t, "CHANGELOG.md", "# Changelog\n\n## [Unreleased]\n\n")

	for sub, pkg := range map[string]string{"transports/foo": "foo", "plugins/bar": "bar"} {
		// Compute the relative path from sub back to repo root: count
		// the slashes in sub and emit that many "../".
		depth := strings.Count(sub, "/") + 1
		repl := strings.Repeat("../", depth)
		repl = strings.TrimSuffix(repl, "/")

		r.WriteFile(t, sub+"/go.mod", fmt.Sprintf(`module go.example.com/%s

go 1.23

replace go.example.com => %s

require go.example.com v0.0.0-00010101000000-000000000000
`, sub, repl))
		r.WriteFile(t, sub+"/"+pkg+".go", fmt.Sprintf("package %s\n\nimport _ \"go.example.com\"\n", pkg))
		r.WriteFile(t, sub+"/CHANGELOG.md", "# Changelog\n\n## [Unreleased]\n\n")
	}
	r.WriteFile(t, ".changeset/README.md", "# Changesets\n")
	return rootKey, fooKey, barKey
}

// WriteChangeset writes a changeset file to .changeset/<name>.md.
// bumps maps package keys to bump levels ("major" / "minor" /
// "patch").
func (r *ScenarioRepo) WriteChangeset(t *testing.T, name string, bumps map[string]string, body string) {
	t.Helper()
	var b strings.Builder
	b.WriteString("---\n")
	for pkg, level := range bumps {
		fmt.Fprintf(&b, "%q: %s\n", pkg, level)
	}
	b.WriteString("---\n\n")
	b.WriteString(body)
	if !strings.HasSuffix(body, "\n") {
		b.WriteString("\n")
	}
	r.WriteFile(t, ".changeset/"+name+".md", b.String())
}

// CommitAll stages every change and creates a commit with the given
// message. Fails the test on git error.
func (r *ScenarioRepo) CommitAll(t *testing.T, message string) {
	t.Helper()
	r.run(t, "git", "add", "-A")
	r.run(t, "git", "commit", "-q", "-m", message)
}

// PushMain pushes the local main branch to origin.
func (r *ScenarioRepo) PushMain(t *testing.T) {
	t.Helper()
	r.run(t, "git", "push", "-q", "origin", "main")
}

// CheckoutMain checks out main and resets it to origin/main.
// Idempotent. Use after monorel auto's feature path leaves the local
// tree on monorel/release.
func (r *ScenarioRepo) CheckoutMain(t *testing.T) {
	t.Helper()
	r.run(t, "git", "fetch", "-q", "origin", "main")
	r.run(t, "git", "checkout", "-q", "-B", "main", "origin/main")
	r.run(t, "git", "reset", "-q", "--hard", "origin/main")
}

// MonorelResult bundles the outcome of a monorel CLI invocation.
type MonorelResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// Monorel runs the monorel binary in the local working tree with the
// Gitea token in the environment. The caller asserts on the result.
func (r *ScenarioRepo) Monorel(t *testing.T, args ...string) *MonorelResult {
	t.Helper()
	cmd := exec.Command(monorelBin, args...)
	cmd.Dir = r.Local
	cmd.Env = append(os.Environ(),
		"GITEA_TOKEN="+giteaToken,
		"GIT_AUTHOR_NAME="+r.Author,
		"GIT_AUTHOR_EMAIL="+r.Email,
		"GIT_COMMITTER_NAME="+r.Author,
		"GIT_COMMITTER_EMAIL="+r.Email,
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	result := &MonorelResult{Stdout: stdout.String(), Stderr: stderr.String()}
	if exit, ok := err.(*exec.ExitError); ok {
		result.ExitCode = exit.ExitCode()
	} else if err != nil {
		t.Fatalf("monorel %s: %v", strings.Join(args, " "), err)
	}
	return result
}

// MonorelOK is Monorel + asserts exit 0; returns stdout. Convenience
// for the common case.
func (r *ScenarioRepo) MonorelOK(t *testing.T, args ...string) string {
	t.Helper()
	res := r.Monorel(t, args...)
	if res.ExitCode != 0 {
		t.Fatalf("monorel %s: exit %d\nstdout: %s\nstderr: %s",
			strings.Join(args, " "), res.ExitCode, res.Stdout, res.Stderr)
	}
	return res.Stdout
}

// HeadMessage returns HEAD's full commit message in the local working
// tree.
func (r *ScenarioRepo) HeadMessage(t *testing.T) string {
	t.Helper()
	out, err := r.runOutput(t, "git", "log", "-1", "--format=%B")
	if err != nil {
		t.Fatalf("git log: %v\n%s", err, out)
	}
	return out
}

// LocalTags returns the sorted list of tags in the local repo.
func (r *ScenarioRepo) LocalTags(t *testing.T) []string {
	t.Helper()
	out, _ := r.runOutput(t, "git", "tag", "--list")
	tags := strings.Fields(out)
	return tags
}

// MergePR squash/rebase/merge PR #num via the Gitea API. Polls until
// Gitea reports the PR mergeable (its mergeable field is computed
// asynchronously after creation), then issues the merge. Strategy is
// one of "squash", "rebase", or "merge".
func (r *ScenarioRepo) MergePR(t *testing.T, num int, strategy string) {
	t.Helper()
	if err := pollMergeable(r.Owner, r.Name, num); err != nil {
		t.Fatalf("PR #%d not mergeable: %v", num, err)
	}
	if err := giteaMergePR(r.Owner, r.Name, num, strategy); err != nil {
		t.Fatalf("merge PR #%d (%s): %v", num, strategy, err)
	}
}

// ClosePR closes PR #num via the Gitea API (sets state=closed
// without merging).
func (r *ScenarioRepo) ClosePR(t *testing.T, num int) {
	t.Helper()
	if err := giteaPatchPRState(r.Owner, r.Name, num, "closed"); err != nil {
		t.Fatalf("close PR #%d: %v", num, err)
	}
}

// PRs returns the open PRs on the repo.
func (r *ScenarioRepo) PRs(t *testing.T, state string) []GiteaPR {
	t.Helper()
	prs, err := giteaListPRs(r.Owner, r.Name, state)
	if err != nil {
		t.Fatalf("list PRs: %v", err)
	}
	return prs
}

// Releases returns the Gitea Release objects for the repo.
func (r *ScenarioRepo) Releases(t *testing.T) []GiteaRelease {
	t.Helper()
	rels, err := giteaListReleases(r.Owner, r.Name)
	if err != nil {
		t.Fatalf("list releases: %v", err)
	}
	return rels
}

// RemoteTags returns the tag names on the Gitea remote.
func (r *ScenarioRepo) RemoteTags(t *testing.T) []string {
	t.Helper()
	tags, err := giteaListTags(r.Owner, r.Name)
	if err != nil {
		t.Fatalf("list tags: %v", err)
	}
	return tags
}

// =====================================================================
// Gitea API helpers
// =====================================================================

func giteaRequest(method, path string, body any) (*http.Response, error) {
	var buf *bytes.Buffer
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		buf = bytes.NewBuffer(raw)
	}
	var reqBody *bytes.Buffer
	if buf != nil {
		reqBody = buf
	}
	var req *http.Request
	var err error
	if reqBody != nil {
		req, err = http.NewRequest(method, giteaHost+path, reqBody)
	} else {
		req, err = http.NewRequest(method, giteaHost+path, nil)
	}
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "token "+giteaToken)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return http.DefaultClient.Do(req)
}

func giteaCreateRepo(name string) error {
	body := map[string]any{
		"name":           name,
		"private":        false,
		"auto_init":      true,
		"default_branch": "main",
	}
	resp, err := giteaRequest("POST", "/api/v1/user/repos", body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("create repo %s: HTTP %d", name, resp.StatusCode)
	}
	return nil
}

func giteaDeleteRepo(name string) error {
	resp, err := giteaRequest("DELETE", "/api/v1/repos/"+adminUsername+"/"+name, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 && resp.StatusCode != 404 {
		return fmt.Errorf("delete repo %s: HTTP %d", name, resp.StatusCode)
	}
	return nil
}

// GiteaPR is the slim PR shape e2e tests assert on.
type GiteaPR struct {
	Number    int    `json:"number"`
	State     string `json:"state"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	Mergeable bool   `json:"mergeable"`
	Head      struct {
		Ref string `json:"ref"`
	} `json:"head"`
}

// GiteaRelease is the slim Release shape e2e tests assert on.
type GiteaRelease struct {
	TagName string `json:"tag_name"`
	Name    string `json:"name"`
	Body    string `json:"body"`
}

func giteaListPRs(owner, repo, state string) ([]GiteaPR, error) {
	resp, err := giteaRequest("GET", fmt.Sprintf("/api/v1/repos/%s/%s/pulls?state=%s",
		url.PathEscape(owner), url.PathEscape(repo), url.QueryEscape(state)), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("list PRs: HTTP %d", resp.StatusCode)
	}
	var out []GiteaPR
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

func giteaListReleases(owner, repo string) ([]GiteaRelease, error) {
	resp, err := giteaRequest("GET", fmt.Sprintf("/api/v1/repos/%s/%s/releases",
		url.PathEscape(owner), url.PathEscape(repo)), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("list releases: HTTP %d", resp.StatusCode)
	}
	var out []GiteaRelease
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

func giteaListTags(owner, repo string) ([]string, error) {
	resp, err := giteaRequest("GET", fmt.Sprintf("/api/v1/repos/%s/%s/tags",
		url.PathEscape(owner), url.PathEscape(repo)), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("list tags: HTTP %d", resp.StatusCode)
	}
	var out []struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	tags := make([]string, len(out))
	for i, t := range out {
		tags[i] = t.Name
	}
	return tags, nil
}

func giteaMergePR(owner, repo string, num int, strategy string) error {
	body := map[string]string{"Do": strategy}
	resp, err := giteaRequest("POST", fmt.Sprintf("/api/v1/repos/%s/%s/pulls/%d/merge",
		url.PathEscape(owner), url.PathEscape(repo), num), body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("merge: HTTP %d", resp.StatusCode)
	}
	return nil
}

func giteaPatchPRState(owner, repo string, num int, state string) error {
	body := map[string]string{"state": state}
	resp, err := giteaRequest("PATCH", fmt.Sprintf("/api/v1/repos/%s/%s/pulls/%d",
		url.PathEscape(owner), url.PathEscape(repo), num), body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("patch state: HTTP %d", resp.StatusCode)
	}
	return nil
}

// pollMergeable polls a PR until Gitea reports it mergeable or the
// deadline passes. Gitea's mergeable field is computed asynchronously
// after PR creation/update; without polling, fresh PRs return HTTP
// 405 on merge attempts.
func pollMergeable(owner, repo string, num int) error {
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := giteaRequest("GET", fmt.Sprintf("/api/v1/repos/%s/%s/pulls/%d",
			url.PathEscape(owner), url.PathEscape(repo), num), nil)
		if err != nil {
			return err
		}
		var pr GiteaPR
		if err := json.NewDecoder(resp.Body).Decode(&pr); err != nil {
			resp.Body.Close()
			return err
		}
		resp.Body.Close()
		if pr.Mergeable {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("PR #%d still not mergeable after 30s", num)
}

// sliceEq compares two sorted slices for equality.
func sliceEq(a, b []string) bool {
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

// readFile reads a file and fails the test on error.
func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

// contains reports whether s appears in slice.
func contains(slice []string, s string) bool {
	for _, e := range slice {
		if e == s {
			return true
		}
	}
	return false
}

// mapKeys returns the keys of m as a slice (unordered).
func mapKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
