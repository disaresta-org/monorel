package provider_test

import (
	"testing"

	"monorel.disaresta.com/internal/provider"
)

func TestEmailEnvVars(t *testing.T) {
	if got := provider.EmailEnvVars("bitbucket"); !equalSlices(got, []string{"BITBUCKET_EMAIL"}) {
		t.Errorf("EmailEnvVars(bitbucket) = %v", got)
	}
	if got := provider.EmailEnvVars("github"); got != nil {
		t.Errorf("EmailEnvVars(github) = %v, want nil", got)
	}
}

func TestTokenEnvVars_Bitbucket(t *testing.T) {
	if got := provider.TokenEnvVars("bitbucket"); !equalSlices(got, []string{"BITBUCKET_TOKEN"}) {
		t.Errorf("TokenEnvVars(bitbucket) = %v", got)
	}
}

func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
