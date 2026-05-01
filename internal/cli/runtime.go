package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/disaresta-org/monorel/internal/changeset"
	"github.com/disaresta-org/monorel/internal/config"
	"github.com/disaresta-org/monorel/internal/git"
)

// Runtime is the shared state that read-only commands (plan, status,
// preview) compute by combining monorel.toml + .changeset/*.md + git
// tags. Constructed via [loadRuntime].
type Runtime struct {
	// ConfigPath is the absolute path to monorel.toml.
	ConfigPath string

	// RepoDir is the directory that contains monorel.toml. Treated
	// as the repository root for git invocations and the parent of
	// .changeset/.
	RepoDir string

	// Config is the parsed monorel.toml.
	Config *config.Config

	// Repo is a git.Repo bound to RepoDir.
	Repo git.Repo

	// Changesets are pending changesets loaded from .changeset/.
	Changesets []*changeset.Changeset

	// Tags is every tag in the repository.
	Tags []string

	// PreState is the pre-release-mode state loaded from
	// .changeset/pre.json, or nil when not in pre-release mode.
	// Read-only; pre subcommands construct their own to write.
	PreState *changeset.PreState

	// ChangesetDir is the absolute path to .changeset/. The pre and
	// release commands write/delete files here.
	ChangesetDir string
}

// configPathFlag is the persistent --config flag value. Set on the
// root command in [newRootCmd] via the persistent flag set.
const configPathFlagName = "config"

// addPersistentFlags registers flags shared across every subcommand.
// Called from newRootCmd.
func addPersistentFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().String(configPathFlagName, "monorel.toml",
		"Path to monorel.toml (the repository root is its parent directory).")
}

// loadRuntime resolves the --config flag, loads everything a read-only
// command needs, and returns it bundled. Does NOT validate that the
// working tree is clean — release-side commands do that themselves.
func loadRuntime(cmd *cobra.Command) (*Runtime, error) {
	configPath, err := cmd.Flags().GetString(configPathFlagName)
	if err != nil {
		return nil, err
	}
	configPath, err = filepath.Abs(configPath)
	if err != nil {
		return nil, fmt.Errorf("resolve --config: %w", err)
	}
	if _, err := os.Stat(configPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("monorel.toml not found at %s; run `monorel init` first", configPath)
		}
		return nil, fmt.Errorf("stat %s: %w", configPath, err)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, err
	}

	repoDir := filepath.Dir(configPath)
	repo := git.Open(repoDir)

	changesetDir := filepath.Join(repoDir, ".changeset")
	changesets, err := changeset.LoadAll(changesetDir)
	if err != nil {
		return nil, fmt.Errorf("load changesets: %w", err)
	}

	preState, err := changeset.LoadPreState(changesetDir)
	if err != nil {
		return nil, fmt.Errorf("load pre-release state: %w", err)
	}

	tags, err := repo.ListTags("")
	if err != nil {
		return nil, fmt.Errorf("list tags: %w", err)
	}

	return &Runtime{
		ConfigPath:   configPath,
		RepoDir:      repoDir,
		Config:       cfg,
		Repo:         repo,
		Changesets:   changesets,
		Tags:         tags,
		PreState:     preState,
		ChangesetDir: changesetDir,
	}, nil
}
