package config

import "testing"

func TestProviderBitbucket_IsKnown(t *testing.T) {
	if !IsKnownProvider(ProviderBitbucket) {
		t.Error("ProviderBitbucket should be recognized")
	}
}

func TestKnownProviders_IncludesBitbucket(t *testing.T) {
	for _, p := range KnownProviders {
		if p == ProviderBitbucket {
			return
		}
	}
	t.Error("KnownProviders should include ProviderBitbucket")
}
