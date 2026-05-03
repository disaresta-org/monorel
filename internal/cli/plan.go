package cli

import (
	"encoding/json"
	"io"
	"strings"

	"github.com/spf13/cobra"
	loglayer "go.loglayer.dev/v2"

	"monorel.disaresta.com/changeset"
	"monorel.disaresta.com/plan"
)

func newPlanCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plan",
		Short: "Compute the proposed releases without applying them (dry-run).",
		Long: `Reads .changeset/*.md, the configured packages, and the latest matching
git tag per package. Prints the proposed bump for each affected package.

Pass --json to emit a machine-readable plan (the output schema matches
the internal/plan.ReleasePlan type).`,
		RunE: runPlan,
	}
	cmd.Flags().Bool("json", false, "Emit the plan as JSON.")
	return cmd
}

func runPlan(cmd *cobra.Command, _ []string) error {
	rt, err := loadRuntime(cmd)
	if err != nil {
		return err
	}
	p, err := plan.Plan(rt.Config, rt.Changesets, rt.Tags, rt.PreState)
	if err != nil {
		return err
	}

	asJSON, _ := cmd.Flags().GetBool("json")
	if asJSON {
		return writePlanJSON(cmd.OutOrStdout(), p)
	}
	emitPlanTable(rt.Log, p)
	return nil
}

// emitPlanTable writes a human-readable table via the cli transport.
// Empty plans surface a single line so the user knows the run was
// successful but produced nothing to release.
func emitPlanTable(log *loglayer.LogLayer, p *plan.ReleasePlan) {
	if p.IsEmpty() {
		log.Info("No pending changesets. Nothing to release.")
		return
	}

	rows := make([]loglayer.Metadata, 0, len(p.Releases))
	for _, r := range p.Releases {
		from := r.From
		if from == "" {
			from = "(initial)"
		}
		rows = append(rows, loglayer.Metadata{
			"package":    r.Name,
			"from":       from,
			"bump":       r.Bump.String(),
			"to":         r.To,
			"tag":        r.Tag,
			"changesets": strings.Join(changesetNames(r.Changesets), ","),
		})
	}
	// Table-only entry: the cli transport detects slice-of-map
	// metadata and renders an aligned table without a headline.
	log.MetadataOnly(rows)
	log.Info("%d package(s) to release; %d changeset(s) consumed.",
		len(p.Releases), len(p.Consumed))
}

// writePlanJSON writes a JSON document to w. The shape is intentionally
// the public ReleasePlan type's JSON encoding, so machine consumers can
// rely on it across versions (changes are SemVer-versioned).
func writePlanJSON(w io.Writer, p *plan.ReleasePlan) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(toJSONPlan(p))
}

// jsonPlan is the JSON-stable view of a ReleasePlan. Decoupled from
// the internal plan struct so we can add fields there without breaking
// consumers of the JSON output.
type jsonPlan struct {
	Releases []jsonRelease `json:"releases"`
	Consumed []string      `json:"consumed"`
}

type jsonRelease struct {
	Name       string   `json:"name"`
	From       string   `json:"from"`
	To         string   `json:"to"`
	Bump       string   `json:"bump"`
	Tag        string   `json:"tag"`
	Initial    bool     `json:"initial"`
	Changesets []string `json:"changesets"`
}

func toJSONPlan(p *plan.ReleasePlan) jsonPlan {
	out := jsonPlan{
		Releases: make([]jsonRelease, len(p.Releases)),
		Consumed: make([]string, len(p.Consumed)),
	}
	for i, r := range p.Releases {
		out.Releases[i] = jsonRelease{
			Name:       r.Name,
			From:       r.From,
			To:         r.To,
			Bump:       r.Bump.String(),
			Tag:        r.Tag,
			Initial:    r.Initial,
			Changesets: changesetNames(r.Changesets),
		}
	}
	for i, cs := range p.Consumed {
		out.Consumed[i] = cs.Name
	}
	return out
}

func changesetNames(cs []*changeset.Changeset) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.Name
	}
	return out
}
