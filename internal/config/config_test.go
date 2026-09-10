package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SLK_HOME", dir)
	t.Setenv("SLK_TOKEN", "")

	require.NoError(t, Save("xoxp-abc", "work"))

	// File lands under <SLK_HOME>/work/config.yaml with 0600.
	info, err := os.Stat(filepath.Join(dir, "work", "config.yaml"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())

	cfg, err := Load("work")
	require.NoError(t, err)
	assert.Equal(t, "xoxp-abc", cfg.Token)
}

func TestEnvTokenOverridesFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SLK_HOME", dir)
	require.NoError(t, Save("xoxp-file", "work"))

	t.Setenv("SLK_TOKEN", "xoxp-env")
	cfg, err := Load("work")
	require.NoError(t, err)
	assert.Equal(t, "xoxp-env", cfg.Token)
}

func TestValidateRequiresToken(t *testing.T) {
	assert.Error(t, (&Config{}).Validate())
	assert.NoError(t, (&Config{Token: "xoxp-x"}).Validate())
}
