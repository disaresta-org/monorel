package plan_test

import (
	"fmt"
	"strings"

	"monorel.disaresta.com/changeset"
	"monorel.disaresta.com/config"
	"monorel.disaresta.com/plan"
)

// ExamplePlan computes the next-release plan from the three static
// inputs that drive every monorel run: the parsed monorel.toml, the
// pending changesets, and the existing tag list. The result is a
// pure function of these inputs; nothing in the plan package touches
// the filesystem or the network.
func ExamplePlan() {
	cfg := &config.Config{
		Provider: config.ProviderConfig{Name: "github", Owner: "acme", Repo: "widget"},
		Packages: map[string]config.PackageConfig{
			"transports/zerolog": {
				TagPrefix: "transports/zerolog",
				Path:      "transports/zerolog",
				Changelog: "transports/zerolog/CHANGELOG.md",
			},
		},
	}

	cs, _ := changeset.Parse(strings.NewReader(`---
"transports/zerolog": minor
---

Adds Lazy() helper.
`), "quick-otter")

	tags := []string{"transports/zerolog/v1.6.0", "transports/zerolog/v1.6.1"}

	p, err := plan.Plan(cfg, []*changeset.Changeset{cs}, tags, nil)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	for _, r := range p.Releases {
		fmt.Printf("%s: %s -> %s (tag %s)\n", r.Name, r.From, r.To, r.Tag)
	}
	// Output:
	// transports/zerolog: v1.6.1 -> v1.7.0 (tag transports/zerolog/v1.7.0)
}

// ExamplePlan_preRelease shows the planner under pre-release mode.
// When a [changeset.PreState] is supplied, the planner appends a
// channel suffix and increments the per-package counter rather than
// producing the next stable version. Counters are read-only here;
// the release applier writes them back. This is the path that
// powers `monorel pre enter rc` -> `monorel release` cycles.
func ExamplePlan_preRelease() {
	cfg := &config.Config{
		Provider: config.ProviderConfig{Name: "github", Owner: "acme", Repo: "widget"},
		Packages: map[string]config.PackageConfig{
			"transports/zerolog": {
				TagPrefix: "transports/zerolog",
				Path:      "transports/zerolog",
				Changelog: "transports/zerolog/CHANGELOG.md",
			},
		},
	}

	cs, _ := changeset.Parse(strings.NewReader(`---
"transports/zerolog": minor
---

Adds Lazy() helper.
`), "quick-otter")

	tags := []string{"transports/zerolog/v1.6.1"}

	pre := &changeset.PreState{
		Mode:     "pre",
		Channel:  "rc",
		Counters: map[string]int{"transports/zerolog": 0},
	}

	p, err := plan.Plan(cfg, []*changeset.Changeset{cs}, tags, pre)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	for _, r := range p.Releases {
		fmt.Printf("%s: -> %s (tag %s)\n", r.Name, r.To, r.Tag)
	}
	// Output:
	// transports/zerolog: -> v1.7.0-rc.0 (tag transports/zerolog/v1.7.0-rc.0)
}
