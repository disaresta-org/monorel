package release

import (
	"fmt"
	"strings"

	"github.com/disaresta-org/monorel/internal/plan"
)

// RenderPreview formats a [plan.ReleasePlan] as markdown suitable for
// an always-open release PR body. The output starts with a header
// summarizing the plan, then a per-package section with the rendered
// CHANGELOG entry for each release.
//
// pre is the optional pre-release state (when non-nil, the header
// notes the channel). preState may be nil for stable plans.
//
// An empty plan returns a short "nothing to release" body that the
// orchestrator can use when closing an open PR.
func RenderPreview(p *plan.ReleasePlan, today string) string {
	if p == nil || p.IsEmpty() {
		return "_No pending changesets. The release PR will be closed._\n"
	}
	if today == "" {
		today = "<release-date>"
	}

	var b strings.Builder
	fmt.Fprintln(&b, "This PR was opened by [monorel](https://github.com/disaresta-org/monorel). Merge it to release the following packages:")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "| Package | Bump | From | To |")
	fmt.Fprintln(&b, "|---|---|---|---|")
	for _, r := range p.Releases {
		from := r.From
		if from == "" {
			from = "_(initial)_"
		}
		fmt.Fprintf(&b, "| `%s` | %s | %s | **%s** |\n", r.Name, r.Bump, from, r.To)
	}
	fmt.Fprintln(&b)

	if anyPrerelease(p) {
		fmt.Fprintln(&b, "> **Pre-release**: this release is part of a pre-release window. CHANGELOGs and changeset files are preserved; merging this PR ships pre-release tags.")
		fmt.Fprintln(&b)
	}

	fmt.Fprintln(&b, "## Released changes")
	for _, r := range p.Releases {
		entry := buildEntry(r, today)
		if entry.IsEmpty() {
			continue
		}
		fmt.Fprintln(&b)
		fmt.Fprintf(&b, "### %s %s\n", r.Name, r.To)

		// Reuse changelog.Render() but reshape its headings to fit
		// under our per-package H3:
		//   - drop the top `## [VERSION] - DATE` line (we already
		//     emit the version in the per-package H3 above).
		//   - bump `### Major / Minor / Patch Changes` to H4.
		body := entry.Render()
		if i := strings.Index(body, "\n"); i >= 0 {
			body = body[i+1:]
		}
		body = strings.ReplaceAll(body, "\n### ", "\n#### ")
		body = strings.TrimSpace(body)
		if strings.HasPrefix(body, "### ") {
			body = "#" + body
		}
		if body != "" {
			fmt.Fprintln(&b, body)
		}
	}

	if len(p.Consumed) > 0 {
		fmt.Fprintln(&b)
		fmt.Fprintf(&b, "_Consumed %d changeset(s): ", len(p.Consumed))
		names := make([]string, len(p.Consumed))
		for i, cs := range p.Consumed {
			names[i] = "`" + cs.Name + "`"
		}
		fmt.Fprint(&b, strings.Join(names, ", "))
		fmt.Fprintln(&b, "._")
	}

	return b.String()
}

func anyPrerelease(p *plan.ReleasePlan) bool {
	for _, r := range p.Releases {
		if r.Prerelease {
			return true
		}
	}
	return false
}
