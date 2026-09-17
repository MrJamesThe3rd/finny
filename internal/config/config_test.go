package config_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/MrJamesThe3rd/finny/internal/config"
)

// AllowPrivateBackendHosts defaulting to false is the secure-by-default promise
// behind the SSRF guard: a typo in the envconfig tag would silently open it.
func TestLoad_StorageDefaults(t *testing.T) {
	t.Setenv("AUTH_JWTSECRET", "test-secret")

	cfg, err := config.Load()
	require.NoError(t, err)

	assert.False(t, cfg.Storage.AllowPrivateBackendHosts, "private backend hosts must be denied unless opted in")
	assert.Equal(t, "./data/documents", cfg.Storage.LocalRoot)
}

func TestLoad_StorageOverrides(t *testing.T) {
	t.Setenv("AUTH_JWTSECRET", "test-secret")
	t.Setenv("STORAGE_ALLOWPRIVATEBACKENDHOSTS", "true")
	t.Setenv("STORAGE_LOCALROOT", "/srv/finny/documents")

	cfg, err := config.Load()
	require.NoError(t, err)

	assert.True(t, cfg.Storage.AllowPrivateBackendHosts)
	assert.Equal(t, "/srv/finny/documents", cfg.Storage.LocalRoot)
}
