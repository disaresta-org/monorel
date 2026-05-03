package provider_test

import (
	"slices"
	"testing"

	"monorel.disaresta.com/internal/provider"
)

func TestEmailEnvVars(t *testing.T) {
	if got := provider.EmailEnvVars("bitbucket"); !slices.Equal(got, []string{"BITBUCKET_EMAIL"}) {
		t.Errorf("EmailEnvVars(bitbucket) = %v", got)
	}
	if got := provider.EmailEnvVars("github"); got != nil {
		t.Errorf("EmailEnvVars(github) = %v, want nil", got)
	}
}

func TestTokenEnvVars_Bitbucket(t *testing.T) {
	if got := provider.TokenEnvVars("bitbucket"); !slices.Equal(got, []string{"BITBUCKET_TOKEN"}) {
		t.Errorf("TokenEnvVars(bitbucket) = %v", got)
	}
}
