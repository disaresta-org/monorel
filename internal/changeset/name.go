package changeset

import (
	"crypto/rand"
	"math/big"
	"strings"
)

// adjectives and nouns combine into two-word changeset filenames like
// "happy-cats" or "swift-pandas". Lists are deliberately small (~50
// each) — the goal is memorable, not collision-resistant. Birthday-
// paradox math says ~50 changesets is when a duplicate becomes
// likely; that's beyond what any single PR carries.
//
// The lists are alphabetically sorted to keep diffs clean when adding
// new entries. Stay on the wholesome side; these names appear in PR
// titles and changelogs.
var (
	adjectives = []string{
		"agile", "amber", "ample", "blue", "bold",
		"brave", "bright", "calm", "clever", "cool",
		"crisp", "deep", "eager", "early", "fair",
		"fast", "fine", "fresh", "gentle", "glad",
		"happy", "honest", "humble", "icy", "jolly",
		"keen", "kind", "late", "light", "lively",
		"lucky", "merry", "mild", "neat", "nimble",
		"open", "plain", "polite", "proud", "quick",
		"quiet", "red", "rich", "rosy", "round",
		"safe", "sharp", "shiny", "shy", "silent",
		"silver", "smooth", "snug", "soft", "solid",
		"steady", "stout", "subtle", "sunny", "swift",
		"tame", "tidy", "tiny", "true", "warm",
		"witty", "wise", "young", "zealous", "zesty",
	}
	nouns = []string{
		"acorn", "anchor", "apple", "aspen", "badger",
		"bear", "beaver", "bee", "berry", "bird",
		"bison", "blossom", "branch", "breeze", "brook",
		"butterfly", "cactus", "canyon", "cardinal", "cedar",
		"cherry", "cloud", "clover", "comet", "crane",
		"creek", "crow", "daisy", "dolphin", "ember",
		"falcon", "fern", "field", "finch", "fir",
		"fox", "garden", "glade", "harbor", "harvest",
		"hawk", "hazel", "heron", "hill", "ivy",
		"jay", "lake", "lemon", "lily", "lynx",
		"maple", "meadow", "moose", "moss", "olive",
		"orchard", "otter", "owl", "panda", "peach",
		"pear", "pebble", "pine", "plum", "pond",
		"raven", "river", "robin", "rose", "salmon",
		"sparrow", "spring", "stream", "tiger", "vale",
		"walnut", "wave", "willow", "wolf", "wren",
	}
)

// RandomName returns a "<adjective>-<noun>" filename suitable for a
// new changeset. Uses crypto/rand so concurrent invocations don't
// collide on a shared seed.
func RandomName() string {
	adj := pickRandom(adjectives)
	noun := pickRandom(nouns)
	return adj + "-" + noun
}

// pickRandom returns a uniformly-random element of items. Falls back
// to items[0] on rand failure (extremely rare; Go's crypto/rand only
// fails on system-entropy issues).
func pickRandom(items []string) string {
	n := big.NewInt(int64(len(items)))
	idx, err := rand.Int(rand.Reader, n)
	if err != nil {
		return items[0]
	}
	return items[int(idx.Int64())]
}

// IsValidName reports whether name is a plausible changeset filename:
// non-empty, lowercase letters / digits / hyphens, no slashes, no
// leading/trailing/consecutive hyphens.
//
// We don't enforce the two-word format (users may rename files
// manually); the rule is "what's safe to use as a filename and
// readable in PR titles."
func IsValidName(name string) bool {
	if name == "" || strings.Contains(name, "/") || strings.Contains(name, "..") {
		return false
	}
	if strings.HasPrefix(name, "-") || strings.HasSuffix(name, "-") {
		return false
	}
	if strings.Contains(name, "--") {
		return false
	}
	for _, r := range name {
		if !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-') {
			return false
		}
	}
	return true
}

