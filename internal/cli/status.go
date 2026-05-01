package cli

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

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
	return writeStatus(cmd.OutOrStdout(), rt.Changesets)
}

// writeStatus writes a tabular summary: one row per (changeset,
// package) pair. Sorted by changeset name, then package name, so the
// output is stable across runs.
func writeStatus(w io.Writer, changesets []*changeset.Changeset) error {
	if len(changesets) == 0 {
		_, err := fmt.Fprintln(w, "No pending changesets.")
		return err
	}
	// Sort by changeset name for determinism (LoadAll already sorts,
	// but defensive: status may be called on caller-supplied slices
	// in tests).
	sorted := append([]*changeset.Changeset(nil), changesets...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "CHANGESET\tPACKAGE\tBUMP\tSUMMARY")
	for _, cs := range sorted {
		summary := firstLine(cs.Body)
		for _, pkg := range cs.PackageNames() {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", cs.Name, pkg, cs.Bumps[pkg], summary)
		}
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	fmt.Fprintf(w, "\n%d changeset(s) pending.\n", len(sorted))
	return nil
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
