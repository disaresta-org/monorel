package config

import "testing"

func TestKnownProviders(t *testing.T) {
	for _, p := range []string{ProviderGitea, ProviderGitHub, ProviderGitLab} {
		t.Run(p, func(t *testing.T) {
			if !IsKnownProvider(p) {
				t.Errorf("%q should be a known provider", p)
			}
			// Also assert it appears in the slice directly (defends
			// against an IsKnownProvider regression that decouples
			// the predicate from the canonical list).
			found := false
			for _, k := range KnownProviders {
				if k == p {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("%q missing from KnownProviders slice", p)
			}
		})
	}
}

// TestProviderBitbucket_NotDispatched pins the disable: the constant
// still exists for the in-tree implementation, but it must NOT be in
// KnownProviders. End-to-end Pipelines verification hasn't been
// completed; reintroducing it requires a follow-up that adds it back
// here AND reinstates the case in internal/provider/factory.
func TestProviderBitbucket_NotDispatched(t *testing.T) {
	if IsKnownProvider(ProviderBitbucket) {
		t.Errorf("ProviderBitbucket is in KnownProviders; the bitbucket provider is intentionally disabled until live Pipelines verification is complete (see internal/provider/factory/factory.go)")
	}
}
