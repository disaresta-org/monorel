package config

import "testing"

func TestKnownProviders(t *testing.T) {
	for _, p := range []string{ProviderBitbucket, ProviderGitea, ProviderGitHub, ProviderGitLab} {
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
