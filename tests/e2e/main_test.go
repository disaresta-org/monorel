//go:build e2e

// Package e2e_test is monorel's end-to-end test suite against a live
// Forgejo / Gitea instance.
//
// The suite uses testcontainers-go's Forgejo module to spin up a
// fresh Forgejo container per test run. Forgejo is API-compatible
// with Gitea (same /api/v1/... shape) so the same tests validate
// monorel against both.
//
// Run locally:
//
//	go test -tags=e2e -v ./tests/e2e/...
//
// The container is auto-managed: testcontainers pulls the image,
// starts it, creates the admin user, and tears it down at the end
// of the run. No prior `docker run` is required.
//
// Run in CI: see .github/workflows/e2e-gitea.yml. The runner needs
// a working Docker daemon (the default ubuntu-latest image has one).
//
// Each scenario lives in a separate Test* function. TestMain
// provisions the Forgejo instance once (admin user + API token) and
// builds the monorel binary. Individual scenarios run against fresh
// repos so they don't interfere.
package e2e_test

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/testcontainers/testcontainers-go/modules/forgejo"
)

// Provisioned state shared across tests. Populated by TestMain.
var (
	giteaHost     string
	giteaToken    string
	giteaUsername string
	monorelBin    string
)

// Container fields the testcontainers Forgejo module uses for admin
// credentials. Same value gets returned by container.AdminUsername()
// / AdminPassword(), but we pass them explicitly so the test code
// can reach them via constants.
const (
	adminUsername = "monorel"
	adminEmail    = "monorel@example.com"
	adminPassword = "monorel-e2e-pw-changeme" //nolint:gosec // test-only credential
)

// forgejoImage is the container image testcontainers pulls. Pinned
// to a specific minor version for reproducibility; bump deliberately.
const forgejoImage = "codeberg.org/forgejo/forgejo:11.0"

func TestMain(m *testing.M) {
	flag.Parse()

	cleanup, err := setup()
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "e2e: setup failed: %v\n", err)
		os.Exit(2)
	}
	code := m.Run()
	os.Exit(code)
}

// setup provisions a Forgejo container, generates an API token, and
// resolves the monorel binary path. Returns a cleanup function that
// terminates the container; callers should defer it.
//
// Two paths:
//
//   - Pre-provisioned: if GITEA_TOKEN and GITEA_HOST are set,
//     skip the container setup and trust the caller's existing
//     instance. Useful for iterating against a long-running local
//     Forgejo / Gitea.
//   - Fresh: spin up a Forgejo container via testcontainers, mint a
//     token via the /api/v1/users/<admin>/tokens endpoint. This is
//     the CI path.
func setup() (func(), error) {
	if tok := os.Getenv("GITEA_TOKEN"); tok != "" {
		host := os.Getenv("GITEA_HOST")
		if host == "" {
			host = "http://localhost:3000"
		}
		giteaHost = host
		giteaToken = tok
		giteaUsername = envOr("GITEA_USERNAME", adminUsername)
		if err := buildMonorel(); err != nil {
			return nil, err
		}
		return nil, nil
	}

	ctx := context.Background()
	container, err := forgejo.Run(ctx, forgejoImage,
		forgejo.WithAdminCredentials(adminUsername, adminPassword, adminEmail),
	)
	if err != nil {
		return nil, fmt.Errorf("start forgejo container: %w", err)
	}
	cleanup := func() {
		if err := container.Terminate(context.Background()); err != nil {
			fmt.Fprintf(os.Stderr, "e2e: container.Terminate: %v\n", err)
		}
	}

	host, err := container.ConnectionString(ctx)
	if err != nil {
		return cleanup, fmt.Errorf("connection string: %w", err)
	}
	giteaHost = host
	giteaUsername = container.AdminUsername()

	tok, err := generateToken(container.AdminUsername(), container.AdminPassword())
	if err != nil {
		return cleanup, fmt.Errorf("generate token: %w", err)
	}
	giteaToken = tok

	if err := buildMonorel(); err != nil {
		return cleanup, err
	}
	return cleanup, nil
}

// generateToken mints a Gitea / Forgejo API token using basic auth
// against the admin user. The token has full repo + user scopes;
// e2e tests run against ephemeral repos so the wide scope is fine.
func generateToken(user, pass string) (string, error) {
	body := strings.NewReader(`{"name":"e2e","scopes":["write:repository","write:user","read:user"]}`)
	req, err := http.NewRequest("POST", giteaHost+"/api/v1/users/"+user+"/tokens", body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(user, pass)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("token creation returned %d", resp.StatusCode)
	}
	var out struct {
		Sha1 string `json:"sha1"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if out.Sha1 == "" {
		return "", fmt.Errorf("token creation succeeded but sha1 was empty")
	}
	return out.Sha1, nil
}

// buildMonorel resolves MONOREL_BIN from the environment, building
// from source if it's unset.
func buildMonorel() error {
	if v := os.Getenv("MONOREL_BIN"); v != "" {
		monorelBin = v
		return nil
	}
	dir, err := os.Getwd()
	if err != nil {
		return err
	}
	repoRoot := filepath.Clean(filepath.Join(dir, "..", ".."))
	out := filepath.Join(repoRoot, "bin", "monorel-e2e")
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	cmd := exec.Command("go", "build", "-o", out, "./cmd/monorel")
	cmd.Dir = repoRoot
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("go build monorel: %w", err)
	}
	monorelBin = out
	return nil
}

func envOr(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}
