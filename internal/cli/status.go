package cli

import (
	"sort"
	"strings"

	"github.com/spf13/cobra"
	loglayer "go.loglayer.dev/v2"

	"monorel.disaresta.com/changeset"
)

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Print pending changesets and which packages they affect.",
		Long: `Lists every .changeset/*.md file with its bump levels per package and the
first line of its body. Use this to audit what's queued for the next
release before merging the release PR.

Empty output means there are no pending changesets.`,
		RunE: runStatus,
	}
}

func runStatus(cmd *cobra.Command, _ []string) error {
	rt, err := loadRuntime(cmd)
	if err != nil {
		return err
	}
	emitStatus(rt.Log, rt.Changesets)
	return nil
}

// emitStatus prints a tabular summary: one row per (changeset,
// package) pair. Sorted by changeset name, then package name, so the
// output is stable across runs. The cli transport renders the row
// slice as an aligned table.
func emitStatus(log *loglayer.LogLayer, changesets []*changeset.Changeset) {
	if len(changesets) == 0 {
		log.Info("No pending changesets.")
		return
	}
	// Sort by changeset name for determinism (LoadAll already sorts,
	// but defensive: status may be called on caller-supplied slices
	// in tests).
	sorted := append([]*changeset.Changeset(nil), changesets...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })

	rows := make([]loglayer.Metadata, 0, len(sorted))
	for _, cs := range sorted {
		summary := firstLine(cs.Body)
		for _, pkg := range cs.PackageNames() {
			rows = append(rows, loglayer.Metadata{
				"changeset": cs.Name,
				"package":   pkg,
				"bump":      cs.Bumps[pkg].String(),
				"summary":   summary,
			})
		}
	}
	// Emit the table on its own (no headline) and follow with the
	// summary line. The cli transport detects the slice-of-map
	// metadata shape and renders an aligned table; calling
	// MetadataOnly avoids prepending an empty leading line.
	log.MetadataOnly(rows)
	log.Info("%d changeset(s) pending.", len(sorted))
}

// firstLine returns the first non-empty line of body, trimmed. Empty
// bodies yield "(no summary)" so the column never collapses.
func firstLine(body string) string {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return "(no summary)"
}
